# CLI Flags Reference

Purpose: canonical reference for all command-line flags.

## Global flags

Only `-h`/`--help` and `-V`/`--version` are global. All other flags belong to a command.

## LLM command flags

`refine`, `critique`, `rewrite`, and `apply` accept:

| Flag | Alias | Description | Default |
|------|-------|-------------|---------|
| `--provider` | `-p` | Provider to use | Configured provider (built-in `gemini`) |
| `--model` | `-m` | Provider-specific model override | Provider default |
| `--base-url` | | Custom API endpoint override | Provider default |
| `--file` | `-f` | Read input from a file | unset |
| `--output` | `-o` | Write output to a file and stdout | unset |
| `--copy` | `-c` | Copy buffered output to the clipboard | `false` |
| `--dry-run` | | Show resolved settings without an API call | `false` |
| `--stream` | | Stream tokens to stdout | `false` |
| `--verbose` | `-v` | Show timing output on stderr | `false` |

`refine` alone accepts `-s`/`--style`. `rewrite` alone accepts `--mode`.

## Image command flags

`image` accepts `--profile`, `--count`, `--categories`, `--no-artist`, `--no-platform`, `--json`, `--seed`, `--file`, `--output`, and `--copy`. It does not accept provider or model flags because it runs offline.
Category names must exist in the loaded component library and may appear only once;
unknown or duplicate names fail instead of producing a partial prompt.

## Prompt maintenance flags

`prompts upgrade` accepts `--dry-run` to preview missing prompt installation and
versioned replacement candidates without writing them. `prompts status` does not
accept `--dry-run` because status is already read-only.

## Usage examples

```bash
# Run a catalog prompt by exact name or alias with piped input
defuddle parse -m "$url" | prompter apply system-architect > plan.md

# Use OpenAI
prompter refine -p openai "explain this code"

# Use local OMLX
prompter refine -p omlx -m Ornith-1.5-35B-A3B-oQ4e-mtp "explain this code"

# Specify model
prompter refine -m gpt-5.6-luna "my prompt"

# Provider + model
prompter refine -p openai -m gpt-5.6-luna "my prompt"

# Override base URL
prompter refine --base-url https://api.example.com "my prompt"

# Pick an enhancement style
prompter refine -s code "write a retry loop"
prompter refine --style concise "summarize this"
prompter refine -s spec "add OAuth login with refresh token rotation"

# Read input from a file
prompter refine -f draft-prompt.txt

# Write the generated prompt to a file and stdout
prompter refine "write a release checklist" -o prompt.txt

# Copy the result to clipboard
prompter refine --copy "write a release checklist"

# Build offline image prompts
prompter image "desert observatory"
prompter image "portrait of a clockmaker" --profile minimal
prompter image "futurist tram" --categories quality,composition --json

# Preview starter prompt maintenance
prompter prompts upgrade --dry-run
```

`--output` writes the response to the specified file while simultaneously emitting it to `stdout` (dual-sink). Standard shell redirection (`> prompt.txt`) directs stdout exclusively to the file. `--output` is for non-streamed responses and cannot be combined with `--stream`.

OMLX supports streaming through its OpenAI-compatible endpoint.

```bash
# Check resolved provider/model/output budget/input without an API call
prompter refine --dry-run -s spec -f draft-prompt.txt

# Stream tokens as they are generated
prompter refine --stream "write a haiku about goroutines"

# Show timing
prompter refine -v "my prompt"
```

Dry-run output includes the resolved maximum output-token budget. It does not make
a provider request.

If a provider stops without a successful completion terminal state, including
because it reached its output-token limit, Prompter exits nonzero. A streamed call
may already have written partial text; discard that output on any nonzero exit.

## Configuration

Default values can be set in `~/.config/prompter/config.json`. CLI flags always override config values.

`components_file` points to the JSON component library used by `image`. When the file is missing, Prompter uses its embedded default components.

`default_copy` copies every non-streamed generated or image-prompt result to the
clipboard. Streaming rejects copying because streamed output is not buffered.

## Related docs

- `common-tasks.md`
- `setup.md`
