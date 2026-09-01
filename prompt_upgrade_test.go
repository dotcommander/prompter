package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectPromptVaultClassifiesCurrentLegacyCustomAndMissing(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	currentCritique, err := starterFS.ReadFile("prompts/starter/critique.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "critique.md"), currentCritique, 0o644); err != nil {
		t.Fatal(err)
	}
	legacyEnhance := []byte("known legacy enhance\n")
	if err := os.WriteFile(filepath.Join(vault, "enhance.md"), legacyEnhance, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "refactor.md"), []byte("user custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	legacy := make(promptChecksumCatalog)
	legacy.add("enhance.md", checksum(legacyEnhance))
	items, err := inspectPromptVaultWithChecksums(vault, legacy)
	if err != nil {
		t.Fatal(err)
	}
	states := promptStatesByName(items)
	if states["critique.md"] != promptCurrent {
		t.Errorf("critique.md = %q, want current", states["critique.md"])
	}
	if states["enhance.md"] != promptUpgradable {
		t.Errorf("enhance.md = %q, want upgrade", states["enhance.md"])
	}
	if states["refactor.md"] != promptCustom {
		t.Errorf("refactor.md = %q, want custom", states["refactor.md"])
	}
	if states["unit-test.md"] != promptMissing {
		t.Errorf("unit-test.md = %q, want missing", states["unit-test.md"])
	}
}

func TestApplyPromptUpgradeStagesKnownStockWithoutReplacing(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	path := filepath.Join(vault, "enhance.md")
	legacy := []byte("legacy\n")
	current := []byte("current\n")
	if err := os.WriteFile(path, legacy, 0o640); err != nil {
		t.Fatal(err)
	}
	item := promptUpgrade{name: "enhance.md", state: promptUpgradable, current: current}
	if err := applyPromptUpgrade(vault, &item); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, legacy) {
		t.Fatalf("enhance.md = %q, want original %q", got, legacy)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
	candidate, err := os.ReadFile(promptCandidatePath(path, current))
	if err != nil {
		t.Fatalf("read prompt candidate: %v", err)
	}
	if !bytes.Equal(candidate, current) {
		t.Fatalf("prompt candidate = %q, want %q", candidate, current)
	}
}

func TestApplyPromptUpgradePreservesChangedDestination(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	path := filepath.Join(vault, "enhance.md")
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	item := promptUpgrade{name: "enhance.md", state: promptUpgradable, current: []byte("current\n")}
	if err := applyPromptUpgrade(vault, &item); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "changed\n" {
		t.Fatalf("changed destination overwritten: %q", got)
	}
	candidate, err := os.ReadFile(promptCandidatePath(path, item.current))
	if err != nil {
		t.Fatal(err)
	}
	if string(candidate) != "current\n" {
		t.Fatalf("candidate = %q, want current", candidate)
	}
}

func TestPromptCandidatePathsSupportSequentialVersions(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	path := filepath.Join(vault, "enhance.md")
	if err := os.WriteFile(path, []byte("legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, current := range [][]byte{[]byte("version two\n"), []byte("version three\n")} {
		item := promptUpgrade{name: "enhance.md", state: promptUpgradable, current: current}
		if err := applyPromptUpgrade(vault, &item); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(promptCandidatePath(path, current)); err != nil {
			t.Fatalf("versioned candidate missing: %v", err)
		}
	}
}

func TestPromptUpgradePreservesCustomAndIsIdempotent(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	customPath := filepath.Join(vault, "refactor.md")
	custom := []byte("my custom prompt\n")
	if err := os.WriteFile(customPath, custom, 0o644); err != nil {
		t.Fatal(err)
	}

	var dryRun bytes.Buffer
	if err := runPromptMaintenance(&dryRun, vault, "upgrade", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(vault, "critique.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created critique.md: %v", err)
	}

	var first bytes.Buffer
	if err := runPromptMaintenance(&first, vault, "upgrade", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.String(), "custom") || !strings.Contains(first.String(), "refactor.md.new.") {
		t.Fatalf("upgrade output missing custom candidate:\n%s", first.String())
	}
	gotCustom, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotCustom, custom) {
		t.Fatalf("custom prompt changed: %q", gotCustom)
	}
	if _, err := os.Stat(promptCandidatePath(customPath, mustStarterPrompt(t, "refactor.md"))); err != nil {
		t.Fatalf("new prompt candidate missing: %v", err)
	}

	var second bytes.Buffer
	if err := runPromptMaintenance(&second, vault, "upgrade", false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(second.String(), "install") || strings.Contains(second.String(), "upgrade ") {
		t.Fatalf("second pass was not idempotent:\n%s", second.String())
	}
}

func TestPromptMaintenanceRejectsConflictingCandidate(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	current := mustStarterPrompt(t, "refactor.md")
	path := filepath.Join(vault, "refactor.md")
	if err := os.WriteFile(path, []byte("custom prompt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	candidate := promptCandidatePath(path, current)
	if err := os.WriteFile(candidate, []byte("conflicting candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err := runPromptMaintenance(&output, vault, "status", false)
	if err == nil || !strings.Contains(err.Error(), "candidate contains different content") {
		t.Fatalf("runPromptMaintenance error = %v, want conflicting candidate rejection", err)
	}
}

func mustStarterPrompt(t *testing.T, name string) []byte {
	t.Helper()
	data, err := starterFS.ReadFile("prompts/starter/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReleasedPromptChecksumCatalog(t *testing.T) {
	t.Parallel()

	checksums, err := loadReleasedPromptChecksums()
	if err != nil {
		t.Fatal(err)
	}
	if len(checksums) != 8 {
		t.Fatalf("checksum count = %d, want 8", len(checksums))
	}
	critique := checksums["critique.md"]
	for _, want := range []string{
		"a3324f4846cbf6bed891cfc928dc722b239c1097f6ecd7ae28a27138b20ca4f8",
		"b504a8190fe1130f447385ac783c2dadeac378b560b6b287c02d9807e26d966a",
	} {
		if _, ok := critique[want]; !ok {
			t.Errorf("critique checksum catalog missing %s", want)
		}
	}
}

func promptStatesByName(items []promptUpgrade) map[string]promptState {
	states := make(map[string]promptState, len(items))
	for _, item := range items {
		states[item.name] = item.state
	}
	return states
}
