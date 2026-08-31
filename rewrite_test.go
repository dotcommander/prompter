package main

import (
	"strings"
	"testing"
)

func TestPreprocessRewriteInputPreservesProseMentioningCruft(t *testing.T) {
	t.Parallel()

	content := "Our privacy policy explains how readings are retained.\nSign in\n## Section\nSubscribe to the newsletter for updates.\n"
	got := preprocessRewriteInput(content)

	if !strings.Contains(got, "Our privacy policy explains how readings are retained.") {
		t.Errorf("legitimate prose line deleted: %q", got)
	}
	if !strings.Contains(got, "## Section") || !strings.Contains(got, "Subscribe to the newsletter") {
		t.Errorf("legitimate content deleted: %q", got)
	}
	if strings.Contains(got, "Sign in\n") {
		t.Errorf("standalone cruft line kept: %q", got)
	}
}

func TestPreprocessRewriteInputPreservesLongUnbrokenText(t *testing.T) {
	t.Parallel()

	url := "https://example.com/docs/" + strings.Repeat("a", 110)
	content := "See the reference:\n" + url + "\n\x00garbage\xff\xfe\n"
	got := preprocessRewriteInput(content)

	if !strings.Contains(got, url) {
		t.Errorf("long unbroken legitimate line deleted: %q", got)
	}
	if strings.Contains(got, "garbage") {
		t.Errorf("control-character line kept: %q", got)
	}
}

func TestPreprocessRewriteInputCollapsesDuplicatesAndBlanks(t *testing.T) {
	t.Parallel()

	got := preprocessRewriteInput("para\n\n\n\npara\nother\nother\n\nend")
	if got != "para\n\npara\nother\n\nend" {
		t.Errorf("dedup/blank behavior changed: %q", got)
	}
}

func TestPreprocessRewriteInputPreservesCRLFText(t *testing.T) {
	t.Parallel()

	got := preprocessRewriteInput("first line\r\nsecond line\r\n")
	if got != "first line\r\nsecond line" {
		t.Fatalf("CRLF text changed: %q", got)
	}
}
