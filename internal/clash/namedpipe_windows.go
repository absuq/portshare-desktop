//go:build windows

package clash

import (
	"bytes"
	"context"
	"io"
	"os"
)

type systemPipeTransport struct {
	pipePath string
}

func (t systemPipeTransport) RoundTrip(ctx context.Context, request []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(t.pipePath, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := file.Write(request); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, file)
	return buffer.Bytes(), err
}
