package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime/debug"
	"strings"

	"github.com/dotcommander/prompter/internal/config"
	"github.com/dotcommander/prompter/internal/provider"
)

func printUsageTo(w io.Writer) {
	fmt.Fprintf(w, `Usage: prompter [flags] [input]
       prompter enhance <context>
       prompter run <name-or-alias> [input]
       prompter critique <prompt>
       prompter rewrite --file notes.md --mode clean
       prompter assemble <subject>
       prompter find
       prompter stats
       prompter config
       prompter styles
       prompter providers
       prompter update
       prompter version
       echo "prompt" | prompter

Enhances prompts and rewrites rough markdown into structured output.

Commands:
  enhance <context>     Run prompt enhancement on context
  run <name-or-alias>  Run a catalog prompt with input from args, --file, or stdin
  critique <prompt>     Analyze what's wrong with a prompt without rewriting
  rewrite <content>     Clean and restructure rough markdown/doc text
  assemble <subject>    Assemble an image prompt from local components
  find                  Launch interactive prompt finder (default when no args on TTY)
  stats                 Show local prompt component statistics
  config                Configure prompter interactively (or view resolved settings)
  styles                List available enhancement styles and rewrite modes
  providers             List supported LLM providers and configuration status
  update                Install the latest released version of prompter
  version               Show prompter version and build information
  init [dir]            Initialize prompt vault with curated starter prompts
  (no args)             Show prompt finder

Common Flags:
  -p, --provider <name> Provider: %s
  -m, --model <name>    Model to use
  -f, --file <path>     Read prompt input from file
  -o, --output <path>   Write generated prompt to file and stdout
  -c, --copy            Copy generated prompt to clipboard
      --stream          Stream tokens as they arrive
      --dry-run         Show resolved runtime settings without an API call
      --base-url <url>  Custom API endpoint
  -v, --verbose         Show timing to stderr
  -V, --version         Show prompter version

Enhancement & Rewrite Flags:
  -s, --style <name>    Enhancement style: %s
      --mode <name>     Rewrite mode: %s

Assemble Flags:
      --profile <name>  Assembly profile: default, minimal, maximal
      --count <n>       Number of assembled variations (default 1)
      --categories <csv> Custom assembly modifier categories
      --no-artist       Omit artist reference from assembled prompts
      --no-platform     Omit platform reference from assembled prompts
      --json            Output assemble/stats data as JSON
      --seed <value>    Deterministic assembly seed
`, provider.KnownNamesString(), strings.Join(availableStyles(), ", "), strings.Join(availableRewriteModes(), ", "))
}

func printCommandUsageTo(w io.Writer, cmd string) {
	switch cmd {
	case commandEnhance:
		fmt.Fprintf(w, "Usage: prompter enhance [flags] <context>\n\nEnhances rough prompt input.\n\n  --style <name>     %s\n%s", strings.Join(availableStyles(), ", "), llmFlagHelp())
	case commandRun:
		fmt.Fprintf(w, "Usage: prompter run [flags] <name-or-alias> [input]\n\nRuns an exact catalog prompt name or alias with input from args, --file, or stdin.\n%s", llmFlagHelp())
	case commandCritique:
		fmt.Fprintf(w, "Usage: prompter critique [flags] <prompt>\n\nAnalyzes flaws and missing constraints.\n%s", llmFlagHelp())
	case commandRewrite:
		fmt.Fprintf(w, "Usage: prompter rewrite [flags] <content>\n\n  --mode <name>      %s\n%s", strings.Join(availableRewriteModes(), ", "), llmFlagHelp())
	case commandAssemble:
		fmt.Fprint(w, "Usage: prompter assemble [flags] <subject>\n\nAssembles an image prompt from local components.\n\nFlags: --profile, --count, --categories, --no-artist, --no-platform, --json, --seed, --file, --output, --copy\n")
	case commandFind, commandSearch, commandBrowse:
		fmt.Fprint(w, "Usage: prompter find\n\nLaunches the interactive prompt finder.\n")
	case commandStats:
		fmt.Fprint(w, "Usage: prompter stats [--json]\n\nShows local component-library statistics.\n")
	case commandConfig:
		fmt.Fprint(w, "Usage: prompter config\n\nLaunches the interactive configuration wizard when run on a terminal,\nor displays resolved non-secret configuration when piped.\n")
	case commandStyles, commandModes:
		fmt.Fprint(w, "Usage: prompter styles\n\nLists enhancement styles and rewrite modes.\n")
	case commandProviders:
		fmt.Fprint(w, "Usage: prompter providers\n\nLists providers and configuration status.\n")
	case commandUpdate:
		fmt.Fprint(w, "Usage: prompter update\n\nInstalls the latest released version using the Go toolchain.\n")
	case commandVersion:
		fmt.Fprint(w, "Usage: prompter version\n\nDisplays prompter version and build information.\n")
	case commandInit:
		fmt.Fprint(w, "Usage: prompter init [flags] [directory]\n\nInitializes the prompt vault with starter prompts (refactor, code-review, system-architect, etc.).\n\nFlags: --force (overwrite existing prompt files)\n")
	default:
		printUsageTo(w)
	}
}

func llmFlagHelp() string {
	return fmt.Sprintf(`
Flags: --provider, --model, --file, --output, --copy, --stream, --dry-run, --base-url, --verbose
Providers: %s
`, provider.KnownNamesString())
}

