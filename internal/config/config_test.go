package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "prompter")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("HOME", home)
	viper.Reset()
	t.Cleanup(viper.Reset)
	return home
}

func clearAllProviderEnvVars(t *testing.T) {
	t.Helper()
	for _, name := range []string{"GEMINI", "OPENAI", "GROQ", "CEREBRAS", "OPENROUTER", "SYNTHETIC", "ZAI", "WORMHOLE", "OMLX"} {
		t.Setenv(name+"_API_KEY", "")
		t.Setenv(name+"_MODEL", "")
		t.Setenv(name+"_BASE_URL", "")
		t.Setenv("PROMPTER_"+name+"_API_KEY", "")
		t.Setenv("PROMPTER_"+name+"_MODEL", "")
		t.Setenv("PROMPTER_"+name+"_BASE_URL", "")
	}
	t.Setenv("PROMPTER_PROVIDER", "")
	t.Setenv("GEMINI_PROJECT_ID", "")
	t.Setenv("GEMINI_LOCATION", "")
	t.Setenv("PROMPTER_GEMINI_PROJECT_ID", "")
	t.Setenv("PROMPTER_GEMINI_LOCATION", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GCP_PROJECT", "")
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantProviders map[string]ProviderConfig
		wantDefaults  bool
		wantErr       bool
	}{
		{
			name: "all legacy provider blocks",
			content: `{
				"provider":"cerebras",
				"prompts_dirs":["~/one","/tmp/two"],
				"openai":{"api_key":"openai-key","model":"openai-model","base_url":"openai-url"},
				"synthetic":{"api_key":"synthetic-key","model":"synthetic-model","base_url":"synthetic-url"},
				"cerebras":{"api_key":"cerebras-key","model":"cerebras-model","base_url":"cerebras-url"},
				"groq":{"api_key":"groq-key","model":"groq-model","base_url":"groq-url"},
				"openrouter":{"api_key":"openrouter-key","model":"openrouter-model","base_url":"openrouter-url"},
				"zai":{"api_key":"zai-key","model":"zai-model","base_url":"zai-url"},
				"wormhole":{"api_key":"wormhole-key","model":"wormhole-model","base_url":"wormhole-url"},
				"gemini":{"api_key":"gemini-key","model":"gemini-model","base_url":"gemini-url","project_id":"project","location":"location"},
				"omlx":{"api_key":"omlx-key","model":"omlx-model","base_url":"omlx-url"}
			}`,
			wantProviders: map[string]ProviderConfig{
				"openai":     {APIKey: "openai-key", Model: "openai-model", BaseURL: "openai-url"},
				"synthetic":  {APIKey: "synthetic-key", Model: "synthetic-model", BaseURL: "synthetic-url"},
				"cerebras":   {APIKey: "cerebras-key", Model: "cerebras-model", BaseURL: "cerebras-url"},
				"groq":       {APIKey: "groq-key", Model: "groq-model", BaseURL: "groq-url"},
				"openrouter": {APIKey: "openrouter-key", Model: "openrouter-model", BaseURL: "openrouter-url"},
				"zai":        {APIKey: "zai-key", Model: "zai-model", BaseURL: "zai-url"},
				"wormhole":   {APIKey: "wormhole-key", Model: "wormhole-model", BaseURL: "wormhole-url"},
				"gemini":     {APIKey: "gemini-key", Model: "gemini-model", BaseURL: "gemini-url", ProjectID: "project", Location: "location"},
				"omlx":       {APIKey: "omlx-key", Model: "omlx-model", BaseURL: "omlx-url"},
			},
		},
		{
			name:          "partial config",
			content:       `{"provider":"openai","openai":{"api_key":"sk-test123","model":"gpt-4"}}`,
			wantProviders: map[string]ProviderConfig{"openai": {APIKey: "sk-test123", Model: "gpt-4"}},
			wantDefaults:  true,
		},
		{name: "invalid JSON", content: `{invalid json`, wantErr: true},
		{name: "missing provider", content: `{"openai":{"model":"gpt-4"}}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := writeTestConfig(t, tt.content)
			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			for name, want := range tt.wantProviders {
				if got := cfg.Providers[name]; !reflect.DeepEqual(got, want) {
					t.Errorf("Providers[%q] = %+v, want %+v", name, got, want)
				}
			}
			if len(cfg.Providers) != 9 {
				t.Errorf("len(Providers) = %d, want 9", len(cfg.Providers))
			}
			if tt.wantDefaults {
				if cfg.Effort != "low" || cfg.MaxOutputTokens != DefaultMaxOutputTokens || cfg.MaxRetries != DefaultMaxRetries {
					t.Errorf("defaults = effort %q, tokens %d, retries %d", cfg.Effort, cfg.MaxOutputTokens, cfg.MaxRetries)
				}
				wantComponents := filepath.Join(home, ".config", "prompter", "components.json")
				if cfg.ComponentsFile != wantComponents {
					t.Errorf("ComponentsFile = %q, want %q", cfg.ComponentsFile, wantComponents)
				}
			}
			if tt.name == "all legacy provider blocks" {
				wantDirs := []string{filepath.Join(home, "one"), "/tmp/two"}
				if !reflect.DeepEqual(cfg.PromptsDirs, wantDirs) {
					t.Errorf("PromptsDirs = %v, want %v", cfg.PromptsDirs, wantDirs)
				}
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAllProviderEnvVars(t)
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load unexpected error = %v", err)
	}
	if cfg.Provider != "gemini" {
		t.Errorf("Provider = %q, want gemini", cfg.Provider)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %d, want %d", cfg.Timeout, DefaultTimeout)
	}
	if len(cfg.Providers) != 9 {
		t.Errorf("len(Providers) = %d, want 9", len(cfg.Providers))
	}
	if cfg.Providers["gemini"].Model != "gemini-3.7-flash" {
		t.Errorf("gemini model = %q, want gemini-3.7-flash", cfg.Providers["gemini"].Model)
	}
}

func TestLoadStandardEnvironmentVariables(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAllProviderEnvVars(t)
	viper.Reset()
	t.Cleanup(viper.Reset)

	t.Setenv("OPENAI_API_KEY", "sk-direct-openai")
	t.Setenv("GROQ_API_KEY", "gsk-direct-groq")
	t.Setenv("PROMPTER_PROVIDER", "groq")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load unexpected error = %v", err)
	}
	if cfg.Provider != "groq" {
		t.Errorf("Provider = %q, want groq", cfg.Provider)
	}
	if cfg.Providers["openai"].APIKey != "sk-direct-openai" {
		t.Errorf("openai APIKey = %q, want sk-direct-openai", cfg.Providers["openai"].APIKey)
	}
	if cfg.Providers["groq"].APIKey != "gsk-direct-groq" {
		t.Errorf("groq APIKey = %q, want gsk-direct-groq", cfg.Providers["groq"].APIKey)
	}
}

func TestLoadAutoDetectsActiveProviderFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAllProviderEnvVars(t)
	viper.Reset()
	t.Cleanup(viper.Reset)

	// Case 1: No config, OPENAI_API_KEY set -> auto-selects openai
	t.Setenv("OPENAI_API_KEY", "sk-openai-detected")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", cfg.Provider)
	}

	// Case 2: No config, GROQ_API_KEY set (with no OpenAI key) -> auto-selects groq
	clearAllProviderEnvVars(t)
	t.Setenv("GROQ_API_KEY", "gsk-groq-detected")
	viper.Reset()
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "groq" {
		t.Errorf("Provider = %q, want groq", cfg.Provider)
	}
}

func TestLoadEnvironmentOverridesLegacyProviderFields(t *testing.T) {
	writeTestConfig(t, `{
		"provider":"gemini",
		"openai":{"api_key":"file-key"},
		"synthetic":{"api_key":"file-key"},
		"cerebras":{"api_key":"file-key"},
		"groq":{"api_key":"file-key"},
		"openrouter":{"api_key":"file-key"},
		"zai":{"api_key":"file-key"},
		"wormhole":{"api_key":"file-key"},
		"gemini":{"api_key":"file-key","project_id":"file-project"},
		"omlx":{"api_key":"file-key"}
	}`)
	for _, name := range []string{"OPENAI", "SYNTHETIC", "CEREBRAS", "GROQ", "OPENROUTER", "ZAI", "WORMHOLE", "GEMINI", "OMLX"} {
		t.Setenv("PROMPTER_"+name+"_API_KEY", strings.ToLower(name)+"-env-key")
	}
	t.Setenv("PROMPTER_GEMINI_PROJECT_ID", "env-project")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, name := range []string{"openai", "synthetic", "cerebras", "groq", "openrouter", "zai", "wormhole", "gemini", "omlx"} {
		if got, want := cfg.Providers[name].APIKey, name+"-env-key"; got != want {
			t.Errorf("%s APIKey = %q, want %q", name, got, want)
		}
	}
	if got := cfg.Providers["gemini"].ProjectID; got != "env-project" {
		t.Errorf("gemini ProjectID = %q, want env-project", got)
	}
}

func TestLoadMaxOutputTokensExplicit(t *testing.T) {
	writeTestConfig(t, `{"provider":"wormhole","max_output_tokens":123,"wormhole":{"model":"groq/openai/gpt-oss-120b"}}`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.MaxOutputTokensExplicit || cfg.MaxOutputTokens != 123 {
		t.Fatalf("MaxOutputTokensExplicit = %v, MaxOutputTokens = %d", cfg.MaxOutputTokensExplicit, cfg.MaxOutputTokens)
	}
}

func TestLoadDefaultCopy(t *testing.T) {
	writeTestConfig(t, `{"provider":"gemini","default_copy":true,"gemini":{"model":"gemini-3.7-flash"}}`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.DefaultCopy {
		t.Errorf("cfg.DefaultCopy = %v, want true", cfg.DefaultCopy)
	}
}

func TestSaveConfigAndKeyEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg.Provider = "groq"
	groqCfg := cfg.Providers["groq"]
	groqCfg.KeyEnv = "MY_GROQ_KEY_ENV"
	groqCfg.Model = "custom-groq-model"
	cfg.Providers["groq"] = groqCfg
	cfg.DefaultCopy = true

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Read file directly and check that no plaintext secrets were written
	configBytes, err := os.ReadFile(filepath.Join(home, ".config", "prompter", "config.json"))
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(configBytes), `"key_env": "MY_GROQ_KEY_ENV"`) {
		t.Errorf("saved config missing key_env: %s", string(configBytes))
	}

	// Now set the custom env var and test Load()
	t.Setenv("MY_GROQ_KEY_ENV", "gsk-custom-secret")
	viper.Reset()
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load reloaded config: %v", err)
	}
	if loaded.Provider != "groq" {
		t.Errorf("loaded.Provider = %q, want groq", loaded.Provider)
	}
	if loaded.Providers["groq"].APIKey != "gsk-custom-secret" {
		t.Errorf("loaded.Providers[groq].APIKey = %q, want gsk-custom-secret", loaded.Providers["groq"].APIKey)
	}
	if loaded.Providers["groq"].Model != "custom-groq-model" {
		t.Errorf("loaded.Providers[groq].Model = %q, want custom-groq-model", loaded.Providers["groq"].Model)
	}
	if !loaded.DefaultCopy {
		t.Errorf("loaded.DefaultCopy = %v, want true", loaded.DefaultCopy)
	}
}

func TestPathExpandAndUnexpand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// expandPath
	if got := expandPath("~/foo/bar"); got != filepath.Join(home, "foo", "bar") {
		t.Errorf("expandPath(~/foo/bar) = %q, want %q", got, filepath.Join(home, "foo", "bar"))
	}
	if got := expandPath("~"); got != home {
		t.Errorf("expandPath(~) = %q, want %q", got, home)
	}
	if got := expandPath("/var/log"); got != "/var/log" {
		t.Errorf("expandPath(/var/log) = %q, want /var/log", got)
	}
	if got := expandPath(""); got != "" {
		t.Errorf("expandPath(\"\") = %q, want \"\"", got)
	}

	// unexpandPath
	if got := unexpandPath(filepath.Join(home, "my", "path")); got != "~/my/path" {
		t.Errorf("unexpandPath(%q) = %q, want ~/my/path", filepath.Join(home, "my", "path"), got)
	}
	if got := unexpandPath(home); got != "~" {
		t.Errorf("unexpandPath(%q) = %q, want ~", home, got)
	}
	if got := unexpandPath("/tmp/other"); got != "/tmp/other" {
		t.Errorf("unexpandPath(/tmp/other) = %q, want /tmp/other", got)
	}

	// expandPaths & unexpandPaths
	paths := []string{"~/a", "/b"}
	expanded := expandPaths(paths)
	wantExpanded := []string{filepath.Join(home, "a"), "/b"}
	if !reflect.DeepEqual(expanded, wantExpanded) {
		t.Errorf("expandPaths = %v, want %v", expanded, wantExpanded)
	}
	unexpanded := unexpandPaths(expanded)
	if !reflect.DeepEqual(unexpanded, paths) {
		t.Errorf("unexpandPaths = %v, want %v", unexpanded, paths)
	}
}

func TestSaveWritesPortableHomePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfg := &Config{
		Provider:       "gemini",
		PromptFile:     filepath.Join(home, ".config", "prompter", "prompts", "enhance.md"),
		PromptsDir:     filepath.Join(home, ".config", "prompter", "prompts.d"),
		PromptsDirs:    []string{filepath.Join(home, ".config", "prompter", "prompts.d"), "/tmp/shared/prompts"},
		ComponentsFile: filepath.Join(home, ".config", "prompter", "components.json"),
		Effort:         "low",
		Timeout:        60,
		Providers:      DefaultProviders(),
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(home, ".config", "prompter", "config.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	str := string(raw)

	if !strings.Contains(str, `"prompt_file": "~/.config/prompter/prompts/enhance.md"`) {
		t.Errorf("config.json should contain portable prompt_file, got: %s", str)
	}
	if !strings.Contains(str, `"prompts_dir": "~/.config/prompter/prompts.d"`) {
		t.Errorf("config.json should contain portable prompts_dir, got: %s", str)
	}
	if !strings.Contains(str, `"~/.config/prompter/prompts.d"`) || !strings.Contains(str, `"/tmp/shared/prompts"`) {
		t.Errorf("config.json should contain portable prompts_dirs, got: %s", str)
	}
	if !strings.Contains(str, `"components_file": "~/.config/prompter/components.json"`) {
		t.Errorf("config.json should contain portable components_file, got: %s", str)
	}
}
