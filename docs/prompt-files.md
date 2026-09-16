# Prompt files

Create reusable Markdown prompts for `apply` and `browse`, then let Prompter resolve them by exact name or alias.

## First prompt

**Prerequisite:** place a Markdown file in the primary prompt directory, which defaults to `~/.config/prompter/prompts.d`. You do not need to create a configuration file first.

```markdown
---
description: Turn rough notes into a concise task brief
aliases:
  - brief
---

Write a concise task brief from the supplied notes.
```

Save this as `~/.config/prompter/prompts.d/task-brief.md`, then run the following source-checked, unexecuted command after configuring a provider:

```bash
printf '%s\n' 'Fix the parsing error and add a focused test.' | prompter apply task-brief
```

`apply` finds the prompt by its filename-derived name or alias, removes frontmatter, and uses the remaining body as the system prompt. Arguments, `--file`, and standard input provide the user input.

## Frontmatter

Frontmatter is optional. Supported fields are `name`, `description`, `aliases`, `triggers`, `examples`, and `validation`. The browser uses the first five fields to search and rank entries. Invalid frontmatter produces a warning and does not stop the file from being indexed.

Prompter scans Markdown files recursively, with a maximum depth of five directories and a maximum of 1,000 files. It resolves symlinked prompt directories and prevents cycles; scan, stat, and read failures are reported rather than silently omitting entries.

## Validation

A prompt may declare a `validation` mapping in its frontmatter. Prompter validates buffered `apply` output and rejects `--stream` for validated prompts. Validation supports word-ratio bounds, short-input sentence bounds, terminal punctuation, a leading control fence, semantic validation, and either zero or one corrective retry.

Semantic validation adds a provider judge call for each candidate, so use it only when that extra remote request is acceptable. If validation still fails after its permitted retry, Prompter returns an error instead of emitting the buffered response.

## Starter-prompt maintenance

`prompter prompts status` compares the primary vault with embedded starter prompts. `prompter prompts upgrade --dry-run` previews changes without writing. Without `--dry-run`, missing prompts are installed; stock and customized files are preserved, with replacement candidates written beside them as `<name>.md.new.<hash>`.

## Image component library

`image` reads `components_file`, which defaults to `~/.config/prompter/components.json`. When that file is absent, Prompter uses embedded default components. The component JSON has `subjects`, `modifiers`, `artists`, and `platforms` arrays; inspect `prompter image --help` for the command options that select them.

## Related docs

- [Prompt browser](finder.md)
- [CLI flags](flags.md)
- [Setup](setup.md)