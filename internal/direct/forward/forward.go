package forward

import (
	"context"
	"errors"
	"net"
	"sync"

	direct "github.com/absuq/portshare-desktop/internal/direct"
)

type Options struct {
	LocalAddress string
	PeerAddress  string
	TargetHost   string
	TargetPort   int
	DirectClient direct.Client
}

type Forward struct {
	options Options

	mu       sync.Mutex
	listener net.Listener
	active   map[net.Conn]struct{}
	stopped  bool
}

func New(options Options) *Forward {
	return &Forward{
		options: options,
		active:  make(map[net.Conn]struct{}),
		stopped: true,
	}
}

func (f *Forward) Start(ctx context.Context) error {
	f.mu.Lock()
	if f.listener != nil {
		f.mu.Unlock()
		return errors.New("forward already started")
	}
	listener, err := net.Listen("tcp", f.options.LocalAddress)
	if err != nil {
		f.mu.Unlock()
		return err
	}
	f.listener = listener
	f.stopped = false
	f.mu.Unlock()

	go f.acceptLoop(ctx, listener)
	return nil
}

func (f *Forward) LocalAddress() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listener == nil {
		return ""
	}
	return f.listener.Addr().String()
}

func (f *Forward) Stop() {
	f.mu.Lock()
	f.stopped = true
	listener := f.listener
	f.listener = nil
	active := make([]net.Conn, 0, len(f.active))
	for conn := range f.active {
		active = append(active, conn)
	}
	f.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	for _, conn := range active {
		_ = conn.Close()
	}
}

func (f *Forward) acceptLoop(ctx context.Context, listener net.Listener) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-done:
		}
	}()
	defer close(done)

	for {
		conn, err := listener.Accept()
		if err != nil {
			f.clearListener(listener)
			return
		}
		if !f.addActive(conn) {
			_ = conn.Close()
			return
		}
		go f.handle(ctx, conn)
	}
}

func (f *Forward) clearListener(listener net.Listener) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listener == listener {
		f.listener = nil
	}
}

func (f *Forward) handle(ctx context.Context, local net.Conn) {
	defer f.removeActive(local)

	remote, err := f.options.DirectClient.OpenTCP(
		ctx,
		f.options.PeerAddress,
		f.options.TargetHost,
		f.options.TargetPort,
	)
	if err != nil {
		_ = local.Close()
		return
	}
	if !f.addActive(remote) {
		_ = remote.Close()
		_ = local.Close()
		return
	}
	defer f.removeActive(remote)

	direct.PipeBidirectional(local, remote)
}

func (f *Forward) addActive(conns ...net.Conn) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return false
	}
	for _, conn := range conns {
		f.active[conn] = struct{}{}
	}
	return true
}

func (f *Forward) removeActive(conns ...net.Conn) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, conn := range conns {
		delete(f.active, conn)
	}
}
