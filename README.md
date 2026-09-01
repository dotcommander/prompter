# Prompter

Turn rough notes into structured prompts from the command line:

```bash
echo "write a bash backup script" | prompter refine
```

Prompter refines and critiques rough prompts, restructures Markdown, applies
reusable catalog prompts, and builds image prompts offline. Generated text goes
to `stdout`; errors, progress, and timing go to `stderr`.

## Install

```bash
# Homebrew
brew install dotcommander/tap/prompter

# Go
go install github.com/dotcommander/prompter@latest
```

Gemini works with Google Application Default Credentials. Other remote
providers use their standard API-key environment variables, such as
`OPENAI_API_KEY` or `GROQ_API_KEY`; see the [providers guide](docs/providers.md)
for per-provider details, including how to use `GEMINI_API_KEY` with the AI
Studio endpoint.

## Commands

| Command | Purpose |
|---|---|
| `prompter refine [input]` | Improve rough prompt text. |
| `prompter critique [input]` | Analyze ambiguity and missing constraints without rewriting. |
| `prompter rewrite [input]` | Restructure Markdown or documentation. |
| `prompter apply <name> [input]` | Apply an exact catalog prompt name or alias. |
| `prompter browse` | Search the local prompt vault interactively. |
| `prompter image <subject>` | Build an image-generation prompt offline. |
| `prompter configure` | Configure Prompter, or print resolved settings non-interactively. |
| `prompter models refresh` | Cache a short list of affordable model choices per provider from Models.dev, OpenRouter, and the local OMLX server. |

Bare `prompter` displays help on an interactive terminal. Piped input defaults
to `prompter refine` when no command is given.

```bash
prompter refine -s code "design a retry loop"
prompter critique "summarize this better"
prompter rewrite --file notes.md --mode clean
cat spec.md | prompter apply system-architect > architecture.md
prompter image "desert observatory" --count 3
prompter browse
prompter configure
prompter models refresh
```

Use `prompter <command> --help` for command flags. Common LLM overrides include
`-p/--provider`, `-m/--model`, `-f/--file`, `-o/--output`, `-c/--copy`,
`--stream`, `--dry-run`, `--base-url`, and `-v/--verbose`.

## Behavior

- `refine`, `critique`, `rewrite`, and `apply` call the configured LLM provider.
- `image` is deterministic and offline; it builds prompt text but does not
  generate an image.
- `browse` copies the selected prompt to the clipboard and writes it to
  `stdout`. On first run it seeds an empty vault with starter prompts, any of
  which can be used via `apply <name-or-alias>`.
- `apply` strips YAML frontmatter. Prompts may declare output validation;
  validated calls buffer output and reject invalid responses.
- Streaming can emit partial text before a provider reports truncation. Always
  discard streamed output when Prompter exits nonzero.

Configuration precedence is:

```text
CLI flags > environment variables > ~/.config/prompter/config.json > defaults
```

Set `PROMPTER_DEFAULT_COPY=true` or `"default_copy": true` to copy buffered
results automatically.

## Documentation

- [Setup](docs/setup.md)
- [Flags](docs/flags.md)
- [Common tasks](docs/common-tasks.md)
- [Prompt files](docs/prompt-files.md)
- [Providers](docs/providers.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Contributor architecture](AGENTS.md)
