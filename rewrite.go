package main

import (
	"fmt"
	"slices"
	"strings"
)

var rewriteModes = []string{"clean", "academic", "blog", "extract", "code", "synthesis"}

func availableRewriteModes() []string {
	modes := append([]string(nil), rewriteModes...)
	slices.Sort(modes)
	return modes
}

func resolveRewritePrompt(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "clean"
	}
	if !slices.Contains(rewriteModes, mode) {
		if slices.Contains(availableStyles(), mode) {
			return "", fmt.Errorf("unknown rewrite mode %q: %q is an enhancement style (use 'prompter --style %s') (valid: %s)", mode, mode, mode, strings.Join(availableRewriteModes(), ", "))
		}
		return "", fmt.Errorf("unknown rewrite mode %q (valid: %s)", mode, strings.Join(availableRewriteModes(), ", "))
	}

	return strings.ReplaceAll(defaultRewritePrompt, "{{MODE}}", mode), nil
}

var rewriteCruftPatterns = []string{
	"open in app", "sign up", "sign in", "log in", "login",
	"download now", "get started", "upgrade now", "subscribe",
	"share this", "follow us", "cookie", "privacy policy",
	"terms of service", "all rights reserved", "powered by",
	"built with", "copyright 20", "click here",
	"try for free", "start free", "free trial",
}

func preprocessRewriteInput(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	prevLine := ""
	blankCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isRewriteCruft(trimmed) || isRewriteBinaryLine(trimmed) {
			continue
		}
		if trimmed != "" && trimmed == prevLine {
			continue
		}
		if trimmed == "" {
			blankCount++
			if blankCount > 1 {
				continue
			}
		} else {
			blankCount = 0
		}
		out = append(out, line)
		prevLine = trimmed
	}

	return strings.TrimSpace(strings.Join(out, "\n"))
}

func isRewriteCruft(line string) bool {
	lower := strings.ToLower(line)
	for _, pattern := range rewriteCruftPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func isRewriteBinaryLine(line string) bool {
	return len(line) > 100 && !strings.Contains(line, " ")
}
