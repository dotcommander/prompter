# Use CLI Output in Automation

Purpose: integrate prompter in scripts and tools using a stable stdout/stderr contract.

## Prerequisites

- Prompter installed on the machine
- Configured provider API key

## Main workflow or contract

- Input contract:
  - `prompter "text"` or `echo "text" | prompter` runs enhancement.
  - `prompter` with no input starts the interactive finder. Do not use this mode in unattended automation.
- Output contract:
  - Enhanced prompt text is written to stdout.
  - Errors and verbose timing are written to stderr.
- Exit behavior:
  - Exit code `0` on success.
  - Exit code `1` on validation, config, or provider errors.
  - Exit code `130` on Ctrl+C.

Automation-safe examples:

```bash
# Safe non-interactive call
prompter enhance "normalize this prompt"

# Capture stdout only
result="$(prompter "write release notes")"

# Keep stderr separate
prompter -v "write changelog" 1>enhanced.txt 2>debug.log
```

## Verification checklist

- Integration path uses `prompter enhance ...` or passes explicit input
- Automation does not call bare `prompter` without input
- Caller handles non-zero exit codes

## Related docs

- `setup.md`
- `common-tasks.md`
- `troubleshooting.md`
