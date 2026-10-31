package main

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single word", "four", 1},
		{"words beat byte estimate", "a b c d e", 5},
		{"byte estimate beats words", "hello world", 3},
		{"whitespace collapses", "  spaced \t out \n\n text ", 6},
		{"unicode bytes count", "语言模型", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := estimateTokens(tt.input); got != tt.want {
				t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestRoutingEnabled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		mode           string
		flagProvider   string
		providerOrigin string
		want           bool
	}{
		{"auto without explicit selection", "auto", "", "built-in default", true},
		{"auto with ambient key detection", "auto", "", "environment: GEMINI_API_KEY", true},
		{"off mode never routes", "off", "", "built-in default", false},
		{"provider flag wins", "auto", "groq", "built-in default", false},
		{"provider env wins", "auto", "", "environment: PROMPTER_PROVIDER", false},
		{"provider file key wins", "auto", "", "config file", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := routingEnabled(tt.mode, tt.flagProvider, tt.providerOrigin); got != tt.want {
				t.Errorf("routingEnabled(%q, %q, %q) = %t, want %t", tt.mode, tt.flagProvider, tt.providerOrigin, got, tt.want)
			}
		})
	}
}

func TestDecideRoute(t *testing.T) {
	t.Parallel()
	t.Run("short input stays local", func(t *testing.T) {
		t.Parallel()
		decision, err := decideRoute("omlx", "gemini", 4096, &promptInput{source: "tighten this prompt"})
		if err != nil {
			t.Fatalf("decideRoute error = %v", err)
		}
		if decision.Provider != "omlx" {
			t.Errorf("decision.Provider = %q, want omlx", decision.Provider)
		}
		if !strings.Contains(decision.Detail, "local budget") {
			t.Errorf("decision.Detail missing local budget context: %s", decision.Detail)
		}
	})
	t.Run("large input escalates to frontier", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("explain this repository in detail. ", 512)
		decision, err := decideRoute("omlx", "gemini", 4096, &promptInput{source: long})
		if err != nil {
			t.Fatalf("decideRoute error = %v", err)
		}
		if decision.Provider != "gemini" {
			t.Errorf("decision.Provider = %q, want gemini", decision.Provider)
		}
		if !strings.Contains(decision.Detail, "> 4096 local budget") {
			t.Errorf("decision.Detail missing escalation context: %s", decision.Detail)
		}
	})
	t.Run("unknown provider fails fast", func(t *testing.T) {
		t.Parallel()
		if _, err := decideRoute("nowhere", "gemini", 4096, &promptInput{source: "input"}); err == nil || !strings.Contains(err.Error(), "not a known provider") {
			t.Fatalf("decideRoute error = %v, want unknown provider failure", err)
		}
	})
}

func TestPreflightContextWindow(t *testing.T) {
	t.Parallel()
	t.Run("disabled window skips gate", func(t *testing.T) {
		t.Parallel()
		if err := preflightContextWindow(0, 4096, "system", &promptInput{source: "input"}, "groq"); err != nil {
			t.Fatalf("preflightContextWindow error = %v, want skip", err)
		}
	})
	t.Run("roomy window passes", func(t *testing.T) {
		t.Parallel()
		if err := preflightContextWindow(1_000_000, 4096, "system", &promptInput{source: "input"}, "groq"); err != nil {
			t.Fatalf("preflightContextWindow error = %v, want pass", err)
		}
	})
	t.Run("overflow fails fast with guidance", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("word ", 4000)
		err := preflightContextWindow(512, 128, "system", &promptInput{source: long}, "omlx")
		if err == nil {
			t.Fatal("preflightContextWindow returned nil, want overflow error")
		}
		for _, want := range []string{"context window exceeded", "max output tokens", "omlx context_window"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error missing %q: %v", want, err)
			}
		}
	})
}
