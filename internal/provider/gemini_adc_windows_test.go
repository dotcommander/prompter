package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsNativeGcloudScriptADC(t *testing.T) {
	root := filepath.Join(t.TempDir(), "User with spaces")
	dir := filepath.Join(root, "Google", "Cloud SDK", "google-cloud-sdk", "bin")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "gcloud.cmd")
	body := "@echo off\r\nif not \"%~1\"==\"auth\" exit /b 3\r\nif not \"%~2\"==\"application-default\" exit /b 4\r\nif not \"%~3\"==\"print-access-token\" exit /b 5\r\necho fixture-token\r\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", root)
	if os.Getenv("ComSpec") == "" {
		t.Setenv("ComSpec", filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"))
	}
	t.Setenv("PATH", dir)
	t.Setenv("PATHEXT", ".EXE;.CMD")
	if got, err := resolveGcloud(nil); err != nil || got != path {
		t.Fatalf("PATH: %q %v", got, err)
	}
	if token, err := runGcloudADC(context.Background(), nil); err != nil || token != "fixture-token" {
		t.Fatalf("PATH invocation: %q %v", token, err)
	}
	t.Setenv("PATH", t.TempDir())
	if token, err := runGcloudADC(context.Background(), gcloudDefaultCandidates()); err != nil || token != "fixture-token" {
		t.Fatalf("fallback invocation: %q %v", token, err)
	}
}
