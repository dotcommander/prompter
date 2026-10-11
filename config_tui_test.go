package main

import (
	"charm.land/huh/v2"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dotcommander/prompter/internal/config"
)

func TestGeminiConfigurationStatusDoesNotClaimUncheckedADC(t *testing.T) {
	for _, name := range []string{"GEMINI_API_KEY", "PROMPTER_GEMINI_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS"} {
		t.Setenv(name, "")
	}
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{"gemini": {}}}

	configured, detail := isProviderConfigured("gemini", cfg)
	if configured || !strings.Contains(detail, "unchecked") {
		t.Fatalf("isProviderConfigured = %t, %q", configured, detail)
	}
}

func TestGeminiConfigurationStatusRecognizesPrompterAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("PROMPTER_GEMINI_API_KEY", "test-key")
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{"gemini": {APIKey: "test-key", BaseURL: "https://generativelanguage.googleapis.com"}}}

	configured, detail := isProviderConfigured("gemini", cfg)
	if !configured || !strings.Contains(detail, "PROMPTER_GEMINI_API_KEY") {
		t.Fatalf("isProviderConfigured = %t, %q", configured, detail)
	}
}

func TestPopularModelsFor(t *testing.T) {
	t.Parallel()

	want := map[string][]string{
		"gemini":     {"gemini-3.7-flash"},
		"openai":     {"gpt-5.6-luna"},
		"groq":       {"qwen/qwen3.8-27b", "qwen/qwen3.6-27b"},
		"cerebras":   {"gpt-oss-120b", "gemma-4-31b"},
		"deepseek":   {"deepseek-v4.1-flash", "deepseek-v4.1-flash-vision-exp"},
		"openrouter": {"openrouter/free", "anthropic/claude-sonnet-5", "meta-llama/llama-3.3-70b-instruct"},
		"zai":        {"glm-5.3-flash", "glm-5.3"},
		"omlx":       {"Ornith-1.5-35B-A3B-oQ4e-mtp", "Qwen2.5-Coder-7B-Instruct-4bit", "Llama-3.2-3B-Instruct-4bit"},
	}

	for p, wantModels := range want {
		models := popularModelsFor(p)
		if len(models) != len(wantModels) {
			t.Errorf("popularModelsFor(%q) returned %d models, want %d", p, len(models), len(wantModels))
			continue
		}
		for i, m := range models {
			if m.id == "" || m.label == "" {
				t.Errorf("popularModelsFor(%q) returned invalid model: %+v", p, m)
			}
			if m.id != wantModels[i] {
				t.Errorf("popularModelsFor(%q)[%d].id = %q, want %q", p, i, m.id, wantModels[i])
			}
		}
	}

	// Unknown provider should return nil
	if unknown := popularModelsFor("unknown-prov"); unknown != nil {
		t.Errorf("popularModelsFor(unknown-prov) = %v, want nil", unknown)
	}
	if got, want := popularModelsFor("zai"), []modelChoice{
		{"glm-5.3-flash", "glm-5.3-flash (Default / High Speed)"},
		{"glm-5.3", "glm-5.3 (Latest Flagship)"},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("popularModelsFor(\"zai\") = %#v, want %#v", got, want)
	}
}

func TestConfigWizardQuitAndValidation(t *testing.T) {
	for _, value := range configKeyMap().Quit.Keys() {
		if value == "q" {
			t.Fatal("q is reserved for quitting")
		}
	}
	for _, value := range []string{"", "https://example.com/v1", "http://localhost:8000/v1"} {
		if err := validateBaseURL(value); err != nil {
			t.Fatalf("valid URL rejected: %v", err)
		}
	}
	for _, value := range []string{"localhost:8000", "ftp://example.com", "https:///path", "https://"} {
		if validateBaseURL(value) == nil {
			t.Fatal("invalid URL accepted")
		}
	}
	for _, value := range []string{"sk-private-credential", "AIzaSyPrivateCredential", "gsk_privatecredential", "bad name"} {
		err := config.ValidateKeyEnv(value)
		if err == nil || strings.Contains(err.Error(), value) {
			t.Fatal("unsafe selector validation")
		}
	}
}

func TestConfigWizardSavePersistsPortableConfigWithoutSecrets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	old := runConfigStep
	defer func() { runConfigStep = old }()
	runConfigStep = func(*huh.Form) error { return nil }

	cfg := &config.Config{Provider: "groq", Providers: config.DefaultProviders()}
	cfg.PromptsDir = filepath.Join(home, ".config", "prompter", "prompts.d")
	if err := RunConfigForm(cfg); err != nil {
		t.Fatalf("RunConfigForm: %v", err)
	}

	path := filepath.Join(home, ".config", "prompter", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("saved config is not valid JSON: %v\n%s", err, data)
	}
	if saved["provider"] != "groq" {
		t.Errorf("provider = %v, want groq", saved["provider"])
	}
	if got, _ := saved["prompts_dir"].(string); got != "~/.config/prompter/prompts.d" {
		t.Errorf("prompts_dir = %q, want portable ~/.config/prompter/prompts.d", got)
	}
	for key, value := range saved {
		fields, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if _, has := fields["api_key"]; has {
			t.Errorf("provider %q persisted an api_key", key)
		}
	}
}

func TestConfigWizardCancellationDoesNotSave(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := runConfigStep
	defer func() { runConfigStep = old }()
	for cancelledStep := 1; cancelledStep <= 3; cancelledStep++ {
		calls := 0
		runConfigStep = func(*huh.Form) error {
			calls++
			if calls == cancelledStep {
				return huh.ErrUserAborted
			}
			return nil
		}
		cfg := &config.Config{Provider: "groq", Providers: config.DefaultProviders()}
		if err := RunConfigForm(cfg); !errors.Is(err, ErrConfigCancelled) {
			t.Fatalf("step %d: %v", cancelledStep, err)
		}
		if cfg.Provider != "groq" {
			t.Fatal("cancelled wizard mutated provider")
		}
		if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".config/prompter/config.json")); !os.IsNotExist(err) {
			t.Fatal("cancelled wizard saved")
		}
	}
}
