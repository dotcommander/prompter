package main

import (
	"os"
	"path/filepath"
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

func TestSaveConfigAndVault_SeedsStarterPromptsWhenEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	promptsDir := filepath.Join(home, ".config", "prompter", "prompts.d")
	cfg := &config.Config{
		Provider:   "gemini",
		PromptsDir: promptsDir,
		Providers:  config.DefaultProviders(),
	}

	if err := saveConfigAndVault(cfg); err != nil {
		t.Fatalf("saveConfigAndVault: %v", err)
	}

	// Verify config.json was created
	configFile := filepath.Join(home, ".config", "prompter", "config.json")
	if _, err := os.Stat(configFile); err != nil {
		t.Fatalf("config.json was not created: %v", err)
	}

	// Verify starter prompts were seeded
	expectedFiles := []string{
		"enhance.md",
		"critique.md",
		"rewrite.md",
		"refactor.md",
		"code-review.md",
		"system-architect.md",
		"git-commit.md",
		"unit-test.md",
	}
	for _, name := range expectedFiles {
		p := filepath.Join(promptsDir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("expected starter prompt %s: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("starter prompt %s is empty", name)
		}
	}
}

func TestSaveConfigAndVault_PreservesExistingPrompts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	promptsDir := filepath.Join(home, ".config", "prompter", "prompts.d")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	refactorPath := filepath.Join(promptsDir, "refactor.md")
	customContent := []byte("custom prompt content\n")
	if err := os.WriteFile(refactorPath, customContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &config.Config{
		Provider:   "gemini",
		PromptsDir: promptsDir,
		Providers:  config.DefaultProviders(),
	}

	if err := saveConfigAndVault(cfg); err != nil {
		t.Fatalf("saveConfigAndVault: %v", err)
	}

	got, err := os.ReadFile(refactorPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(customContent) {
		t.Errorf("existing prompt was overwritten, got %q, want %q", string(got), string(customContent))
	}
}
