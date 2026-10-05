package provider

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func gcloudCommand(ctx context.Context, path string, args ...string) *exec.Cmd {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".cmd" && ext != ".bat" {
		return exec.CommandContext(ctx, path, args...)
	}
	interpreter := os.Getenv("ComSpec")
	if interpreter == "" {
		interpreter = "cmd.exe"
	}
	cmd := exec.CommandContext(ctx, interpreter)
	// Go's normal argument quoting targets executable argv, not cmd.exe's /c
	// parser. /s strips the outer pair, leaving the script path quoted intact.
	line := `"` + path + `"`
	for _, arg := range args {
		line += " " + syscall.EscapeArg(arg)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(interpreter) + ` /d /s /c "` + line + `"`}
	return cmd
}
