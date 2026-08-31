# Fuzzy Finder

Purpose: browse and select prompt files interactively.

## Triggering the Finder

The finder is launched using `prompter browse`:

```bash
# Launch interactive fuzzy finder
prompter browse
```

Running bare `prompter` on an interactive terminal displays help. An empty pipe
or other piped input without a command exits with a `command required` error
instead of opening the finder.

## How Search Works

The finder uses weighted fuzzy search to rank results.

| Field | Weight | Example |
|-------|--------|---------|
| Name | 1000 | `ultrathink` matches filename/name |
| Aliases | 500 | `ut` matches an alias, role trigger, or role example |
| Path | 300 | `think` matches file path |
| Description | 100 | matches frontmatter description |
| Body | 10 | matches prompt content |

## Keyboard Controls

| Key | Action |
|-----|--------|
| `Enter` | Select prompt and copy to clipboard |
| `Esc` / `Ctrl+C` | Cancel and exit |
| Type | Filter prompts by search term |
| ↑/↓ | Navigate results |

## Output

When you select a prompt:

1. Clipboard: content is copied to clipboard (macOS, Linux, Windows).
2. Stdout: full content is also printed to stdout.

```bash
# Save selected prompt to a file
prompter browse > prompt.txt
```

## Configuration

Finder scans `prompts_dir` from config:

```json
{
  "prompts_dir": "~/.config/prompter/prompts.d",
  "prompts_dirs": [
    "~/.config/prompter/prompts.d",
    "~/.config/roles/prompts"
  ]
}
```

If `prompts_dirs` is unset, the finder also scans `~/.config/roles/prompts` when that directory exists.

## Related docs

- `prompt-files.md`
- `common-tasks.md`
- `use-json-output.md`
