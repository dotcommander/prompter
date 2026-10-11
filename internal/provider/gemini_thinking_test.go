package provider

import (
	"strings"
	"testing"
)

func TestValidateGeminiThinkingLevel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		model  string
		effort string
		ok     bool
	}{
		{"pro preview rejects medium", "gemini-3-pro-preview", "medium", false},
		{"pro preview accepts high", "gemini-3-pro-preview", "high", true},
		{"3.6 flash accepts minimal", "gemini-3.6-flash", "minimal", true},
		{"3.7 flash rejects minimal", "gemini-3.7-flash", "minimal", false},
		{"3.7 flash accepts medium", "gemini-3.7-flash", "medium", true},
		{"2.5 flash lite accepts low", "gemini-2.5-flash-lite", "low", true},
		{"models prefix and case normalize", "models/Gemini-3.6-Flash", "Minimal", true},
		{"unknown model fails open", "gemini-4-nano", "minimal", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateGeminiThinkingLevel(tc.model, tc.effort)
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error for %s %s", tc.model, tc.effort)
			}
		})
	}
}

func TestGeminiBuildRequestBodyLowercasesAndValidatesThinkingLevel(t *testing.T) {
	t.Parallel()
	provider := NewGemini("", "", "", "gemini-3.6-flash", "", 0, 1024)

	body, err := provider.(*geminiProvider).buildRequestBody(CallRequest{Effort: "LOW"})
	if err != nil {
		t.Fatalf("buildRequestBody: %v", err)
	}
	if !strings.Contains(string(body), `"thinkingLevel":"low"`) {
		t.Fatalf("payload missing lowercase thinkingLevel: %s", body)
	}

	if _, err := provider.(*geminiProvider).buildRequestBody(CallRequest{Effort: "minimal"}); err != nil {
		t.Fatalf("minimal is supported on gemini-3.6-flash: %v", err)
	}
	unsupported := NewGemini("", "", "", "gemini-3-pro-preview", "", 0, 1024)
	if _, err := unsupported.(*geminiProvider).buildRequestBody(CallRequest{Effort: "medium"}); err == nil {
		t.Fatal("expected error for medium on gemini-3-pro-preview")
	}
}
