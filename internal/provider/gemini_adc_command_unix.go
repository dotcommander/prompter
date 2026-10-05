//go:build !windows

package provider

import (
	"context"
	"os/exec"
)

func gcloudCommand(ctx context.Context, path string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, path, args...)
}
