package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the resolved configuration used at runtime.
type Config struct {
	Provider        string
	PromptFile      string
	PromptsDir      string
	PromptsDirs     []string
	ComponentsFile  string
	Effort          string
	Timeout         int // seconds; 0 = use default
	MaxOutputTokens int // max tokens in response; <=0 = use default (4096)
	// MaxOutputTokensExplicit records whether config/env explicitly set
	// max_output_tokens before Load applied the default.
	MaxOutputTokensExplicit bool
	MaxRetries              int // HTTP retry count; <=0 = use default (3)
	DefaultCopy             bool

	// Cached system prompt loaded once at startup.
	SystemPrompt string `json:"-"`

	Providers map[string]ProviderConfig
}

// ProviderConfig holds the per-provider fields from the config file.
type ProviderConfig struct {
	APIKey    string `json:"api_key,omitempty" mapstructure:"api_key"`
	KeyEnv    string `json:"key_env,omitempty" mapstructure:"key_env"`
	Model     string `json:"model,omitempty" mapstructure:"model"`
	BaseURL   string `json:"base_url,omitempty" mapstructure:"base_url"`
	ProjectID string `json:"project_id,omitempty" mapstructure:"project_id"`
	Location  string `json:"location,omitempty" mapstructure:"location"`
}

// ConfigFile mirrors the JSON structure of the config file.
type ConfigFile struct {
	Provider        string   `json:"provider,omitempty" mapstructure:"provider"`
	PromptFile      string   `json:"prompt_file,omitempty" mapstructure:"prompt_file"`
	PromptsDir      string   `json:"prompts_dir,omitempty" mapstructure:"prompts_dir"`
	PromptsDirs     []string `json:"prompts_dirs,omitempty" mapstructure:"prompts_dirs"`
	ComponentsFile  string   `json:"components_file,omitempty" mapstructure:"components_file"`
	Effort          string   `json:"effort,omitempty" mapstructure:"effort"`
	Timeout         int      `json:"timeout,omitempty" mapstructure:"timeout"`
	MaxOutputTokens int      `json:"max_output_tokens,omitempty" mapstructure:"max_output_tokens"`
	MaxRetries      int      `json:"max_retries,omitempty" mapstructure:"max_retries"`
	DefaultCopy     bool     `json:"default_copy,omitempty" mapstructure:"default_copy"`

	OpenAI     ProviderConfig `json:"openai" mapstructure:"openai"`
	Cerebras   ProviderConfig `json:"cerebras" mapstructure:"cerebras"`
	DeepSeek   ProviderConfig `json:"deepseek" mapstructure:"deepseek"`
	Groq       ProviderConfig `json:"groq" mapstructure:"groq"`
	OpenRouter ProviderConfig `json:"openrouter" mapstructure:"openrouter"`
	Zai        ProviderConfig `json:"zai" mapstructure:"zai"`
	Gemini     ProviderConfig `json:"gemini" mapstructure:"gemini"`
	Omlx       ProviderConfig `json:"omlx" mapstructure:"omlx"`
}

const (
	DefaultTimeout         = 60   // seconds
	StreamingTimeout       = 180  // seconds
	DefaultMaxOutputTokens = 4096 // tokens
	DefaultMaxRetries      = 3    // retries
)

func getConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "prompter", "config.json")
}

func expandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, filepath.FromSlash(path[2:]))
		}
	}
	return path
}

func expandPaths(paths []string) []string {
	expanded := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		expanded = append(expanded, expandPath(path))
	}
	return expanded
}

func unexpandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	homePrefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, homePrefix) {
		rel := path[len(homePrefix):]
		return "~/" + filepath.ToSlash(rel)
	}
	return path
}

func unexpandPaths(paths []string) []string {
	unexpanded := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		unexpanded = append(unexpanded, unexpandPath(p))
	}
	return unexpanded
}

