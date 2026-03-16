package executor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const copilotCompatDefaultUserAgent = "cli-proxy-copilot-compat"

// CopilotCompatExecutor routes GitHub Copilot-compatible requests to either the
// Responses API or Chat Completions API depending on the downstream request shape.
type CopilotCompatExecutor struct {
	cfg *config.Config
}

func NewCopilotCompatExecutor(cfg *config.Config) *CopilotCompatExecutor {
	return &CopilotCompatExecutor{cfg: cfg}
}

func (e *CopilotCompatExecutor) Identifier() string { return "copilot-compat" }

func (e *CopilotCompatExecutor) PrepareRequest(req *http.Request, auth *cliproxyauth.Auth) error {
	if req == nil {
		return nil
	}
	apiKey, _ := codexCreds(auth)
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	var attrs map[string]string
	if auth != nil {
		attrs = auth.Attributes
	}
	util.ApplyCustomHeadersFromAttrs(req, attrs)
	return nil
}

func (e *CopilotCompatExecutor) HttpRequest(ctx context.Context, auth *cliproxyauth.Auth, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("copilot compat executor: request is nil")
	}
	if ctx == nil {
		ctx = req.Context()
	}
	httpReq := req.WithContext(ctx)
	if err := e.PrepareRequest(httpReq, auth); err != nil {
		return nil, err
	}
	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	return httpClient.Do(httpReq)
}

func (e *CopilotCompatExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	if e.shouldUseResponsesAPI(req.Model, opts) {
		if opts.Alt == "responses/compact" {
			return e.executeResponsesCompact(ctx, auth, req, opts)
		}
		return e.executeResponses(ctx, auth, req, opts)
	}
	if opts.Alt == "responses/compact" {
		return cliproxyexecutor.Response{}, statusErr{code: http.StatusBadRequest, msg: "/responses/compact is only supported for Copilot responses models"}
	}
	return e.executeChat(ctx, auth, req, opts)
}

func (e *CopilotCompatExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (_ *cliproxyexecutor.StreamResult, err error) {
	if e.shouldUseResponsesAPI(req.Model, opts) {
		if opts.Alt == "responses/compact" {
			return nil, statusErr{code: http.StatusBadRequest, msg: "streaming not supported for /responses/compact"}
		}
		return e.executeResponsesStream(ctx, auth, req, opts)
	}
	if opts.Alt == "responses/compact" {
		return nil, statusErr{code: http.StatusBadRequest, msg: "/responses/compact is only supported for Copilot responses models"}
	}
	return e.executeChatStream(ctx, auth, req, opts)
}

func (e *CopilotCompatExecutor) Refresh(_ context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	return auth, nil
}

func (e *CopilotCompatExecutor) CountTokens(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	_ = auth
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	if e.shouldUseResponsesAPI(req.Model, opts) {
		from := opts.SourceFormat
		to := sdktranslator.FormatOpenAIResponse
		body := sdktranslator.TranslateRequest(from, to, baseModel, req.Payload, false)
		var err error
		body, err = thinking.ApplyThinking(body, req.Model, from.String(), to.String(), e.Identifier())
		if err != nil {
			return cliproxyexecutor.Response{}, err
		}
		body = sanitizeCopilotCompatResponsesPayload(body, baseModel, true)
		enc, err := tokenizerForCodexModel(baseModel)
		if err != nil {
			return cliproxyexecutor.Response{}, fmt.Errorf("copilot compat executor: tokenizer init failed: %w", err)
		}
		count, err := countCodexInputTokens(enc, body)
		if err != nil {
			return cliproxyexecutor.Response{}, fmt.Errorf("copilot compat executor: token counting failed: %w", err)
		}
		usageJSON := []byte(fmt.Sprintf(`{"response":{"usage":{"input_tokens":%d,"output_tokens":0,"total_tokens":%d}}}`, count, count))
		translated := sdktranslator.TranslateTokenCount(ctx, to, from, count, usageJSON)
		return cliproxyexecutor.Response{Payload: []byte(translated)}, nil
	}

	from := opts.SourceFormat
	to := sdktranslator.FormatOpenAI
	translated := sdktranslator.TranslateRequest(from, to, baseModel, req.Payload, false)
	var err error
	translated, err = thinking.ApplyThinking(translated, req.Model, from.String(), to.String(), e.Identifier())
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}
	translated = sanitizeCopilotCompatChatPayload(translated, baseModel)
	enc, err := tokenizerForModel(baseModel)
	if err != nil {
		return cliproxyexecutor.Response{}, fmt.Errorf("copilot compat executor: tokenizer init failed: %w", err)
	}
	count, err := countOpenAIChatTokens(enc, translated)
	if err != nil {
		return cliproxyexecutor.Response{}, fmt.Errorf("copilot compat executor: token counting failed: %w", err)
	}
	usageJSON := buildOpenAIUsageJSON(count)
	out := sdktranslator.TranslateTokenCount(ctx, to, from, count, usageJSON)
	return cliproxyexecutor.Response{Payload: []byte(out)}, nil
}

