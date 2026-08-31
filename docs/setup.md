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

Update to future releases with the same installation method, for example:

```bash
go install github.com/dotcommander/prompter@latest
```

2. Run immediately (Gemini with Google ADC is ready out-of-the-box):

```bash
prompter refine "explain this code"
```

3. Or use any remote provider using standard environment variables:

```bash
export OPENAI_API_KEY="sk-..."
prompter refine -p openai "explain this code"
```

4. Interactive configuration wizard (optional):

```bash
prompter configure
```

`prompter configure` launches an interactive setup form in your terminal to configure:
- Default LLM provider (Gemini, OpenAI, Groq, Cerebras, OpenRouter, Synthetic, Zai, Wormhole, OMLX)
- Default model identifier
- API key environment variable constant names (e.g. `$OPENAI_API_KEY`)
- Reasoning effort level (`low`, `medium`, `high`)
- Default automatic clipboard copying

Settings are saved to `~/.config/prompter/config.json` using portable machine paths.

Re-running `prompter configure` safely refreshes the configuration file while preserving existing preferences.
 
5. Browse your prompt vault (optional):

```bash
prompter browse
```

The browser searches configured prompt directories recursively. If the primary vault is empty, it seeds the curated starter prompts before opening.


## Verification checklist

- Command prints enhanced text
- `prompter refine -v "test"` includes timing output

## Related docs

- `common-tasks.md`
- `flags.md`
- `troubleshooting.md`
- `use-json-output.md`
