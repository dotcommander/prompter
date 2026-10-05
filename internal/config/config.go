package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
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
	MaxRetries              int // Compatibility setting; generation replay remains disabled.
	DefaultCopy             bool

	// Cached system prompt loaded once at startup.
	SystemPrompt string `json:"-"`

	Providers map[string]ProviderConfig
	persisted map[string]any
	baseline  map[string]any
	explicit  map[string]any
	origins   map[string]string
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
	if strings.HasPrefix(path, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(path, "~\\")) {
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
	path = filepath.Clean(path)
	home, err := os.UserHomeDir()
	home = filepath.Clean(home)
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
			Model:   "deepseek-v4.1-flash",
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
	return resolveProvider(name, fileCfg, defaultCfg, nil)
}

func resolveProvider(name string, fileCfg ProviderConfig, defaultCfg ProviderConfig, origins map[string]string) ProviderConfig {
	upper := strings.ToUpper(name)
	res := fileCfg
	resolve := func(field, file, fallback string, envs ...string) string {
		for _, env := range envs {
			if value := os.Getenv(env); value != "" {
				if origins != nil {
					origins[name+"."+field] = "environment: " + safeEnvName(env)
				}
				return value
			}
		}
		if file != "" {
			if origins != nil {
				origins[name+"."+field] = "config file"
			}
			return file
		}
		if origins != nil {
			origins[name+"."+field] = "built-in default"
		}
		return fallback
	}
	// Resolve the selector before looking up its selected environment variable.
	res.KeyEnv = fileCfg.KeyEnv
	if origins != nil {
		origins[name+".key_env"] = "built-in default"
		if fileCfg.KeyEnv != "" {
			origins[name+".key_env"] = "config file"
		}
	}
	if selector, exists := os.LookupEnv("PROMPTER_" + upper + "_KEY_ENV"); exists {
		res.KeyEnv = selector
		if origins != nil {
			origins[name+".key_env"] = "environment: PROMPTER_" + upper + "_KEY_ENV"
		}
	}
	res.APIKey = resolve("api_key", fileCfg.APIKey, defaultCfg.APIKey, "PROMPTER_"+upper+"_API_KEY", res.KeyEnv, upper+"_API_KEY")
	res.Model = resolve("model", fileCfg.Model, defaultCfg.Model, "PROMPTER_"+upper+"_MODEL", upper+"_MODEL")
	res.BaseURL = resolve("base_url", fileCfg.BaseURL, defaultCfg.BaseURL, "PROMPTER_"+upper+"_BASE_URL", upper+"_BASE_URL")
	if name == "gemini" {
		res.ProjectID = resolve("project_id", fileCfg.ProjectID, defaultCfg.ProjectID, "PROMPTER_GEMINI_PROJECT_ID", "GEMINI_PROJECT_ID", "GOOGLE_CLOUD_PROJECT", "GCP_PROJECT")
		res.Location = resolve("location", fileCfg.Location, defaultCfg.Location, "PROMPTER_GEMINI_LOCATION", "GEMINI_LOCATION")
	}
	return res
}

