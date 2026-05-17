//go:build windows

package firewall

import (
	"context"

	"github.com/absuq/portshare-desktop/internal/winexec"
)

type execRunner struct{}

func newDefaultRunner() CommandRunner {
	return execRunner{}
}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return winexec.NewCommand(ctx, name, args...).CombinedOutput()
}
