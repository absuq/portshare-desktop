package forward

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	direct "github.com/absuq/portshare-desktop/internal/direct"
)

func TestLocalForwardReachesPeerTarget(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello through portshare"))
	}))
	defer target.Close()
	targetHost, targetPort := splitHostPort(t, target.Listener.Addr().String())

	control, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()

	server := direct.NewServer(direct.ServerConfig{DeviceID: "device-b", DeviceName: "desktop-b", Secret: "shared"})
	go func() { _ = server.Serve(control) }()
	defer server.Close()

	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	fwd := New(Options{
		LocalAddress: "127.0.0.1:0",
		PeerAddress:  control.Addr().String(),
		TargetHost:   targetHost,
		TargetPort:   targetPort,
		DirectClient: client,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := fwd.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer fwd.Stop()

	resp, err := http.Get("http://" + fwd.LocalAddress())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello through portshare" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestForwardStopClosesActiveConnection(t *testing.T) {
	target := newEchoTarget(t)
	server, control := startDirectServer(t)
	defer server.Close()
	defer control.Close()
	targetHost, targetPort := splitHostPort(t, target.Addr().String())

	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	fwd := New(Options{
		LocalAddress: "127.0.0.1:0",
		PeerAddress:  control.Addr().String(),
		TargetHost:   targetHost,
		TargetPort:   targetPort,
		DirectClient: client,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := fwd.Start(ctx); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("tcp", fwd.LocalAddress())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	assertRoundTrip(t, conn)

	fwd.Stop()
	assertConnClosesSoon(t, conn)
}

func TestForwardAddActiveRejectsAfterStop(t *testing.T) {
	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	fwd := New(Options{
		LocalAddress: "127.0.0.1:0",
		PeerAddress:  "127.0.0.1:1",
		TargetHost:   "127.0.0.1",
		TargetPort:   1,
		DirectClient: client,
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fwd.Start(ctx); err != nil {
		t.Fatal(err)
	}
	fwd.Stop()

	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	if fwd.addActive(local) {
		t.Fatal("expected addActive to reject connection after Stop")
	}
}

func TestServerCloseClosesActiveOpenTCPConnection(t *testing.T) {
	target := newEchoTarget(t)
	server, control := startDirectServer(t)
	defer control.Close()
	targetHost, targetPort := splitHostPort(t, target.Addr().String())

	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.OpenTCP(ctx, control.Addr().String(), targetHost, targetPort)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	assertRoundTrip(t, conn)

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	assertConnClosesSoon(t, conn)
}

func TestForwardStartTwiceReturnsErrorAndStopIsIdempotent(t *testing.T) {
	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	fwd := New(Options{
		LocalAddress: "127.0.0.1:0",
		PeerAddress:  "127.0.0.1:1",
		TargetHost:   "127.0.0.1",
		TargetPort:   1,
		DirectClient: client,
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fwd.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fwd.Start(ctx); err == nil {
		t.Fatal("expected second Start to fail")
	}

	fwd.Stop()
	fwd.Stop()
}

func TestForwardStartRequiresDirectClient(t *testing.T) {
	fwd := New(Options{
		LocalAddress: "127.0.0.1:0",
		PeerAddress:  "127.0.0.1:1",
		TargetHost:   "127.0.0.1",
		TargetPort:   1,
	})
	if err := fwd.Start(context.Background()); err == nil {
		t.Fatal("expected missing direct client error")
	}
}

func TestForwardStartRejectsCanceledContext(t *testing.T) {
	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	fwd := New(Options{
		LocalAddress: "127.0.0.1:0",
		PeerAddress:  "127.0.0.1:1",
		TargetHost:   "127.0.0.1",
		TargetPort:   1,
		DirectClient: client,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := fwd.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if got := fwd.LocalAddress(); got != "" {
		t.Fatalf("expected no listener after canceled start, got %q", got)
	}
}

func TestForwardStopCancelsPendingOpenTCP(t *testing.T) {
	client := newBlockingDirectClient()
	fwd := New(Options{
		LocalAddress: "127.0.0.1:0",
		PeerAddress:  "127.0.0.1:1",
		TargetHost:   "127.0.0.1",
		TargetPort:   1,
		DirectClient: client,
	})
	if err := fwd.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("tcp", fwd.LocalAddress())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	select {
	case <-client.entered:
	case <-time.After(time.Second):
		t.Fatal("expected OpenTCP to start")
	}

	fwd.Stop()
	select {
	case err := <-client.done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected Stop to cancel pending OpenTCP")
	}
}

func TestOpenTCPReturnsTargetDialFailure(t *testing.T) {
	server, control := startDirectServer(t)
	defer server.Close()
	defer control.Close()

	unused := reserveUnusedPort(t)
	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.OpenTCP(ctx, control.Addr().String(), "127.0.0.1", unused)
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected target dial failure")
	}
}

func splitHostPort(t *testing.T, address string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func startDirectServer(t *testing.T) (*direct.Server, net.Listener) {
	t.Helper()
	control, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := direct.NewServer(direct.ServerConfig{DeviceID: "device-b", DeviceName: "desktop-b", Secret: "shared"})
	go func() { _ = server.Serve(control) }()
	return server, control
}

func newEchoTarget(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener
}

func assertRoundTrip(t *testing.T, conn net.Conn) {
	t.Helper()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ping" {
		t.Fatalf("unexpected echo: %q", buf)
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
}

func assertConnClosesSoon(t *testing.T, conn net.Conn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	_, err := conn.Read(b[:])
	if err == nil {
		t.Fatal("expected connection to close")
	}
	if isTimeout(err) {
		t.Fatalf("expected connection to close before deadline, got timeout: %v", err)
	}
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func reserveUnusedPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port := splitHostPort(t, listener.Addr().String())
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

type blockingDirectClient struct {
	entered chan struct{}
	done    chan error
	once    sync.Once
}

func newBlockingDirectClient() *blockingDirectClient {
	return &blockingDirectClient{
		entered: make(chan struct{}),
		done:    make(chan error, 1),
	}
}

func (c *blockingDirectClient) OpenTCP(ctx context.Context, peerAddress, targetHost string, targetPort int) (net.Conn, error) {
	_ = peerAddress
	_ = targetHost
	_ = targetPort
	c.once.Do(func() { close(c.entered) })
	<-ctx.Done()
	c.done <- ctx.Err()
	return nil, ctx.Err()
}
