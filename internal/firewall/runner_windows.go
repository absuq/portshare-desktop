//go:build windows

package firewall

import (
	"context"
	"os/exec"
)

type execRunner struct{}

func newDefaultRunner() CommandRunner {
	return execRunner{}
}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
