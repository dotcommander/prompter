# Providers

Purpose: provider implementation contract and extension path.

```bash
prompter -p wormhole "tighten this prompt"
prompter -p wormhole --base-url http://127.0.0.1:8080/v1 "tighten this prompt"
```

## Prerequisites

- Familiarity with `main.go` and `internal/provider/`
- Access to a provider API key and base URL for remote providers
- A running Wormhole proxy when using `wormhole`

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
- Gemini uses the Vertex AI `GenerateContent` implementation (or AI Studio when `GEMINI_API_KEY` is provided), constructed via `provider.NewGemini`.
- Cerebras, Synthetic, Groq, OpenRouter, Zai, Wormhole, and OMLX use standard Chat Completions (`/v1/chat/completions`), constructed via `provider.NewChat`.
- For third-party OpenAI-compatible servers (vLLM, Ollama, LocalAI) that expose `/v1/chat/completions`, use `-p wormhole` or `-p omlx` with `--base-url <url>` (no API key required).
- Wormhole uses its OpenAI-compatible HTTP endpoint; model prefixes such as `groq/openai/gpt-oss-120b` select the upstream provider.
- Unknown provider names fail fast with a clear error from `resolveProvider`.

Configuration values are loaded from standard environment variables (`<PROVIDER>_API_KEY`),
`PROMPTER_<PROVIDER>_*` variables, or an optional JSON block per provider. After those values are merged,
`config.Load` normalizes the nine closed provider blocks into `Config.Providers`.

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

Current registered providers: `cerebras`, `gemini`, `groq`, `omlx`, `openai`, `openrouter`, `synthetic`, `wormhole`, `zai`.

`omlx` targets the local MLX server on localhost:8000:

- `api_key` is optional for unauthenticated local access (defaults to `"local"`).
- `model` defaults to `LFM2.5-2.6B-4bit`.
- `base_url` defaults to `http://127.0.0.1:8000/v1`.
- Standard OpenAI chat completions endpoint.

`gemini` targets Google Vertex AI generateContent / Grimoire:

- Uses Google Cloud Application Default Credentials (ADC) by default (`gcloud auth application-default print-access-token`).
- `gemini.project_id` defaults to `grimoire-2025`.
- `gemini.location` defaults to `global`.
- `gemini.model` defaults to `gemini-3.7-flash`.
- Streaming and reasoning effort (`thinkingConfig`) are supported.

`wormhole` targets the local OpenAI-compatible proxy:

- `api_key` may be empty for an unauthenticated loopback proxy. Set it to the
  Wormhole bearer token when `WORMHOLE_API_KEY` is configured.
- `model` accepts Wormhole's `provider/model` routing form.
- `base_url` defaults to `http://127.0.0.1:8080/v1`.
- Streaming and `max_output_tokens` use the normal Chat Completions behavior.

To add a provider:

1. Add a named legacy block to `internal/config.ConfigFile` and `configTemplate`, then normalize it into `Config.Providers`.
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
