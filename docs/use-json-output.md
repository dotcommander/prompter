# Automation

Prompter is built for pipelines: machine-readable output goes to stdout, diagnostics go to stderr, and exit codes are stable.

## Stdout and stderr contract

- Operation output (enriched prompt, assembled image prompt, redirected `--config` settings) goes to stdout.
- Diagnostics — dry-run settings, spinners, verbose timing, errors — go to stderr.
- Interactive forms (`prompter --config` on a TTY, help) never write machine output to stdout.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success, including `--dry-run`, help, and version output. |
| `1` | Runtime or input failure, including missing input on a bare non-interactive run. |
| `2` | Usage failure: unknown flag, operation collision, positional input with `--config`, or a retired command word. |
| `130` | Canceled with SIGINT. |

Always check the exit status; a nonzero code means stdout must not be trusted.

## Image operation JSON

`--image --json` emits one JSON object per result with the subject, selected modifiers, profile, and full prompt text:

```bash
prompter --image "portrait of a clockmaker" --count 2 --json | jq -r '.[].full_prompt'
```

The JSON schema is stable: parse it with any JSON tool. With `--count` greater than 1, the array holds one object per variation. The same seed and subject produce the same output.

## Streaming caveat

`--stream` writes tokens to stdout as they arrive. If the call fails after streaming has begun, partial text may already be on stdout while the process exits nonzero. In automation, prefer the default buffered call, or discard captured streamed output on a nonzero exit.

## Recommended pipeline patterns

Buffer a result before using it:

```bash
out=$(prompter refine --file notes.md) || exit $?
printf '%s\n' "$out"
```

Record diagnostics separately from output:

```bash
prompter refine "rough prompt" > result.md 2> refine.log
```

Dry-run a configuration in CI before any provider call:

```bash
prompter refine --dry-run --provider groq > /dev/null
```

## Input limits

Input from arguments, files, or stdin is capped at 1 MB; exceeding it exits 1. `timeout` controls request duration. `max_retries` remains accepted for compatibility but generation requests are not automatically replayed after ambiguous failures; see [Troubleshooting](troubleshooting.md#timeout-or-retry-behavior).

## Related pages

- Flag reference: [CLI flags](flags.md)
- Configuration precedence: [Configuration and local state](../README.md#configuration-and-local-state)
