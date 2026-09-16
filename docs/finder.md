# Prompt browser

Browse a local prompt vault, choose one entry interactively, and receive its body on standard output.

## Triggering the Finder

**Prerequisite:** run this command in an interactive terminal. `browse` requires interactive standard input and standard error; it rejects piped input. An empty pipe without a command instead defaults to `refine` and then fails because its input is empty.

```bash
prompter browse
```

The browser scans the configured prompt directories and lets you filter entries. Press `Enter` to select an entry. `Esc` or `Ctrl+C` cancels without a selection.

## Output

Selecting an entry attempts clipboard integration on macOS, Linux, and Windows, then prints its body to stdout. If the clipboard write fails, Prompter reports that diagnostic and still prints the selected body to stdout.

If the primary prompt directory is empty, the browser creates it and seeds starter prompts before scanning. To avoid the interactive interface, use `prompter prompts status` or `prompter prompts upgrade --dry-run` instead.

## How Search Works

The browser uses weighted fuzzy search to rank entries across frontmatter and body text with the following weights:

| Field | Weight |
| --- | ---: |
| Name | 1000 |
| Aliases, triggers, and examples | 500 |
| Path | 300 |
| Description | 100 |
| Body | 10 |

Higher-weight matches appear first. Frontmatter supplies the searchable name, description, aliases, triggers, and examples; see [Prompt files](prompt-files.md) for the file format.

## Keyboard controls

| Key | Action |
| --- | --- |
| `Enter` | Select the highlighted prompt. |
| `Esc` or `Ctrl+C` | Cancel. |
| Type | Filter entries. |
| Arrow keys, `Ctrl+P`/`Ctrl+K`, `Ctrl+N`/`Ctrl+J` | Move the selection. |
| `PgUp` or `PgDn` | Move five entries. |

## Configuration

The primary default directory is `~/.config/prompter/prompts.d`. Unless `prompts_dirs` is configured, Prompter also searches `~/.config/roles/prompts`.

## Related docs

- [Prompt files](prompt-files.md)
- [Automation](use-json-output.md)
- [Troubleshooting](troubleshooting.md)