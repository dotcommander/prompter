package main

import (
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
