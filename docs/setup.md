# Setup

Purpose: install prompter, configure one provider, and verify a working run.

## Prerequisites

- Go installed and `$(go env GOPATH)/bin` in your `PATH`
- API key for one remote provider, or a running local Wormhole proxy

## Main workflow

1. Install:
 
**Using Homebrew (macOS & Linux):**
```bash
brew install dotcommander/tap/prompter
```

**Or using the Go toolchain:**
```bash
go install github.com/dotcommander/prompter@latest
```

**Or using the one-line installer:**
```bash
curl -fsSL https://raw.githubusercontent.com/dotcommander/prompter/main/install.sh | sh
```

After installation, update to future releases with:

```bash
prompter update
```

2. Run immediately (Gemini with Google ADC is ready out-of-the-box):

```bash
prompter "explain this code"
```

3. Or use any remote provider using standard environment variables:

```bash
export OPENAI_API_KEY="sk-..."
prompter -p openai "explain this code"
```

4. Interactive configuration wizard (optional):

```bash
prompter config
```

`prompter config` launches an interactive setup form in your terminal to configure:
- Default LLM provider (Gemini, OpenAI, Groq, Cerebras, OpenRouter, Synthetic, Zai, Wormhole, OMLX)
- Default model identifier
- API key environment variable constant names (e.g. `$OPENAI_API_KEY`)
- Reasoning effort level (`low`, `medium`, `high`)
- Default automatic clipboard copying

Settings are saved to `~/.config/prompter/config.json` using portable machine paths.

When upgrading `prompter` in the future with `prompter update`, re-running `prompter config` safely refreshes and updates your configuration file with any new provider settings while preserving your existing preferences.
 
5. Seed your prompt vault (optional):

```bash
prompter init
```

Populates `~/.config/prompter/prompts.d/` with curated starter prompts (`enhance`, `critique`, `rewrite`, `refactor`, `code-review`, `system-architect`, `git-commit`, `unit-test`). Run `prompter` to search and browse your vault interactively.


## Verification checklist

- Command prints enhanced text
- `prompter -v "test"` includes timing output

## Related docs

- `common-tasks.md`
- `flags.md`
- `troubleshooting.md`
- `use-json-output.md`
