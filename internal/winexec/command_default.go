//go:build !windows

package winexec

import (
	"context"
	"os/exec"
)

func NewCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
