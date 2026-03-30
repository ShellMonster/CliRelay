package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

const copilotCompatReasoningOpaqueKey = "reasoning_opaque"

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
	translated, cacheID := prepareCodexCompatiblePayload(from, req, translated, false)
	httpReq, err := buildCodexCompatibleRequest(ctx, url, translated, cacheID)
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
	translated, cacheID := prepareCodexCompatiblePayload(from, req, translated, false)
	httpReq, err := buildCodexCompatibleRequest(ctx, url, translated, cacheID)
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
		var sawTerminalChunk bool
		var pendingUsage usage.Detail
		var hasPendingUsage bool
		for scanner.Scan() {
			line := bytes.Clone(scanner.Bytes())
			appendAPIResponseChunk(ctx, e.cfg, line)
			reporter.appendOutputChunk(line)
			payload := jsonPayload(line)
			if isCopilotResponsesTerminalPayload(payload) {
				sawTerminalChunk = true
			}
			if errMsg := copilotChatTerminalError(payload); errMsg != nil {
				recordAPIResponseError(ctx, e.cfg, errMsg)
				reporter.publishFailure(ctx)
				out <- cliproxyexecutor.StreamChunk{Err: errMsg}
				return
			}
			if detail, ok := parseOpenAIStreamUsage(line); ok {
				pendingUsage = detail
				hasPendingUsage = true
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
			return
		}
		if !sawTerminalChunk {
			errIncomplete := statusErr{code: http.StatusRequestTimeout, msg: "stream error: stream disconnected before completion: stream closed before finish_reason"}
			recordAPIResponseError(ctx, e.cfg, errIncomplete)
			reporter.publishFailure(ctx)
			out <- cliproxyexecutor.StreamChunk{Err: errIncomplete}
			return
		}
		if hasPendingUsage {
			reporter.publish(ctx, pendingUsage)
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
	translated, cacheID := prepareCodexCompatiblePayload(from, req, translated, true)
	httpReq, err := buildCodexCompatibleRequest(ctx, url, translated, cacheID)
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
	translated, cacheID := prepareCodexCompatiblePayload(from, req, translated, true)
	httpReq, err := buildCodexCompatibleRequest(ctx, url, translated, cacheID)
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
	translated, cacheID := prepareCodexCompatiblePayload(from, req, translated, true)
	httpReq, err := buildCodexCompatibleRequest(ctx, url, translated, cacheID)
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
		var sawFinishedChunk bool
		var pendingUsage usage.Detail
		var hasPendingUsage bool
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
			payload := jsonPayload(normalized)
			if isCopilotResponsesFinishedPayload(payload) {
				sawFinishedChunk = true
			}
			if errMsg := copilotResponsesTerminalError(payload); errMsg != nil {
				recordAPIResponseError(ctx, e.cfg, errMsg)
				reporter.publishFailure(ctx)
				out <- cliproxyexecutor.StreamChunk{Err: errMsg}
				return
			}
			reporter.appendOutputChunk(normalized)
			if detail, ok := parseCodexUsage(payload); ok {
				pendingUsage = detail
				hasPendingUsage = true
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
			return
		}
		if !sawFinishedChunk {
			errIncomplete := statusErr{code: http.StatusRequestTimeout, msg: "stream error: stream disconnected before completion: stream closed before response.completed"}
			recordAPIResponseError(ctx, e.cfg, errIncomplete)
			reporter.publishFailure(ctx)
			out <- cliproxyexecutor.StreamChunk{Err: errIncomplete}
			return
		}
		if hasPendingUsage {
			reporter.publish(ctx, pendingUsage)
		}
		reporter.ensurePublished(ctx)
	}()
	return &cliproxyexecutor.StreamResult{Headers: httpResp.Header.Clone(), Chunks: out}, nil
}

func isCopilotResponsesFinishedPayload(payload []byte) bool {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return false
	}
	typeName := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	return typeName == "response.completed" || typeName == "response.incomplete" || typeName == "response.done"
}

func isCopilotResponsesTerminalPayload(payload []byte) bool {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return false
	}
	if finishReason := strings.TrimSpace(gjson.GetBytes(payload, "choices.0.finish_reason").String()); finishReason != "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(gjson.GetBytes(payload, "object").String()), "chat.completion.chunk") && gjson.GetBytes(payload, "usage").Exists()
}

func copilotResponsesTerminalError(payload []byte) error {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return nil
	}
	if strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "error" {
		return nil
	}
	message := strings.TrimSpace(gjson.GetBytes(payload, "message").String())
	if message == "" {
		message = "upstream responses stream returned error"
	}
	return statusErr{code: http.StatusBadGateway, msg: message}
}

