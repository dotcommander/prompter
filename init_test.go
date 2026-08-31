package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotcommander/prompter/internal/config"
)

func TestRunInit_CreatesStarterPrompts(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		PromptsDir: tmpDir,
	}

	var stdout, stderr bytes.Buffer
	if err := runInit(&stdout, &stderr, cfg, tmpDir, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}

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
		filePath := filepath.Join(tmpDir, name)
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("expected starter prompt %s to exist: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("starter prompt %s is empty", name)
		}
		if !strings.Contains(string(data), "---") {
			t.Errorf("starter prompt %s missing frontmatter", name)
		}
	}

	if !strings.Contains(stdout.String(), "Prompt vault initialized") {
		t.Errorf("stdout missing confirmation, got: %s", stdout.String())
	}
}

func TestRunInit_IdempotentAndForce(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		PromptsDir: tmpDir,
	}

	var stdout, stderr bytes.Buffer
	// First run: creates files
	if err := runInit(&stdout, &stderr, cfg, tmpDir, false); err != nil {
		t.Fatalf("runInit initial: %v", err)
	}

	// Modify one file
	testFile := filepath.Join(tmpDir, "refactor.md")
	customContent := []byte("custom modified content")
	if err := os.WriteFile(testFile, customContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Second run without force: should skip existing
	stdout.Reset()
	if err := runInit(&stdout, &stderr, cfg, tmpDir, false); err != nil {
		t.Fatalf("runInit second: %v", err)
	}
	if !strings.Contains(stdout.String(), "Skipped existing") {
		t.Errorf("stdout should report skipped files, got: %s", stdout.String())
	}
	gotContent, _ := os.ReadFile(testFile)
	if string(gotContent) != string(customContent) {
		t.Errorf("refactor.md was unexpectedly overwritten without --force")
	}

	// Third run with force: should overwrite
	stdout.Reset()
	if err := runInit(&stdout, &stderr, cfg, tmpDir, true); err != nil {
		t.Fatalf("runInit with force: %v", err)
	}
	gotContent, _ = os.ReadFile(testFile)
	if string(gotContent) == string(customContent) {
		t.Errorf("refactor.md should have been overwritten with --force")
	}
}

func TestParseArgs_InitCommand(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{"init", "--force", "/tmp/custom-vault"})
	if err != nil {
		t.Fatalf("parseArgs(init) error: %v", err)
	}
	if f.command != "init" {
		t.Errorf("command = %q, want %q", f.command, "init")
	}
	if !f.force {
		t.Errorf("force = false, want true")
	}
	if len(f.args) != 1 || f.args[0] != "/tmp/custom-vault" {
		t.Errorf("args = %v, want [/tmp/custom-vault]", f.args)
	}
}
