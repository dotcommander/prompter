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

func TestParseArgs_InitCommandRemoved(t *testing.T) {
	t.Parallel()
	if _, err := parseArgs([]string{"init"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("parseArgs(init) error = %v, want unknown command", err)
	}
}

func TestRunInitForceRefusesSymlinkDestinations(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.md")
	original := []byte("original contents\n")
	if err := os.WriteFile(victim, original, 0o644); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	if err := os.Symlink(victim, filepath.Join(tmpDir, "refactor.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	cfg := &config.Config{PromptsDir: tmpDir}

	t.Run("force refuses before any write", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		err := runInit(&stdout, &stderr, cfg, tmpDir, true)
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected symlink refusal, got: %v", err)
		}
		assertVictimIntact(t, victim, original)
	})

	t.Run("without force symlinks are skipped not followed", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		if err := runInit(&stdout, &stderr, cfg, tmpDir, false); err != nil {
			t.Fatalf("runInit: %v", err)
		}
		if !strings.Contains(stdout.String(), "refactor.md") {
			t.Errorf("expected refactor.md reported skipped: %s", stdout.String())
		}
		assertVictimIntact(t, victim, original)
	})
}

func assertVictimIntact(t *testing.T, victim string, original []byte) {
	t.Helper()
	data, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if string(data) != string(original) {
		t.Errorf("file outside the vault was modified: %q", data)
	}
}
