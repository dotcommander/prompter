package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
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
			return "", fmt.Errorf("unknown rewrite mode %q: %q is an enhancement style (use 'prompter refine --style %s') (valid: %s)", mode, mode, mode, strings.Join(availableRewriteModes(), ", "))
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
	"built with", "click here",
	"try for free", "start free", "free trial",
}

func preprocessRewriteInput(content string) string {
	return preprocessRewriteInputForMode(content, "clean")
}

func preprocessRewriteInputForMode(content, mode string) string {
	if strings.TrimSpace(mode) == "code" {
		return strings.Trim(content, "\r\n")
	}

	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	prevLine := ""
	blankCount := 0
	var fenceMarker byte
	fenceLength := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fenceLength > 0 {
			out = append(out, line)
			if isRewriteFenceClose(trimmed, fenceMarker, fenceLength) {
				fenceMarker = 0
				fenceLength = 0
				prevLine = ""
				blankCount = 0
			}
			continue
		}
		if marker, length, ok := rewriteFenceOpen(trimmed); ok {
			out = append(out, line)
			fenceMarker = marker
			fenceLength = length
			continue
		}
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

func rewriteFenceOpen(line string) (byte, int, bool) {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0, 0, false
	}
	marker := line[0]
	length := 0
	for length < len(line) && line[length] == marker {
		length++
	}
	return marker, length, length >= 3
}

func isRewriteFenceClose(line string, marker byte, minimumLength int) bool {
	if line == "" || line[0] != marker {
		return false
	}
	length := 0
	for length < len(line) && line[length] == marker {
		length++
	}
	return length >= minimumLength && strings.TrimSpace(line[length:]) == ""
}

// cruftDecoration is trimmed from both ends of a line before whole-line
// comparison, so standalone markers like "— Sign in —" or "Subscribe:" still
// match while sentences that merely mention a cruft phrase are preserved.
var cruftDecoration = " \t:-—|*·•"

func isRewriteCruft(line string) bool {
	trimmed := strings.Trim(strings.TrimSpace(line), cruftDecoration)
	if trimmed == "" {
		return false
	}
	for _, pattern := range rewriteCruftPatterns {
		if strings.EqualFold(trimmed, pattern) {
			return true
		}
	}
	return false
}

func isRewriteBinaryLine(line string) bool {
	// Structural rule: drop lines that are actually binary (invalid UTF-8 or
	// control characters). Length-without-spaces deleted legitimate long URLs
	// and space-less scripts (e.g. CJK prose).
	if !utf8.ValidString(line) {
		return true
	}
	return strings.ContainsFunc(line, func(r rune) bool {
		return (r < ' ' || r == 0x7f) && r != '\t' && r != '\r'
	})
}
