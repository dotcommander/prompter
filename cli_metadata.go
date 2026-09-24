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
	fmt.Fprint(w, `Usage: prompter [flags] [input]

Turns rough prompt material into production-grade AI prompts.

Operations:
  (default) | refine [input]  Improve rough prompt input with the active LLM provider
  --image [subject]           Build an image-generation prompt offline from local components
  --config                    Configure prompter, or print resolved settings when output is redirected

Global flags:
  -h, --help                 Show this help
  -V, --version              Show version and build information

Run "prompter refine --help", "prompter --image --help", or "prompter --config --help" for operation-specific flags.
`)
}

func printCommandUsageTo(w io.Writer, cmd string) {
	switch cmd {
	case commandRefine:
		fmt.Fprintf(w, "Usage: prompter refine [flags] [input]\n\nImproves rough prompt input. The bare form \"prompter [input]\" and piped input run the same operation.\n\n  -s, --style <name>  Style: %s\n%s", strings.Join(availableStyles(), ", "), llmFlagHelp())
	case commandImage:
		fmt.Fprint(w, `Usage: prompter --image [flags] [subject]

Builds an image-generation prompt from local components. This operation is offline and does not generate an image.

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
	case commandConfig:
		fmt.Fprint(w, "Usage: prompter --config\n\nOpens the configuration wizard on a terminal, or prints resolved non-secret settings when output is redirected.\n")
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
		fmt.Fprintln(w, authKeyLine(cfg.Provider, p))
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
	if cfg.ComponentsFile != "" {
		fmt.Fprintf(w, "Components File:   %s\n", cfg.ComponentsFile)
	}
}

// authKeyLine describes the active provider's credential source for the
// resolved-settings display without exposing the secret itself.
func authKeyLine(providerName string, p config.ProviderConfig) string {
	keyVar := p.KeyEnv
	if keyVar == "" {
		keyVar = defaultKeyEnvFor(providerName)
	}
	switch providerName {
	case "omlx":
		return fmt.Sprintf("Auth Key:          $%s (local server / keyless)", keyVar)
	case "gemini":
		if os.Getenv("GEMINI_API_KEY") != "" {
			return "Auth Key:          $GEMINI_API_KEY (detected ✓)"
		}
		return "Auth Key:          Google ADC (not checked)"
	}
	if os.Getenv(keyVar) != "" {
		return fmt.Sprintf("Auth Key:          $%s (detected ✓)", keyVar)
	}
	return fmt.Sprintf("Auth Key:          $%s (not set ✗)", keyVar)
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
const AppVersion = "0.5.0"

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
