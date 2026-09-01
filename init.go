package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/prompter/internal/config"
)

type openFileFunc func(string, int, os.FileMode) (*os.File, error)

const promptInitMarker = ".prompter-init-incomplete"

func runInit(stdout, stderr io.Writer, cfg *config.Config, targetDir string) error {
	return runInitWithOpen(stdout, stderr, cfg, targetDir, os.OpenFile)
}

func runInitWithOpen(stdout, stderr io.Writer, cfg *config.Config, targetDir string, openFile openFileFunc) error {
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
	markerPath := filepath.Join(targetDir, promptInitMarker)
	if err := ensurePromptInitMarker(markerPath, openFile); err != nil {
		return fmt.Errorf("create prompt initialization marker: %w", err)
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

		content, err := starterFS.ReadFile("prompts/starter/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read starter prompt %s: %w", entry.Name(), err)
		}

		destPath := filepath.Join(targetDir, entry.Name())
		created, err := writeStarterExclusive(destPath, content, openFile)
		if err != nil {
			return fmt.Errorf("write starter prompt %s: %w", destPath, err)
		}
		if created {
			written = append(written, entry.Name())
		} else {
			skipped = append(skipped, entry.Name())
		}
	}

	if err := os.Remove(markerPath); err != nil {
		return fmt.Errorf("remove prompt initialization marker: %w", err)
	}

	fmt.Fprintf(stdout, "✓ Prompt vault initialized at %s\n", targetDir)
	if len(written) > 0 {
		fmt.Fprintln(stdout, "\nInstalled starter prompts:")
		for _, name := range written {
			fmt.Fprintf(stdout, "  + %s\n", name)
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintln(stdout, "\nSkipped existing prompts:")
		for _, name := range skipped {
			fmt.Fprintf(stdout, "  • %s\n", name)
		}
	}

	fmt.Fprintln(stdout, "\nReady! Try running:")
	fmt.Fprintln(stdout, "  prompter apply refactor \"def my_func(): ...\"")
	fmt.Fprintln(stdout, "  prompter browse             (to search your vault)")

	return nil
}

func ensurePromptInitMarker(path string, openFile openFileFunc) error {
	present, err := promptInitMarkerPresent(path)
	if err != nil {
		return err
	}
	if present {
		return nil
	}

	marker, err := openFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		present, inspectErr := promptInitMarkerPresent(path)
		if inspectErr != nil {
			return inspectErr
		}
		if present {
			return nil
		}
		return fmt.Errorf("marker changed during creation")
	}
	if err != nil {
		return err
	}
	if err := marker.Close(); err != nil {
		return fmt.Errorf("close marker: %w", err)
	}
	return nil
}

func promptInitMarkerPresent(path string) (bool, error) {
	info, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
		return false, nil
	case err != nil:
		return false, err
	case !info.Mode().IsRegular():
		return false, fmt.Errorf("marker is not a regular file")
	default:
		return true, nil
	}
}

func writeStarterExclusive(path string, content []byte, openFile openFileFunc) (bool, error) {
	file, err := openFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	removePartial := true
	defer func() {
		_ = file.Close()
		if removePartial {
			_ = os.Remove(path)
		}
	}()
	written, err := file.Write(content)
	if err != nil {
		return false, err
	}
	if written != len(content) {
		return false, io.ErrShortWrite
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	removePartial = false
	return true, nil
}
