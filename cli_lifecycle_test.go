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

func TestPrintResultPreservesFinalNewline(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		input  string
		output string
	}{
		{name: "adds missing newline", input: "result", output: "result\n"},
		{name: "preserves final newline", input: "result\n", output: "result\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			printResult(&output, test.input)
			if got := output.String(); got != test.output {
				t.Fatalf("printResult(%q) = %q, want %q", test.input, got, test.output)
			}
		})
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
	t.Parallel()

	wantErr := errors.New("command failed")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var stdout, stderr bytes.Buffer
	pipeline := &executionPipeline{
		ctx:    ctx,
		stdout: &stdout,
		stderr: &stderr,
		flags:  &flags{},
		cfg:    &config.Config{},
		runCommand: func(gotCtx context.Context, _ *flags, _ *config.Config, _ *slog.Logger, output commandOutput) error {
			if gotCtx != ctx {
				t.Fatal("command context does not match pipeline context")
			}
			if output.stdout != &stdout || output.stderr != &stderr {
				t.Fatalf("command output = %+v, want pipeline writers", output)
			}
			return wantErr
		},
	}

	status := pipeline.executeCommand()
	if status.code != 1 || !errors.Is(status.err, wantErr) {
		t.Fatalf("executeCommand() = %+v, want code 1 and command error", status)
	}
}

func TestExecutionPipelineCanceledContextReturns130(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := executeContext(ctx, []string{"--help"}, &stdout, &stderr); code != 130 {
		t.Fatalf("executeContext canceled code = %d, want 130", code)
	}
	if strings.Contains(stderr.String(), "error:") {
		t.Fatalf("stderr = %q, want no command error", stderr.String())
	}
}

func TestRunWithOutputDryRunUsesProvidedStderr(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	f := &flags{command: commandRefine, dryRun: true, provider: "groq", args: []string{"input"}}
	cfg := &config.Config{
		Provider:        "groq",
		Effort:          "low",
		Timeout:         60,
		MaxOutputTokens: 4096,
		Providers: map[string]config.ProviderConfig{
			"groq": {Model: "model", BaseURL: "http://test"},
		},
	}

	if err := runWithOutput(context.Background(), f, cfg, newLoggerTo(&stderr, false), commandOutput{stdout: &stdout, stderr: &stderr}); err != nil {
		t.Fatalf("runWithOutput dry run: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Dry run: no API call made") {
		t.Fatalf("stderr = %q, want dry-run diagnostics", stderr.String())
	}
}

func TestSpinnerStopUsesProvidedStderr(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	NewSpinner(newLoggerTo(&stderr, false), "model", &stderr).Stop()
	if got := stderr.String(); got != "\r\033[K" {
		t.Fatalf("spinner stderr = %q, want clear sequence", got)
	}
}

func TestRunAssembleUsesProvidedOutput(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := runAssemble(
		&flags{args: []string{"portrait of a clockmaker"}, count: 1, profile: "minimal"},
		&config.Config{},
		commandOutput{stdout: &stdout, stderr: &stderr},
	)
	if err != nil {
		t.Fatalf("runAssemble: %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatal("stdout is empty, want assembled prompt")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
