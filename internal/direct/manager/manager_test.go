package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	direct "github.com/absuq/portshare-desktop/internal/direct"
	"github.com/absuq/portshare-desktop/internal/direct/store"
	"github.com/absuq/portshare-desktop/internal/tailscale"
)

type fakeTailscale struct {
	report tailscale.ReadyReport
	route  tailscale.PeerRoute
}

func (f fakeTailscale) CheckReady(context.Context) tailscale.ReadyReport { return f.report }
func (f fakeTailscale) PingPeer(context.Context, string) (tailscale.PeerRoute, error) {
	return f.route, nil
}

type fakePairClient struct {
	mu    sync.Mutex
	peers map[string]direct.PairedPeer
}

func (f *fakePairClient) Pair(ctx context.Context, address string) (direct.PairedPeer, error) {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.peers != nil {
		if peer, ok := f.peers[address]; ok {
			if peer.Address == "" {
				peer.Address = address
			}
			return peer, nil
		}
	}
	return direct.PairedPeer{DeviceID: "device-b", DeviceName: "desktop-b", Address: address}, nil
}

type MemoryPeerStore struct {
	mu    sync.Mutex
	peers []store.TrustedPeer
}

func NewMemoryPeerStore() *MemoryPeerStore { return &MemoryPeerStore{} }
func (s *MemoryPeerStore) LoadPeers() ([]store.TrustedPeer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.TrustedPeer(nil), s.peers...), nil
}
func (s *MemoryPeerStore) SavePeers(peers []store.TrustedPeer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers = append([]store.TrustedPeer(nil), peers...)
	return nil
}
func (s *MemoryPeerStore) Peers() []store.TrustedPeer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.TrustedPeer(nil), s.peers...)
}

func TestRealDirectClientAndStoreSatisfyManagerDependencies(t *testing.T) {
	var _ PairClient = direct.NewClient(direct.ClientConfig{})
	var _ PeerStore = store.New(filepath.Join(t.TempDir(), "peers.json"))
}

func TestStartControlServerRejectsEmptySecret(t *testing.T) {
	m := New(Config{DeviceID: "device-a", DeviceName: "desktop-a"})
	err := m.StartControlServer(context.Background(), "127.0.0.1:0", "")
	if err == nil {
		t.Fatal("expected empty secret to be rejected")
	}
}

func TestStartControlServerPairsWithClientAndStopClosesListener(t *testing.T) {
	m := New(Config{DeviceID: "device-b", DeviceName: "desktop-b"})
	if err := m.StartControlServer(context.Background(), "127.0.0.1:0", "shared"); err != nil {
		t.Fatal(err)
	}
	address := m.ControlAddress()
	if address == "" {
		t.Fatal("expected control address")
	}

	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	peer, err := client.Pair(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	if peer.DeviceID != "device-b" || peer.DeviceName != "desktop-b" {
		t.Fatalf("unexpected peer: %+v", peer)
	}

	if err := m.StopControlServer(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Pair(context.Background(), address); err == nil {
		t.Fatal("expected pair to fail after control server stops")
	}
}

func TestStartControlServerRestartsExistingListener(t *testing.T) {
	m := New(Config{DeviceID: "device-b", DeviceName: "desktop-b"})
	if err := m.StartControlServer(context.Background(), "127.0.0.1:0", "one"); err != nil {
		t.Fatal(err)
	}
	first := m.ControlAddress()
	if err := m.StartControlServer(context.Background(), "127.0.0.1:0", "two"); err != nil {
		t.Fatal(err)
	}
	second := m.ControlAddress()
	if second == "" || second == first {
		t.Fatalf("expected replacement listener, first=%q second=%q", first, second)
	}
	_ = m.StopControlServer(context.Background())
}

func TestStartControlServerCanRestartSameAddress(t *testing.T) {
	m := New(Config{DeviceID: "device-b", DeviceName: "desktop-b"})
	if err := m.StartControlServer(context.Background(), "127.0.0.1:0", "one"); err != nil {
		t.Fatal(err)
	}
	address := m.ControlAddress()
	if err := m.StartControlServer(context.Background(), address, "two"); err != nil {
		t.Fatal(err)
	}
	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "two"})
	if _, err := client.Pair(context.Background(), address); err != nil {
		t.Fatal(err)
	}
	_ = m.StopControlServer(context.Background())
}