func (e *CopilotCompatExecutor) executeChat(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	reporter := newUsageReporter(ctx, e.Identifier(), baseModel, auth)
	defer reporter.trackFailure(ctx, &err)

	apiKey, baseURL, err := e.resolveCredentials(auth)
	if err != nil {
		return resp, err
	}

	from := opts.SourceFormat
	to := sdktranslator.FormatOpenAI
	originalPayload := originalRequestPayload(req.Payload, opts.OriginalRequest)
	originalTranslated := sdktranslator.TranslateRequest(from, to, baseModel, originalPayload, false)
	translated := sdktranslator.TranslateRequest(from, to, baseModel, req.Payload, false)
	requestedModel := payloadRequestedModel(opts, req.Model)
	translated = applyPayloadConfigWithRoot(e.cfg, baseModel, to.String(), "", translated, originalTranslated, requestedModel)
	translated, err = thinking.ApplyThinking(translated, req.Model, from.String(), to.String(), e.Identifier())
	if err != nil {
		return resp, err
	}
	translated = sanitizeCopilotCompatChatPayload(translated, baseModel)

	url := strings.TrimSuffix(baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(translated))
	if err != nil {
		return resp, err
	}
	applyCopilotCompatHeaders(httpReq, auth, apiKey, translated, false, false)

	authID, authLabel, authType, authValue := requestAuthLogFields(auth)
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      translated,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		recordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	defer closeLoggedBody("copilot compat executor", httpResp.Body)
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		logWithRequestID(ctx).Debugf("request error, error status: %d, error message: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		return resp, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		recordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	appendAPIResponseChunk(ctx, e.cfg, body)
	reporter.publishWithContent(ctx, parseOpenAIUsage(body), string(req.Payload), string(body))
	var param any
	out := sdktranslator.TranslateNonStream(ctx, to, from, req.Model, opts.OriginalRequest, translated, body, &param)
	return cliproxyexecutor.Response{Payload: []byte(out), Headers: httpResp.Header.Clone()}, nil
}

func (e *CopilotCompatExecutor) executeChatStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (_ *cliproxyexecutor.StreamResult, err error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	reporter := newUsageReporter(ctx, e.Identifier(), baseModel, auth)
	defer reporter.trackFailure(ctx, &err)

	apiKey, baseURL, err := e.resolveCredentials(auth)
	if err != nil {
		return nil, err
	}

	from := opts.SourceFormat
	to := sdktranslator.FormatOpenAI
	originalPayload := originalRequestPayload(req.Payload, opts.OriginalRequest)
	originalTranslated := sdktranslator.TranslateRequest(from, to, baseModel, originalPayload, true)
	translated := sdktranslator.TranslateRequest(from, to, baseModel, req.Payload, true)
	requestedModel := payloadRequestedModel(opts, req.Model)
	translated = applyPayloadConfigWithRoot(e.cfg, baseModel, to.String(), "", translated, originalTranslated, requestedModel)
	translated, err = thinking.ApplyThinking(translated, req.Model, from.String(), to.String(), e.Identifier())
	if err != nil {
		return nil, err
	}
	translated = sanitizeCopilotCompatChatPayload(translated, baseModel)

	url := strings.TrimSuffix(baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	applyCopilotCompatHeaders(httpReq, auth, apiKey, translated, true, false)

	authID, authLabel, authType, authValue := requestAuthLogFields(auth)
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      translated,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		recordAPIResponseError(ctx, e.cfg, err)
		return nil, err
	}
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		closeLoggedBody("copilot compat executor", httpResp.Body)
		logWithRequestID(ctx).Debugf("request error, error status: %d, error message: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		return nil, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	out := make(chan cliproxyexecutor.StreamChunk)
	reporter.setInputContent(string(req.Payload))
	go func() {
		defer close(out)
		defer closeLoggedBody("copilot compat executor", httpResp.Body)
		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(nil, 52_428_800)
		var param any
		for scanner.Scan() {
			line := bytes.Clone(scanner.Bytes())
			appendAPIResponseChunk(ctx, e.cfg, line)
			reporter.appendOutputChunk(line)
			if detail, ok := parseOpenAIStreamUsage(line); ok {
				reporter.publish(ctx, detail)
			}
			if len(line) == 0 {
				continue
			}
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			chunks := sdktranslator.TranslateStream(ctx, to, from, req.Model, opts.OriginalRequest, translated, line, &param)
			for i := range chunks {
				out <- cliproxyexecutor.StreamChunk{Payload: []byte(chunks[i])}
			}
		}
		if errScan := scanner.Err(); errScan != nil {
			recordAPIResponseError(ctx, e.cfg, errScan)
			reporter.publishFailure(ctx)
			out <- cliproxyexecutor.StreamChunk{Err: errScan}
		}
		reporter.ensurePublished(ctx)
	}()
	return &cliproxyexecutor.StreamResult{Headers: httpResp.Header.Clone(), Chunks: out}, nil
}

func (e *CopilotCompatExecutor) executeResponses(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	reporter := newUsageReporter(ctx, e.Identifier(), baseModel, auth)
	defer reporter.trackFailure(ctx, &err)

	apiKey, baseURL, err := e.resolveCredentials(auth)
	if err != nil {
		return resp, err
	}

	from := opts.SourceFormat
	to := sdktranslator.FormatOpenAIResponse
	originalPayload := originalRequestPayload(req.Payload, opts.OriginalRequest)
	originalTranslated := sdktranslator.TranslateRequest(from, to, baseModel, originalPayload, false)
	translated := sdktranslator.TranslateRequest(from, to, baseModel, req.Payload, false)
	requestedModel := payloadRequestedModel(opts, req.Model)
	translated = applyPayloadConfigWithRoot(e.cfg, baseModel, to.String(), "", translated, originalTranslated, requestedModel)
	translated, err = thinking.ApplyThinking(translated, req.Model, from.String(), to.String(), e.Identifier())
	if err != nil {
		return resp, err
	}
	translated = sanitizeCopilotCompatResponsesPayload(translated, baseModel, false)

	url := strings.TrimSuffix(baseURL, "/") + "/responses"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(translated))
	if err != nil {
		return resp, err
	}
	applyCopilotCompatHeaders(httpReq, auth, apiKey, translated, true, true)

	authID, authLabel, authType, authValue := requestAuthLogFields(auth)
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      translated,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		recordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	defer closeLoggedBody("copilot compat executor", httpResp.Body)
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		logWithRequestID(ctx).Debugf("request error, error status: %d, error message: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		return resp, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	data, err := io.ReadAll(httpResp.Body)
	if err != nil {
		recordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	appendAPIResponseChunk(ctx, e.cfg, data)

	state := newCopilotCompatResponsesState()
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		if !bytes.HasPrefix(bytes.TrimSpace(line), []byte("data:")) {
			continue
		}
		normalized := normalizeCopilotCompatResponseIDs(line, state)
		payload := jsonPayload(normalized)
		if len(payload) == 0 {
			continue
		}
		if gjson.GetBytes(payload, "type").String() != "response.completed" {
			continue
		}
		if detail, ok := parseCodexUsage(payload); ok {
			reporter.publishWithContent(ctx, detail, string(req.Payload), string(data))
		}
		out := translateResponsesNonStream(ctx, from, req.Model, originalPayload, translated, normalized)
		reporter.ensurePublished(ctx)
		return cliproxyexecutor.Response{Payload: []byte(out), Headers: httpResp.Header.Clone()}, nil
	}
	return resp, statusErr{code: http.StatusRequestTimeout, msg: "stream error: stream disconnected before completion: stream closed before response.completed"}
}

func (e *CopilotCompatExecutor) executeResponsesCompact(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	reporter := newUsageReporter(ctx, e.Identifier(), baseModel, auth)
	defer reporter.trackFailure(ctx, &err)

	apiKey, baseURL, err := e.resolveCredentials(auth)
	if err != nil {
		return resp, err
	}

	from := opts.SourceFormat
	to := sdktranslator.FormatOpenAIResponse
	originalPayload := originalRequestPayload(req.Payload, opts.OriginalRequest)
	originalTranslated := sdktranslator.TranslateRequest(from, to, baseModel, originalPayload, false)
	translated := sdktranslator.TranslateRequest(from, to, baseModel, req.Payload, false)
	requestedModel := payloadRequestedModel(opts, req.Model)
	translated = applyPayloadConfigWithRoot(e.cfg, baseModel, to.String(), "", translated, originalTranslated, requestedModel)
	translated, err = thinking.ApplyThinking(translated, req.Model, from.String(), to.String(), e.Identifier())
	if err != nil {
		return resp, err
	}
	translated = sanitizeCopilotCompatResponsesPayload(translated, baseModel, true)

	url := strings.TrimSuffix(baseURL, "/") + "/responses/compact"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(translated))
	if err != nil {
		return resp, err
	}
	applyCopilotCompatHeaders(httpReq, auth, apiKey, translated, false, true)

	authID, authLabel, authType, authValue := requestAuthLogFields(auth)
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      translated,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		recordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	defer closeLoggedBody("copilot compat executor", httpResp.Body)
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		logWithRequestID(ctx).Debugf("request error, error status: %d, error message: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		return resp, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		recordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	appendAPIResponseChunk(ctx, e.cfg, body)
	state := newCopilotCompatResponsesState()
	normalized := normalizeCopilotCompatResponseIDs(body, state)
	reporter.publishWithContent(ctx, parseCopilotCompatResponseUsage(normalized), string(req.Payload), string(body))
	out := translateResponsesCompactNonStream(ctx, from, req.Model, originalPayload, translated, normalized)
	return cliproxyexecutor.Response{Payload: []byte(out), Headers: httpResp.Header.Clone()}, nil
}

func (e *CopilotCompatExecutor) executeResponsesStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (_ *cliproxyexecutor.StreamResult, err error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	reporter := newUsageReporter(ctx, e.Identifier(), baseModel, auth)
	defer reporter.trackFailure(ctx, &err)

	apiKey, baseURL, err := e.resolveCredentials(auth)
	if err != nil {
		return nil, err
	}

	from := opts.SourceFormat
	to := sdktranslator.FormatOpenAIResponse
	originalPayload := originalRequestPayload(req.Payload, opts.OriginalRequest)
	originalTranslated := sdktranslator.TranslateRequest(from, to, baseModel, originalPayload, true)
	translated := sdktranslator.TranslateRequest(from, to, baseModel, req.Payload, true)
	requestedModel := payloadRequestedModel(opts, req.Model)
	translated = applyPayloadConfigWithRoot(e.cfg, baseModel, to.String(), "", translated, originalTranslated, requestedModel)
	translated, err = thinking.ApplyThinking(translated, req.Model, from.String(), to.String(), e.Identifier())
	if err != nil {
		return nil, err
	}
	translated = sanitizeCopilotCompatResponsesPayload(translated, baseModel, false)

	url := strings.TrimSuffix(baseURL, "/") + "/responses"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	applyCopilotCompatHeaders(httpReq, auth, apiKey, translated, true, true)

	authID, authLabel, authType, authValue := requestAuthLogFields(auth)
	recordAPIRequest(ctx, e.cfg, upstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      translated,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		recordAPIResponseError(ctx, e.cfg, err)
		return nil, err
	}
	recordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		closeLoggedBody("copilot compat executor", httpResp.Body)
		logWithRequestID(ctx).Debugf("request error, error status: %d, error message: %s", httpResp.StatusCode, summarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		return nil, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	out := make(chan cliproxyexecutor.StreamChunk)
	reporter.setInputContent(string(req.Payload))
	go func() {
		defer close(out)
		defer closeLoggedBody("copilot compat executor", httpResp.Body)
		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(nil, 52_428_800)
		state := newCopilotCompatResponsesState()
		var param any
		for scanner.Scan() {
			line := bytes.Clone(scanner.Bytes())
			appendAPIResponseChunk(ctx, e.cfg, line)
			if len(line) == 0 {
				continue
			}
			if !bytes.HasPrefix(bytes.TrimSpace(line), []byte("data:")) {
				continue
			}
			normalized := normalizeCopilotCompatResponseIDs(line, state)
			reporter.appendOutputChunk(normalized)
			if detail, ok := parseCodexUsage(jsonPayload(normalized)); ok {
				reporter.publish(ctx, detail)
			}
			chunks := translateResponsesStream(ctx, to, from, req.Model, opts.OriginalRequest, translated, normalized, &param)
			for i := range chunks {
				out <- cliproxyexecutor.StreamChunk{Payload: []byte(chunks[i])}
			}
		}
		if errScan := scanner.Err(); errScan != nil {
			recordAPIResponseError(ctx, e.cfg, errScan)
			reporter.publishFailure(ctx)
			out <- cliproxyexecutor.StreamChunk{Err: errScan}
		}
		reporter.ensurePublished(ctx)
	}()
	return &cliproxyexecutor.StreamResult{Headers: httpResp.Header.Clone(), Chunks: out}, nil
}

func (e *CopilotCompatExecutor) resolveCredentials(auth *cliproxyauth.Auth) (apiKey string, baseURL string, err error) {
	apiKey, baseURL = codexCreds(auth)
	if strings.TrimSpace(baseURL) == "" {
		return "", "", statusErr{code: http.StatusUnauthorized, msg: "missing provider baseURL"}
	}
	if strings.TrimSpace(apiKey) == "" {
		return "", "", statusErr{code: http.StatusUnauthorized, msg: "missing provider api key"}
	}
	return strings.TrimSpace(apiKey), strings.TrimSpace(baseURL), nil
}

func (e *CopilotCompatExecutor) shouldUseResponsesAPI(model string, opts cliproxyexecutor.Options) bool {
	baseModel := strings.ToLower(strings.TrimSpace(thinking.ParseSuffix(model).ModelName))
	if !isCopilotResponsesModel(baseModel) {
		return false
	}
	if opts.Alt == "responses/compact" {
		return true
	}
	switch opts.SourceFormat {
	case sdktranslator.FormatOpenAIResponse, sdktranslator.FormatClaude, sdktranslator.FormatGemini, sdktranslator.FormatGeminiCLI, sdktranslator.FormatAntigravity:
		return true
	default:
		return false
	}
}

func isCopilotResponsesModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" || strings.HasPrefix(model, "gpt-5-mini") {
		return false
	}
	if !strings.HasPrefix(model, "gpt-") {
		return false
	}
	rest := strings.TrimPrefix(model, "gpt-")
	if len(rest) == 0 || rest[0] < '0' || rest[0] > '9' {
		return false
	}
	return rest[0] >= '5'
}

func originalRequestPayload(payload []byte, original []byte) []byte {
	if len(original) > 0 {
		return original
	}
	return payload
}

func requestAuthLogFields(auth *cliproxyauth.Auth) (id, label, authType, authValue string) {
	if auth == nil {
		return "", "", "", ""
	}
	authType, authValue = auth.AccountInfo()
	return auth.ID, auth.Label, authType, authValue
}

func applyCopilotCompatHeaders(r *http.Request, auth *cliproxyauth.Auth, token string, body []byte, stream bool, responses bool) {
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	if stream {
		r.Header.Set("Accept", "text/event-stream")
		r.Header.Set("Cache-Control", "no-cache")
	} else {
		r.Header.Set("Accept", "application/json")
	}
	r.Header.Set("Connection", "Keep-Alive")

	userAgent := downstreamHeaderValue(r, "User-Agent")
	if userAgent == "" {
		userAgent = copilotCompatDefaultUserAgent
	}
	r.Header.Set("User-Agent", userAgent)
	r.Header.Set("Openai-Intent", "conversation-edits")
	r.Header.Set("x-initiator", copilotCompatInitiator(body, responses))
	if copilotCompatHasVision(body, responses) {
		r.Header.Set("Copilot-Vision-Request", "true")
	} else {
		r.Header.Del("Copilot-Vision-Request")
	}

	var attrs map[string]string
	if auth != nil {
		attrs = auth.Attributes
	}
	util.ApplyCustomHeadersFromAttrs(r, attrs)
}

func downstreamHeaderValue(r *http.Request, name string) string {
	if r == nil {
		return ""
	}
	if ginCtx, ok := r.Context().Value("gin").(*gin.Context); ok && ginCtx != nil && ginCtx.Request != nil {
		return strings.TrimSpace(ginCtx.Request.Header.Get(name))
	}
	return ""
}

func copilotCompatInitiator(body []byte, responses bool) string {
	payload := gjson.ParseBytes(body)
	if responses {
		input := payload.Get("input")
		if input.IsArray() {
			items := input.Array()
			if len(items) > 0 {
				if role := strings.TrimSpace(items[len(items)-1].Get("role").String()); role != "" && !strings.EqualFold(role, "user") {
					return "agent"
				}
			}
		}
		return "user"
	}
	messages := payload.Get("messages")
	if messages.IsArray() {
		items := messages.Array()
		if len(items) > 0 {
			if role := strings.TrimSpace(items[len(items)-1].Get("role").String()); role != "" && !strings.EqualFold(role, "user") {
				return "agent"
			}
		}
	}
	return "user"
}

func copilotCompatHasVision(body []byte, responses bool) bool {
	payload := gjson.ParseBytes(body)
	if responses {
		input := payload.Get("input")
		if !input.IsArray() {
			return false
		}
		for _, item := range input.Array() {
			content := item.Get("content")
			if !content.IsArray() {
				continue
			}
			for _, part := range content.Array() {
				if strings.EqualFold(strings.TrimSpace(part.Get("type").String()), "input_image") {
					return true
				}
			}
		}
		return false
	}
	messages := payload.Get("messages")
	if !messages.IsArray() {
		return false
	}
	for _, message := range messages.Array() {
		content := message.Get("content")
		if !content.IsArray() {
			continue
		}
		for _, part := range content.Array() {
			if strings.EqualFold(strings.TrimSpace(part.Get("type").String()), "image_url") {
				return true
			}
		}
	}
	return false
}

func sanitizeCopilotCompatChatPayload(body []byte, model string) []byte {
	body, _ = sjson.DeleteBytes(body, "stream_options")
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gemini-") {
		body, _ = sjson.DeleteBytes(body, "reasoning_effort")
		body = sanitizeCopilotCompatGeminiTools(body)
	}
	return body
}

