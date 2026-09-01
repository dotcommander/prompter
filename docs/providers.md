# Providers

Purpose: provider implementation contract and extension path.

```bash
prompter refine -p omlx "tighten this prompt"
prompter refine -p omlx --base-url http://127.0.0.1:8000/v1 "tighten this prompt"
```

## Prerequisites

- Familiarity with `main.go` and `internal/provider/`
- Access to a provider API key and base URL for remote providers
- A running OMLX server when using `omlx`

## Main workflow or contract

All providers implement the `Provider` interface in `internal/provider/provider.go`:

```go
type CallRequest struct {
    Model        string
    SystemPrompt string
    UserPrompt   string
    Effort       string
}

type Provider interface {
    Name() string
    Model() string
    APIKey() string
    Call(ctx context.Context, req CallRequest) (string, error)
    StreamCall(ctx context.Context, req CallRequest, w io.Writer) error
}
```

Current behavior:

- OpenAI uses the Responses API (`Instructions` + input string), constructed via `provider.NewOpenAI`.
- Gemini uses the Vertex AI `GenerateContent` implementation with ADC by default, or the Google AI Studio endpoint when `gemini.base_url` points at `generativelanguage.googleapis.com` (sending `GEMINI_API_KEY` as `x-goog-api-key`), constructed via `provider.NewGemini`.
- Cerebras, DeepSeek, Groq, OpenRouter, Zai, and OMLX use standard Chat Completions (`/v1/chat/completions`), constructed via `provider.NewChat`.
- For third-party OpenAI-compatible servers (vLLM, Ollama, LocalAI) that expose `/v1/chat/completions`, use `-p omlx` with `--base-url <url>` (no API key required).
- Unknown provider names fail fast with a clear error from `resolveProvider`.

Configuration values are loaded from standard environment variables (`<PROVIDER>_API_KEY`),
`PROMPTER_<PROVIDER>_*` variables, or an optional JSON block per provider. After those values are merged,
`config.Load` normalizes the eight closed provider blocks into `Config.Providers`.

`resolveProvider` copies those values into `provider.ProviderSettings`, applies a
`--base-url` override only to the selected copy, and passes the map to
`provider.NewRegistry`:

```go
rc := provider.RegistryConfig{
	MaxOutputTokens: cfg.MaxOutputTokens,
	MaxRetries:      cfg.MaxRetries,
	Providers:       settings,
}
registry := provider.NewRegistry(rc)
```

The registry's private descriptor list is the source of truth for names,
transports, and keyless-local behavior. It is intentionally closed: configuration
cannot register arbitrary providers or change authentication and path rules.

Current registered providers: `cerebras`, `deepseek`, `gemini`, `groq`, `omlx`, `openai`, `openrouter`, `zai`.

Built-in model defaults:

- `openai`: `gpt-5.6-luna`
- `groq`: `qwen/qwen3.8-27b`
- `cerebras`: `gpt-oss-120b`
- `deepseek`: `deepseek-v4-pro`
- `openrouter`: `openrouter/free`
- `zai`: `glm-5.3-flash`
- `gemini`: `gemini-3.7-flash`
- `omlx`: `Ornith-1.5-35B-A3B-oQ4e-mtp`

`prompter models refresh` fetches Models.dev metadata and caches up to five
eligible choices per provider. Models with output pricing at or above $15 per
million tokens are excluded for all providers except Cerebras and Groq; choices
at or below $5 are preferred. OpenRouter is ranked by OpenRouter's official [`sort=top-weekly`](https://openrouter.ai/docs/api/api-reference/models/list-all-models-and-their-properties)
order before those price rules are applied. OMLX choices come from the configured local `/v1/models`
endpoint when it is available.

`omlx` targets the local MLX server on localhost:8000:

- `api_key` is optional for unauthenticated local access (defaults to `"local"`).
- `model` defaults to `Ornith-1.5-35B-A3B-oQ4e-mtp`.
- `base_url` defaults to `http://127.0.0.1:8000/v1`.
- Standard OpenAI chat completions endpoint.

`gemini` targets Google Vertex AI `generateContent`:

- Uses Google Cloud Application Default Credentials (ADC) by default (`gcloud auth application-default print-access-token`).
- Vertex requires `gemini.project_id` or one of `PROMPTER_GEMINI_PROJECT_ID`,
  `GEMINI_PROJECT_ID`, `GOOGLE_CLOUD_PROJECT`, or `GCP_PROJECT`.
- `gemini.location` defaults to `global`.
- `gemini.model` defaults to `gemini-3.7-flash`.
- Streaming and reasoning effort (`thinkingConfig`) are supported.

To add a provider:

1. Add a named legacy block to `internal/config.ConfigFile`, then normalize it into `Config.Providers` and include it in `Save`.
2. Add a closed descriptor selecting `NewOpenAI`, `NewGemini`, or `NewChat` in `internal/provider`.
3. Add configuration and registry characterization tests.
4. Update CLI help text provider list.
5. Run full verification.

## Verification checklist

- Provider resolves from `-p/--provider`
- Missing key/model errors are explicit
- Local providers that do not require keys have an explicit sentinel contract
- API calls return provider-prefixed errors
- `go build ./...` and `go test ./...` pass

## Related docs

- `change-prompter.md`
- `setup.md`