// Load resolves environment over persisted settings over built-in defaults.
// Presence is retained separately so empty lists and explicit zero remain intent.
func Load() (*Config, error) {
	persisted := map[string]any{}
	var file ConfigFile
	loaded := false
	if path := getConfigPath(); path != "" {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err == nil {
			loaded = true
			if err := json.Unmarshal(data, &file); err != nil {
				return nil, fmt.Errorf("parse config: %w", err)
			}
			if err := json.Unmarshal(data, &persisted); err != nil {
				return nil, fmt.Errorf("parse config: %w", err)
			}
			if persisted == nil {
				return nil, fmt.Errorf("parse config: expected object")
			}
		}
	}
	cfg := &Config{persisted: persisted, origins: map[string]string{}}
	stringValue := func(field, env, value, fallback string) string {
		if v := os.Getenv(env); v != "" {
			cfg.origins[field] = "environment: " + env
			return v
		}
		if _, exists := persisted[field]; exists {
			cfg.origins[field] = "config file"
			if value != "" || fallback == "" {
				return value
			}
		}
		cfg.origins[field] = "built-in default"
		return fallback
	}
	cfg.Provider = stringValue("provider", "PROMPTER_PROVIDER", file.Provider, "")
	_, providerPresent := persisted["provider"]
	if loaded && providerPresent && cfg.Provider == "" {
		return nil, fmt.Errorf("config: provider must not be empty")
	}
	if cfg.Provider == "" {
		cfg.Provider = detectDefaultProvider()
		for _, env := range []string{"PROMPTER_" + strings.ToUpper(cfg.Provider) + "_API_KEY", strings.ToUpper(cfg.Provider) + "_API_KEY"} {
			if os.Getenv(env) != "" {
				cfg.origins["provider"] = "environment: " + env
				break
			}
		}
	}
	cfg.PromptFile = expandPath(stringValue("prompt_file", "PROMPTER_PROMPT_FILE", file.PromptFile, ""))
	cfg.PromptsDir = expandPath(stringValue("prompts_dir", "PROMPTER_PROMPTS_DIR", file.PromptsDir, "~/.config/prompter/prompts.d"))
	cfg.ComponentsFile = expandPath(stringValue("components_file", "PROMPTER_COMPONENTS_FILE", file.ComponentsFile, "~/.config/prompter/components.json"))
	cfg.Effort = stringValue("effort", "PROMPTER_EFFORT", file.Effort, "low")
	if !slices.Contains(validEfforts, cfg.Effort) {
		return nil, fmt.Errorf("invalid effort %q: must be low, medium, or high", cfg.Effort)
	}
	if env, exists := os.LookupEnv("PROMPTER_PROMPTS_DIRS"); exists {
		cfg.PromptsDirs = expandPaths(strings.Split(env, ","))
		cfg.origins["prompts_dirs"] = "environment: PROMPTER_PROMPTS_DIRS"
	} else if _, exists := persisted["prompts_dirs"]; exists {
		cfg.PromptsDirs = expandPaths(file.PromptsDirs)
		cfg.origins["prompts_dirs"] = "config file"
	} else {
		cfg.PromptsDirs = expandPaths([]string{"~/.config/prompter/prompts.d", "~/.config/roles/prompts"})
		cfg.origins["prompts_dirs"] = "built-in default"
	}
	number := func(field, env string, value, fallback, minimum int) (int, error) {
		_, present := persisted[field]
		cfg.origins[field] = "built-in default"
		if !present {
			value = fallback
		} else {
			cfg.origins[field] = "config file"
		}
		if text, exists := os.LookupEnv(env); exists && text != "" {
			parsed, err := strconv.Atoi(text)
			if err != nil {
				return 0, fmt.Errorf("config: %s must be an integer", env)
			}
			value = parsed
			present = true
			cfg.origins[field] = "environment: " + env
		}
		if value < minimum {
			return 0, fmt.Errorf("config: %s must be >= %d", field, minimum)
		}
		if field == "max_output_tokens" {
			cfg.MaxOutputTokensExplicit = present
		}
		return value, nil
	}
	var err error
	if cfg.Timeout, err = number("timeout", "PROMPTER_TIMEOUT", file.Timeout, DefaultTimeout, 1); err != nil {
		return nil, err
	}
	if cfg.MaxOutputTokens, err = number("max_output_tokens", "PROMPTER_MAX_OUTPUT_TOKENS", file.MaxOutputTokens, DefaultMaxOutputTokens, 1); err != nil {
		return nil, err
	}
	if cfg.MaxRetries, err = number("max_retries", "PROMPTER_MAX_RETRIES", file.MaxRetries, DefaultMaxRetries, 0); err != nil {
		return nil, err
	}
	cfg.DefaultCopy = file.DefaultCopy
	cfg.origins["default_copy"] = "built-in default"
	if _, exists := persisted["default_copy"]; exists {
		cfg.origins["default_copy"] = "config file"
	}
	if text, exists := os.LookupEnv("PROMPTER_DEFAULT_COPY"); exists && text != "" {
		cfg.origins["default_copy"] = "environment: PROMPTER_DEFAULT_COPY"
		cfg.DefaultCopy, err = strconv.ParseBool(text)
		if err != nil {
			return nil, fmt.Errorf("config: PROMPTER_DEFAULT_COPY must be a boolean")
		}
	}
	files := map[string]ProviderConfig{"openai": file.OpenAI, "cerebras": file.Cerebras, "deepseek": file.DeepSeek, "groq": file.Groq, "openrouter": file.OpenRouter, "zai": file.Zai, "gemini": file.Gemini, "omlx": file.Omlx}
	cfg.Providers = map[string]ProviderConfig{}
	for name, defaults := range DefaultProviders() {
		cfg.Providers[name] = resolveProvider(name, files[name], defaults, cfg.origins)
	}
	cfg.baseline = cfg.snapshot()
	return cfg, nil
}

// Save atomically stores deliberate changes without freezing inherited settings.
func Save(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("cannot save nil config")
	}
	path := getConfigPath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	intent := cfg.persistenceIntent()
	data, err := json.MarshalIndent(intent, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := writeConfigAtomically(path, append(data, '\n'), os.Rename); err != nil {
		return err
	}
	cfg.persisted = intent
	cfg.baseline = cfg.snapshot()
	cfg.explicit = nil
	return nil
}

// writeConfigAtomically keeps the last complete config in place until a fully
// written, synced replacement is ready. The temporary file shares its directory
// with the destination so rename does not cross filesystems.
func writeConfigAtomically(path string, data []byte, replace func(string, string) error) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()
	defer tmp.Close()
	if err = tmp.Chmod(0600); err != nil {
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary config: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err = replace(tmpPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// validEfforts is the single source of truth for accepted effort values.
var validEfforts = []string{"low", "medium", "high"}
