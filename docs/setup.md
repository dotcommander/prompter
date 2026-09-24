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

Provider-specific environment variables and endpoints are documented in [Providers](providers.md). Gemini uses Google Application Default Credentials plus a project ID by default.

## Verify the installation

```bash
prompter --version
prompter refine --dry-run --provider groq
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
