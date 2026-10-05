package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeGcloud creates an executable gcloud shim whose shell body is body.
// The shim receives gcloud arguments on its command line.
func writeFakeGcloud(t *testing.T, dir, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture; Windows invocation covered by native Windows fixture")
	}
	path := filepath.Join(dir, "gcloud")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write fake gcloud: %v", err)
	}
	return path
}

func TestResolveGcloudPrefersPATHLookup(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeGcloud(t, dir, "echo token")

	t.Setenv("PATH", dir)

	got, err := resolveGcloud([]string{"/nonexistent/gcloud"})
	if err != nil {
		t.Fatalf("resolveGcloud: %v", err)
	}
	if got != fake {
		t.Fatalf("resolveGcloud = %q, want PATH entry %q", got, fake)
	}
}

func TestResolveGcloudFallsBackToCandidates(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeGcloud(t, dir, "echo token")
	notExecutable := filepath.Join(dir, "gcloud-plain")
	if err := os.WriteFile(notExecutable, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatalf("write non-executable candidate: %v", err)
	}

	t.Setenv("PATH", t.TempDir())

	got, err := resolveGcloud([]string{notExecutable, fake})
	if err != nil {
		t.Fatalf("resolveGcloud: %v", err)
	}
	if got != fake {
		t.Fatalf("resolveGcloud = %q, want candidate %q", got, fake)
	}
}

func TestResolveGcloudNotFoundNamesSearchedLocations(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := resolveGcloud([]string{"/definitely/not/gcloud"})
	if err == nil {
		t.Fatal("resolveGcloud unexpectedly succeeded")
	}
	msg := err.Error()
	for _, want := range []string{"PATH", "/definitely/not/gcloud"} {
		if !strings.Contains(msg, want) {
			t.Errorf("resolveGcloud error missing %q: %s", want, msg)
		}
	}
}

func TestRunGcloudADCReturnsToken(t *testing.T) {
	dir := t.TempDir()
	writeFakeGcloud(t, dir, `echo "  adc-token-value  "`)

	t.Setenv("PATH", dir)

	token, err := runGcloudADC(context.Background(), nil)
	if err != nil {
		t.Fatalf("runGcloudADC: %v", err)
	}
	if token != "adc-token-value" {
		t.Fatalf("runGcloudADC token = %q, want trimmed adc-token-value", token)
	}
}

func TestRunGcloudADCFailureIncludesExitStatusAndStderr(t *testing.T) {
	dir := t.TempDir()
	writeFakeGcloud(t, dir, "echo 'reauth required for account' >&2; exit 3")

	t.Setenv("PATH", dir)

	_, err := runGcloudADC(context.Background(), nil)
	if err == nil {
		t.Fatal("runGcloudADC unexpectedly succeeded")
	}
	msg := err.Error()
	for _, want := range []string{"exit 3", "reauth required for account"} {
		if !strings.Contains(msg, want) {
			t.Errorf("runGcloudADC error missing %q: %s", want, msg)
		}
	}
}

func TestRunGcloudADCEmptyTokenFails(t *testing.T) {
	dir := t.TempDir()
	writeFakeGcloud(t, dir, "exit 0")

	t.Setenv("PATH", dir)

	if _, err := runGcloudADC(context.Background(), nil); err == nil {
		t.Fatal("runGcloudADC unexpectedly succeeded on empty token")
	}
}

func TestGetAccessTokenFailureCarriesAccurateRemediation(t *testing.T) {
	t.Parallel()

	prov := NewGemini("", "project", "global", "gemini-3.7-flash", "https://aiplatform.googleapis.com/v1", 0, 1024)
	gemini := prov.(*geminiProvider)
	gemini.tokenResolver = func(context.Context) (string, error) {
		return "", errors.New("gcloud executable not found (searched PATH)")
	}

	_, err := gemini.getAccessToken(context.Background())
	if err == nil {
		t.Fatal("getAccessToken unexpectedly succeeded")
	}
	msg := err.Error()
	for _, want := range []string{
		"Google ADC token resolution failed",
		"searched PATH",
		"gcloud auth application-default login",
		"PROMPTER_GEMINI_BASE_URL",
		"https://generativelanguage.googleapis.com/v1",
		"prompter --config",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("getAccessToken error missing %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "prompter configure") {
		t.Errorf("getAccessToken error references retired command \"prompter configure\": %s", msg)
	}
}
