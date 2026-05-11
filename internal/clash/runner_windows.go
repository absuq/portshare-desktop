//go:build windows

package clash

import (
	"context"

	"github.com/absuq/portshare-desktop/internal/winexec"
)

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return winexec.NewCommand(ctx, name, args...).CombinedOutput()
}
