package codexcompat

import (
	. "github.com/router-for-me/CLIProxyAPI/v6/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/interfaces"
	codexclaude "github.com/router-for-me/CLIProxyAPI/v6/internal/translator/codex/claude"
	codexgemini "github.com/router-for-me/CLIProxyAPI/v6/internal/translator/codex/gemini"
	codexgeminicli "github.com/router-for-me/CLIProxyAPI/v6/internal/translator/codex/gemini-cli"
	codexchat "github.com/router-for-me/CLIProxyAPI/v6/internal/translator/codex/openai/chat-completions"
	codexresponses "github.com/router-for-me/CLIProxyAPI/v6/internal/translator/codex/openai/responses"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/translator/translator"
)

func init() {
	translator.Register(
		OpenAI,
		CodexCompat,
		codexchat.ConvertOpenAIRequestToCodex,
		interfaces.TranslateResponse{
			Stream:    codexchat.ConvertCodexResponseToOpenAI,
			NonStream: codexchat.ConvertCodexResponseToOpenAINonStream,
		},
	)

	translator.Register(
		OpenaiResponse,
		CodexCompat,
		codexresponses.ConvertOpenAIResponsesRequestToCodex,
		interfaces.TranslateResponse{
			Stream:    ConvertCodexCompatResponseToOpenAIResponses,
			NonStream: ConvertCodexCompatResponseToOpenAIResponsesNonStream,
		},
	)

	translator.Register(
		Claude,
		CodexCompat,
		codexclaude.ConvertClaudeRequestToCodex,
		interfaces.TranslateResponse{
			Stream:     codexclaude.ConvertCodexResponseToClaude,
			NonStream:  codexclaude.ConvertCodexResponseToClaudeNonStream,
			TokenCount: codexclaude.ClaudeTokenCount,
		},
	)

	translator.Register(
		Gemini,
		CodexCompat,
		codexgemini.ConvertGeminiRequestToCodex,
		interfaces.TranslateResponse{
			Stream:     codexgemini.ConvertCodexResponseToGemini,
			NonStream:  codexgemini.ConvertCodexResponseToGeminiNonStream,
			TokenCount: codexgemini.GeminiTokenCount,
		},
	)

	translator.Register(
		GeminiCLI,
		CodexCompat,
		codexgeminicli.ConvertGeminiCLIRequestToCodex,
		interfaces.TranslateResponse{
			Stream:     codexgeminicli.ConvertCodexResponseToGeminiCLI,
			NonStream:  codexgeminicli.ConvertCodexResponseToGeminiCLINonStream,
			TokenCount: codexgeminicli.GeminiCLITokenCount,
		},
	)
}
