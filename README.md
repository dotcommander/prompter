# Prompter (For Dummies & Curious Minds)

> *"The beauty of a clear prompt is like the beauty of a fundamental law of physics: once you see it in action, everything else becomes obvious."*  
> — Inspired by Walter Lewin's MIT Physics Lectures

---

## What is Prompter? (The 10-Second Mental Model)

Think of raw human ideas as **scattered, uncollimated light**. When you type `"fix this code"` or `"write a script"` into an LLM, the model has to guess your intent, context, boundaries, and tone.

**`prompter` is the optical collimator for your thoughts.** It takes your rough, messy, fragmented notes and focuses them into a razor-sharp, production-grade AI prompt in a fraction of a second.

```
       [ Rough Idea / Messy Notes ]
                   │
                   ▼
         ┌───────────────────┐
         │     prompter      │  <-- Enhances, critiques, or restructures
         └───────────────────┘
                   │
                   ▼
  [ Production-Ready, Crystal-Clear Prompt ]  --> (Piped directly to stdout / LLM)
```

---

## 🚀 Quick Start: From Zero to Magic in 60 Seconds

### Step 1: Install `prompter`

**Using Homebrew (macOS & Linux):**
```bash
brew install dotcommander/tap/prompter
```

**Or using Go:**
```bash
go install github.com/dotcommander/prompter@latest
```

**Or using the one-line installer:**
```bash
curl -fsSL https://raw.githubusercontent.com/dotcommander/prompter/main/install.sh | sh
```

*(Or build locally: `GOWORK=off go build -o prompter .`)*

### Step 2: Run Your First Prompt!
`prompter` works out of the box with zero setup required using Google ADC (Application Default Credentials) for Gemini, or standard environment variables (`OPENAI_API_KEY`, `GROQ_API_KEY`, `GEMINI_API_KEY`, etc.):

```bash
echo "write a bash script to back up my files" | prompter
```
**Boom!** You will see your rough sentence expanded into a structured, constraint-rich prompt complete with edge cases, error handling requirements, and clear output formatting.

### Step 3: Interactive Configuration Wizard (Optional)
Customize your default provider, model, API key variable names, and clipboard settings with the built-in configuration wizard:

```bash
prompter config
```
*(You can re-run `prompter config` at any time after updating `prompter` to adjust your settings or refresh your configuration file.)*

### Step 4: Seed Your Starter Prompt Vault (Optional)
Populate your local vault (`~/.config/prompter/prompts.d`) with curated starter prompts (`refactor`, `code-review`, `system-architect`, `git-commit`, `unit-test`):

```bash
prompter init
```
Once seeded, run `prompter` to browse your new vault interactively, or run them directly (e.g. `prompter run refactor <code-file>`)!

---

## 🧪 The 5 Essential Experiments (Core Commands)

### 1. The Default Enhancer: Turn Rough Ideas into Production Prompts
Pass text directly as an argument or pipe it through `stdin`:
```bash
prompter "explain quantum computing to a 10 year old"
# OR via Unix pipe:
cat rough_spec.txt | prompter
```

### 2. The Interactive Finder: Browse Your Prompt Vault (`prompter`)
Run `prompter` completely empty (no arguments, no piped input):
```bash
prompter
```
*What happens?* A full-screen interactive fuzzy finder appears. Search across all your local `.md` prompt templates, press **Enter**, and the prompt is instantly copied to your clipboard and printed to `stdout`!

Run a discovered prompt by exact name or alias while preserving Unix pipelines:

```bash
defuddle parse -m "$url" | prompter run grai-transform > readit.md
```

`run` strips YAML frontmatter and sends the prompt body as the system prompt. Duplicate exact names or aliases fail with the conflicting paths instead of choosing silently.
Prompts may declare deterministic and optional semantic output validation in frontmatter. Validated prompts buffer output, retry generation once when configured, and fail without printing an invalid response if the corrected output still violates the declared contract.

### 3. The Offline Image Prompt Assembler: Deterministic Art Direction
No internet? No API credits? No problem!
```bash
prompter assemble "cyberpunk street vendor"
```
Combines modular camera angles, lighting conditions, artistic styles, and composition rules entirely offline.
To inspect your local component library:
```bash
prompter stats
```

### 4. The Critique: Diagnose Flaws Without Rewriting
Want to know why your current prompt is failing before you change it?
```bash
prompter critique "write a marketing email for my SaaS"
```
Analyzes missing constraints, ambiguous instructions, and failure modes.

