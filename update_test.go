package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRunUpdateInstallsLatestModule(t *testing.T) {
	t.Parallel()
	var stdout, stderr strings.Builder
	var gotName string
	var gotArgs []string
	runner := func(_ context.Context, _ io.Reader, _, _ io.Writer, name string, args ...string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}

	if err := runUpdate(context.Background(), &stdout, &stderr, runner); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	if gotName != "go" || strings.Join(gotArgs, " ") != "install "+latestModule {
		t.Fatalf("command = %q %q, want go install %s", gotName, gotArgs, latestModule)
	}
	if !strings.Contains(stdout.String(), "updated") {
		t.Fatalf("stdout = %q, want success message", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Updating") {
		t.Fatalf("stderr = %q, want progress message", stderr.String())
	}
}

func TestRunUpdateReturnsInstallFailure(t *testing.T) {
	t.Parallel()
	want := errors.New("go unavailable")
	runner := func(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
		return want
	}

	err := runUpdate(context.Background(), io.Discard, io.Discard, runner)
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "update prompter") {
		t.Fatalf("runUpdate error = %v, want wrapped install failure", err)
	}
}

func TestParseArgsUpdateRejectsArgumentsAndFlags(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"update", "extra"}, {"update", "--verbose"}} {
		if _, err := parseArgs(args); err == nil || !strings.Contains(err.Error(), "does not accept") {
			t.Fatalf("parseArgs(%q) error = %v, want rejected update arguments", args, err)
		}
	}
}
