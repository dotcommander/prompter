package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func readIntent(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStandardEnvironmentOverridesFileAndSelector(t *testing.T) {
	clearAllProviderEnvVars(t)
	writeTestConfig(t, `{"provider":"gemini","openai":{"api_key":"file","key_env":"OLD_KEY","model":"file-model","base_url":"file-url"},"gemini":{"project_id":"file-project","location":"file-location"}}`)
	t.Setenv("OPENAI_API_KEY", "standard-key")
	t.Setenv("OPENAI_MODEL", "standard-model")
	t.Setenv("OPENAI_BASE_URL", "https://standard.test")
	t.Setenv("GEMINI_PROJECT_ID", "env-project")
	t.Setenv("GEMINI_LOCATION", "env-location")
	t.Setenv("OLD_KEY", "")
	t.Setenv("PROMPTER_OPENAI_KEY_ENV", "SELECTED_KEY")
	t.Setenv("SELECTED_KEY", "selected-key")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Providers["openai"]
	if p.APIKey != "selected-key" || p.KeyEnv != "SELECTED_KEY" || p.Model != "standard-model" || p.BaseURL != "https://standard.test" {
		t.Fatal("provider credential or noncredential precedence fields do not match the expected fixture")
	}
	if cfg.Providers["gemini"].ProjectID != "env-project" || cfg.Providers["gemini"].Location != "env-location" {
		t.Fatal("Gemini precedence")
	}
	if cfg.CredentialStatus("openai").Source != "environment: SELECTED_KEY" {
		t.Fatal("selected source")
	}
	t.Setenv("PROMPTER_OPENAI_API_KEY", "prefixed-key")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers["openai"].APIKey != "prefixed-key" || cfg.CredentialStatus("openai").Source != "environment: PROMPTER_OPENAI_API_KEY" {
		t.Fatal("prefixed source")
	}
	t.Setenv("PROMPTER_OPENAI_API_KEY", "")
	t.Setenv("SELECTED_KEY", "")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers["openai"].APIKey != "standard-key" {
		t.Fatal("standard key must override file")
	}
}

func TestSavePreservesSparsePersistedIntent(t *testing.T) {
	clearAllProviderEnvVars(t)
	writeTestConfig(t, `{"provider":"openai","max_retries":0,"default_copy":false,"timeout":60,"prompts_dirs":[],"extension":{"enabled":true},"openai":{"api_key":"file-key","model":"gpt-5.6-luna"}}`)
	t.Setenv("OPENAI_MODEL", "inherited-model")
	t.Setenv("PROMPTER_TIMEOUT", "123")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxRetries != 0 || len(cfg.PromptsDirs) != 0 {
		t.Fatal("zero/empty intent lost")
	}
	effort := "high"
	cfg.ApplyPatch(ConfigPatch{Effort: &effort})
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got := readIntent(t)
	if got["timeout"] != float64(60) || got["max_retries"] != float64(0) || got["default_copy"] != false {
		t.Fatalf("persisted values changed: %v", got)
	}
	if len(got["prompts_dirs"].([]any)) != 0 || got["extension"] == nil {
		t.Fatal("unrelated intent lost")
	}
	provider := got["openai"].(map[string]any)
	if provider["model"] != "gpt-5.6-luna" || provider["api_key"] != nil {
		t.Fatal("saved provider model does not match persisted intent or an API key was retained")
	}
	if got["gemini"] != nil || got["groq"] != nil || got["max_output_tokens"] != nil {
		t.Fatal("inherited defaults stamped")
	}
}

func TestExplicitPatchesRoundTripDefaultsClearsAndEmptyLists(t *testing.T) {
	clearAllProviderEnvVars(t)
	writeTestConfig(t, `{"provider":"groq","default_copy":true,"prompts_dirs":["~/one"],"prompt_file":"~/prompt.md","groq":{"model":"custom","key_env":"OLD_KEY"}}`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	no := false
	zero := 0
	timeout := DefaultTimeout
	tokens := DefaultMaxOutputTokens
	empty := ""
	model := DefaultProviders()["groq"].Model
	dirs := []string{}
	cfg.ApplyPatch(ConfigPatch{DefaultCopy: &no, MaxRetries: &zero, Timeout: &timeout, MaxOutputTokens: &tokens, PromptFile: &empty, PromptsDirs: &dirs, Providers: map[string]ProviderPatch{"groq": {Model: &model, KeyEnv: &empty}}})
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got := readIntent(t)
	if got["prompt_file"] != "" || got["default_copy"] != false || got["max_retries"] != float64(0) || got["timeout"] != float64(DefaultTimeout) || got["max_output_tokens"] != float64(DefaultMaxOutputTokens) {
		t.Fatalf("patch intent lost: %v", got)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultCopy || cfg.MaxRetries != 0 || cfg.PromptFile != "" || len(cfg.PromptsDirs) != 0 || cfg.Providers["groq"].KeyEnv != "" || cfg.Providers["groq"].Model != model {
		t.Fatal("patch round trip")
	}
}

func TestPromptDirectoryPresenceAndEnvironment(t *testing.T) {
	for _, tt := range []struct {
		name, content, env string
		set                bool
		want               []string
	}{
		{name: "absent", content: `{"provider":"openai"}`, want: []string{"~/.config/prompter/prompts.d", "~/.config/roles/prompts"}},
		{name: "file empty", content: `{"provider":"openai","prompts_dirs":[]}`, want: []string{}},
		{name: "trimmed env", content: `{"provider":"openai","prompts_dirs":["~/file"]}`, env: " ~/first , , /tmp/second ", set: true, want: []string{"~/first", "/tmp/second"}},
		{name: "empty env", content: `{"provider":"openai","prompts_dirs":["~/file"]}`, env: "", set: true, want: []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writeTestConfig(t, tt.content)
			if tt.set {
				t.Setenv("PROMPTER_PROMPTS_DIRS", tt.env)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.PromptsDirs, expandPaths(tt.want)) {
				t.Fatalf("got %v", cfg.PromptsDirs)
			}
		})
	}
}

func TestStrictNumericEnvironment(t *testing.T) {
	for _, env := range []string{"PROMPTER_TIMEOUT", "PROMPTER_MAX_OUTPUT_TOKENS", "PROMPTER_MAX_RETRIES"} {
		for _, bad := range []string{"12junk", "1.5", "-1", "999999999999999999999999999"} {
			t.Run(env+bad, func(t *testing.T) {
				writeTestConfig(t, `{"provider":"openai"}`)
				t.Setenv(env, bad)
				if _, err := Load(); err == nil {
					t.Fatal("invalid numeric accepted")
				}
			})
		}
	}
	for _, field := range []string{"timeout", "max_output_tokens"} {
		t.Run(field, func(t *testing.T) {
			writeTestConfig(t, `{"provider":"openai","`+field+`":0}`)
			if _, err := Load(); err == nil {
				t.Fatal("nonpositive file value accepted")
			}
		})
	}
}

func TestDirectConfigSaveKeepsOnlySuppliedProviders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &Config{Provider: "openai", Providers: map[string]ProviderConfig{"openai": {Model: "custom", APIKey: "never-persist"}}}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got := readIntent(t)
	if got["groq"] != nil || got["gemini"] != nil || got["openai"].(map[string]any)["api_key"] != nil {
		t.Fatal("saved data includes an unexpected provider or an API key")
	}
}

func TestCredentialStatusAndSafeSelector(t *testing.T) {
	cfg := &Config{Providers: map[string]ProviderConfig{"gemini": {APIKey: "present", BaseURL: "https://aiplatform.googleapis.com/v1"}, "openai": {APIKey: "present", KeyEnv: "unsafe-value"}}}
	if status := cfg.CredentialStatus("gemini"); !status.Unchecked || status.Available {
		t.Fatalf("ADC status: %+v", status)
	}
	p := cfg.Providers["gemini"]
	p.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	cfg.Providers["gemini"] = p
	if status := cfg.CredentialStatus("gemini"); status.Unchecked || !status.Available {
		t.Fatalf("Studio status: %+v", status)
	}
	for _, input := range []string{"sk-test-value", "AIzaExample", "gsk_example", "sk_example", "1KEY", "KEY-NAME", "KEY\nNAME"} {
		if err := ValidateKeyEnv(input); err == nil || strings.Contains(err.Error(), input) {
			t.Fatalf("unsafe selector accepted/echoed")
		}
		if strings.Contains(SafeKeyEnv(input), input) {
			t.Fatal("unsafe selector displayed")
		}
	}
	for _, input := range []string{"", "KEY", "_KEY", "KEY_2"} {
		if err := ValidateKeyEnv(input); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPortablePathNormalization(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := home + string(filepath.Separator) + "one" + string(filepath.Separator) + ".." + string(filepath.Separator) + "two"
	if got := unexpandPath(path); got != "~/two" {
		t.Fatalf("cleaned path = %q", got)
	}
	if runtime.GOOS != "windows" {
		if got := expandPath(`~\literal`); got != `~\literal` {
			t.Fatalf("Unix backslash changed %q", got)
		}
		if got := unexpandPath(filepath.Join(home, `literal\name`)); got != `~/literal\name` {
			t.Fatalf("Unix backslash changed %q", got)
		}
	}
}

func TestSparseSaveFromMissingFileReloads(t *testing.T) {
	for _, changeEffort := range []bool{false, true} {
		name := "unchanged"
		if changeEffort {
			name = "effort only"
		}
		t.Run(name, func(t *testing.T) {
			clearAllProviderEnvVars(t)
			t.Setenv("HOME", t.TempDir())
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if changeEffort {
				effort := "high"
				cfg.ApplyPatch(ConfigPatch{Effort: &effort})
			}
			if err := Save(cfg); err != nil {
				t.Fatal(err)
			}
			intent := readIntent(t)
			if changeEffort {
				if len(intent) != 1 || intent["effort"] != "high" {
					t.Fatal("effort-only save must contain only deliberate effort intent")
				}
			} else if len(intent) != 0 {
				t.Fatal("unchanged save must remain sparse")
			}
			reloaded, err := Load()
			if err != nil {
				t.Fatalf("reload sparse config: %v", err)
			}
			if reloaded.Provider != "gemini" {
				t.Fatal("omitted provider must resolve to default")
			}
			expectedEffort := "low"
			if changeEffort {
				expectedEffort = "high"
			}
			if reloaded.Effort != expectedEffort {
				t.Fatal("reloaded effort does not match deliberate intent")
			}
		})
	}
}
