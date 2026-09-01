# Common Tasks

Use these recipes after completing [setup](setup.md). See [flags](flags.md) for
provider, model, file, clipboard, and streaming examples.

## Editor & Workflow Integrations

### Neovim / Vim

Replace selected text with a refined prompt:

```vim
:'<,'>!prompter refine -s code
```

Optional `init.lua` bindings:

```lua
vim.keymap.set("v", "<leader>pe", ":!prompter refine<CR>", { desc = "Prompter: Refine prompt" })
vim.keymap.set("v", "<leader>pc", ":!prompter refine -s code<CR>", { desc = "Prompter: Refine code prompt" })
vim.keymap.set("v", "<leader>pq", ":!prompter critique<CR>", { desc = "Prompter: Critique prompt" })
```

### Git helpers

```bash
git-prompt-commit() {
  { printf '%s\n\n' "Generate a conventional commit message for this staged diff:"; git diff --staged; } |
    prompter refine -s concise
}

git-prompt-review() {
  { printf '%s\n\n' "Review this diff for bugs, edge cases, and performance issues:"; git diff main...HEAD; } |
    prompter refine
}
```

### Pipelines

```bash
defuddle parse -m "https://example.com/spec" | prompter apply system-architect > architecture.md
{ printf '%s\n\n' "Find correctness and security risks in this diff:"; git diff main...HEAD; } |
  prompter critique
```

## Verification checklist

- Generated content is on `stdout`; diagnostics remain on `stderr`.
- Pipeline callers reject output whenever Prompter exits nonzero.

## Related docs

- `flags.md`
- `use-json-output.md`