func detectDefaultProvider() string {
	if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("PROMPTER_GEMINI_API_KEY") != "" {
		return "gemini"
	}
	if os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("PROMPTER_OPENAI_API_KEY") != "" {
		return "openai"
	}
	if os.Getenv("GROQ_API_KEY") != "" || os.Getenv("PROMPTER_GROQ_API_KEY") != "" {
		return "groq"
	}
	if os.Getenv("CEREBRAS_API_KEY") != "" || os.Getenv("PROMPTER_CEREBRAS_API_KEY") != "" {
		return "cerebras"
	}
	if os.Getenv("DEEPSEEK_API_KEY") != "" || os.Getenv("PROMPTER_DEEPSEEK_API_KEY") != "" {
		return "deepseek"
	}
	if os.Getenv("OPENROUTER_API_KEY") != "" || os.Getenv("PROMPTER_OPENROUTER_API_KEY") != "" {
		return "openrouter"
	}
	if os.Getenv("ZAI_API_KEY") != "" || os.Getenv("PROMPTER_ZAI_API_KEY") != "" {
		return "zai"
	}
	return "gemini"
}

// DefaultProviders returns the built-in default configuration for all known providers.
func DefaultProviders() map[string]ProviderConfig {
	return map[string]ProviderConfig{
		"openai": {
			Model:   "gpt-5.6-luna",
			BaseURL: "",
		},
		"cerebras": {
			Model:   "gpt-oss-120b",
			BaseURL: "https://api.cerebras.ai/v1",
		},
		"deepseek": {
			Model:   "deepseek-v4-pro",
			BaseURL: "https://api.deepseek.com",
		},
		"groq": {
			Model:   "qwen/qwen3.8-27b",
			BaseURL: "https://api.groq.com/openai/v1",
		},
		"openrouter": {
			Model:   "openrouter/free",
			BaseURL: "https://openrouter.ai/api/v1",
		},
		"zai": {
			Model:   "glm-5.3-flash",
			BaseURL: "https://api.z.ai/api/coding/paas/v4",
		},
		"gemini": {
			Model:    "gemini-3.7-flash",
			Location: "global",
			BaseURL:  "https://aiplatform.googleapis.com/v1",
		},
		"omlx": {
			Model:   "Ornith-1.5-35B-A3B-oQ4e-mtp",
			BaseURL: "http://127.0.0.1:8000/v1",
		},
	}
}

func resolveProviderConfig(name string, fileCfg ProviderConfig, defaultCfg ProviderConfig) ProviderConfig {
	upper := strings.ToUpper(name)
	res := fileCfg

	// APIKey resolution:
	// 1. PROMPTER_<PROVIDER>_API_KEY
	// 2. Explicit key_env (e.g. key_env: "MY_KEY" -> os.Getenv("MY_KEY"))
	// 3. Explicit fileCfg.APIKey
	// 4. Standard <PROVIDER>_API_KEY
	// 5. defaultCfg.APIKey
	if env := os.Getenv("PROMPTER_" + upper + "_API_KEY"); env != "" {
		res.APIKey = env
	} else if res.KeyEnv != "" && os.Getenv(res.KeyEnv) != "" {
		res.APIKey = os.Getenv(res.KeyEnv)
	} else if res.APIKey == "" {
		if env := os.Getenv(upper + "_API_KEY"); env != "" {
			res.APIKey = env
		} else {
			res.APIKey = defaultCfg.APIKey
		}
	}

	// Model
	if env := os.Getenv("PROMPTER_" + upper + "_MODEL"); env != "" {
		res.Model = env
	} else if res.Model == "" {
		if env := os.Getenv(upper + "_MODEL"); env != "" {
			res.Model = env
		} else {
			res.Model = defaultCfg.Model
		}
	}

	// BaseURL
	if env := os.Getenv("PROMPTER_" + upper + "_BASE_URL"); env != "" {
		res.BaseURL = env
	} else if res.BaseURL == "" {
		if env := os.Getenv(upper + "_BASE_URL"); env != "" {
			res.BaseURL = env
		} else {
			res.BaseURL = defaultCfg.BaseURL
		}
	}

	// Gemini-specific project_id & location
	if name == "gemini" {
		if env := os.Getenv("PROMPTER_GEMINI_PROJECT_ID"); env != "" {
			res.ProjectID = env
		} else if res.ProjectID == "" {
			if env := os.Getenv("GEMINI_PROJECT_ID"); env != "" {
				res.ProjectID = env
			} else if env := os.Getenv("GOOGLE_CLOUD_PROJECT"); env != "" {
				res.ProjectID = env
			} else if env := os.Getenv("GCP_PROJECT"); env != "" {
				res.ProjectID = env
			} else {
				res.ProjectID = defaultCfg.ProjectID
			}
		}

		if env := os.Getenv("PROMPTER_GEMINI_LOCATION"); env != "" {
			res.Location = env
		} else if res.Location == "" {
			if env := os.Getenv("GEMINI_LOCATION"); env != "" {
				res.Location = env
			} else {
				res.Location = defaultCfg.Location
			}
		}
	}

	return res
}

