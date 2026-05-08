package manager

import (
	"context"
	"fmt"
	"path/filepath"
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

type fakeAccessAuthorizer struct {
	mu    sync.Mutex
	calls []TrustedPeerAccess
	err   error
}

func (f *fakeAccessAuthorizer) AllowTrustedPeer(_ context.Context, access TrustedPeerAccess) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, access)
	return f.err
}

func (f *fakeAccessAuthorizer) Calls() []TrustedPeerAccess {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]TrustedPeerAccess(nil), f.calls...)
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

func TestPairPeerAuthorizesTrustedPeerAccess(t *testing.T) {
	mem := NewMemoryPeerStore()
	authorizer := &fakeAccessAuthorizer{}
	m := New(Config{
		Tailscale: fakeTailscale{report: tailscale.ReadyReport{
			Ready:  true,
			Code:   tailscale.CodeOK,
			Status: tailscale.Status{LocalIPv4: "100.79.83.104"},
		}},
		PairClient: &fakePairClient{peers: map[string]direct.PairedPeer{
			"100.109.251.97:17890": {DeviceID: "device-b", DeviceName: "desktop-b"},
		}},
		PeerStore:        mem,
		AccessAuthorizer: authorizer,
	})

	if _, err := m.PairPeer(context.Background(), "100.109.251.97:17890"); err != nil {
		t.Fatal(err)
	}

	calls := authorizer.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected one firewall authorization, got %+v", calls)
	}
	if calls[0].LocalTailscaleIP != "100.79.83.104" || calls[0].PeerTailscaleIP != "100.109.251.97" {
		t.Fatalf("unexpected access request: %+v", calls[0])
	}
	peers := mem.Peers()
	if len(peers) != 1 || peers[0].AccessAuthorizedAt.IsZero() {
		t.Fatalf("expected stored peer to record access authorization, got %+v", peers)
	}
}

func TestAuthenticatedIncomingPeerIsStoredAndAuthorized(t *testing.T) {
	mem := NewMemoryPeerStore()
	authorizer := &fakeAccessAuthorizer{}
	m := New(Config{
		PeerStore:        mem,
		AccessAuthorizer: authorizer,
		DeviceID:         "device-b",
		DeviceName:       "desktop-b",
	})
	if err := m.StartControlServer(context.Background(), "127.0.0.1:0", "shared"); err != nil {
		t.Fatal(err)
	}
	defer m.StopControlServer(context.Background())

	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	if _, err := client.Pair(context.Background(), m.ControlAddress()); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		if len(mem.Peers()) == 1 && len(authorizer.Calls()) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected incoming peer to be stored and authorized, peers=%+v calls=%+v", mem.Peers(), authorizer.Calls())
		}
		time.Sleep(10 * time.Millisecond)
	}
	peer := mem.Peers()[0]
	if peer.ID != "device-a" || peer.DisplayName != "desktop-a" || peer.TailscaleIP != "127.0.0.1" {
		t.Fatalf("unexpected incoming trusted peer: %+v", peer)
	}
	if peer.AccessAuthorizedAt.IsZero() {
		t.Fatalf("expected incoming peer authorization timestamp, got %+v", peer)
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
