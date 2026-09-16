# Setup

Build Prompter from this checkout, confirm the offline path, then configure a provider only when you need remote prompt operations.

## First check: offline image prompt

**Prerequisite:** this module declares Go `1.26.3`. Run the command from the repository root; `GOWORK=off` selects this module rather than a parent Go workspace.

```bash
GOWORK=off go run . image "desert observatory" --profile minimal
```

It prints an assembled image-generation prompt. The image command loads a local component library and builds text; it does not make a provider request or generate an image.

Source-checked, unexecuted variation: add `--json` to produce a JSON object for the default single result.

```bash
GOWORK=off go run . image "desert observatory" --json
```

## Build a local binary

Source-checked, unexecuted for this documentation task:

```bash
GOWORK=off go build -o prompter .
```

This creates `./prompter`. Use `./prompter --help` to list commands before making a remote request.

## Configure a remote provider

Remote `refine`, `critique`, `rewrite`, and `apply` operations need a resolved provider. `prompter configure` opens its form only when standard input and output are interactive terminals. With redirected output, it prints the resolved non-secret configuration instead.

Prompter reads configuration in this order:

```text
CLI flags > environment variables > ~/.config/prompter/config.json > defaults
```

The configuration file can hold the provider, model, endpoint, prompt locations, component-library location, timeout, output-token budget, retry count, and buffered-result clipboard preference. See [Providers](providers.md) for supported providers and [CLI flags](flags.md) for command overrides.

## Verify a change

Source-checked, unexecuted for this documentation task:

```bash
GOWORK=off go build ./...
GOWORK=off go test -count=1 ./...
GOWORK=off go test -count=1 ./doctests/...
GOWORK=off go vet ./...
gofmt -l .
```

The `justfile` provides the same checks through `just qa`; it runs formatting verification, vet, tests, doctests, and a build.

## Related docs

- [CLI flags](flags.md)
- [Prompt files](prompt-files.md)
- [Providers](providers.md)
- [Troubleshooting](troubleshooting.md)