func copilotChatTerminalError(payload []byte) error {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return nil
	}
	if !gjson.GetBytes(payload, "error").Exists() {
		return nil
	}
	message := strings.TrimSpace(gjson.GetBytes(payload, "error.message").String())
	if message == "" {
		message = strings.TrimSpace(gjson.GetBytes(payload, "error").String())
	}
	if message == "" {
		message = "upstream chat stream returned error"
	}
	return statusErr{code: http.StatusBadGateway, msg: message}
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
	if opts.SourceFormat == sdktranslator.FormatCodex || opts.SourceFormat == sdktranslator.FormatCodexCompat {
		return false
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
	body = restoreCopilotCompatChatReasoning(body)
	body = normalizeCopilotCompatChatReasoningEffort(body, model)
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gemini-") {
		body, _ = sjson.DeleteBytes(body, "reasoning_effort")
		body = sanitizeCopilotCompatGeminiTools(body)
	}
	return body
}

func restoreCopilotCompatChatReasoning(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}
	for i, message := range messages.Array() {
		if !strings.EqualFold(strings.TrimSpace(message.Get("role").String()), "assistant") {
			continue
		}
		reasoningOpaque := copilotCompatReasoningOpaque(message)
		reasoningText := strings.TrimSpace(message.Get("reasoning_text").String())
		content := message.Get("content")
		if reasoningOpaque == "" && content.IsArray() {
			for _, part := range content.Array() {
				if opaque := copilotCompatReasoningOpaque(part); opaque != "" && reasoningOpaque == "" {
					reasoningOpaque = opaque
				}
				if reasoningText == "" && strings.EqualFold(strings.TrimSpace(part.Get("type").String()), "reasoning") {
					reasoningText = copilotCompatReasoningText(part)
				}
			}
		}
		if reasoningOpaque == "" {
			body, _ = sjson.DeleteBytes(body, fmt.Sprintf("messages.%d.reasoning_text", i))
			body, _ = sjson.DeleteBytes(body, fmt.Sprintf("messages.%d.%s", i, copilotCompatReasoningOpaqueKey))
			body, _ = sjson.DeleteBytes(body, fmt.Sprintf("messages.%d.providerOptions.copilot.reasoningOpaque", i))
			continue
		}
		if reasoningText != "" && !message.Get("reasoning_text").Exists() {
			body, _ = sjson.SetBytes(body, fmt.Sprintf("messages.%d.reasoning_text", i), reasoningText)
		}
		if !message.Get(copilotCompatReasoningOpaqueKey).Exists() {
			body, _ = sjson.SetBytes(body, fmt.Sprintf("messages.%d.%s", i, copilotCompatReasoningOpaqueKey), reasoningOpaque)
		}
		if content.IsArray() {
			filtered := make([]any, 0, len(content.Array()))
			for _, part := range content.Array() {
				if strings.EqualFold(strings.TrimSpace(part.Get("type").String()), "reasoning") {
					continue
				}
				filtered = append(filtered, part.Value())
			}
			body, _ = sjson.SetBytes(body, fmt.Sprintf("messages.%d.content", i), filtered)
		}
	}
	return body
}

func normalizeCopilotCompatChatReasoningEffort(body []byte, model string) []byte {
	root := gjson.ParseBytes(body)
	if root.Get("reasoning.effort").Exists() && !root.Get("reasoning_effort").Exists() {
		body, _ = sjson.SetBytes(body, "reasoning_effort", root.Get("reasoning.effort").String())
	}
	if shouldStripCopilotChatReasoningObject(model) {
		body, _ = sjson.DeleteBytes(body, "reasoning")
	}
	return body
}

func shouldStripCopilotChatReasoningObject(model string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(trimmed, "gpt-5") && !strings.HasPrefix(trimmed, "gpt-5-mini") {
		return false
	}
	return true
}

func sanitizeCopilotCompatResponsesPayload(body []byte, model string, compact bool) []byte {
	body = restoreCopilotCompatResponsesInput(body)
	body, _ = sjson.SetBytes(body, "model", strings.TrimSpace(model))
	body, _ = sjson.DeleteBytes(body, "prompt_cache_retention")
	body, _ = sjson.DeleteBytes(body, "safety_identifier")
	if compact {
		body, _ = sjson.DeleteBytes(body, "stream")
	} else {
		body, _ = sjson.SetBytes(body, "stream", true)
	}
	if !gjson.GetBytes(body, "store").Exists() {
		body, _ = sjson.SetBytes(body, "store", true)
	}
	if isCopilotResponsesModel(model) && !gjson.GetBytes(body, "reasoning.summary").Exists() {
		body, _ = sjson.SetBytes(body, "reasoning.summary", "auto")
	}
	body = ensureCopilotCompatInclude(body, "reasoning.encrypted_content")
	if !gjson.GetBytes(body, "instructions").Exists() {
		body, _ = sjson.SetBytes(body, "instructions", "")
	}
	return body
}

