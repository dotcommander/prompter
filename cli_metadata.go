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
	fmt.Fprint(w, `Usage: prompter <command> [flags]

Transforms prompts, applies prompt templates, and builds offline image prompts.

Commands:
  refine [input]              Improve a rough prompt
  critique [input]            Analyze a prompt without rewriting it
  rewrite [input]             Restructure rough Markdown or documentation
  apply <prompt-name> [input] Apply a catalog prompt
  browse                      Open the interactive prompt browser
  image <subject>             Build an image-generation prompt offline
  configure                   Configure prompter or print resolved settings
  models refresh              Refresh model choices from Models.dev, OpenRouter, and local OMLX
  prompts status|upgrade      Inspect or safely upgrade starter prompts

Global flags:
  -h, --help                 Show this help
  -V, --version              Show version and build information

Run "prompter <command> --help" for command-specific flags.
`)
}

func printCommandUsageTo(w io.Writer, cmd string) {
	switch cmd {
	case commandRefine:
		fmt.Fprintf(w, "Usage: prompter refine [flags] [input]\n\nImproves rough prompt input.\n\n  -s, --style <name>  Style: %s\n%s", strings.Join(availableStyles(), ", "), llmFlagHelp())
	case commandApply:
		fmt.Fprintf(w, "Usage: prompter apply [flags] <prompt-name> [input]\n\nApplies an exact catalog prompt name or alias to input from args, --file, or stdin.\n%s", llmFlagHelp())
	case commandCritique:
		fmt.Fprintf(w, "Usage: prompter critique [flags] [input]\n\nAnalyzes flaws and missing constraints without rewriting.\n%s", llmFlagHelp())
	case commandRewrite:
		fmt.Fprintf(w, "Usage: prompter rewrite [flags] [input]\n\nRestructures rough Markdown or documentation.\n\n  --mode <name>  Mode: %s\n%s", strings.Join(availableRewriteModes(), ", "), llmFlagHelp())
	case commandImage:
		fmt.Fprint(w, `Usage: prompter image [flags] <subject>

Builds an image-generation prompt from local components. This command is offline and does not generate an image.

Flags:
      --profile <name>    Profile: default, minimal, maximal
      --count <n>         Number of variations (default 1)
      --categories <csv>  Modifier categories
      --no-artist         Omit artist references
      --no-platform       Omit platform references
      --json              Emit JSON
      --seed <value>      Deterministic seed
  -f, --file <path>       Read the subject from a file
  -o, --output <path>     Write output to a file and stdout
  -c, --copy              Copy output to the clipboard
`)
	case commandBrowse:
		fmt.Fprint(w, "Usage: prompter browse\n\nOpens the interactive local prompt browser.\n")
	case commandConfigure:
		fmt.Fprint(w, "Usage: prompter configure\n\nOpens the configuration wizard on a terminal, or prints resolved non-secret settings when output is redirected.\n")
	case commandModels:
		fmt.Fprint(w, "Usage: prompter models refresh\n\nRefreshes cached model choices from Models.dev, OpenRouter, and the local OMLX server.\n")
	case commandPrompts:
		fmt.Fprint(w, "Usage: prompter prompts status\n       prompter prompts upgrade [--dry-run]\n\nInspects starter prompts, installs missing files, and stages versioned replacements without overwriting existing files.\n")
	default:
		printUsageTo(w)
	}
}

func llmFlagHelp() string {
	return fmt.Sprintf(`
Flags:
  -p, --provider <name>  Provider name
  -m, --model <name>     Model override
  -f, --file <path>      Read input from a file
  -o, --output <path>    Write output to a file and stdout
  -c, --copy             Copy buffered output to the clipboard
      --stream           Stream tokens to stdout
      --dry-run          Show resolved settings without an API call
      --base-url <url>   Override the provider endpoint
  -v, --verbose          Show timing on stderr

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
		if cfg.Provider == "omlx" {
			fmt.Fprintf(w, "Auth Key:          $%s (local server / keyless)\n", keyVar)
		} else if cfg.Provider == "gemini" {
			if os.Getenv("GEMINI_API_KEY") != "" {
				fmt.Fprintf(w, "Auth Key:          $GEMINI_API_KEY (detected ✓)\n")
			} else {
				fmt.Fprintf(w, "Auth Key:          Google ADC (not checked)\n")
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
			status = "ADC/unchecked"
		case name == "omlx":
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
	if err != nil {
		return "[invalid URL redacted]"
	}
	if parsed.User != nil {
		parsed.User = url.User("redacted")
	}
	query := parsed.Query()
	for key := range query {
		if sensitiveURLParameter(key) {
			query.Set(key, "redacted")
		}
	}
	parsed.RawQuery = query.Encode()
	if parsed.Fragment != "" {
		parsed.Fragment = "redacted"
	}
	return parsed.String()
}

func sensitiveURLParameter(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(key))
	switch normalized {
	case "apikey", "accesskey", "auth", "authorization", "bearer", "key", "password", "secret", "token", "accesstoken":
		return true
	default:
		return false
	}
}

// AppVersion is the baseline semver for prompter releases.
const AppVersion = "0.2.3"

func getVersionString() string {
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version == "v"+AppVersion {
		return fmt.Sprintf("prompter %s", info.Main.Version)
	}

	version := "v" + AppVersion
	var commit, vcsTime string
	if ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				commit = s.Value
			case "vcs.time":
				vcsTime = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					version += "+dirty"
				}
			}
		}
	}
	if len(commit) > 7 {
		commit = commit[:7]
	}
	if commit != "" && vcsTime != "" {
		return fmt.Sprintf("prompter %s (commit %s, built %s)", version, commit, vcsTime)
	}
	if commit != "" {
		return fmt.Sprintf("prompter %s (commit %s)", version, commit)
	}
	return fmt.Sprintf("prompter %s", version)
}

func printVersion(w io.Writer) {
	fmt.Fprintln(w, getVersionString())
}
