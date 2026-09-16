# Automation and image JSON

Use explicit commands in scripts, capture standard output as the result, and treat a nonzero exit as failure even when a stream has already written text.

## First automation check

**Prerequisite:** the image command is the credential-free path. Run this source-checked, unexecuted example from the repository root when you need structured output.

```bash
GOWORK=off go run . image "desert observatory" --json
```

With the default `--count 1`, it prints one JSON object containing the assembled prompt and its selected components. With `--count N` for `N > 1`, it prints a JSON array. The same image assembly that produces text output produces these objects; it does not contact a provider.

## Standard streams and exit codes

Remote generated text and image results are written to standard output. Errors and verbose timing are written to standard error. Prompter exits `0` on success, `1` for runtime, configuration, validation, or provider errors, `2` for command-line syntax or command errors, and `130` after cancellation.

When standard input is piped with no command, Prompter defaults to `refine`. Interactive bare `prompter` prints usage. Use an explicit command in scripts so a future reader can see the intended operation.

## Capture safely

The following source-checked, unexecuted example captures only standard output:

```bash
result="$(prompter refine "normalize this prompt")"
```

It can make a remote provider request. Check the exit status before using `result`. With `--stream`, Prompter can write partial text before a provider reports an incomplete terminal state, so discard captured stream output after any nonzero exit.

`--output` is a buffered-output feature: it writes the result to its named file and standard output. It cannot be combined with `--stream`. `--dry-run` writes resolved-setting diagnostics to standard error and does not make a provider request.

## Related docs

- [Common tasks](common-tasks.md)
- [CLI flags](flags.md)
- [Prompt files](prompt-files.md)
- [Troubleshooting](troubleshooting.md)