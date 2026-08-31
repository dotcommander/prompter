package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotcommander/prompter/internal/config"
)

func TestParseArgsRun(t *testing.T) {
	t.Parallel()
	f, err := parseArgs([]string{"run", "grai-transform", "source text", "--provider", "openai"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if f.command != commandRun || f.promptName != "grai-transform" {
		t.Fatalf("command = %q, prompt = %q", f.command, f.promptName)
	}
	if strings.Join(f.args, " ") != "source text" {
		t.Fatalf("args = %v", f.args)
	}
	if f.provider != "openai" {
		t.Fatalf("provider = %q", f.provider)
	}
}

func TestParseArgsRunRequiresSelector(t *testing.T) {
	t.Parallel()
	if _, err := parseArgs([]string{"run"}); err == nil || !strings.Contains(err.Error(), "prompt name or alias") {
		t.Fatalf("parseArgs error = %v", err)
	}
}

func TestFindPromptEntry(t *testing.T) {
	t.Parallel()
	entries := []PromptEntry{
		{Name: "exact", Path: "/one.md", Aliases: []string{"shared"}},
		{Name: "other", Path: "/two.md", Aliases: []string{"exact", "shared"}},
	}

	entry, err := findPromptEntry(entries, "EXACT")
	if err != nil {
		t.Fatalf("findPromptEntry exact name: %v", err)
	}
	if entry.Path != "/one.md" {
		t.Fatalf("exact name path = %q", entry.Path)
	}

	_, err = findPromptEntry(entries, "shared")
	if err == nil || !strings.Contains(err.Error(), "/one.md, /two.md") {
		t.Fatalf("ambiguous alias error = %v", err)
	}

	_, err = findPromptEntry(entries, "missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing prompt error = %v", err)
	}
}

func TestResolveCommandSystemPromptRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "deep-time.md")
	writeRunTestFile(t, path, "---\ndescription: test\naliases:\n  - grai-transform\n---\n\n# Prompt body\n")
	cfg := &config.Config{PromptsDir: dir, PromptsDirs: []string{dir}}
	f := &flags{command: commandRun, promptName: "grai-transform"}

	if err := resolveCommandSystemPrompt(f, cfg); err != nil {
		t.Fatalf("resolveCommandSystemPrompt: %v", err)
	}
	if cfg.SystemPrompt != "# Prompt body" {
		t.Fatalf("system prompt = %q", cfg.SystemPrompt)
	}
	if f.outputValidation != nil {
		t.Fatalf("output validation = %+v, want nil", f.outputValidation)
	}
}

func TestResolveCommandSystemPromptRunRejectsStyle(t *testing.T) {
	t.Parallel()
	err := resolveCommandSystemPrompt(&flags{command: commandRun, promptName: "x", styleSet: true}, &config.Config{})
	if err == nil || !strings.Contains(err.Error(), "does not accept") {
		t.Fatalf("resolveCommandSystemPrompt error = %v", err)
	}
}

func TestResolveCommandSystemPromptRunRejectsMalformedValidation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid-validation.md")
	writeRunTestFile(t, path, "---\nvalidation:\n  semantic_validation: \"true\"\n---\nBody\n")
	cfg := &config.Config{PromptsDir: dir, PromptsDirs: []string{dir}}
	f := &flags{command: commandRun, promptName: "invalid-validation"}

	err := resolveCommandSystemPrompt(f, cfg)
	if err == nil || !strings.Contains(err.Error(), "semantic_validation must be a boolean") {
		t.Fatalf("resolveCommandSystemPrompt error = %v, want invalid validation type", err)
	}
}

func writeRunTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