func restoreCopilotCompatResponsesInput(body []byte) []byte {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || !input.IsArray() {
		return body
	}
	for i, item := range input.Array() {
		itemType := strings.TrimSpace(item.Get("type").String())
		switch itemType {
		case "message", "":
			content := item.Get("content")
			if !content.Exists() || !content.IsArray() {
				continue
			}
			for j, part := range content.Array() {
				partType := strings.ToLower(strings.TrimSpace(part.Get("type").String()))
				if partType == "reasoning" {
					body = restoreCopilotCompatReasoningPart(body, fmt.Sprintf("input.%d.content.%d", i, j), part)
					continue
				}
				if partType == "output_text" || partType == "input_text" || partType == "text" {
					if opaque := copilotCompatReasoningOpaque(part); opaque != "" {
						body, _ = sjson.SetBytes(body, fmt.Sprintf("input.%d.%s", i, copilotCompatReasoningOpaqueKey), opaque)
					}
				}
			}
		case "reasoning":
			body = restoreCopilotCompatReasoningItem(body, fmt.Sprintf("input.%d", i), item)
		}
	}
	return body
}

func restoreCopilotCompatReasoningPart(body []byte, path string, part gjson.Result) []byte {
	text := copilotCompatReasoningText(part)
	if text != "" {
		body, _ = sjson.SetBytes(body, path+".summary", []map[string]string{{"type": "summary_text", "text": text}})
	}
	if opaque := copilotCompatReasoningOpaque(part); opaque != "" {
		body, _ = sjson.SetBytes(body, path+".encrypted_content", opaque)
	}
	if itemID := copilotCompatReasoningItemID(part); itemID != "" {
		body, _ = sjson.SetBytes(body, path+".id", itemID)
	}
	body, _ = sjson.SetBytes(body, path+".type", "reasoning")
	body, _ = sjson.DeleteBytes(body, path+".text")
	body, _ = sjson.DeleteBytes(body, path+".item_id")
	body, _ = sjson.DeleteBytes(body, path+"."+copilotCompatReasoningOpaqueKey)
	body, _ = sjson.DeleteBytes(body, path+".providerOptions")
	return body
}

func restoreCopilotCompatReasoningItem(body []byte, path string, item gjson.Result) []byte {
	if opaque := copilotCompatReasoningOpaque(item); opaque != "" && !item.Get("encrypted_content").Exists() {
		body, _ = sjson.SetBytes(body, path+".encrypted_content", opaque)
	}
	if text := copilotCompatReasoningText(item); text != "" && !item.Get("summary").Exists() {
		body, _ = sjson.SetBytes(body, path+".summary", []map[string]string{{"type": "summary_text", "text": text}})
	}
	if itemID := copilotCompatReasoningItemID(item); itemID != "" && !item.Get("id").Exists() {
		body, _ = sjson.SetBytes(body, path+".id", itemID)
	}
	body, _ = sjson.DeleteBytes(body, path+".text")
	body, _ = sjson.DeleteBytes(body, path+".item_id")
	body, _ = sjson.DeleteBytes(body, path+"."+copilotCompatReasoningOpaqueKey)
	body, _ = sjson.DeleteBytes(body, path+".providerOptions")
	return body
}

func copilotCompatReasoningOpaque(node gjson.Result) string {
	if opaque := strings.TrimSpace(node.Get(copilotCompatReasoningOpaqueKey).String()); opaque != "" {
		return opaque
	}
	return strings.TrimSpace(node.Get("providerOptions.copilot.reasoningOpaque").String())
}

func copilotCompatReasoningItemID(node gjson.Result) string {
	if itemID := strings.TrimSpace(node.Get("item_id").String()); itemID != "" {
		return itemID
	}
	if itemID := strings.TrimSpace(node.Get("id").String()); itemID != "" {
		return itemID
	}
	return strings.TrimSpace(node.Get("providerOptions.copilot.itemId").String())
}

