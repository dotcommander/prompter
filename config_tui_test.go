package main

import (
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
	if configured || detail != "ADC availability not checked" {
		t.Fatalf("isProviderConfigured = %t, %q", configured, detail)
	}
}

func TestGeminiConfigurationStatusRecognizesPrompterAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("PROMPTER_GEMINI_API_KEY", "test-key")
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{"gemini": {}}}

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
		"deepseek":   {"deepseek-v4.1-flash", "deepseek-v4.1-flash", "deepseek-v4.1-flash-vision-exp"},
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
