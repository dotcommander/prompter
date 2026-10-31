package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/prompter/internal/config"
	"github.com/dotcommander/prompter/internal/provider"
)

func TestPromptInputReusesEnvelopeAndEstimate(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"", "refine this", "语言模型", strings.Repeat("word ", 1024)} {
		t.Run(fmt.Sprintf("bytes=%d", len(source)), func(t *testing.T) {
			t.Parallel()
			input := &promptInput{source: source}
			if input.bounded != "" || input.tokenEstimate != 0 {
				t.Fatal("input should be prepared lazily")
			}
			want := boundPromptInput(source)
			if got := input.bound(); got != want {
				t.Fatal("prepared envelope differs from the boundary protocol")
			}
			wantTokens := estimateTokens(want)
			if got := input.estimatedTokens(); got != wantTokens || input.tokenEstimate != wantTokens {
				t.Fatalf("estimate = %d, cached = %d, want %d", got, input.tokenEstimate, wantTokens)
			}
			if input.bound() != want || input.estimatedTokens() != wantTokens {
				t.Fatal("repeated access changed the prepared input")
			}
		})
	}
}

func TestPromptInputSharedByRoutingPreflightAndDryRun(t *testing.T) {
	t.Parallel()
	input := &promptInput{source: "refine this"}
	wantTokens := estimateTokens(boundPromptInput(input.source))
	decision, err := decideRoute("omlx", "gemini", wantTokens, input)
	if err != nil || decision.Provider != "omlx" {
		t.Fatalf("route at exact budget = %+v, %v", decision, err)
	}
	window := estimateTokens("system") + wantTokens + 100
	if err := preflightContextWindow(window, 100, "system", input, "omlx"); err != nil {
		t.Fatalf("preflight at exact window: %v", err)
	}
	if err := preflightContextWindow(window-1, 100, "system", input, "omlx"); err == nil {
		t.Fatal("preflight accepted an overflowing window")
	}
	cfg := &config.Config{
		SystemPrompt: "system", MaxOutputTokens: 100,
		Providers: map[string]config.ProviderConfig{"omlx": {ContextWindow: window}},
	}
	prov := provider.NewChat("omlx", "local", "model", "http://localhost", 0)
	var out strings.Builder
	printDryRun(&out, prov, "model", &flags{command: commandRefine}, cfg, input, time.Minute, decision.Detail)
	for _, want := range []string{
		fmt.Sprintf("Input bytes: %d\n", len(input.source)),
		fmt.Sprintf("Input tokens (est): %d\n", wantTokens),
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry run missing %q: %s", want, out.String())
		}
	}
	if input.bounded != boundPromptInput(input.source) || input.tokenEstimate != wantTokens {
		t.Fatal("routing, preflight and dry run changed the prepared input")
	}
}

func TestPromptInputDisabledPreflightDoesNotPrepareInput(t *testing.T) {
	t.Parallel()
	input := &promptInput{source: "refine this"}
	if err := preflightContextWindow(0, 100, "system", input, "omlx"); err != nil {
		t.Fatal(err)
	}
	if input.bounded != "" || input.tokenEstimate != 0 {
		t.Fatal("disabled preflight prepared unused input")
	}
}
