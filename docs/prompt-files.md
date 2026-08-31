# Prompt Files

Purpose: define prompt file structure and prompt directory behavior.

## Prerequisites

- `prompts_dir` set in `~/.config/prompter/config.json`

## Main workflow or contract

Prompter scans `prompts_dir` and `prompts_dirs` recursively and indexes `.md` files for finder search.
The same index powers native execution by exact name or alias:

```bash
cat source.md | prompter apply grai-transform
```

The selected prompt body becomes the system prompt. Frontmatter is not sent to the provider. Ambiguous exact matches fail and list their paths.

Optional output validation applies only to `prompter apply` and requires buffered output:

```yaml
validation:
  control_fence: deep-time
  min_word_ratio: 0.8
  max_word_ratio: 2.0
  longer_min_word_ratio: 1.5
  longer_max_word_ratio: 3.0
  short_input_words: 25
  max_short_sentences: 2
  require_terminal_punctuation: true
  semantic_validation: true
  retries: 1
```

The validator strips the named leading control fence before counting source words. It validates word bounds, short-input sentence count, and terminal punctuation. When `semantic_validation` is true, the same provider and model also judge the candidate for changed or invented facts, literals, uncertainty, point of view, and sensory details. A configured retry adds the violations to the system prompt and generates once more. The corrected output is validated again before emission. Semantic validation adds one judge call per candidate, so a successful first draft uses two provider calls and a corrected draft uses four. If the corrected output still fails, `prompter` returns an error and emits no response. `--stream` is rejected for validated prompts because streamed output cannot be recalled.

Required:

- File extension: `.md`
- UTF-8 encoding
- Located under configured `prompts_dir`

Optional frontmatter fields:

- `name`: display/search name; useful when importing role prompt files whose filename differs from role name
- `description`: searchable description
- `aliases`: searchable alternate names
- `triggers`: searchable role-routing phrases
- `examples`: searchable example invocations

Example:

```yaml
---
description: Transform rough prompts into production-ready prompts
aliases:
  - improve
  - polish
triggers:
  - rough prompt
examples:
  - prompter refine "make this better"
---

# Prompt body
```

Multiple directories:

```json
{
  "prompts_dirs": [
    "~/.config/prompter/prompts.d",
    "~/.config/roles/prompts"
  ]
}
```

Existing configs that only set `prompts_dir` also include `~/.config/roles/prompts` automatically when that directory exists. This lets the finder browse existing Role prompt files after `role` is no longer the execution surface.

Scanner limits & features:

- Max depth: 5 levels
- Max files: 1000 markdown files
- Symlink traversal: resolves and traverses symlinked directories and prompt files (e.g. Obsidian vaults or dotfiles) with cycle detection
- Empty vault guidance: displays clear setup instructions if no prompt files are found

Invalid frontmatter does not stop scanning. The file still indexes, and a warning is printed to stderr.

## Component Library

`prompter image` reads reusable image-prompt components from `components_file` in `~/.config/prompter/config.json`.

```json
{
  "components_file": "~/.config/prompter/components.json"
}
```

If the file is missing, Prompter uses embedded default components. A custom component file has this shape:

```json
{
  "subjects": [{"subject": "portrait", "category": "portrait"}],
  "modifiers": [{"text": "highly detailed", "category": "quality", "slot_position": 2, "weight": 0.9}],
  "artists": [{"name": "Annie Leibovitz", "categories": ["portrait"], "weight": 0.7}],
  "platforms": [{"name": "editorial", "phrase": "magazine cover quality", "weight": 0.5}]
}
```

## Verification checklist

- `prompts_dir` exists
- Directory contains `.md` files
- `prompter browse` shows finder entries
- `prompter image "test subject"` prints an assembled image-generation prompt
- Frontmatter parse failures only warn; they do not abort scan

## Related docs

- `finder.md`
- `setup.md`