func printConfig(w io.Writer, cfg *config.Config) {
	fmt.Fprintln(w, "Prompter Configuration")
	fmt.Fprintln(w, "======================")
	fmt.Fprintf(w, "Active Provider:   %s\n", cfg.Provider)
	if p, ok := cfg.Providers[cfg.Provider]; ok {
		if p.Model != "" {
			fmt.Fprintf(w, "Active Model:      %s\n", p.Model)
		}
		keyVar := p.KeyEnv
		if keyVar == "" {
			keyVar = defaultKeyEnvFor(cfg.Provider)
		}
		if cfg.Provider == "wormhole" || cfg.Provider == "omlx" {
			fmt.Fprintf(w, "Auth Key:          $%s (local server / keyless)\n", keyVar)
		} else if cfg.Provider == "gemini" {
			if os.Getenv("GEMINI_API_KEY") != "" {
				fmt.Fprintf(w, "Auth Key:          $GEMINI_API_KEY (detected ✓)\n")
			} else {
				fmt.Fprintf(w, "Auth Key:          Google ADC (ready ✓)\n")
			}
		} else if val := os.Getenv(keyVar); val != "" {
			fmt.Fprintf(w, "Auth Key:          $%s (detected ✓)\n", keyVar)
		} else {
			fmt.Fprintf(w, "Auth Key:          $%s (not set ✗)\n", keyVar)
		}
		if p.BaseURL != "" {
			fmt.Fprintf(w, "Base URL:          %s\n", redactURLUserinfo(p.BaseURL))
		}
	}
	fmt.Fprintf(w, "Effort:            %s\n", cfg.Effort)
	fmt.Fprintf(w, "Timeout:           %ds\n", cfg.Timeout)
	fmt.Fprintf(w, "Max Output Tokens: %d\n", cfg.MaxOutputTokens)
	fmt.Fprintf(w, "Max Retries:       %d\n", cfg.MaxRetries)
	fmt.Fprintf(w, "Default Copy:      %t\n", cfg.DefaultCopy)
	if cfg.PromptFile != "" {
		fmt.Fprintf(w, "Prompt File:       %s\n", cfg.PromptFile)
	}
	if cfg.PromptsDir != "" {
		fmt.Fprintf(w, "Prompts Dir:       %s\n", cfg.PromptsDir)
	}
	if len(cfg.PromptsDirs) > 0 {
		fmt.Fprintf(w, "Prompts Dirs:      %s\n", strings.Join(cfg.PromptsDirs, ", "))
	}
	if cfg.ComponentsFile != "" {
		fmt.Fprintf(w, "Components File:   %s\n", cfg.ComponentsFile)
	}
}

func printStyles(w io.Writer) {
	fmt.Fprintln(w, "Enhancement Styles (-s, --style)")
	descriptions := map[string]string{
		"default":  "standard comprehensive enhancement",
		"code":     "technical programming prompts",
		"concise":  "compact prompts without fluff",
		"creative": "imaginative and exploratory prompts",
		"grai":     "general reasoning AI alignment",
		"spec":     "formal specifications and acceptance criteria",
	}
	for _, style := range availableStyles() {
		description := descriptions[style]
		if description == "" {
			description = "custom user style"
		}
		fmt.Fprintf(w, "  %-10s %s\n", style, description)
	}
	fmt.Fprintln(w, "\nRewrite Modes (--mode)")
	descriptions = map[string]string{
		"clean":     "remove cruft and organize markdown",
		"academic":  "formal scholarly format",
		"blog":      "readable editorial format",
		"code":      "extract code, commands, and workflows",
		"extract":   "extract key facts and action items",
		"synthesis": "combine notes into a cohesive summary",
	}
	for _, mode := range availableRewriteModes() {
		fmt.Fprintf(w, "  %-10s %s\n", mode, descriptions[mode])
	}
}

func printProviders(w io.Writer, cfg *config.Config) {
	fmt.Fprintln(w, "Supported LLM Providers")
	fmt.Fprintf(w, "  %-14s %-25s %-12s %s\n", "PROVIDER", "DEFAULT MODEL", "STATUS", "BASE URL")
	for _, name := range provider.KnownNames() {
		pCfg := cfg.Providers[name]
		status := "not set"
		switch {
		case pCfg.APIKey != "":
			status = "configured"
		case name == "gemini":
			status = "ADC/ready"
		case name == "omlx" || name == "wormhole":
			status = "local/ready"
		}
		displayName := name
		if name == cfg.Provider {
			displayName += " *"
		}
		fmt.Fprintf(w, "  %-14s %-25s %-12s %s\n", displayName, pCfg.Model, status, redactURLUserinfo(pCfg.BaseURL))
	}
	fmt.Fprintln(w, "\n  * = active provider in config")
}

func redactURLUserinfo(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return raw
	}
	parsed.User = url.User("redacted")
	return parsed.String()
}

// AppVersion is the baseline semver for prompter releases.
const AppVersion = "0.1.0"

func getVersionString() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		var commit, vcsTime string
		if ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					commit = s.Value
				}
				if s.Key == "vcs.time" {
					vcsTime = s.Value
				}
			}
		}
		if commit != "" {
			if len(commit) > 7 {
				commit = commit[:7]
			}
			if vcsTime != "" {
				return fmt.Sprintf("prompter v%s (commit %s, built %s)", AppVersion, commit, vcsTime)
			}
			return fmt.Sprintf("prompter v%s (commit %s)", AppVersion, commit)
		}
		return fmt.Sprintf("prompter v%s", AppVersion)
	}
	return fmt.Sprintf("prompter %s", info.Main.Version)
}

func printVersion(w io.Writer) {
	fmt.Fprintln(w, getVersionString())
}
