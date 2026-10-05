# Setup

## Prerequisites

- Go 1.26 or newer (the module declares `go 1.26.3`).
- A terminal for interactive forms.

## Build from a checkout

From the repository root:

```bash
GOWORK=off go build -o prompter .
```

`GOWORK=off` selects this module instead of a parent Go workspace. The build writes a local `prompter` binary.

## Offline first check

```bash
GOWORK=off go run . --image "desert observatory" --profile minimal
```

`--image` builds an image-generation prompt from local components. It performs no network request, so it verifies the build without credentials.

## Configure remote prompting

Remote enrichment (`prompter refine` or the default form) requires a configured provider. Open the form:

```bash
prompter --config
```

The form runs only when stdin and stdout are interactive terminals; it uses the configured model and local model choices, so opening it makes no network request. With redirected output, `prompter --config` prints the resolved non-secret configuration instead.

Provider-specific environment variables and endpoints are documented in [Providers](providers.md). Gemini uses Google Application Default Credentials plus a project ID by default. The form provides Gemini project and location fields locally. Selecting the AI Studio endpoint does not require a Vertex project.

Use Ctrl+C to cancel the form: no settings are saved and the process exits 130. Ordinary `q` is text input. Custom environment selectors must be variable names, such as `TEAM_OPENAI_KEY`, rather than credential values. A custom base URL must be empty or an absolute HTTP(S) URL with a host; displayed endpoint credentials are redacted.

Configuration output identifies credential sources without printing API-key values. ADC status is unchecked: opening the form or printing configuration does not validate credentials.

### Persisted settings

The file stores deliberate settings rather than a copy of all resolved defaults and environment values. Saving preserves unrelated persisted settings, omits API-key values, and retains explicit values equal to defaults. Environment overrides still take precedence after saving. For example:

```json
{
  "provider": "openai",
  "openai": { "key_env": "TEAM_OPENAI_KEY" },
  "prompts_dirs": [],
  "default_copy": false,
  "max_retries": 0
}
```

Here `prompts_dirs: []` disables the directory list, while omitting the field selects defaults. `PROMPTER_PROMPTS_DIRS` accepts comma-separated entries with surrounding whitespace trimmed; setting it to an empty string also disables the list. Nonempty timeout and output-token environment values must be positive integers. Nonempty retry compatibility environment values must be nonnegative integers, and `0` is preserved. Explicit file values follow the same numeric bounds. Malformed numeric values fail configuration loading. Generation requests never retry, regardless of `max_retries`.

## Verify the installation

```bash
prompter --version
prompter refine --dry-run --provider groq "rough prompt"
```

The dry run prints resolved settings to standard error and exits 0 without contacting the provider.

## Verify the repository tests

```bash
GOWORK=off go test -count=1 . ./doctests/...
```

The command runs the root package and documentation-consistency tests. See [Contributor guide](change-prompter.md) for the full verification set.

## Next steps

- Operation and flag reference: [CLI flags](flags.md)
- Automation contracts: [Automation](use-json-output.md)
- Failure recovery: [Troubleshooting](troubleshooting.md)
