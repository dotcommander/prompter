# Common tasks

Use Prompter in a shell pipeline while keeping generated text on standard output and diagnostics on standard error.

## Refine piped input

**Prerequisite:** configure the provider you intend to use before sending remote input. The example is source-checked but unexecuted because it can call that provider.

```bash
printf '%s\n' 'Turn these notes into a release checklist.' | prompter refine
```

On success, the refined prompt is written to standard output. When standard input is piped and no command is supplied, Prompter also defaults to `refine`; the explicit command above makes a script's intent visible.

A successful exit is `0`. If the command exits nonzero, discard captured output: a streamed response can already contain partial text.

## Save a catalog-prompt result

**Prerequisite:** `apply` needs a prompt file whose name or alias matches `system-architect`. The following source-checked example can call the configured provider.

```bash
printf '%s\n' 'Design a rate limiter.' | prompter apply system-architect > architecture.md
```

The selected prompt body's text becomes the system prompt, and the generated result is redirected to `architecture.md`. Use [Prompt files](prompt-files.md) to create or locate the catalog entry.

## Keep diagnostics separate

```bash
prompter refine -v "write release notes" 1>enhanced.txt 2>debug.log
```

This source-checked, unexecuted example sends generated text to `enhanced.txt` and verbose timing to `debug.log`. The `-v` flag writes timing to standard error, so it does not contaminate the generated output.

## Related docs

- [CLI flags](flags.md)
- [Automation](use-json-output.md)
- [Prompt files](prompt-files.md)