package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/dotcommander/prompter/internal/provider"
)

type validationTestProvider struct {
	responses []string
	requests  []provider.CallRequest
}

func (p *validationTestProvider) Name() string   { return "test" }
func (p *validationTestProvider) Model() string  { return "test-model" }
func (p *validationTestProvider) APIKey() string { return "test-key" }
func (p *validationTestProvider) StreamCall(context.Context, provider.CallRequest, io.Writer) error {
	return nil
}
func (p *validationTestProvider) Call(_ context.Context, req provider.CallRequest) (string, error) {
	p.requests = append(p.requests, req)
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func TestOutputValidationFromFrontmatter(t *testing.T) {
	t.Parallel()
	content := []byte("---\naliases: [test]\nvalidation:\n  control_fence: deep-time\n  min_word_ratio: 0.8\n  max_word_ratio: 2.0\n  longer_min_word_ratio: 1.5\n  longer_max_word_ratio: 3.0\n  short_input_words: 25\n  max_short_sentences: 2\n  require_terminal_punctuation: true\n  semantic_validation: true\n  retries: 1\n---\nBody")
	fm, _, err := parseFrontmatter(content)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if fm.Validation == nil {
		t.Fatal("validation is nil")
	}
	if fm.Validation.ControlFence != "deep-time" || fm.Validation.MaxWordRatio != 2 || !fm.Validation.SemanticValidation || fm.Validation.Retries != 1 {
		t.Fatalf("validation = %+v", fm.Validation)
	}
}

func TestOutputValidationRejectsMalformedMetadataTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"ratio string", "max_word_ratio: \"2.0\"", "max_word_ratio must be a number"},
		{"boolean string", "semantic_validation: \"true\"", "semantic_validation must be a boolean"},
		{"fractional retry", "retries: 0.5", "retries must be an integer"},
		{"validation scalar", "validation: enabled", "validation must be a mapping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			validationBlock := "validation:\n  " + tt.content
			if strings.HasPrefix(tt.content, "validation:") {
				validationBlock = tt.content
			}
			fm, _, err := parseFrontmatter([]byte("---\n" + validationBlock + "\n---\nBody"))
			if err != nil {
				t.Fatalf("parseFrontmatter: %v", err)
			}
			if fm.Validation == nil {
				t.Fatal("validation is nil")
			}
			if err := fm.Validation.validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validate error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCallWithSemanticValidationPasses(t *testing.T) {
	t.Parallel()
	prov := &validationTestProvider{responses: []string{"The door moved.", "PASS"}}
	validation := &OutputValidation{SemanticValidation: true, RequireTerminalPunctuation: true, Retries: 1}

	result, retried, err := callWithOutputValidation(context.Background(), prov, provider.CallRequest{}, "The door moved.", validation)
	if err != nil {
		t.Fatalf("callWithOutputValidation: %v", err)
	}
	if retried || result != "The door moved." || len(prov.requests) != 2 {
		t.Fatalf("result = %q, retried = %t, requests = %d", result, retried, len(prov.requests))
	}
}

func TestCallWithSemanticValidationRetriesAndRevalidates(t *testing.T) {
	t.Parallel()
	prov := &validationTestProvider{responses: []string{
		"The door swung inward.",
		"FAIL: candidate invents an inward direction",
		"The door moved.",
		"PASS",
	}}
	validation := &OutputValidation{SemanticValidation: true, RequireTerminalPunctuation: true, Retries: 1}

	result, retried, err := callWithOutputValidation(context.Background(), prov, provider.CallRequest{SystemPrompt: "base"}, "The door moved.", validation)
	if err != nil {
		t.Fatalf("callWithOutputValidation: %v", err)
	}
	if !retried || result != "The door moved." || len(prov.requests) != 4 {
		t.Fatalf("result = %q, retried = %t, requests = %d", result, retried, len(prov.requests))
	}
	if !strings.Contains(prov.requests[2].SystemPrompt, "inward direction") {
		t.Fatalf("retry system prompt = %q", prov.requests[2].SystemPrompt)
	}
}

func TestValidateOutput(t *testing.T) {
	t.Parallel()
	validation := OutputValidation{ControlFence: "deep-time", MinWordRatio: 0.8, MaxWordRatio: 2, LongerMinWordRatio: 1.5, LongerMaxWordRatio: 3, ShortInputWords: 25, MaxShortSentences: 2, RequireTerminalPunctuation: true}
	input := "```deep-time\nlength: preserve\n```\nDrought caused the migration."

	if violations := validateOutput(validation, input, "Drought may have caused migration."); len(violations) != 0 {
		t.Fatalf("valid output violations = %v", violations)
	}
	violations := validateOutput(validation, input, "Drought is consistent with the migration, though direct causation remains unproved on the available record")
	joined := strings.Join(violations, "\n")
	for _, want := range []string{"maximum is 8", "terminal punctuation"} {
		if !strings.Contains(joined, want) {
			t.Errorf("violations = %v, missing %q", violations, want)
		}
	}
}

func TestCallWithOutputValidationRetriesOnce(t *testing.T) {
	t.Parallel()
	prov := &validationTestProvider{responses: []string{"Drought is consistent with migration without enough evidence", "Drought may have caused migration."}}
	validation := &OutputValidation{ControlFence: "deep-time", MinWordRatio: 0.8, MaxWordRatio: 2, ShortInputWords: 25, MaxShortSentences: 2, RequireTerminalPunctuation: true, Retries: 1}
	req := provider.CallRequest{SystemPrompt: "base", UserPrompt: "source"}
	input := "```deep-time\nlength: preserve\n```\nDrought caused the migration."

	result, retried, err := callWithOutputValidation(context.Background(), prov, req, input, validation)
	if err != nil {
		t.Fatalf("callWithOutputValidation: %v", err)
	}
	if !retried || result != "Drought may have caused migration." || len(prov.requests) != 2 {
		t.Fatalf("result = %q, retried = %t, requests = %d", result, retried, len(prov.requests))
	}
	if !strings.Contains(prov.requests[1].SystemPrompt, "Runtime correction") {
		t.Fatalf("retry system prompt = %q", prov.requests[1].SystemPrompt)
	}
}

func TestCallWithOutputValidationRejectsFailedRetry(t *testing.T) {
	t.Parallel()
	prov := &validationTestProvider{responses: []string{"unfinished", "still unfinished"}}
	validation := &OutputValidation{RequireTerminalPunctuation: true, Retries: 1}
	_, retried, err := callWithOutputValidation(context.Background(), prov, provider.CallRequest{}, "source text", validation)
	if !retried || err == nil || !strings.Contains(err.Error(), "after corrective retry") {
		t.Fatalf("retried = %t, error = %v", retried, err)
	}
}
