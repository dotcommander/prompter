# CLI Flags Reference

Purpose: canonical reference for all command-line flags.

## Flags

| Flag | Alias | Description | Default |
|------|-------|-------------|---------|
| `-p` | `--provider` | Provider to use for AI enhancement | `gemini` |
| `-m` | `--model` | Model override (provider-specific) | Provider default |
| `--base-url` | | Custom API endpoint override | Provider default |
| `-s` | `--style` | Enhancement style: `default`, `code`, `concise`, `creative`, `grai`, `spec` | `default` |
| `-f` | `--file` | Read prompt input from a file | unset |
| `-o` | `--output` | Write generated prompt to a file and stdout | unset |
| `-c` | `--copy` | Copy generated prompt to clipboard | `false` |
| | `--profile` | Assembly profile: `default`, `minimal`, `maximal` | `default` |
| | `--count` | Number of assembled variations | `1` |
| | `--categories` | Comma-separated modifier categories for assembly | unset |
| | `--no-artist` | Omit artist reference from assembled prompts | `false` |
| | `--no-platform` | Omit platform reference from assembled prompts | `false` |
| | `--json` | Output assemble/stats data as JSON | `false` |
| | `--seed` | Deterministic assembly seed | derived from input |
| `--dry-run` | | Show resolved runtime settings without an API call | `false` |
| `--stream` | | Stream tokens to stdout as they arrive | `false` |
| `-v` | `--verbose` | Show timing output | `false` |
| `-V` | `--version` | Show prompter version and build information | `false` |

## Usage examples

```bash
# Run a catalog prompt by exact name or alias with piped input
defuddle parse -m "$url" | prompter run grai-transform > readit.md

# Use OpenAI
prompter -p openai "explain this code"

# Use local Wormhole
prompter -p wormhole -m groq/openai/gpt-oss-120b "explain this code"

# Specify model
prompter -m gpt-4o "my prompt"

# Provider + model
prompter -p openai -m gpt-4o-mini "my prompt"

# Override base URL
prompter --base-url https://api.example.com "my prompt"

# Pick an enhancement style
prompter -s code "write a retry loop"
prompter --style concise "summarize this"
prompter -s grai "extract names"
prompter -s spec "add OAuth login with refresh token rotation"

# Read input from a file
prompter -f draft-prompt.txt

# Write the generated prompt to a file and stdout
prompter "write a release checklist" -o prompt.txt

# Copy the result to clipboard
prompter --copy "write a release checklist"

# Assemble offline image prompts
prompter assemble "desert observatory"
prompter assemble "portrait of a clockmaker" --profile minimal
prompter assemble "futurist tram" --categories quality,composition --json

# Show component-library stats
prompter stats
```

`--output` writes the response to the specified file while simultaneously emitting it to `stdout` (dual-sink), making it ideal for saving a copy while piping into downstream commands (e.g. `prompter -o prompt.txt "..." | llm`). Standard shell redirection (`> prompt.txt`) directs stdout exclusively to the file. `--output` is for non-streamed responses and cannot be combined with `--stream`.

Wormhole supports streaming through its OpenAI-compatible endpoint.

```bash
# Check resolved provider/model/input without an API call
prompter --dry-run -s spec -f draft-prompt.txt

# Stream tokens as they are generated
prompter --stream "write a haiku about goroutines"

# Show timing
prompter -v "my prompt"
```

## Configuration

Default values can be set in `~/.config/prompter/config.json`. CLI flags always override config values.

`components_file` points to a JSON component library for `assemble` and `stats`. When the file is missing, Prompter uses its embedded default components.

`default_copy` copies every non-streamed generated or assembled result to the
clipboard. Streaming rejects copying because streamed output is not buffered.

## Related docs

- `common-tasks.md`
- `setup.md`