func copilotCompatReasoningText(node gjson.Result) string {
	if text := strings.TrimSpace(node.Get("text").String()); text != "" {
		return text
	}
	summary := node.Get("summary")
	if summary.Exists() && summary.IsArray() {
		for _, part := range summary.Array() {
			if strings.EqualFold(strings.TrimSpace(part.Get("type").String()), "summary_text") {
				if text := strings.TrimSpace(part.Get("text").String()); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func ensureCopilotCompatInclude(body []byte, value string) []byte {
	value = strings.TrimSpace(value)
	if value == "" {
		return body
	}
	include := gjson.GetBytes(body, "include")
	if !include.Exists() || !include.IsArray() {
		body, _ = sjson.SetBytes(body, "include", []string{value})
		return body
	}
	for _, item := range include.Array() {
		if strings.EqualFold(strings.TrimSpace(item.String()), value) {
			return body
		}
	}
	body, _ = sjson.SetRawBytes(body, "include.-1", marshalJSONString(value))
	return body
}

func translateResponsesStream(ctx context.Context, upstreamFormat, sourceFormat sdktranslator.Format, model string, originalRequest, translatedRequest, line []byte, param *any) []string {
	if sourceFormat == sdktranslator.FormatOpenAIResponse {
		return []string{string(line)}
	}
	chunks := sdktranslator.TranslateStream(ctx, upstreamFormat, sourceFormat, model, originalRequest, translatedRequest, line, param)
	return annotateCopilotCompatTranslatedChunks(chunks)
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
	return annotateCopilotCompatTranslatedJSON(sdktranslator.TranslateNonStream(ctx, sdktranslator.FormatOpenAIResponse, sourceFormat, model, originalRequest, translatedRequest, line, &param))
}

func translateResponsesCompactNonStream(ctx context.Context, sourceFormat sdktranslator.Format, model string, originalRequest, translatedRequest, body []byte) string {
	if sourceFormat == sdktranslator.FormatOpenAIResponse {
		return string(body)
	}
	var param any
	return annotateCopilotCompatTranslatedJSON(sdktranslator.TranslateNonStream(ctx, sdktranslator.FormatOpenAIResponse, sourceFormat, model, originalRequest, translatedRequest, body, &param))
}

func annotateCopilotCompatTranslatedChunks(chunks []string) []string {
	if len(chunks) == 0 {
		return chunks
	}
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, annotateCopilotCompatTranslatedJSON(chunk))
	}
	return out
}

func annotateCopilotCompatTranslatedJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	prefix := ""
	payload := trimmed
	if strings.HasPrefix(payload, "data:") {
		prefix = "data: "
		payload = strings.TrimSpace(strings.TrimPrefix(payload, "data:"))
	}
	if !gjson.Valid(payload) {
		return raw
	}
	annotated := annotateCopilotCompatTranslatedPayload([]byte(payload))
	if prefix == "" {
		return string(annotated)
	}
	return prefix + string(annotated)
}

func annotateCopilotCompatTranslatedPayload(payload []byte) []byte {
	root := gjson.ParseBytes(payload)
	choices := root.Get("choices")
	if choices.Exists() && choices.IsArray() {
		for i, choice := range choices.Array() {
			message := choice.Get("message")
			if !message.Exists() {
				continue
			}
			if opaque := copilotCompatReasoningOpaque(message); opaque != "" {
				payload, _ = sjson.SetBytes(payload, fmt.Sprintf("choices.%d.message.providerOptions.copilot.reasoningOpaque", i), opaque)
			}
		}
	}
	messages := root.Get("messages")
	if messages.Exists() && messages.IsArray() {
		for i, message := range messages.Array() {
			content := message.Get("content")
			if !content.IsArray() {
				continue
			}
			for j, part := range content.Array() {
				typeName := strings.ToLower(strings.TrimSpace(part.Get("type").String()))
				switch typeName {
				case "reasoning":
					if itemID := strings.TrimSpace(part.Get("item_id").String()); itemID != "" {
						payload, _ = sjson.SetBytes(payload, fmt.Sprintf("messages.%d.content.%d.providerOptions.copilot.itemId", i, j), itemID)
					}
					if enc := strings.TrimSpace(part.Get("encrypted_content").String()); enc != "" {
						payload, _ = sjson.SetBytes(payload, fmt.Sprintf("messages.%d.content.%d.providerOptions.copilot.reasoningOpaque", i, j), enc)
					}
				case "text":
					if opaque := strings.TrimSpace(message.Get(copilotCompatReasoningOpaqueKey).String()); opaque != "" {
						payload, _ = sjson.SetBytes(payload, fmt.Sprintf("messages.%d.content.%d.providerOptions.copilot.reasoningOpaque", i, j), opaque)
					}
				}
			}
		}
	}
	if rawOpaque := strings.TrimSpace(root.Get("providerMetadata.copilot.reasoningOpaque").String()); rawOpaque != "" {
		payload, _ = sjson.SetBytes(payload, "providerMetadata.openai.reasoningEncryptedContent", rawOpaque)
	}
	return payload
}

func marshalJSONString(v string) []byte {
	raw, _ := json.Marshal(v)
	return raw
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
