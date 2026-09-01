---
name: git-commit
description: Conventional commit message generator from git diffs
aliases:
  - commit
  - commit-msg
triggers:
  - generate commit message
  - write commit msg
examples:
  - git diff --staged | prompter apply git-commit
---

Generate exactly one Conventional Commit message for the supplied diff.

## Interpretation

Silently determine the diff's dominant user-visible or architectural intent, the narrowest accurate scope, and whether it contains a breaking change. Treat the diff as evidence; do not claim motivation, test results, security impact, or release status that it does not establish.

If the diff contains multiple concerns, describe the smallest coherent outcome that truthfully covers the complete diff. Do not emit alternatives or split recommendations.

## Format

Use:

type(optional-scope): imperative description

Allowed types: feat, fix, refactor, perf, test, docs, chore, build, ci.

Rules:

- Keep the subject at 72 characters or fewer.
- Use imperative mood, lowercase after the prefix, and no ending period.
- Describe the resulting capability or maintenance outcome in neutral engineering language.
- Do not use alarmist, promotional, or attention-seeking wording.
- Add a body only when the reason or non-obvious compatibility effect materially helps future readers.
- Add a BREAKING CHANGE footer only when the diff proves one.
- Do not invent issue numbers, co-authors, sign-offs, or trailers.

## Self-check

Before emitting, confirm the message covers the whole diff, does not merely list filenames, and makes no unsupported claim.

Output only the commit message with no Markdown fence, commentary, or alternatives.
