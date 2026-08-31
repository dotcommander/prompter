package usage_test

import (
	"os"
	"strings"
	"testing"
)

// TestFinderTrigger explains what triggers the finder (no input)
func TestFinderTrigger(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/finder.md")
	if err != nil {
		t.Fatalf("Failed to read finder.md: %v", err)
	}

	text := string(content)

	// Check that "no input" is documented as the trigger
	checks := []struct {
		name        string
		mustContain []string
	}{
		{
			name:        "trigger section exists",
			mustContain: []string{"## Triggering the Finder"},
		},
		{
			name:        "no input trigger documented",
			mustContain: []string{"without any input", "no input", "prompter"},
		},
		{
			name:        "empty pipe documented",
			mustContain: []string{"empty pipe", "empty input"},
		},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			foundAny := false
			for _, needle := range tc.mustContain {
				if strings.Contains(text, needle) {
					foundAny = true
					break
				}
			}
			if !foundAny {
				t.Errorf("Expected to find at least one of: %v", tc.mustContain)
			}
		})
	}
}

// TestFinderSearchWeighting explains search weighting (name > aliases > path > description > body)
func TestFinderSearchWeighting(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/finder.md")
	if err != nil {
		t.Fatalf("Failed to read finder.md: %v", err)
	}

	text := string(content)

	// Check search weighting documentation
	tests := []struct {
		name        string
		mustContain []string
		weights     []string
	}{
		{
			name:        "search section exists",
			mustContain: []string{"## How Search Works"},
		},
		{
			name:        "weighted search mentioned",
			mustContain: []string{"weighted", "weight", "rank"},
		},
		{
			name:    "all fields documented with weights",
			weights: []string{"Name", "Aliases", "Path", "Description", "Body"},
		},
		{
			name:        "weight values documented",
			mustContain: []string{"1000", "500", "300", "100", "10"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if len(tc.mustContain) > 0 {
				for _, needle := range tc.mustContain {
					if !strings.Contains(text, needle) {
						t.Errorf("Expected to find: %s", needle)
					}
				}
			}
			if len(tc.weights) > 0 {
				for _, field := range tc.weights {
					if !strings.Contains(text, field) {
						t.Errorf("Expected field documentation for: %s", field)
					}
				}
			}
		})
	}

	// Verify order: name (1000) > aliases (500) > path (300) > description (100) > body (10)
	// Use more specific patterns to avoid matching 10 inside 100
	nameIdx := strings.Index(text, "| 1000 |")
	aliasIdx := strings.Index(text, "| 500 |")
	pathIdx := strings.Index(text, "| 300 |")
	descIdx := strings.Index(text, "| 100 |")
	bodyIdx := strings.Index(text, "| 10 |")

	if nameIdx < 0 || aliasIdx < 0 || pathIdx < 0 || descIdx < 0 || bodyIdx < 0 {
		t.Fatal("Not all weight values found in documentation")
	}

	// Verify descending order of documentation (name first, then aliases, etc.)
	if !(nameIdx < aliasIdx && aliasIdx < pathIdx && pathIdx < descIdx && descIdx < bodyIdx) {
		t.Error("Weights should be documented in descending order: 1000 > 500 > 300 > 100 > 10")
	}
}

// TestFinderClipboardOutput explains clipboard behavior (pbcopy)
func TestFinderClipboardOutput(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/finder.md")
	if err != nil {
		t.Fatalf("Failed to read finder.md: %v", err)
	}

	text := string(content)

	// Check clipboard documentation
	tests := []struct {
		name        string
		mustContain []string
	}{
		{
			name:        "output section exists",
			mustContain: []string{"## Output"},
		},
		{
			name:        "clipboard mentioned",
			mustContain: []string{"Clipboard", "clipboard", "pbcopy"},
		},
		{
			name:        "stdout also documented",
			mustContain: []string{"Stdout", "stdout", "printed to stdout"},
		},
		{
			name:        "platform-specific clipboard tools",
			mustContain: []string{"macOS", "Linux", "Windows", "xclip", "xsel"},
		},
		{
			name:        "selection action documented",
			mustContain: []string{"Enter", "select", "copy to clipboard"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			foundAny := false
			for _, needle := range tc.mustContain {
				if strings.Contains(text, needle) {
					foundAny = true
					break
				}
			}
			if !foundAny {
				t.Errorf("Expected to find at least one of: %v", tc.mustContain)
			}
		})
	}
}

// TestFinderFileExists verifies the documentation file exists at the correct path
func TestFinderFileExists(t *testing.T) {
	t.Parallel()
	path := "../docs/finder.md"
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("finder.md does not exist at %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatal("finder.md is a directory, not a file")
	}
}

// TestFinderHasValidMarkdown verifies basic markdown structure
func TestFinderHasValidMarkdown(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/finder.md")
	if err != nil {
		t.Fatalf("Failed to read finder.md: %v", err)
	}

	text := string(content)

	// Must have a title
	if !strings.Contains(text, "# ") {
		t.Error("Documentation must have a title (starting with #)")
	}

	// Must have some sections
	if !strings.Contains(text, "## ") {
		t.Error("Documentation should have sections (starting with ##)")
	}

	// Must have code examples
	if !strings.Contains(text, "```") {
		t.Error("Documentation should have code examples (fenced code blocks)")
	}
}
