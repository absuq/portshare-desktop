package direct

import (
	"io"
	"net"
)

type closeWriter interface {
	CloseWrite() error
}

// PipeBidirectional copies both directions and half-closes each write side when
// its copy direction finishes, allowing data already returning on the other
// direction to drain before both connections are closed.
func PipeBidirectional(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go copyAndHalfClose(done, a, b)
	go copyAndHalfClose(done, b, a)
	<-done
	<-done
	_ = a.Close()
	_ = b.Close()
}

func copyAndHalfClose(done chan<- struct{}, dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if cw, ok := dst.(closeWriter); ok {
		_ = cw.CloseWrite()
	} else {
		_ = dst.Close()
	}
	done <- struct{}{}
}
