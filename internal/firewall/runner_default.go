//go:build !windows

package firewall

import (
	"context"
	"errors"
)

type noopRunner struct{}

func newDefaultRunner() CommandRunner {
	return noopRunner{}
}

func (noopRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("Windows 防火墙授权仅支持 Windows")
}
