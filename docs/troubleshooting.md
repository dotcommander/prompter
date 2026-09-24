# Troubleshooting

## A retired command printed a migration error

Older command words (`critique`, `rewrite`, `apply`, `browse`, `models refresh`, `prompts status|upgrade`, `image`, `configure`, `config`) are removed and exit with code 2. Use `--image` for image assembly, `--config` for configuration, and `refine` (or the bare default form) for enrichment. To pass a retired word as literal input, put it after `--`:

```bash
prompter -- critique
```

## "Only one operation" or collision error

Only one operation runs per invocation. `--image` and `--config` cannot be combined, an operation flag cannot be mixed with `refine`, and `--config` rejects positional input. Exit code is 2.

## "input required" on a bare run

Running `prompter` with no arguments on a non-interactive stdin exits 1. Pass text, use `--file PATH`, or pipe stdin:

```bash
printf 'rough prompt' | prompter
```

On an interactive terminal, a bare run prints help instead.

## Dry run fails or shows the wrong provider

`prompter refine --dry-run` prints the resolved provider, model, base URL, and credential source to stderr without contacting the provider. Check the printed `Credential source`: it names the environment variable or configuration field being read. See [Providers](providers.md) for the per-provider variables.

## Credential or authentication errors exit 1

Remote enrichment resolves a provider; a missing API key or invalid ADC setup fails before any request when detectable, and otherwise the provider returns an authentication error. Configure credentials with `prompter --config` or environment variables, then re-run the dry run to confirm the resolved source.

## Streamed output looks truncated after a failure

`--stream` writes tokens as they arrive. If the call fails mid-stream, partial text may be on stdout even though the exit code is nonzero. Discard streamed output after a nonzero exit; use the default buffered call in automation.

## `--config` opened nothing

The configuration form requires an interactive terminal on both stdin and stdout. With redirected output, `--config` prints the resolved non-secret configuration instead — that is the expected behavior in scripts and CI.

## Timeout or retry behavior

Request timeouts come from configuration (`timeout` in seconds); streaming enforces a minimum timeout of 180 seconds. `max_retries` remains in configuration for compatibility but generation requests are not automatically retried: after a transport or server failure, the provider may already have processed the request. A manual retry is a new request and may incur additional usage. `prompter --config` shows the resolved values.

## Input size errors

Input is capped at 1 MB from any source (arguments, `--file`, stdin). Exceeding it exits 1; split the input or trim the file.

## Still stuck?

Check [Common tasks](common-tasks.md) for the intended invocation, and [CLI flags](flags.md) for the exact accepted flags.