// Load reads and parses the config file if present, returning a populated Config.
// If the config file does not exist, safe defaults and environment variables are used.
func Load() (*Config, error) {
	path := getConfigPath()

	viper.SetEnvPrefix("PROMPTER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	configFileLoaded := false
	if path != "" {
		viper.SetConfigFile(path)
		if err := viper.ReadInConfig(); err != nil {
			if !os.IsNotExist(err) {
				var notFound viper.ConfigFileNotFoundError
				if !strings.Contains(err.Error(), "no such file") && !errors.As(err, &notFound) {
					return nil, fmt.Errorf("read config: %w", err)
				}
			}
		} else {
			configFileLoaded = true
		}
	}

	var cfgFile ConfigFile
	if configFileLoaded {
		if err := viper.Unmarshal(&cfgFile); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	providerName := cfgFile.Provider
	if envProvider := os.Getenv("PROMPTER_PROVIDER"); envProvider != "" {
		providerName = envProvider
	}
	if configFileLoaded && providerName == "" {
		return nil, fmt.Errorf("config missing: provider")
	}
	if providerName == "" {
		providerName = detectDefaultProvider()
	}

	defaults := DefaultProviders()
	fileProviders := map[string]ProviderConfig{
		"openai":     cfgFile.OpenAI,
		"cerebras":   cfgFile.Cerebras,
		"deepseek":   cfgFile.DeepSeek,
		"groq":       cfgFile.Groq,
		"openrouter": cfgFile.OpenRouter,
		"zai":        cfgFile.Zai,
		"gemini":     cfgFile.Gemini,
		"omlx":       cfgFile.Omlx,
	}

	providers := make(map[string]ProviderConfig, len(defaults))
	for name, defCfg := range defaults {
		var fCfg ProviderConfig
		if configFileLoaded {
			fCfg = fileProviders[name]
		}
		providers[name] = resolveProviderConfig(name, fCfg, defCfg)
	}

	promptFile := cfgFile.PromptFile
	if envPromptFile := os.Getenv("PROMPTER_PROMPT_FILE"); envPromptFile != "" {
		promptFile = envPromptFile
	}

	promptsDir := cfgFile.PromptsDir
	if envPromptsDir := os.Getenv("PROMPTER_PROMPTS_DIR"); envPromptsDir != "" {
		promptsDir = envPromptsDir
	}
	if promptsDir == "" {
		promptsDir = expandPath("~/.config/prompter/prompts.d")
	}

	promptsDirs := cfgFile.PromptsDirs
	if len(promptsDirs) == 0 {
		promptsDirs = []string{
			expandPath("~/.config/prompter/prompts.d"),
			expandPath("~/.config/roles/prompts"),
		}
	} else {
		promptsDirs = expandPaths(promptsDirs)
	}

	componentsFile := cfgFile.ComponentsFile
	if envCompFile := os.Getenv("PROMPTER_COMPONENTS_FILE"); envCompFile != "" {
		componentsFile = envCompFile
	}
	if componentsFile == "" {
		componentsFile = expandPath("~/.config/prompter/components.json")
	}

	effort := cfgFile.Effort
	if envEffort := os.Getenv("PROMPTER_EFFORT"); envEffort != "" {
		effort = envEffort
	}
	if effort == "" {
		effort = "low"
	}
	if !slices.Contains(validEfforts, effort) {
		return nil, fmt.Errorf("invalid effort %q: must be low, medium, or high", effort)
	}

	timeout := cfgFile.Timeout
	if envTimeout := os.Getenv("PROMPTER_TIMEOUT"); envTimeout != "" {
		var t int
		if _, err := fmt.Sscanf(envTimeout, "%d", &t); err == nil {
			timeout = t
		}
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	if timeout < 0 {
		return nil, fmt.Errorf("config: timeout must be >= 0, got %d", timeout)
	}

	maxOutputTokens := cfgFile.MaxOutputTokens
	maxOutputTokensExplicit := viper.IsSet("max_output_tokens")
	if envTokens := os.Getenv("PROMPTER_MAX_OUTPUT_TOKENS"); envTokens != "" {
		var tokens int
		if _, err := fmt.Sscanf(envTokens, "%d", &tokens); err == nil {
			maxOutputTokens = tokens
			maxOutputTokensExplicit = true
		}
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = DefaultMaxOutputTokens
	}

	maxRetries := cfgFile.MaxRetries
	if envRetries := os.Getenv("PROMPTER_MAX_RETRIES"); envRetries != "" {
		var retries int
		if _, err := fmt.Sscanf(envRetries, "%d", &retries); err == nil {
			maxRetries = retries
		}
	}
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}

	defaultCopy := cfgFile.DefaultCopy
	if envCopy := os.Getenv("PROMPTER_DEFAULT_COPY"); envCopy != "" {
		defaultCopy = strings.EqualFold(envCopy, "true") || envCopy == "1"
	}

	cfg := &Config{
		Provider:                providerName,
		PromptFile:              expandPath(promptFile),
		PromptsDir:              expandPath(promptsDir),
		PromptsDirs:             promptsDirs,
		ComponentsFile:          expandPath(componentsFile),
		Effort:                  effort,
		Timeout:                 timeout,
		MaxOutputTokens:         maxOutputTokens,
		MaxOutputTokensExplicit: maxOutputTokensExplicit,
		MaxRetries:              maxRetries,
		DefaultCopy:             defaultCopy,
		Providers:               providers,
	}

	return cfg, nil
}

// Save writes the Config to ~/.config/prompter/config.json.
func Save(cfg *Config) error {
	path := getConfigPath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	cleanProvider := func(p ProviderConfig) ProviderConfig {
		return ProviderConfig{
			KeyEnv:    p.KeyEnv,
			Model:     p.Model,
			BaseURL:   p.BaseURL,
			ProjectID: p.ProjectID,
			Location:  p.Location,
		}
	}

	cfgFile := ConfigFile{
		Provider:        cfg.Provider,
		PromptFile:      unexpandPath(cfg.PromptFile),
		PromptsDir:      unexpandPath(cfg.PromptsDir),
		PromptsDirs:     unexpandPaths(cfg.PromptsDirs),
		ComponentsFile:  unexpandPath(cfg.ComponentsFile),
		Effort:          cfg.Effort,
		Timeout:         cfg.Timeout,
		MaxOutputTokens: cfg.MaxOutputTokens,
		MaxRetries:      cfg.MaxRetries,
		DefaultCopy:     cfg.DefaultCopy,
		OpenAI:          cleanProvider(cfg.Providers["openai"]),
		Cerebras:        cleanProvider(cfg.Providers["cerebras"]),
		DeepSeek:        cleanProvider(cfg.Providers["deepseek"]),
		Groq:            cleanProvider(cfg.Providers["groq"]),
		OpenRouter:      cleanProvider(cfg.Providers["openrouter"]),
		Zai:             cleanProvider(cfg.Providers["zai"]),
		Gemini:          cleanProvider(cfg.Providers["gemini"]),
		Omlx:            cleanProvider(cfg.Providers["omlx"]),
	}

	data, err := json.MarshalIndent(cfgFile, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, append(data, '\n'), 0600)
}

// validEfforts is the single source of truth for accepted effort values.
var validEfforts = []string{"low", "medium", "high"}
