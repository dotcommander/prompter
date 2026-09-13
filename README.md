# Prompter

Turn rough prompt material into a usable prompt from the terminal. From this checkout, start with the offline image assembler: it prints an assembled prompt to standard output and needs no provider credentials.

| If you want to… | Start with… | What happens |
| --- | --- | --- |
| Assemble an image prompt without a network call | `image <subject>` | Prints an assembled prompt built from local components. |
| Improve, critique, rewrite, or apply a prompt | `refine`, `critique`, `rewrite`, or `apply` | Sends the prepared input to the configured LLM provider. |
| Find or maintain local prompt files | `browse` or `prompts status\|upgrade` | Searches the prompt vault or reports/stages starter-prompt changes. |
| Inspect or change resolved settings | `configure` | Opens the terminal wizard, or prints non-secret settings when output is redirected. |
| Refresh model choices | `models refresh` | Fetches catalog data and updates the local model-choice cache. |

## First use: assemble an offline image prompt

**Prerequisite:** this module declares Go `1.26.3`. The command below is safe to run from a checkout; `GOWORK=off` makes it use this module rather than a parent workspace.

```bash
GOWORK=off go run . image "desert observatory" --profile minimal
```

It prints an image-generation prompt to standard output. In the checked runtime, the result was:

```text
desert observatory, clean composition, concept art
```

The command selects a local assembly profile and combines the supplied subject with embedded components. It builds prompt text only; it does not generate an image.

Source-checked variation (not executed for this README): request structured output with `--json`.

```bash
GOWORK=off go run . image "desert observatory" --json
```

## Configuration and local state

Prompter resolves settings in this order:

```text
CLI flags > environment variables > ~/.config/prompter/config.json > defaults
```

`prompter configure` opens a configuration form on an interactive terminal. When standard output is redirected, it prints resolved non-secret settings instead. Configuration includes the active provider, model, provider endpoint, prompt locations, image component file, timeout, output-token budget, retry count, and buffered-result clipboard preference.

The prompt vault is configured through `prompts_dir` and `prompts_dirs`. `browse` searches local Markdown prompt files; an empty primary vault can be seeded with starter prompts. `models refresh` stores its catalog cache at `~/.config/prompter/models-dev.json`.

## Non-goals and operating limits

- `image` assembles an image-generation prompt; it does not create an image.
- `models refresh` is a networked catalog refresh, unlike the offline image path above.
- A streamed provider response can write partial text before the provider reports a failed or incomplete terminal state. Treat streamed output from a nonzero exit as unusable.
- Prompt-output validation requires buffered output, so validated `apply` prompts reject `--stream`.

## Capability reference

### LLM prompt operations

`refine`, `critique`, `rewrite`, and `apply` accept input from arguments, `--file`, or standard input. `apply` selects a catalog prompt by exact name or alias; its prompt body becomes the system prompt and its frontmatter is not sent to the provider.

Use command help to inspect supported flags before making a remote request:

```bash
prompter refine --help
```

The LLM commands support provider/model selection, file input, output-file writing, buffered clipboard copying, dry-run inspection, streaming, endpoint overrides, and verbose timing. `--output` writes the buffered result both to the named file and standard output; it cannot be combined with `--stream`.

### Local prompt vault

```bash
prompter prompts status
prompter prompts upgrade --dry-run
```

`status` classifies starter prompts. `upgrade --dry-run` previews missing-prompt installation and versioned replacement candidates without writing them. A non-dry-run upgrade installs missing prompts and stages replacements rather than overwriting existing files.

### Configuration and model catalog

```bash
prompter configure
prompter models refresh
```

Run `configure` on an interactive terminal for the form. Run `models refresh` only when a networked catalog update is intended; it refreshes choices from Models.dev, OpenRouter, and a local OMLX endpoint when available.

## Verification and contribution

From the repository root, run:

```bash
GOWORK=off go test -count=1 ./...
GOWORK=off go build -o prompter .
GOWORK=off go vet ./...
gofmt -l .
```

The first command runs the repository test packages. The build produces a local `prompter` binary. A clean `gofmt -l .` produces no path output.

For detailed command flags, setup notes, prompt-file format, providers, and troubleshooting, see:

- [Setup](docs/setup.md)
- [Flags](docs/flags.md)
- [Prompt files](docs/prompt-files.md)
- [Providers](docs/providers.md)
- [Troubleshooting](docs/troubleshooting.md)

## Limits and non-goals

Remote prompt operations need a configured provider and its applicable authentication. The offline `image` command is the credential-free path.
