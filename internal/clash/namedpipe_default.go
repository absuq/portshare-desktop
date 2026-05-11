//go:build !windows

package clash

import (
	"context"
	"fmt"
)

type systemPipeTransport struct {
	pipePath string
}

func (t systemPipeTransport) RoundTrip(ctx context.Context, request []byte) ([]byte, error) {
	_ = ctx
	_ = request
	return nil, fmt.Errorf("named pipe controller is only supported on Windows: %s", t.pipePath)
}
