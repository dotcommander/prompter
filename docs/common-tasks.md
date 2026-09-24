# Common tasks

Every task here uses one of the three operations: enrichment (`refine` or the default form), offline image assembly (`--image`), or configuration (`--config`).

## Improve a rough prompt

```bash
prompter refine "write release notes from these bullets"
```

Pass a file instead:

```bash
prompter refine --file notes.md
```

## Preview settings before calling a provider

```bash
prompter refine --dry-run
```

The dry run prints the resolved provider, model, base URL, credential source, style, and limits to standard error, then exits 0. No request is sent.

## Pipe input through refinement

```bash
printf 'rough prompt' | prompter
printf 'rough prompt' | prompter refine --stream
```

Piped input without an operation runs enrichment, exactly like `prompter refine`.

## Assemble an image prompt offline

```bash
prompter --image "desert observatory" --profile minimal
prompter --image "moon castle" --count 2 --json
```

`--image` builds prompt text from local components. It makes no network request and does not generate an image.

## Show or change configuration

```bash
prompter --config            # interactive form on a TTY
prompter --config > cfg.txt  # redirected: prints resolved non-secret settings
```

## Save output to a file

```bash
prompter refine --file notes.md --output improved.md
```

## Check the resolved configuration in a script

```bash
prompter --config | grep -- '--config'
```

Redirected `--config` output is plain text, so it can be filtered without JSON parsing.

## Related pages

- Flag reference: [CLI flags](flags.md)
- Provider selection and credentials: [Providers](providers.md)
- Scripting contracts: [Automation](use-json-output.md)
