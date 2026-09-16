# Prompter

Turn rough prompt material into usable AI prompts from the terminal. From this checkout, begin with the credential-free `image` command: it assembles prompt text from local components and prints that text to standard output; it does not generate an image.

| Goal | Command | Result |
| --- | --- | --- |
| Assemble an image prompt offline | `image <subject>` | Prints prompt text built from a subject and local components. |
| Improve or assess prompt material | `refine`, `critique`, or `rewrite` | Sends the prepared input to the selected LLM provider. |
| Apply a saved prompt | `apply <prompt-name> [input]` | Uses a catalog prompt selected by exact name or alias. |
| Work with local prompt files | `browse` or `prompts status\|upgrade` | Browses a vault or inspects and stages starter-prompt updates. |
| Inspect settings or model choices | `configure` or `models refresh` | Prints or changes resolved settings, or refreshes the model-choice cache. |

## First use: assemble an offline image prompt

**Prerequisite:** this module declares Go `1.26.3`. Run this command from the repository root; `GOWORK=off` selects this module instead of a parent Go workspace.

```bash
GOWORK=off go run . image "desert observatory" --profile minimal
```

It prints:

```text
desert observatory, clean composition, concept art
```

The `minimal` profile combines the supplied subject with selected local components. It builds a prompt string only, so it makes no provider request and does not create an image.

Source-checked, not executed for this README: add `--json` to emit the assembled result as JSON.

```bash
GOWORK=off go run . image "desert observatory" --json
```

## Configuration and local state

Prompter resolves settings in this order:

```text
CLI flags > environment variables > ~/.config/prompter/config.json > defaults
```

`prompter configure` opens a configuration form only when standard input and output are interactive terminals. With redirected output, it prints the resolved non-secret configuration instead. The configuration file stores provider, prompt-directory, component-library, timeout, output-token, retry, and clipboard settings.

By default, the primary prompt directory is `~/.config/prompter/prompts.d`; the configured prompt search directories also include `~/.config/roles/prompts`. The default component-library location is `~/.config/prompter/components.json`. `models refresh` stores its cache at `~/.config/prompter/models-dev.json`.

## Non-goals and operating limits

- `image` assembles image-generation prompt text; it does not generate an image.
- `browse` requires interactive standard input and standard error terminals.
- `models refresh` makes network requests to refresh catalog data; it is not part of the offline image path.
- Validated catalog prompts reject `--stream`, because validation needs buffered output.
- A streamed provider call can write partial text before it exits with an error; discard captured streamed output after a nonzero exit.

## Capability reference

### Prompt operations

`refine`, `critique`, `rewrite`, and `apply` take input from positional arguments, `--file`, or standard input. `apply` requires a prompt name or alias; it uses the selected prompt body's text as the system prompt after parsing its frontmatter.

Before a remote operation, inspect the command-specific options:

```bash
prompter <command> --help
```

This source-checked, unexecuted example is the safe way to inspect the accepted flags without contacting a provider. LLM commands support provider, model, endpoint, file-input, output-file, clipboard, dry-run, streaming, and verbose-timing options. `--output` writes a buffered result to both its named file and standard output; it cannot be combined with `--stream`.

### Local prompt vault

`prompter browse` opens the interactive local prompt browser. `prompter prompts status` classifies starter prompts. `prompter prompts upgrade --dry-run` previews changes without writing; without `--dry-run`, upgrades install missing prompts and place replacement candidates alongside existing files rather than overwriting them.

### Configuration and model catalog

`prompter configure` is the terminal configuration route described above. `prompter models refresh` fetches Models.dev and OpenRouter catalog data, then writes the local cache; it also attempts to include choices from the local OMLX server.

## Verification and contribution

Source-checked, not executed for this documentation-only change:

```bash
GOWORK=off go test -count=1 ./...
GOWORK=off go build -o prompter .
GOWORK=off go vet ./...
gofmt -l .
```

The test command runs repository test packages. The build creates a local `prompter` binary. `gofmt -l .` prints paths only for files that need formatting.

For detailed setup, flags, prompt-file format, provider behavior, automation, and troubleshooting, see [the documentation index](docs/index.md), [Setup](docs/setup.md), [CLI flags](docs/flags.md), [Prompt files](docs/prompt-files.md), [Providers](docs/providers.md), [Automation](docs/use-json-output.md), and [Troubleshooting](docs/troubleshooting.md).

## Limits and non-goals

Remote prompt operations need a configured provider. The offline `image` command is the supported credential-free path.