### 5. The Markdown Rewriter: Clean Up Brain Dumps
Have messy bullet points or unstructured meeting notes?
```bash
prompter rewrite --file rough_meeting_notes.md --mode clean
```
Standardizes, formats, and cleans up document hierarchy.

---

## ⚡ Advanced Demonstration: The Unix Way

`prompter` follows strict Unix composability rules:
- **Clean output** goes strictly to `stdout` (ready for piping).
- **Spinners, timings, and progress** go strictly to `stderr`.

### Switch LLM Providers on the Fly
```bash
# Use OpenAI
prompter -p openai "refactor this SQL query"

# Use Groq or Cerebras for ultra-low latency
prompter -p groq "draft a commit message"

# Use a specific model
prompter -p openai -m gpt-5.1 "design an auth system"
```

### Stream Live Responses to Your Screen
```bash
prompter --stream "write a comprehensive guide to goroutines"
```

### Change Enhancement Tone with Styles
```bash
prompter -s concise "summarize this architecture"
prompter -s code "optimize this recursive function"
prompter -s creative "name my new open-source tool"
```

### Inspect the Physics: Dry Run
See the exact system prompt, resolved provider, and parameters without making a remote network call:
```bash
prompter --dry-run "test prompt"
```

---

## 🔌 Daily Developer Workflows & Integrations

### Neovim / Vim In-Place Enhancement
Highlight rough comments or instructions in visual mode and filter them directly through `prompter`:
```vim
:'<,'>!prompter -s code
```

### Git Conventional Commit Generator
Generate clean conventional commit messages straight from your staged changes:
```bash
git diff --staged | prompter -s concise "generate a clear conventional commit message for these staged changes"
```

### Agentic Unix Pipelines
Chain web scrapers, repository extractors, and LLMs:
```bash
# Parse web documentation and transform into structured Markdown
defuddle parse -m "https://example.com/spec" | prompter run grai-transform > spec.md
```

---

## 🎛️ Command & Flag Cheat Sheet

| Command / Flag | Example | What It Does |
|---|---|---|
| *(no args)* | `prompter` | Opens Bubble Tea interactive prompt browser. |
| `find` | `prompter find` | Opens the prompt browser explicitly. |
| `run` | `defuddle parse -m URL \| prompter run grai-transform` | Runs a catalog prompt by exact name or alias. |
| `enhance` | `prompter enhance "context"` | Enhances rough prompt context. |
| `critique` | `prompter critique "prompt"` | Pinpoints flaws and ambiguities in a prompt. |
| `rewrite` | `prompter rewrite --file doc.md` | Cleans and formats messy Markdown. |
| `assemble` | `prompter assemble "subject"` | Assembles image prompts offline. |
| `stats` | `prompter stats` | Shows local image component counts. |
| `config` | `prompter config` | Launches interactive configuration wizard (or shows resolved settings when non-interactive). |
| `styles` | `prompter styles` | Lists enhancement styles and rewrite modes. |
| `providers` | `prompter providers` | Shows provider models and credential status. |
| `init` | `prompter init` | Seeds local vault with starter prompts (`refactor`, `code-review`, etc.). |
| `update` | `prompter update` | Installs the latest released version using the Go toolchain. |
| `version` | `prompter version` / `prompter -V` | Displays version and build information. |
| `-p, --provider` | `-p gemini` / `-p openai` | Selects LLM backend. |
| `-m, --model` | `-m gpt-5.1` | Overrides default model. |
| `-s, --style` | `-s concise` / `-s code` | Sets prompt enhancement tone/style. |
| `--stream` | `prompter --stream "..."` | Streams tokens in real time. |
| `-v, --verbose` | `prompter -v "..."` | Shows timing and spinner on `stderr`. |
| `-V, --version` | `prompter -V` | Displays version information. |
| `--dry-run` | `prompter --dry-run "..."` | Prints resolved configuration & prompt to `stderr`. |
| `--copy` | `prompter --copy "..."` | Copies result directly to system clipboard. |

Set `PROMPTER_DEFAULT_COPY=true` (or `"default_copy": true` in `~/.config/prompter/config.json`) to copy every
non-streamed generated or assembled result automatically.

---

## 📚 Deep Dive & Customization

Want to configure custom providers, loopback proxies (OMLX/Ollama), or create your own prompt libraries?
Check out **[`docs/README.md`](docs/README.md)** and **[`AGENTS.md`](AGENTS.md)** for architecture deep dives!
