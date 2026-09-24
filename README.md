# Prompter

Want a prompt you can inspect or pipe into another tool? Prompter assembles image-generation prompt text offline, or sends rough text to a configured AI provider for enrichment. From this checkout, start with the offline example below: it prints a prompt without credentials or an image-generation request.

| Path | Start here | Result |
| --- | --- | --- |
| Offline image prompt | [Assemble a prompt](#assemble-a-prompt-offline) | Prompt text on stdout; `--json` for structured output. |
| Provider enrichment | [Enrich text](#enrich-text) | Improved prompt text on stdout; requires provider access. |
| Configuration | [Configuration and local state](#configuration-and-local-state) | Interactive form on a terminal, resolved settings when redirected. |

## Assemble a prompt offline

**Prerequisite:** Go compatible with the module's `go 1.26.3` directive. Run this from the repository root. `GOWORK=off` uses this module rather than a parent Go workspace.

```bash
GOWORK=off go run . --image "desert observatory" --profile minimal
```

Expected stdout (verified against the current source and runtime):

```text
desert observatory, clean composition, concept art
```

The subject and selected embedded components become one prompt string. `--image` does not generate an image or call a provider. To use the result in a pipeline, try `printf 'desert observatory\n' | GOWORK=off go run . --image --profile minimal`. For structured output, use `--json`; with `--count 2`, the result is a JSON array of two variations. See [CLI flags](docs/flags.md) for the image options.

## Enrich text

**Prerequisite:** choose a provider and configure its credentials or local endpoint before making a live request. Provider details are in [Providers](docs/providers.md). A dry run needs input but does not call the provider:

```bash
printf 'rough prompt' | GOWORK=off go run . refine --dry-run --provider groq
```

It prints resolved settings to stderr and no prompt to stdout (verified). This lets you inspect the selected model and credential source without sending the text. When configured, omit `--dry-run` to request an enriched prompt:

```bash
printf 'rough prompt' | GOWORK=off go run . refine --provider groq
```

That live command is source-checked, not executed here. The input travels to the selected provider; its response goes to stdout. The bare form `prompter "rough prompt"` and the explicit `prompter refine "rough prompt"` select the same enrichment operation. If installed or built as `prompter`, piped input also works with `printf 'rough prompt' | prompter`. For file input and output, use `--file` and `--output`; the output file is additional to stdout. See [CLI flags](docs/flags.md) and [Automation](docs/use-json-output.md).

## Configuration and local state

Configuration uses CLI overrides where available, environment variables, `~/.config/prompter/config.json`, and built-in defaults in that order. The exact provider variables and defaults are described in [Providers](docs/providers.md). Configuration saves to `~/.config/prompter/config.json`; paths using `~` are expanded when loaded and saved portably. The default image component path is `~/.config/prompter/components.json`: if it is absent, the image operation uses embedded components; it does not need to create that file. Existing components at the configured path are read for assembly, not written by that operation.

`prompter --config` opens a local form only when stdin and stdout are interactive terminals. With redirected output it prints resolved settings instead; it does not contact a model catalog. The displayed settings are not a credential-validation test. Before putting configuration output in a public log, inspect configured endpoint values and paths. The form writes the config file when saved.

## Verification and contribution

From the repository root, after installing a compatible Go toolchain, these commands build and exercise the code, documentation checks, and eval harness tests:

```bash
GOWORK=off go build ./...
GOWORK=off go test -count=1 . ./doctests/... ./internal/config ./internal/provider ./evals/enhance
GOWORK=off go vet ./...
```

The test command was run for this README; the build and vet commands above are source-checked but not executed for this documentation change. `GOWORK=off go run . --help` shows the root operation list. For repository layout and more checks, see [Contributor guide](docs/change-prompter.md); for setup, see [Setup](docs/setup.md).

## Limits and non-goals

- Image assembly produces prompt text, not an image. Remote enrichment requires a configured provider; it may incur provider usage costs. A dry run makes no provider call.
- Only one operation runs per invocation. `--image` and `--config` cannot be combined with each other or `refine`; retired command words return usage errors rather than calling a provider. To pass one as literal enrichment input, put it after `--`.
- File and piped input are limited to 1 MiB. A streamed provider call can leave partial stdout on failure, so check the exit status before consuming captured output. `--stream` cannot be combined with `--output` or `--copy`.
- For failures and exit codes, see [Troubleshooting](docs/troubleshooting.md) and [Automation](docs/use-json-output.md).
