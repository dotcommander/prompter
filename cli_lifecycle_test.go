package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/dotcommander/prompter/internal/config"
)

func TestExecutionPipelineMetadata(t *testing.T) {
	for _, test := range []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
	}{
		{name: "long help", args: []string{"--help"}, wantStderr: "Usage: prompter <command> [flags]"},
		{name: "short help", args: []string{"-h"}, wantStderr: "Usage: prompter <command> [flags]"},
		{name: "long version", args: []string{"--version"}, wantStdout: "prompter v"},
		{name: "short version", args: []string{"-V"}, wantStdout: "prompter v"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := execute(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("execute(%q) code = %d, want 0", test.args, code)
			}
			if !strings.Contains(stdout.String(), test.wantStdout) {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.wantStdout)
			}
			if !strings.Contains(stderr.String(), test.wantStderr) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.wantStderr)
			}
		})
	}
}

func TestExecutionPipelineCommandHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute([]string{commandImage, "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("execute(image --help) code = %d, want 0", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Usage: prompter image") {
		t.Fatalf("stderr = %q, want image usage", stderr.String())
	}
}

func TestExecutionPipelineParseFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute([]string{"unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("execute(unknown) code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); got != "error: unknown command \"unknown\"\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestCommandRequiresInput(t *testing.T) {
	for _, command := range []string{commandRefine, commandCritique, commandApply, commandRewrite, commandImage} {
		if !commandRequiresInput(command) {
			t.Errorf("commandRequiresInput(%q) = false", command)
		}
	}
	for _, command := range []string{commandBrowse, commandConfigure, commandModels, commandPrompts} {
		if commandRequiresInput(command) {
			t.Errorf("commandRequiresInput(%q) = true", command)
		}
	}
}

func TestExecutionPipelineCommandFailure(t *testing.T) {
	wantErr := errors.New("command failed")
	var stderr bytes.Buffer
	pipeline := &executionPipeline{
		stderr: &stderr,
		flags:  &flags{},
		cfg:    &config.Config{},
		runCommand: func(context.Context, *flags, *config.Config, *slog.Logger) error {
			return wantErr
		},
	}

	status := pipeline.executeCommand()
	if status.code != 1 || !errors.Is(status.err, wantErr) {
		t.Fatalf("executeCommand() = %+v, want code 1 and command error", status)
	}
}
