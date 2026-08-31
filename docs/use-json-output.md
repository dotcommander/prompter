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

## Verification checklist

- Integration path uses an explicit command such as `prompter refine ...`
- Automation does not call bare `prompter` without input
- Caller handles non-zero exit codes

## Related docs

- `setup.md`
- `common-tasks.md`
- `troubleshooting.md`
