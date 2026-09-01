package main

import (
	"bytes"
	"errors"
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
	if err := runInit(&stdout, &stderr, cfg, tmpDir); err != nil {
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

func TestRunInit_IdempotentPreservesExistingFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		PromptsDir: tmpDir,
	}

	var stdout, stderr bytes.Buffer
	// First run: creates files
	if err := runInit(&stdout, &stderr, cfg, tmpDir); err != nil {
		t.Fatalf("runInit initial: %v", err)
	}

	// Modify one file
	testFile := filepath.Join(tmpDir, "refactor.md")
	customContent := []byte("custom modified content")
	if err := os.WriteFile(testFile, customContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Second run should skip existing files.
	stdout.Reset()
	if err := runInit(&stdout, &stderr, cfg, tmpDir); err != nil {
		t.Fatalf("runInit second: %v", err)
	}
	if !strings.Contains(stdout.String(), "Skipped existing") {
		t.Errorf("stdout should report skipped files, got: %s", stdout.String())
	}
	gotContent, _ := os.ReadFile(testFile)
	if string(gotContent) != string(customContent) {
		t.Errorf("refactor.md was unexpectedly overwritten")
	}
}

func TestParseArgs_InitCommandRemoved(t *testing.T) {
	t.Parallel()
	if _, err := parseArgs([]string{"init"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("parseArgs(init) error = %v, want unknown command", err)
	}
}

func TestRunInitSkipsSymlinkDestinations(t *testing.T) {
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

	var stdout, stderr bytes.Buffer
	if err := runInit(&stdout, &stderr, cfg, tmpDir); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if !strings.Contains(stdout.String(), "refactor.md") {
		t.Errorf("expected refactor.md reported skipped: %s", stdout.String())
	}
	assertVictimIntact(t, victim, original)
}

func TestRunInitPreservesDestinationCreatedAtWriteBoundary(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{PromptsDir: tmpDir}
	target := filepath.Join(tmpDir, "refactor.md")
	concurrentContent := []byte("created by another process\n")
	created := false
	openFile := func(path string, flag int, perm os.FileMode) (*os.File, error) {
		if path == target && !created {
			created = true
			if err := os.WriteFile(path, concurrentContent, 0o644); err != nil {
				return nil, err
			}
		}
		return os.OpenFile(path, flag, perm)
	}

	var stdout, stderr bytes.Buffer
	if err := runInitWithOpen(&stdout, &stderr, cfg, tmpDir, openFile); err != nil {
		t.Fatalf("runInitWithOpen: %v", err)
	}
	if !strings.Contains(stdout.String(), "refactor.md") {
		t.Fatalf("boundary-created destination was not reported skipped: %s", stdout.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(concurrentContent) {
		t.Fatalf("boundary-created destination = %q, want %q", got, concurrentContent)
	}
}

func TestRunInitLeavesMarkerForResumablePartialFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{PromptsDir: tmpDir}
	failName := "critique.md"
	openFile := func(path string, flag int, perm os.FileMode) (*os.File, error) {
		if filepath.Base(path) == failName {
			return nil, errors.New("injected write failure")
		}
		return os.OpenFile(path, flag, perm)
	}

	var stdout, stderr bytes.Buffer
	if err := runInitWithOpen(&stdout, &stderr, cfg, tmpDir, openFile); err == nil {
		t.Fatal("runInitWithOpen unexpectedly succeeded")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, promptInitMarker)); err != nil {
		t.Fatalf("initialization marker missing after partial failure: %v", err)
	}
	if err := runInit(&stdout, &stderr, cfg, tmpDir); err != nil {
		t.Fatalf("resume runInit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, promptInitMarker)); !os.IsNotExist(err) {
		t.Fatalf("initialization marker remains after successful resume: %v", err)
	}
}

func TestRunInitRejectsSymlinkMarker(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	original := []byte("leave this unchanged\n")
	if err := os.WriteFile(victim, original, 0o644); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(tmpDir, promptInitMarker)
	if err := os.Symlink(victim, markerPath); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runInit(&stdout, &stderr, &config.Config{PromptsDir: tmpDir}, tmpDir)
	if err == nil || !strings.Contains(err.Error(), "marker is not a regular file") {
		t.Fatalf("runInit error = %v, want non-regular marker rejection", err)
	}
	assertVictimIntact(t, victim, original)
	if info, err := os.Lstat(markerPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("marker symlink changed: info=%v err=%v", info, err)
	}
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
