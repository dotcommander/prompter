package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

//go:embed prompts/enhance.md
var defaultEnhancePrompt string

//go:embed prompts/critique.md
var defaultCritiquePrompt string

//go:embed prompts/rewrite.md
var defaultRewritePrompt string

//go:embed prompts/components.json
var defaultComponentsJSON string

//go:embed prompts/styles
var stylesFS embed.FS

//go:embed prompts/starter
var starterFS embed.FS

// resolveStyle returns the system prompt for the given style name.
// Resolution order:
//  1. ~/.config/prompter/styles/<name>.md (user override)
//  2. Embedded prompts/styles/<name>.md
//  3. Error if neither exists
//
// The special name "default" maps to the main enhance.md prompt.
func resolveStyle(name string) (string, error) {
	if name == "" || name == "default" {
		return defaultEnhancePrompt, nil
	}

	userStylesDir := ""
	home, err := os.UserHomeDir()
	if err == nil {
		userStylesDir = filepath.Join(home, ".config", "prompter", "styles")
	}
	return resolveStyleFromDir(name, userStylesDir)
}

func resolveStyleFromDir(name, userStylesDir string) (string, error) {
	if userStylesDir != "" {
		userPath := filepath.Join(userStylesDir, name+".md")
		if data, readErr := os.ReadFile(userPath); readErr == nil {
			return string(data), nil
		}
	}
	// Fall back to embedded
	data, err := stylesFS.ReadFile("prompts/styles/" + name + ".md")
	if err != nil {
		if slices.Contains(availableRewriteModes(), name) {
			return "", fmt.Errorf("unknown style %q: %q is a rewrite mode (use 'prompter rewrite --mode %s') (valid styles: %s)", name, name, name, strings.Join(availableStyles(), ", "))
		}
		return "", fmt.Errorf("unknown style %q (valid: %s)", name, strings.Join(availableStyles(), ", "))
	}
	return string(data), nil
}

func availableStyles() []string {
	entries, err := stylesFS.ReadDir("prompts/styles")
	if err != nil {
		return []string{"default"}
	}
	styles := make([]string, 0, len(entries)+1)
	styles = append(styles, "default")
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		styles = append(styles, strings.TrimSuffix(entry.Name(), ".md"))
	}
	slices.Sort(styles[1:])
	return styles
}
