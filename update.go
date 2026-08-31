package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

const latestModule = "github.com/dotcommander/prompter@latest"

type updateCommandRunner func(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error

func runUpdate(ctx context.Context, stdout, stderr io.Writer, runner updateCommandRunner) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("'go' toolchain not found in PATH.\nTo update prompter without Go, run 'brew upgrade prompter' or download the latest release from https://github.com/dotcommander/prompter/releases")
	}
	fmt.Fprintln(stderr, "Updating prompter to the latest release...")
	if err := runner(ctx, os.Stdin, stdout, stderr, "go", "install", latestModule); err != nil {
		return fmt.Errorf("update prompter: %w", err)
	}
	fmt.Fprintln(stdout, "Prompter updated to the latest release.")
	return nil
}

func runUpdateCommand(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
