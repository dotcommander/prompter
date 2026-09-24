package usage_test

import (
	"os"
	"strings"
	"testing"
)

// TestFlagsMarkdownExists verifies the docs flags file exists
func TestFlagsMarkdownExists(t *testing.T) {
	t.Parallel()
	path := "../docs/flags.md"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("flags.md does not exist at %s", path)
	}
}

// TestFlagsMarkdownContainsRequiredFlags verifies all required flags are documented
func TestFlagsMarkdownContainsRequiredFlags(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/flags.md")
	if err != nil {
		t.Fatalf("failed to read flags.md: %v", err)
	}

	text := string(content)
	requiredFlags := []string{"-p", "-m", "--base-url", "-v", "-s", "--style", "--stream"}

	for _, flag := range requiredFlags {
		if !strings.Contains(text, flag) {
			t.Errorf("flags.md is missing required flag: %s", flag)
		}
	}
}

// TestFlagsMarkdownStructure verifies each flag has required components
func TestFlagsMarkdownStructure(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/flags.md")
	if err != nil {
		t.Fatalf("failed to read flags.md: %v", err)
	}

	text := string(content)

	// Check for table headers indicating structure
	hasFlagHeader := strings.Contains(text, "| Flag |")
	hasAliasHeader := strings.Contains(text, "| Alias |")
	hasDescHeader := strings.Contains(text, "| Description |")
	hasDefaultHeader := strings.Contains(text, "| Default |")

	if !hasFlagHeader {
		t.Error("flags.md missing Flag column header")
	}
	if !hasAliasHeader {
		t.Error("flags.md missing Alias column header")
	}
	if !hasDescHeader {
		t.Error("flags.md missing Description column header")
	}
	if !hasDefaultHeader {
		t.Error("flags.md missing Default column header")
	}
}

// TestFlagsMarkdownHasExamples verifies usage examples are present
func TestFlagsMarkdownHasExamples(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/flags.md")
	if err != nil {
		t.Fatalf("failed to read flags.md: %v", err)
	}

	text := string(content)

	// Check for example code blocks
	if !strings.Contains(text, "```") {
		t.Error("flags.md missing code examples")
	}

	// Check for specific example patterns
	examplePatterns := []string{
		"prompter refine -p",
		"prompter refine -m",
		"prompter refine --base-url",
		"prompter refine -v",
		"prompter refine -s",
		"prompter refine --style",
		"prompter refine --stream",
	}

	for _, pattern := range examplePatterns {
		if !strings.Contains(text, pattern) {
			t.Errorf("flags.md missing example for: %s", pattern)
		}
	}
}

// TestFlagsMarkdownDefaultsMarked verifies default values are clearly indicated
func TestFlagsMarkdownDefaultsMarked(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/flags.md")
	if err != nil {
		t.Fatalf("failed to read flags.md: %v", err)
	}

	text := string(content)

	// Check for default values being marked
	// Look for "default" keyword and code/backtick formatting
	hasDefaultSection := strings.Contains(strings.ToLower(text), "default")
	if !hasDefaultSection {
		t.Error("flags.md does not clearly mark default values")
	}

	// Check that gemini is mentioned as default
	if !strings.Contains(text, "gemini") {
		t.Error("flags.md missing gemini default provider")
	}
}

// TestFlagsMarkdownProviderAlias verifies --provider alias for -p
func TestFlagsMarkdownProviderAlias(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/flags.md")
	if err != nil {
		t.Fatalf("failed to read flags.md: %v", err)
	}

	text := string(content)

	// Check for -p and --provider relationship
	if !strings.Contains(text, "-p") {
		t.Error("flags.md missing -p flag")
	}
	if !strings.Contains(text, "--provider") {
		t.Error("flags.md missing --provider alias")
	}
}

// TestFlagsMarkdownVerboseAlias verifies --verbose alias for -v
func TestFlagsMarkdownVerboseAlias(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/flags.md")
	if err != nil {
		t.Fatalf("failed to read flags.md: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "-v") {
		t.Error("flags.md missing -v flag")
	}
	if !strings.Contains(text, "--verbose") {
		t.Error("flags.md missing --verbose alias")
	}
}

// TestFlagsMarkdownStyleAlias verifies --style alias for -s
func TestFlagsMarkdownStyleAlias(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/flags.md")
	if err != nil {
		t.Fatalf("failed to read flags.md: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "-s") {
		t.Error("flags.md missing -s flag")
	}
	if !strings.Contains(text, "--style") {
		t.Error("flags.md missing --style alias")
	}
}

// TestFlagsMarkdownStreamFlag verifies --stream flag is documented
func TestFlagsMarkdownStreamFlag(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/flags.md")
	if err != nil {
		t.Fatalf("failed to read flags.md: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "--stream") {
		t.Error("flags.md missing --stream flag")
	}
}

// TestFlagsMarkdownConfigurationSection verifies config section exists
func TestFlagsMarkdownConfigurationSection(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("../docs/flags.md")
	if err != nil {
		t.Fatalf("failed to read flags.md: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "Configuration") {
		t.Error("flags.md missing Configuration section")
	}
	if !strings.Contains(text, "config.json") {
		t.Error("flags.md missing config.json reference")
	}
}