func TestReadyUsesTailscaleReport(t *testing.T) {
	m := New(Config{Tailscale: fakeTailscale{report: tailscale.ReadyReport{
		Ready:  true,
		Code:   tailscale.CodeOK,
		Status: tailscale.Status{LocalIPv4: "100.79.83.104"},
	}}})
	state := m.Ready(context.Background())
	if !state.Ready || state.LocalTailscaleIP != "100.79.83.104" {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestPairPeerStoresTrustedPeer(t *testing.T) {
	mem := NewMemoryPeerStore()
	m := New(Config{
		Tailscale: fakeTailscale{report: tailscale.ReadyReport{Ready: true, Code: tailscale.CodeOK}},
		PairClient: &fakePairClient{peers: map[string]direct.PairedPeer{
			"100.109.251.97:17890": {DeviceID: "device-b", DeviceName: "desktop-b"},
		}},
		PeerStore: mem,
	})
	peer, err := m.PairPeer(context.Background(), "100.109.251.97:17890")
	if err != nil {
		t.Fatal(err)
	}
	if peer.DeviceID != "device-b" {
		t.Fatalf("unexpected peer: %+v", peer)
	}
	if len(mem.Peers()) != 1 {
		t.Fatalf("expected one stored peer")
	}
	stored := mem.Peers()[0]
	if stored.TailscaleIP != "100.109.251.97" {
		t.Fatalf("expected tailscale IP without control port, got %+v", stored)
	}
	if stored.FirstPairedAt.IsZero() || stored.LastSeenAt.IsZero() {
		t.Fatalf("expected pairing timestamps, got %+v", stored)
	}
}

func TestPairPeerUpsertsDuplicatePeer(t *testing.T) {
	mem := NewMemoryPeerStore()
	client := &fakePairClient{peers: map[string]direct.PairedPeer{
		"100.109.251.97:17890": {DeviceID: "device-b", DeviceName: "desktop-b"},
		"100.109.251.98:17890": {DeviceID: "device-b", DeviceName: "desktop-b-renamed"},
	}}
	m := New(Config{PairClient: client, PeerStore: mem})

	if _, err := m.PairPeer(context.Background(), "100.109.251.97:17890"); err != nil {
		t.Fatal(err)
	}
	first := mem.Peers()[0].FirstPairedAt
	time.Sleep(10 * time.Millisecond)

	if _, err := m.PairPeer(context.Background(), "100.109.251.98:17890"); err != nil {
		t.Fatal(err)
	}
	peers := mem.Peers()
	if len(peers) != 1 {
		t.Fatalf("expected duplicate peer to be updated in place, got %+v", peers)
	}
	if peers[0].DisplayName != "desktop-b-renamed" || peers[0].TailscaleIP != "100.109.251.98" {
		t.Fatalf("expected updated peer metadata, got %+v", peers[0])
	}
	if !peers[0].FirstPairedAt.Equal(first) {
		t.Fatalf("expected first paired time to be preserved, got %v want %v", peers[0].FirstPairedAt, first)
	}
}

func TestPairPeerSerializesStoreUpdates(t *testing.T) {
	store := newRacingPeerStore()
	client := &fakePairClient{peers: map[string]direct.PairedPeer{
		"100.109.251.97:17890": {DeviceID: "device-a", DeviceName: "desktop-a"},
		"100.109.251.98:17890": {DeviceID: "device-b", DeviceName: "desktop-b"},
	}}
	m := New(Config{PairClient: client, PeerStore: store})

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, address := range []string{"100.109.251.97:17890", "100.109.251.98:17890"} {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			_, err := m.PairPeer(context.Background(), address)
			errs <- err
		}(address)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	peers := store.Peers()
	if len(peers) != 2 {
		t.Fatalf("expected both concurrent pairings to be stored, got %+v", peers)
	}
}

func TestCreateForwardRejectsUnknownPeer(t *testing.T) {
	m := New(Config{PeerStore: NewMemoryPeerStore(), DirectClient: fakeOpenTCPClient{}})
	_, err := m.CreateForward(context.Background(), ForwardRequest{
		PeerID:       "missing",
		TargetHost:   "127.0.0.1",
		TargetPort:   3000,
		LocalAddress: "127.0.0.1:0",
	})
	if err == nil {
		t.Fatalf("expected unknown peer error")
	}
}

func TestCreateForwardRequiresDirectClient(t *testing.T) {
	mem := NewMemoryPeerStore()
	if err := mem.SavePeers([]store.TrustedPeer{{ID: "device-b", TailscaleIP: "100.79.83.104"}}); err != nil {
		t.Fatal(err)
	}
	m := New(Config{PeerStore: mem})
	_, err := m.CreateForward(context.Background(), ForwardRequest{
		PeerID:       "device-b",
		TargetHost:   "127.0.0.1",
		TargetPort:   3000,
		LocalAddress: "127.0.0.1:0",
	})
	if err == nil || !strings.Contains(err.Error(), "direct client") {
		t.Fatalf("expected direct client dependency error, got %v", err)
	}
}

func TestCreateForwardStartsRealForwardAndStopClosesIt(t *testing.T) {
	target := newEchoTarget(t)
	server, control := startDirectServer(t)
	defer server.Close()
	defer control.Close()
	targetHost, targetPort := splitHostPort(t, target.Addr().String())

	mem := NewMemoryPeerStore()
	if err := mem.SavePeers([]store.TrustedPeer{{
		ID:          "device-b",
		DisplayName: "desktop-b",
		TailscaleIP: control.Addr().String(),
	}}); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		PeerStore: mem,
		DirectClient: direct.NewClient(direct.ClientConfig{
			DeviceID:   "device-a",
			DeviceName: "desktop-a",
			Secret:     "shared",
		}),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	running, err := m.CreateForward(ctx, ForwardRequest{
		PeerID:       "device-b",
		TargetHost:   targetHost,
		TargetPort:   targetPort,
		LocalAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if running.LocalAddress == "" || strings.HasSuffix(running.LocalAddress, ":0") {
		t.Fatalf("expected actual listener address, got %+v", running)
	}

	conn, err := net.Dial("tcp", running.LocalAddress)
	if err != nil {
		t.Fatal(err)
	}
	assertRoundTrip(t, conn)
	_ = conn.Close()

	if err := m.StopForward(context.Background(), running.ID); err != nil {
		t.Fatal(err)
	}
	conn, err = net.DialTimeout("tcp", running.LocalAddress, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("expected stopped forward at %s to reject new connections", running.LocalAddress)
	}
}

func TestCreateForwardSurvivesRequestContextCancellation(t *testing.T) {
	target := newEchoTarget(t)
	server, control := startDirectServer(t)
	defer server.Close()
	defer control.Close()
	targetHost, targetPort := splitHostPort(t, target.Addr().String())

	mem := NewMemoryPeerStore()
	if err := mem.SavePeers([]store.TrustedPeer{{
		ID:          "device-b",
		DisplayName: "desktop-b",
		TailscaleIP: control.Addr().String(),
	}}); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		PeerStore: mem,
		DirectClient: direct.NewClient(direct.ClientConfig{
			DeviceID:   "device-a",
			DeviceName: "desktop-a",
			Secret:     "shared",
		}),
	})

	reqCtx, cancelReq := context.WithCancel(context.Background())
	running, err := m.CreateForward(reqCtx, ForwardRequest{
		PeerID:       "device-b",
		TargetHost:   targetHost,
		TargetPort:   targetPort,
		LocalAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.StopForward(context.Background(), running.ID) }()
	cancelReq()
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", running.LocalAddress)
	if err != nil {
		t.Fatalf("expected forward to survive request context cancellation: %v", err)
	}
	defer conn.Close()
	assertRoundTrip(t, conn)
}

func TestStopForwardRejectsUnknownForward(t *testing.T) {
	m := New(Config{})
	err := m.StopForward(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected unknown forward error")
	}
}

type racingPeerStore struct {
	mu             sync.Mutex
	peers          []store.TrustedPeer
	loads          int
	secondLoadSeen chan struct{}
}

func newRacingPeerStore() *racingPeerStore {
	return &racingPeerStore{secondLoadSeen: make(chan struct{})}
}

func (s *racingPeerStore) LoadPeers() ([]store.TrustedPeer, error) {
	s.mu.Lock()
	s.loads++
	loads := s.loads
	if loads == 2 {
		close(s.secondLoadSeen)
	}
	peers := append([]store.TrustedPeer(nil), s.peers...)
	s.mu.Unlock()

	if loads == 1 {
		select {
		case <-s.secondLoadSeen:
		case <-time.After(50 * time.Millisecond):
		}
	}
	return peers, nil
}

func (s *racingPeerStore) SavePeers(peers []store.TrustedPeer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers = append([]store.TrustedPeer(nil), peers...)
	return nil
}

func (s *racingPeerStore) Peers() []store.TrustedPeer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.TrustedPeer(nil), s.peers...)
}

type fakeOpenTCPClient struct{}

func (fakeOpenTCPClient) OpenTCP(context.Context, string, string, int) (net.Conn, error) {
	return nil, errors.New("unused")
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
}

func TestPeerControlAddressAddsDefaultPort(t *testing.T) {
	peer := store.TrustedPeer{ID: "device-b", TailscaleIP: "100.79.83.104"}
	if got := peerControlAddress(peer, 17890); got != "100.79.83.104:17890" {
		t.Fatalf("unexpected peer control address: %s", got)
	}

	peer.TailscaleIP = "127.0.0.1:34567"
	if got := peerControlAddress(peer, 17890); got != "127.0.0.1:34567" {
		t.Fatalf("unexpected explicit peer control address: %s", got)
	}
}

func TestPairPeerReturnsStoreErrors(t *testing.T) {
	m := New(Config{
		PairClient: &fakePairClient{},
		PeerStore:  failingPeerStore{err: fmt.Errorf("load failed")},
	})
	if _, err := m.PairPeer(context.Background(), "100.109.251.97:17890"); err == nil {
		t.Fatal("expected store error")
	}
}

type failingPeerStore struct {
	err error
}

func (s failingPeerStore) LoadPeers() ([]store.TrustedPeer, error) { return nil, s.err }
func (s failingPeerStore) SavePeers([]store.TrustedPeer) error     { return s.err }
