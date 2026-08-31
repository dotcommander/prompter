# Common Tasks

Purpose: copy/paste task recipes for daily CLI use.

## Prerequisites

- Authenticated Gemini provider (Google ADC), local loopback proxy, or API key in environment variables (e.g. `OPENAI_API_KEY`)

## Main workflow

Use these recipes directly.

```bash
# Enhance from argument
prompter "explain this function"

# Enhance from stdin
echo "write a test plan" | prompter

# Explicit enhance command
prompter enhance "rewrite this commit message"

# Use the migrated Grai rewrite policy
prompter -s grai "extract names"

# Copy result to clipboard
prompter --copy "write a deploy checklist"

# Critique without rewriting
prompter critique "summarize stuff better"

# Assemble an image prompt from local components
prompter assemble "desert observatory"

# Assemble multiple variations
prompter assemble "portrait of a clockmaker" --count 3

# Show local component stats
prompter stats

# Pick provider and model
prompter -p openai -m gpt-4o "summarize this"

# Override API endpoint
prompter --base-url https://api.example.com "my prompt"

# Show timing on stderr
prompter -v "my prompt"

# Configure prompter interactively
prompter config

# Initialize starter prompt vault (~/.config/prompter/prompts.d)
prompter init

# Update to the latest released version
prompter update
```

Run interactive finder when you want to browse saved prompts:

```bash
prompter
```

## Editor & Workflow Integrations

### Neovim / Vim
Transform highlighted rough prompt text or comments in-place directly within your buffer:
```vim
" Visual mode: select text and replace with enhanced prompt
:'<,'>!prompter -s code
```

Add keymaps to your `init.lua`:
```lua
-- Keybindings for prompt enhancement in Neovim
vim.keymap.set("v", "<leader>pe", ":!prompter<CR>", { desc = "Prompter: Enhance prompt" })
vim.keymap.set("v", "<leader>pc", ":!prompter -s code<CR>", { desc = "Prompter: Enhance code prompt" })
vim.keymap.set("v", "<leader>pq", ":!prompter critique<CR>", { desc = "Prompter: Critique prompt" })
```

### Git & Terminal Helpers
Add handy prompt functions to your `~/.zshrc` or `~/.bashrc`:
```bash
# Generate conventional git commit messages from staged diff
git-prompt-commit() {
  git diff --staged | prompter -s concise "generate a clear conventional commit message for these staged changes"
}

# Review current branch diff for bugs and security issues
git-prompt-review() {
  git diff main...HEAD | prompter "review this git diff for bugs, edge cases, and performance issues"
}

# Quick clipboard enhancer alias
pe() {
  prompter "$@" | pbcopy
  echo "Enhanced prompt copied to clipboard!"
}
```

### Raycast / Alfred Desktop Hotkey
Create a Raycast script command (`~/.config/raycast/commands/prompter.sh`) to enhance prompts with a global desktop hotkey:
```bash
#!/bin/bash
# @raycast.schemaVersion 1
# @raycast.title Enhance Prompt
# @raycast.mode fullOutput
# @raycast.packageName AI Tools
# @raycast.argument1 { "type": "text", "placeholder": "Rough prompt text..." }

prompter "$1"
```

### Agentic Pipelines (Chaining with Downstream Tools)
Chain `prompter` with web extractors or repository packagers in Unix pipes:
```bash
# Scrape web page with defuddle and transform into clean Markdown spec
defuddle parse -m "https://example.com/spec" | prompter run grai-transform > summary.md

# Pipe codebase dump to find architectural anti-patterns
cat main.go | prompter -s code "find performance bottlenecks and concurrency bugs"
```

## Verification checklist

- Each command returns output without manual edits
- `prompter assemble "test subject"` works without an API key
- `prompter stats` shows component counts
- Finder opens when command has no input
- Verbose mode includes timing output

## Related docs

- `finder.md`
- `flags.md`
- `troubleshooting.md`
- `use-json-output.md`
