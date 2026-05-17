//go:build windows

package winexec

import (
	"context"
	"os/exec"
	"syscall"
)

const CreateNoWindow = 0x08000000

func NewCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: CreateNoWindow,
	}
	return cmd
}
