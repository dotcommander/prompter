# CLI flags

Prompter has three operations: enrichment (the default, also available as the `refine` command word), offline image assembly (`--image`), and configuration (`--config`). Only one operation runs per invocation.

| Operation | How to invoke it | Reads input | Uses the network | What it outputs |
| --- | --- | --- | --- | --- |
| Enrichment (default) | `prompter [input]`, `prompter refine [input]`, or piped stdin | Positional text, `--file`, or stdin | Yes (provider call) | The enriched prompt on stdout |
| Image assembly | `prompter --image [subject]` | Positional subject or `--file` | No (offline) | The assembled prompt text on stdout |
| Configuration | `prompter --config` | None (positional input is rejected) | No | The configuration form on a TTY; resolved settings when redirected |

## Global flags

| Flag | Meaning |
| --- | --- |
| `-h`, `--help` | Show help. With an operation (`prompter --image --help`), shows that operation's flags. |
| `-V`, `--version` | Show version and build information. |

## Enrichment flags

`refine` is the only command word; `prompter "rough prompt"` is equivalent to `prompter refine "rough prompt"`.

### Provider flags

| Flag | Alias | Description | Default |
| --- | --- | --- | --- |
| `--provider PROVIDER` | `-p` | Provider name: `gemini`, `openai`, `cerebras`, `deepseek`, `groq`, `omlx`, `openrouter`, or `zai`. | Configured provider (`gemini` on first use) |
| `--model MODEL` | `-m` | Model override for this call. | Configured provider model |
| `--base-url URL` | — | Provider endpoint override for this call. | Configured provider endpoint |

The provider determines which credentials and environment variables are read; see [Providers](providers.md).

### Style flags

| Flag | Meaning |
| --- | --- |
| `-s STYLE` | Short form of `--style`. |
| `--style STYLE` | Use a named style: `default`, `code`, `concise`, `creative`, or `spec`. User overrides live in `~/.config/prompter/styles/<name>.md`. |

### Input and output handling

| Flag | Meaning |
| --- | --- |
| `-f, --file PATH` | Read input from `PATH` instead of positional text or stdin. |
| `-o, --output FILE` | Also write the result to `FILE`; stdout still receives it. |
| `-c, --copy` | Copy the result to the system clipboard. |
| `--stream` | Stream tokens to stdout as they arrive. Incompatible with `--output` and `--copy`. |
| `--dry-run` | Print resolved settings to stderr and exit 0 without calling the provider. |
| `-v, --verbose` | Print timing diagnostics to stderr. |

### Examples

```bash
prompter refine -p groq 'explain quantum computing simply'
prompter refine -m gpt-5.6-luna 'improve these notes'
prompter refine --base-url https://api.openai.com/v1 -p openai 'improve these notes'
prompter refine -v 'improve these notes'
prompter refine -s concise 'improve these notes'
prompter refine --style spec 'improve these notes'
printf 'rough prompt' | prompter refine --stream
prompter 'explain quantum computing simply' --dry-run
prompter refine --file notes.md --output improved.md
GOWORK=off go run . --image 'desert observatory' --profile minimal
```

## Operation-specific flags

`--image` accepts:

| Flag | Meaning |
| --- | --- |
| `--profile NAME` | Component profile: `default`, `minimal`, or `maximal`. |
| `--count N` | Number of variations to assemble (default 1). |
| `--categories LIST` | Comma-separated modifier categories to include. |
| `--no-artist` | Omit artist references. |
| `--no-platform` | Omit platform references. |
| `--json` | Emit the assembled result as JSON. |
| `--seed VALUE` | Deterministic selection seed. |
| `-f, --file PATH` | Read the subject from `PATH`. |
| `-o, --output FILE` | Also write output to `FILE`. |
| `-c, --copy` | Copy output to the clipboard. |

`--image` example patterns:

```bash
prompter --image "portrait of a clockmaker" --profile minimal
prompter --image "portrait of a clockmaker" --count 2 --json
prompter --image "moon castle" --categories quality,composition --seed 7
```

`--config` takes no flags. Positional input with `--config` is a usage error. On an interactive terminal it opens the configuration form; with redirected output it prints the resolved non-secret configuration, including the active provider and model. See [Configuration](../README.md#configuration-and-local-state) for the config file location.

## Retired operations

The `critique`, `rewrite`, `apply`, `browse`, `models refresh`, and `prompts status|upgrade` operations are removed, as are the old `image` and `configure` command words. Typing one returns a migration error with exit code 2 and does not contact a provider. `--limit N` and similar retired flags are likewise rejected as undefined. Use `--image` and `--config` for their replacements; `refine` remains for enrichment.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success (including `--dry-run` and help/version output). |
| `1` | Runtime or input failure. |
| `2` | Usage failure: unknown flag, operation collision, or retired command. |
| `130` | Canceled with SIGINT. |

## Configuration precedence

Settings resolve in this order:

```text
CLI flags > environment variables > ~/.config/prompter/config.json > defaults
```

Run `prompter --config` with redirected output to print the resolved configuration, or run it on a terminal to change settings interactively.
