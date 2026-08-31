# Change Prompter

Purpose: contributor guide for implementation changes and verification.

## Prerequisites

- Go toolchain installed
- Ability to run `go build` and `go test`

## Main workflow

1. Find the implementation area:
   - `main.go`: CLI parsing, config loading, providers
   - `prompts.go`: prompt loading and frontmatter parsing
   - `finder.go`: interactive fuzzy finder
2. Make the smallest change that solves the target behavior.
3. If you add or change flags, update `flags.md`.
4. If you change finder behavior, update `finder.md`.
5. If you add a provider, update `providers.md` and config examples.

## Verification checklist

Run from repo root:

```bash
go build ./...
go test ./...
```

Expect both commands to pass before opening a PR.

## Related docs

- `providers.md`
- `prompt-files.md`
