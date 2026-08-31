package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/prompter/internal/config"
)

func runInit(stdout, stderr io.Writer, cfg *config.Config, targetDir string, force bool) error {
	if targetDir == "" {
		targetDir = cfg.PromptsDir
	}
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		targetDir = filepath.Join(home, ".config", "prompter", "prompts.d")
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create prompts directory %s: %w", targetDir, err)
	}

	entries, err := starterFS.ReadDir("prompts/starter")
	if err != nil {
		return fmt.Errorf("read embedded starter prompts: %w", err)
	}

	var written []string
	var skipped []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		destPath := filepath.Join(targetDir, entry.Name())
		if _, err := os.Stat(destPath); err == nil && !force {
			skipped = append(skipped, entry.Name())
			continue
		}

		content, err := starterFS.ReadFile("prompts/starter/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read starter prompt %s: %w", entry.Name(), err)
		}

		if err := os.WriteFile(destPath, content, 0o644); err != nil {
			return fmt.Errorf("write starter prompt %s: %w", destPath, err)
		}
		written = append(written, entry.Name())
	}

	fmt.Fprintf(stdout, "✓ Prompt vault initialized at %s\n", targetDir)
	if len(written) > 0 {
		fmt.Fprintln(stdout, "\nInstalled starter prompts:")
		for _, name := range written {
			fmt.Fprintf(stdout, "  + %s\n", name)
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintln(stdout, "\nSkipped existing prompts (use --force to overwrite):")
		for _, name := range skipped {
			fmt.Fprintf(stdout, "  • %s\n", name)
		}
	}

	fmt.Fprintln(stdout, "\nReady! Try running:")
	fmt.Fprintln(stdout, "  prompter run refactor \"def my_func(): ...\"")
	fmt.Fprintln(stdout, "  prompter                    (to search & browse your vault)")

	return nil
}