func sanitizeCopilotCompatResponsesPayload(body []byte, model string, compact bool) []byte {
	body, _ = sjson.SetBytes(body, "model", strings.TrimSpace(model))
	body, _ = sjson.DeleteBytes(body, "previous_response_id")
	body, _ = sjson.DeleteBytes(body, "prompt_cache_retention")
	body, _ = sjson.DeleteBytes(body, "safety_identifier")
	if compact {
		body, _ = sjson.DeleteBytes(body, "stream")
	} else {
		body, _ = sjson.SetBytes(body, "stream", true)
	}
	if !gjson.GetBytes(body, "instructions").Exists() {
		body, _ = sjson.SetBytes(body, "instructions", "")
	}
	return body
}

func translateResponsesStream(ctx context.Context, upstreamFormat, sourceFormat sdktranslator.Format, model string, originalRequest, translatedRequest, line []byte, param *any) []string {
	if sourceFormat == sdktranslator.FormatOpenAIResponse {
		return []string{string(line)}
	}
	return sdktranslator.TranslateStream(ctx, upstreamFormat, sourceFormat, model, originalRequest, translatedRequest, line, param)
}

func translateResponsesNonStream(ctx context.Context, sourceFormat sdktranslator.Format, model string, originalRequest, translatedRequest, line []byte) string {
	if sourceFormat == sdktranslator.FormatOpenAIResponse {
		payload := jsonPayload(line)
		if len(payload) == 0 {
			return string(line)
		}
		if response := gjson.GetBytes(payload, "response"); response.Exists() {
			return response.Raw
		}
		return string(payload)
	}
	var param any
	return sdktranslator.TranslateNonStream(ctx, sdktranslator.FormatOpenAIResponse, sourceFormat, model, originalRequest, translatedRequest, line, &param)
}

func translateResponsesCompactNonStream(ctx context.Context, sourceFormat sdktranslator.Format, model string, originalRequest, translatedRequest, body []byte) string {
	if sourceFormat == sdktranslator.FormatOpenAIResponse {
		return string(body)
	}
	var param any
	return sdktranslator.TranslateNonStream(ctx, sdktranslator.FormatOpenAIResponse, sourceFormat, model, originalRequest, translatedRequest, body, &param)
}

func parseCopilotCompatResponseUsage(body []byte) usage.Detail {
	if detail, ok := parseCodexUsage(body); ok {
		return detail
	}
	return parseOpenAIUsage(body)
}

func closeLoggedBody(prefix string, body io.Closer) {
	if body == nil {
		return
	}
	if err := body.Close(); err != nil {
		log.Errorf("%s: close response body error: %v", prefix, err)
	}
}
