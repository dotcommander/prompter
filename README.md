# Prompter

Turn rough prompt material into usable AI prompts from the terminal. Prompter has three operations: enrichment (the default), offline image-prompt assembly, and configuration.

| Goal | Invocation | Result |
| --- | --- | --- |
| Improve rough prompt input | `prompter [input]` or `prompter refine [input]` | Sends the prepared input to the selected LLM provider. |
| Assemble an image prompt offline | `prompter --image <subject>` | Prints prompt text built from a subject and local components. |
| Inspect or change settings | `prompter --config` | Opens the configuration form, or prints resolved non-secret settings on redirected output. |

## First use: assemble an offline image prompt

**Prerequisite:** this module declares Go `1.26.3`. Run this command from the repository root; `GOWORK=off` selects this module instead of a parent Go workspace.

```bash
GOWORK=off go run . --image "desert observatory" --profile minimal
```

It prints:

```text
desert observatory, clean composition, concept art
```

The `minimal` profile combines the supplied subject with selected local components. It builds a prompt string only, so it makes no provider request and does not create an image.

Source-checked, not executed for this README: add `--json` to emit the assembled result as JSON.

```bash
GOWORK=off go run . --image "desert observatory" --json
```

## Enrichment

Enrichment is the default operation. All three forms run the same path:

```bash
prompter "rough prompt"                 # positional input
prompter refine "rough prompt"          # explicit alias
printf 'rough prompt' | prompter        # piped input
```

Provider, model, endpoint, style, file-input, output-file, clipboard, dry-run, streaming, and verbose-timing flags are documented in [CLI flags](docs/flags.md). `--dry-run` prints resolved settings to standard error without contacting a provider, so it is the safe first check for a new configuration.

## Migrating from earlier versions

Earlier releases exposed subcommands. They are intentionally removed, and typing one returns a migration error instead of silently calling a provider:

| Earlier command | Replacement |
| --- | --- |
| `image` | `--image` |
| `configure`, `config` | `--config` |
| `critique`, `rewrite`, `apply`, `browse`, `models refresh`, `prompts status\|upgrade` | Removed; no replacement |

`refine` remains the only command word. To pass a retired word as literal input, put it after `--` (for example, `prompter -- critique`).

## Configuration and local state

Prompter resolves settings in this order:

```text
CLI flags > environment variables > ~/.config/prompter/config.json > defaults
```

`prompter --config` opens the configuration form only when standard input and output are interactive terminals; the form uses the configured model and local model choices, so opening it makes no network request. With redirected output, it prints the resolved non-secret configuration instead. The configuration file stores provider, prompt-directory, component-library, timeout, output-token, retry, and clipboard settings.

The default component-library location is `~/.config/prompter/components.json`. Existing prompt vaults, catalogs, caches, and component files on disk are never modified or deleted by prompter.

## Non-goals and operating limits

- `--image` assembles image-generation prompt text; it does not generate an image, load a provider, or touch the network.
- Only one operation runs per invocation; combining `--image` with `--config` or mixing an operation flag with `refine` is a usage error.
- A streamed provider call can write partial text before it exits with an error; discard captured streamed output after a nonzero exit.

## Verification and contribution

Source-checked, not executed for this documentation-only change:

```bash
GOWORK=off go test -count=1 . ./doctests/...
GOWORK=off go build -o prompter .
GOWORK=off go vet ./...
gofmt -l .
```

The test command runs the repository test packages. The build creates a local `prompter` binary. `gofmt -l .` prints paths only for files that need formatting.

For detailed setup, flags, provider behavior, automation, and troubleshooting, see [the documentation index](docs/index.md), [Setup](docs/setup.md), [CLI flags](docs/flags.md), [Providers](docs/providers.md), [Automation](docs/use-json-output.md), and [Troubleshooting](docs/troubleshooting.md).

## Limits and non-goals

Remote enrichment needs a configured provider. The offline `--image` operation is the supported credential-free path.
