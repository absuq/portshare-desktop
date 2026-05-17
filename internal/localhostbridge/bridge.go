package localhostbridge

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

type Bridge struct {
	plan     BridgePlan
	mu       sync.Mutex
	listener net.Listener
	closed   bool
	active   map[net.Conn]struct{}
}

func NewBridge(plan BridgePlan) *Bridge {
	return &Bridge{plan: plan, active: make(map[net.Conn]struct{})}
}

func (b *Bridge) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", b.plan.ListenAddress)
	if err != nil {
		return err
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		_ = listener.Close()
		return net.ErrClosed
	}
	b.listener = listener
	b.mu.Unlock()
	go b.serve(listener)
	return nil
}

func (b *Bridge) Addr() net.Addr {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.listener == nil {
		return nil
	}
	return b.listener.Addr()
}

func (b *Bridge) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	listener := b.listener
	active := make([]net.Conn, 0, len(b.active))
	for conn := range b.active {
		active = append(active, conn)
	}
	b.mu.Unlock()

	if listener != nil {
		_ = listener.Close()
	}
	for _, conn := range active {
		_ = conn.Close()
	}
	return nil
}

func (b *Bridge) serve(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if b.isClosed() || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if !b.addActive(conn) {
			_ = conn.Close()
			return
		}
		go b.handle(conn)
	}
}

func (b *Bridge) handle(inbound net.Conn) {
	defer b.removeActive(inbound)
	defer inbound.Close()

	if !b.isAllowedRemote(inbound.RemoteAddr()) {
		return
	}
	_ = inbound.SetDeadline(time.Time{})
	outbound, err := net.DialTimeout("tcp", b.plan.TargetAddress, 5*time.Second)
	if err != nil {
		return
	}
	if !b.addActive(outbound) {
		_ = outbound.Close()
		return
	}
	defer b.removeActive(outbound)
	defer outbound.Close()

	done := make(chan struct{}, 2)
	go copyAndClose(outbound, inbound, done)
	go copyAndClose(inbound, outbound, done)
	<-done
}

func copyAndClose(dst net.Conn, src net.Conn, done chan<- struct{}) {
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	_ = src.Close()
	done <- struct{}{}
}

func (b *Bridge) isAllowedRemote(address net.Addr) bool {
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return false
	}
	for _, allowed := range b.plan.AllowedPeerIPs {
		if host == allowed {
			return true
		}
	}
	return false
}

func (b *Bridge) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func (b *Bridge) addActive(conns ...net.Conn) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	for _, conn := range conns {
		b.active[conn] = struct{}{}
	}
	return true
}

func (b *Bridge) removeActive(conns ...net.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, conn := range conns {
		delete(b.active, conn)
	}
}
