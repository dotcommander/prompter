package main

import (
	"fmt"

	"github.com/dotcommander/prompter/internal/config"
)

// loadSystemPrompt loads the enhancement system prompt from the configured path.
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

	prompt, err := readBoundedFile(cfg.PromptFile)
	if err != nil {
		return fmt.Errorf("read prompt file %s: %w", cfg.PromptFile, err)
	}

	cfg.SystemPrompt = prompt
	return nil
}
