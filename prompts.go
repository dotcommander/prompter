package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dotcommander/prompter/internal/config"
	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
)

// Scan limits prevent unbounded directory traversal.
const (
	maxScanDepth = 5    // Maximum directory depth to scan
	maxScanFiles = 1000 // Maximum number of files to process
)

// PromptEntry represents a prompt file with its metadata.
type PromptEntry struct {
	Name        string   // Filename without .md extension
	Path        string   // Full path to the file
	Description string   // From frontmatter or derived from name
	Aliases     []string // Alternative names, triggers, and examples for searching
	Content     string   // Prompt content without frontmatter
	Validation  *OutputValidation
}

// Frontmatter holds YAML metadata from prompt files.
type Frontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Aliases     []string `yaml:"aliases"`
	Triggers    []string `yaml:"triggers"`
	Examples    []string `yaml:"examples"`
	Validation  *OutputValidation
}

func ScanPromptsDir(dir string) ([]PromptEntry, error) {
	var entries []PromptEntry
	fileCount := 0
	visitedDirs := make(map[string]bool)

	var walk func(currentDir string, depth int) error
	walk = func(currentDir string, depth int) error {
		if depth > maxScanDepth {
			return fmt.Errorf("prompt scan depth exceeds limit %d at %s", maxScanDepth, currentDir)
		}

		realCurrentDir, err := filepath.EvalSymlinks(currentDir)
		if err != nil {
			if depth == 0 {
				return err
			}
			return fmt.Errorf("resolve prompt directory %s: %w", currentDir, err)
		}
		if visitedDirs[realCurrentDir] {
			return nil // Prevent recursive symlink loops
		}
		visitedDirs[realCurrentDir] = true

		items, err := os.ReadDir(realCurrentDir)
		if err != nil {
			return err
		}

		for _, item := range items {
			itemPath := filepath.Join(realCurrentDir, item.Name())
			info, err := os.Stat(itemPath)
			if err != nil {
				return fmt.Errorf("stat prompt path %s: %w", itemPath, err)
			}

			if info.IsDir() {
				if err := walk(itemPath, depth+1); err != nil {
					return err
				}
				continue
			}

			if !strings.HasSuffix(item.Name(), ".md") {
				continue
			}
			if fileCount >= maxScanFiles {
				return fmt.Errorf("prompt scan exceeds file limit %d at %s", maxScanFiles, itemPath)
			}

			data, readErr := os.ReadFile(itemPath)
			if readErr != nil {
				return fmt.Errorf("read prompt file %s: %w", itemPath, readErr)
			}

			fm, body, fmErr := parseFrontmatter(data)
			if fmErr != nil {
				fmt.Fprintf(os.Stderr, "warning: %s: %v\n", itemPath, fmErr)
			}
			name := strings.TrimSuffix(item.Name(), ".md")
			if fm.Name != "" {
				name = fm.Name
			}

			entry := PromptEntry{
				Name:       name,
				Path:       itemPath,
				Aliases:    promptAliases(fm),
				Content:    body,
				Validation: fm.Validation,
			}

			if fm.Description != "" {
				entry.Description = fm.Description
			} else {
				entry.Description = strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
			}

			fileCount++
			entries = append(entries, entry)
		}
		return nil
	}

	if err := walk(dir, 0); err != nil {
		return nil, err
	}
	return entries, nil
}

func promptAliases(fm Frontmatter) []string {
	seen := make(map[string]struct{})
	var aliases []string
	for _, values := range [][]string{fm.Aliases, fm.Triggers, fm.Examples} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			aliases = append(aliases, value)
		}
	}
	return aliases
}

// extractFrontmatterFields extracts description and aliases from parsed metadata.
func extractFrontmatterFields(metadata map[string]interface{}) Frontmatter {
	var fm Frontmatter
	if n, ok := metadata["name"].(string); ok {
		fm.Name = n
	}
	if d, ok := metadata["description"].(string); ok {
		fm.Description = d
	}
	fm.Aliases = metadataStrings(metadata, "aliases")
	fm.Triggers = metadataStrings(metadata, "triggers")
	fm.Examples = metadataStrings(metadata, "examples")
	fm.Validation = outputValidationFromMetadata(metadata["validation"])
	return fm
}

func metadataStrings(metadata map[string]interface{}, key string) []string {
	values, ok := metadata[key].([]any)
	if !ok {
		return nil
	}
	var stringsOut []string
	for _, alias := range values {
		if s, ok := alias.(string); ok {
			stringsOut = append(stringsOut, s)
		}
	}
	return stringsOut
}

