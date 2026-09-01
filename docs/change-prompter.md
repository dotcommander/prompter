# Change Prompter

Purpose: contributor guide for implementation changes and verification.

## Prerequisites

- Go toolchain installed
- Ability to run `go build` and `go test`

## Main workflow

1. Find the implementation area:
   - `main.go`: CLI parsing, dispatch, input/output boundaries
   - `internal/config`: configuration loading and environment precedence
   - `internal/provider`: provider interface, transports, and registry
   - `model_catalog.go`: Models.dev catalog fetch/cache for `models refresh` and `configure`
   - `prompts.go`: prompt loading and frontmatter parsing
   - `finder.go`: interactive fuzzy finder
2. Make the smallest change that solves the target behavior.
3. If you add or change flags, update `flags.md`.
4. If you change finder behavior, update `finder.md`.
5. If you add a provider, update `providers.md` and config examples.

## Verification checklist

Run from repo root (the `justfile` already exports `GOWORK=off`):

```bash
GOWORK=off go build ./...
GOWORK=off go test -count=1 ./...
GOWORK=off go test -count=1 ./doctests/...
GOWORK=off go vet ./...
gofmt -l .
```

Or run `just qa`. Expect all checks to pass before opening a PR. Documentation
changes must keep the doctests in `./doctests/` green.

## Related docs

- `providers.md`
- `prompt-files.md`
