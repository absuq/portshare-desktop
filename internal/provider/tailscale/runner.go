package tailscale

import (
	"context"

	"github.com/absuq/portshare-desktop/internal/winexec"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := winexec.NewCommand(ctx, name, args...)
	return cmd.CombinedOutput()
}