func scanPromptDirs(dirs []string) ([]PromptEntry, error) {
	var entries []PromptEntry

	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		dirEntries, err := ScanPromptsDir(dir)
		if err != nil {
			return nil, err
		}
		entries = append(entries, dirEntries...)
	}

	return entries, nil
}

func promptDirs(cfg *config.Config) ([]string, error) {
	dirs := append([]string{}, cfg.PromptsDirs...)
	if cfg.PromptsDir != "" {
		dirs = append([]string{cfg.PromptsDir}, dirs...)
	}
	if len(cfg.PromptsDirs) == 0 {
		if roleDir := defaultRolePromptsDir(); roleDir != "" {
			if _, err := os.Stat(roleDir); err == nil {
				dirs = append(dirs, roleDir)
			}
		}
	}
	if len(dirs) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		dirs = []string{filepath.Join(home, ".config", "prompter", "prompts.d")}
	}

	seen := make(map[string]struct{})
	unique := dirs[:0]
	for _, dir := range dirs {
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		unique = append(unique, dir)
	}
	return unique, nil
}

func loadCatalogSystemPrompt(cfg *config.Config, selector string) (*OutputValidation, error) {
	dirs, err := promptDirs(cfg)
	if err != nil {
		return nil, err
	}
	entries, err := scanPromptDirs(dirs)
	if err != nil {
		return nil, err
	}
	entry, err := findPromptEntry(entries, selector)
	if err != nil {
		return nil, err
	}
	if entry.Validation != nil {
		if err := entry.Validation.validate(); err != nil {
			return nil, fmt.Errorf("prompt %s validation: %w", entry.Path, err)
		}
	}
	cfg.SystemPrompt = entry.Content
	return entry.Validation, nil
}

func findPromptEntry(entries []PromptEntry, selector string) (PromptEntry, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return PromptEntry{}, fmt.Errorf("prompt name or alias is empty")
	}

	var names []PromptEntry
	var aliases []PromptEntry
	for _, entry := range entries {
		if strings.EqualFold(entry.Name, selector) {
			names = append(names, entry)
			continue
		}
		for _, alias := range entry.Aliases {
			if strings.EqualFold(alias, selector) {
				aliases = append(aliases, entry)
				break
			}
		}
	}

	matches := aliases
	if len(names) > 0 {
		matches = names
	}
	switch len(matches) {
	case 0:
		return PromptEntry{}, fmt.Errorf("prompt %q not found", selector)
	case 1:
		return matches[0], nil
	default:
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.Path)
		}
		sort.Strings(paths)
		return PromptEntry{}, fmt.Errorf("prompt %q is ambiguous: %s", selector, strings.Join(paths, ", "))
	}
}

func defaultRolePromptsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "roles", "prompts")
}

// extractBody returns the content minus YAML frontmatter delimiters.
func extractBody(content []byte) string {
	if !bytes.HasPrefix(content, []byte("---\n")) {
		return string(content)
	}
	rest := content[4:]
	if endIdx := bytes.Index(rest, []byte("\n---\n")); endIdx != -1 {
		return string(rest[endIdx+4:])
	}
	if endIdx := bytes.Index(rest, []byte("\n---")); endIdx != -1 && endIdx+4 == len(rest) {
		return ""
	}
	return string(content)
}

// parseFrontmatter extracts YAML frontmatter and returns the body.
func parseFrontmatter(content []byte) (Frontmatter, string, error) {
	// Normalize CRLF to LF once so goldmark and extractBody see identical input.
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	var fm Frontmatter
	md := goldmark.New(
		goldmark.WithExtensions(meta.Meta),
	)
	ctx := parser.NewContext()
	var buf bytes.Buffer
	if err := md.Convert(content, &buf, parser.WithContext(ctx)); err != nil {
		return fm, string(content), fmt.Errorf("parse frontmatter: %w", err)
	}

	if metadata := meta.Get(ctx); metadata != nil {
		fm = extractFrontmatterFields(metadata)
	}

	body := extractBody(content)
	return fm, strings.TrimSpace(body), nil
}

// loadSystemPrompt loads the system prompt from the configured path.
// The prompt is cached in cfg.SystemPrompt after the first load.
func loadSystemPrompt(cfg *config.Config) error {
	// Return cached prompt if already loaded
	if cfg.SystemPrompt != "" {
		return nil
	}
	if cfg.PromptFile == "" {
		cfg.SystemPrompt = defaultEnhancePrompt
		return nil
	}

	data, err := os.ReadFile(cfg.PromptFile)
	if err != nil {
		return fmt.Errorf("read prompt file %s: %w", cfg.PromptFile, err)
	}

	cfg.SystemPrompt = string(data)
	return nil
}
