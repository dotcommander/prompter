# Contributor guide

This guide covers local changes to the Prompter repository: build, test, and documentation-consistency requirements.

## Repository layout

```text
main.go               Entry point, flag parsing, operation dispatch
cli_flow.go           Operation selection, migration errors, flag grammar
cli_lifecycle.go      Execution pipeline, exit codes, bare-invocation handling
cli_metadata.go       Usage, version, and config printing
prompt_boundary.go    Untrusted-input envelope for enrichment
prompts.go            System-prompt loading
embed.go              Embedded prompts and styles
components.go         Offline image-prompt assembler
config_tui.go         Interactive configuration form
internal/config/      Config resolution, precedence, portable paths
internal/provider/    Provider interface and implementations
prompts/              Embedded prompt and style sources
doctests/             Documentation-consistency tests
evals/enhance/        Source-bound enrichment eval harness
docs/                 User-facing documentation
```

The three operations live in `cli_flow.go` (`selectOperation`): enrichment (`refine` or the default form), `--image`, and `--config`. Retired command words return migration errors with exit code 2 there.

## Build and focused tests

```bash
GOWORK=off go build ./...
GOWORK=off go test -count=1 .
GOWORK=off go test -count=1 ./doctests/...
GOWORK=off go test -count=1 ./evals/enhance
```

`GOWORK=off` keeps the build on this module when a parent `go.work` exists.

## Full verification

Run the focused suites above, plus:

```bash
go vet ./...
gofmt -l .
GOWORK=off go mod verify
```

`gofmt -l .` prints paths only for files needing formatting.

## Documentation-consistency tests

`doctests/` enforces contracts between code and docs:

- `flags_test.go` — [docs/flags.md](flags.md) documents the operation flags, including provider, style, and streaming flags and their defaults.
- `providers_test.go` — every registered provider name appears in [docs/providers.md](providers.md) and the repository `AGENTS.md`.

If you add or rename a flag or provider, update those docs in the same change; the tests fail otherwise.

## Eval harness

`evals/enhance/` runs fixture-based checks of the enrichment path. Fixtures may use only the `refine` operation and its flags; `evalFlagParity` additionally compares the evaluator's value-flag set against every value flag the CLI registers, including the `--image` flags. Keep `fixtureImageValueFlags` in `runner.go` in sync when changing the image grammar.

## Adding an operation? Read this first

The CLI deliberately exposes exactly three operations. New functionality should extend an existing operation (new flags on `refine` or `--image`) unless a spec change authorizes a fourth. Any new operation must: get its own flag set in `parseArgs`, reject collisions with existing operations, appear in `printUsageTo` and `printCommandUsageTo`, and be documented in [CLI flags](flags.md).

## Configuration and local state

Config resolution and portable `~` paths live in `internal/config` with unit tests in `config_test.go`. Prompter never modifies or deletes existing files under `~/.config/prompter` beyond writing `config.json` and the components file it owns.

## Related pages

- [Setup](setup.md) for the build commands.
- [CLI flags](flags.md) for the user-facing grammar.
