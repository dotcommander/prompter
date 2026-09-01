# Enhancement evaluation harness

This harness runs fixed enhancement fixtures through one explicitly selected
Prompter binary and appends sanitized, source-bound JSONL result records.

Build the exact binary first, then pass absolute paths:

```bash
GOWORK=off go build -o /absolute/path/to/prompter .
GOWORK=off go run ./evals/enhance \
  --binary /absolute/path/to/prompter \
  --fixtures ./evals/enhance/testdata/fixtures.json \
  --manifest /absolute/path/to/results.jsonl \
  --prompt /absolute/path/to/enhance.md \
  --root /absolute/path/to/prompter
```

Each dry run and evaluated fixture subprocess has a five-minute deadline.
Override it with `--fixture-timeout` (for example, `--fixture-timeout 2m`).
Captured stdout and stderr are each limited to 16 MiB.

The runner performs a non-provider dry run before each fixture to capture the
effective provider, model, redacted endpoint, credential-source name, retry policy,
Gemini project/location, output-token budget, timeout, and other safe
settings. It pins `PROMPTER_PROMPT_FILE` to the supplied absolute prompt path for
both dry-run and evaluated execution, so the recorded prompt hash identifies the
prompt actually selected through configuration precedence. It records hashes rather
than raw model output or stderr. Provider request IDs and usage remain explicit JSON
`null` values because the CLI does not currently expose them.

Rows are resumable only when binary, Git HEAD, tracked dirty diff, untracked source,
prompt, fixture, and effective-setting hashes match. The manifest itself is excluded
from the untracked-source hash because it is an output artifact. Manifests must be
outside the source repository. An exclusive `<manifest>.lock` directory serializes
runners, and each appended row is synced before the next fixture. A completed
matching row prevents a duplicate provider call. Failed or incomplete rows remain
retryable. Malformed or incompatible manifest rows stop execution instead of being
ignored. If a runner is forcibly killed, remove its stale lock directory only after
confirming no evaluator still owns it.

Fixtures may invoke only `refine`, with optional provider, model, style, and verbose
flags; prompt input belongs in the fixture's `input` field. File, output, endpoint,
clipboard, streaming, dry-run, positional-input, and other command surfaces are
rejected before execution. The harness passes only required configuration and the
selected provider's credential environment to subprocesses, and records flag values
as `<redacted>` rather than persisting raw arguments. Do not place credentials or
sensitive source text in fixture input because that input is intentionally sent to
the selected provider.
