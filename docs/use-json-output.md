# Use CLI Output in Automation

Purpose: integrate prompter in scripts and tools using a stable stdout/stderr contract.

## Prerequisites

- Prompter installed on the machine
- Configured provider API key

## Main workflow or contract

- Input contract:
  - `prompter refine "text"` or `echo "text" | prompter refine` runs refinement.
  - `prompter` with no command prints usage help. Do not use `prompter browse` in unattended automation.
- Output contract:
  - Enhanced prompt text is written to stdout.
  - Errors and verbose timing are written to stderr.
- Exit behavior:
  - Exit code `0` on success.
  - Exit code `1` on validation, config, or provider errors.
  - Exit code `2` on unknown commands or invalid command-line syntax.
  - Exit code `130` on Ctrl+C.
  - Streaming may write partial text before a truncation error; discard captured
    output whenever the exit code is nonzero.

Automation-safe examples:

```bash
# Safe non-interactive call
prompter refine "normalize this prompt"

# Capture stdout only
result="$(prompter refine "write release notes")"

# Keep stderr separate
prompter refine -v "write changelog" 1>enhanced.txt 2>debug.log
```

## JSON output (`image`)

Only `prompter image` emits JSON (`--json`). The shape depends on `--count`:

- `--count 1` (default) → a single JSON object
- `--count N` (N > 1) → a JSON array of objects

```bash
# Single result (default) is a bare object
prompter image "desert observatory" --json | jq -r '.full_prompt'

# Multiple results are an array
prompter image "desert observatory" --count 3 --json | jq -r '.[].full_prompt'

# Normalize either shape before further processing
prompter image "portrait of a clockmaker" --json |
  jq -c 'if type == "array" then .[] else . end'
```

Remote commands (`refine`, `critique`, `rewrite`, `apply`) emit plain text on
stdout; they have no JSON mode. Use `--dry-run` for resolved-setting diagnostics
on stderr, but rely on the exit code and stdout/stderr contract above.

## Verification checklist

- Integration path uses an explicit command such as `prompter refine ...`
- Automation does not call bare `prompter` without input
- Caller handles non-zero exit codes

## Related docs

- `setup.md`
- `common-tasks.md`
- `troubleshooting.md`
