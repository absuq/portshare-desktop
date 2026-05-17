package manager

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/absuq/portshare-desktop/internal/clash"
	direct "github.com/absuq/portshare-desktop/internal/direct"
	"github.com/absuq/portshare-desktop/internal/direct/store"
	"github.com/absuq/portshare-desktop/internal/linkguardian"
	"github.com/absuq/portshare-desktop/internal/netdiag"
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
	mu          sync.Mutex
	calls       []TrustedPeerAccess
	revokeCalls []TrustedPeerAccess
	err         error
	revokeErr   error
}

func (f *fakeAccessAuthorizer) AllowTrustedPeer(_ context.Context, access TrustedPeerAccess) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, access)
	return f.err
}

func (f *fakeAccessAuthorizer) RevokeTrustedPeer(_ context.Context, access TrustedPeerAccess) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeCalls = append(f.revokeCalls, access)
	return f.revokeErr
}

func (f *fakeAccessAuthorizer) Calls() []TrustedPeerAccess {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]TrustedPeerAccess(nil), f.calls...)
}

func (f *fakeAccessAuthorizer) RevokeCalls() []TrustedPeerAccess {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]TrustedPeerAccess(nil), f.revokeCalls...)
}

type fakeLocalhostBridge struct {
	mu       sync.Mutex
	localIP  string
	peers    []string
	refresh  int
	closed   bool
	active   []int
	refreshC chan struct{}
}

type fakeNetworkDiagnostics struct {
	report       netdiag.PeerPathReport
	reportPeer   string
	applyRequest netdiag.BypassRequest
	active       netdiag.ActiveBypass
	cleared      netdiag.ActiveBypass
	reprobe      netdiag.ReprobeResult
}

func (f *fakeNetworkDiagnostics) DiagnosePeer(_ context.Context, peer string) (netdiag.PeerPathReport, error) {
	f.reportPeer = peer
	return f.report, nil
}

func (f *fakeNetworkDiagnostics) ApplyBypass(_ context.Context, request netdiag.BypassRequest) (netdiag.ActiveBypass, error) {
	f.applyRequest = request
	if f.active.EndpointIP == "" {
		f.active = netdiag.ActiveBypass{
			PeerTailscaleIP: request.PeerTailscaleIP,
			EndpointIP:      request.EndpointIP,
			InterfaceIndex:  request.Candidate.InterfaceIndex,
			NextHop:         request.Candidate.NextHop,
			CreatedAt:       time.Now().UTC(),
		}
	}
	return f.active, nil
}

func (f *fakeNetworkDiagnostics) ClearBypass(_ context.Context, bypass netdiag.ActiveBypass) error {
	f.cleared = bypass
	return nil
}

func (f *fakeNetworkDiagnostics) Reprobe(_ context.Context, request netdiag.ReprobeRequest) netdiag.ReprobeResult {
	if f.reprobe.RestunAttempted || f.reprobe.RebindAttempted {
		return f.reprobe
	}
	return netdiag.ReprobeResult{
		RestunAttempted: request.Restun,
		RebindAttempted: request.Rebind,
	}
}

type fakeClashEgress struct {
	report       clash.DiscoveryReport
	applyRequest clash.ApplyRequest
	applyResult  clash.ApplyResult
	restored     bool
}

func (f *fakeClashEgress) Discover(ctx context.Context) (clash.DiscoveryReport, error) {
	_ = ctx
	return f.report, nil
}

func (f *fakeClashEgress) RefreshNodes(ctx context.Context) (clash.DiscoveryReport, error) {
	_ = ctx
	return f.report, nil
}

func (f *fakeClashEgress) ApplyNode(ctx context.Context, request clash.ApplyRequest) (clash.ApplyResult, error) {
	_ = ctx
	f.applyRequest = request
	if f.applyResult.NodeName == "" {
		f.applyResult = clash.ApplyResult{GroupName: request.GroupName, NodeName: request.NodeName, RouteType: "direct", Latency: "25ms"}
	}
	return f.applyResult, nil
}

func (f *fakeClashEgress) RestoreNode(ctx context.Context) error {
	_ = ctx
	f.restored = true
	return nil
}

func (b *fakeLocalhostBridge) SetLocalTailscaleIP(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.localIP = ip
}

func (b *fakeLocalhostBridge) SetAllowedPeers(peers []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.peers = append([]string(nil), peers...)
}

func (b *fakeLocalhostBridge) Refresh(context.Context) error {
	b.mu.Lock()
	b.refresh++
	refreshC := b.refreshC
	b.mu.Unlock()
	if refreshC != nil {
		select {
		case refreshC <- struct{}{}:
		default:
		}
	}
	return nil
}

func (b *fakeLocalhostBridge) ActivePorts() []int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]int(nil), b.active...)
}

func (b *fakeLocalhostBridge) ConflictPorts() []int {
	return []int{3000}
}

func (b *fakeLocalhostBridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return nil
}

func (b *fakeLocalhostBridge) Snapshot() (string, []string, int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.localIP, append([]string(nil), b.peers...), b.refresh, b.closed
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

func TestStartControlServerConfiguresLocalhostBridge(t *testing.T) {
	mem := NewMemoryPeerStore()
	if err := mem.SavePeers([]store.TrustedPeer{{ID: "device-b", TailscaleIP: "100.109.251.97"}}); err != nil {
		t.Fatal(err)
	}
	bridge := &fakeLocalhostBridge{refreshC: make(chan struct{}, 1)}
	m := New(Config{
		PeerStore:       mem,
		LocalhostBridge: bridge,
		DeviceID:        "device-a",
		DeviceName:      "desktop-a",
	})

	if err := m.StartControlServer(context.Background(), "127.0.0.1:0", "shared"); err != nil {
		t.Fatal(err)
	}
	defer m.StopControlServer(context.Background())

	select {
	case <-bridge.refreshC:
	case <-time.After(time.Second):
		t.Fatal("expected localhost bridge refresh")
	}
	localIP, peers, refresh, closed := bridge.Snapshot()
	if localIP != "127.0.0.1" || len(peers) != 1 || peers[0] != "100.109.251.97" || refresh == 0 || closed {
		t.Fatalf("unexpected bridge state: localIP=%q peers=%+v refresh=%d closed=%v", localIP, peers, refresh, closed)
	}
}

func TestStopControlServerClosesLocalhostBridge(t *testing.T) {
	bridge := &fakeLocalhostBridge{}
	m := New(Config{LocalhostBridge: bridge, DeviceID: "device-a", DeviceName: "desktop-a"})
	if err := m.StartControlServer(context.Background(), "127.0.0.1:0", "shared"); err != nil {
		t.Fatal(err)
	}

	if err := m.StopControlServer(context.Background()); err != nil {
		t.Fatal(err)
	}

	_, _, _, closed := bridge.Snapshot()
	if !closed {
		t.Fatal("expected localhost bridge to close when direct mode stops")
	}
}

func TestSetLocalhostBridgeEnabledClosesAndRestartsBridge(t *testing.T) {
	mem := NewMemoryPeerStore()
	if err := mem.SavePeers([]store.TrustedPeer{{ID: "device-b", TailscaleIP: "100.109.251.97"}}); err != nil {
		t.Fatal(err)
	}
	bridge := &fakeLocalhostBridge{refreshC: make(chan struct{}, 4)}
	m := New(Config{
		PeerStore:       mem,
		LocalhostBridge: bridge,
		DeviceID:        "device-a",
		DeviceName:      "desktop-a",
	})

	if !m.LocalhostBridgeEnabled() {
		t.Fatal("expected localhost bridge to be enabled by default")
	}
	if err := m.StartControlServer(context.Background(), "127.0.0.1:0", "shared"); err != nil {
		t.Fatal(err)
	}
	defer m.StopControlServer(context.Background())
	waitForBridgeRefresh(t, bridge.refreshC)

	if err := m.SetLocalhostBridgeEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if m.LocalhostBridgeEnabled() {
		t.Fatal("expected localhost bridge to be disabled")
	}
	_, _, refreshAfterDisable, closed := bridge.Snapshot()
	if !closed {
		t.Fatal("expected bridge to be closed when disabled")
	}
	if err := m.refreshLocalhostBridge(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _, refreshAfterDisabledRefresh, _ := bridge.Snapshot()
	if refreshAfterDisabledRefresh != refreshAfterDisable {
		t.Fatalf("disabled bridge should not refresh, before=%d after=%d", refreshAfterDisable, refreshAfterDisabledRefresh)
	}

	if err := m.SetLocalhostBridgeEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if !m.LocalhostBridgeEnabled() {
		t.Fatal("expected localhost bridge to be enabled")
	}
	waitForBridgeRefresh(t, bridge.refreshC)
	localIP, peers, refreshAfterEnable, _ := bridge.Snapshot()
	if localIP != "127.0.0.1" || len(peers) != 1 || peers[0] != "100.109.251.97" || refreshAfterEnable <= refreshAfterDisabledRefresh {
		t.Fatalf("unexpected bridge state after enable: localIP=%q peers=%+v refresh=%d", localIP, peers, refreshAfterEnable)
	}
}

func waitForBridgeRefresh(t *testing.T, refreshC <-chan struct{}) {
	t.Helper()
	select {
	case <-refreshC:
	case <-time.After(time.Second):
		t.Fatal("expected localhost bridge refresh")
	}
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

func TestPairPeerRefreshesLocalhostBridgePeers(t *testing.T) {
	mem := NewMemoryPeerStore()
	bridge := &fakeLocalhostBridge{}
	m := New(Config{
		Tailscale: fakeTailscale{report: tailscale.ReadyReport{
			Ready:  true,
			Code:   tailscale.CodeOK,
			Status: tailscale.Status{LocalIPv4: "100.79.83.104"},
		}},
		PairClient: &fakePairClient{peers: map[string]direct.PairedPeer{
			"100.109.251.97:17890": {DeviceID: "device-b", DeviceName: "desktop-b"},
		}},
		PeerStore:       mem,
		LocalhostBridge: bridge,
	})

	if _, err := m.PairPeer(context.Background(), "100.109.251.97:17890"); err != nil {
		t.Fatal(err)
	}

	localIP, peers, refresh, _ := bridge.Snapshot()
	if localIP != "100.79.83.104" || len(peers) != 1 || peers[0] != "100.109.251.97" || refresh == 0 {
		t.Fatalf("unexpected bridge state after pair: localIP=%q peers=%+v refresh=%d", localIP, peers, refresh)
	}
}

func TestRemoveTrustedPeerRevokesFirewallAndRefreshesBridge(t *testing.T) {
	mem := NewMemoryPeerStore()
	if err := mem.SavePeers([]store.TrustedPeer{
		{
			ID:          "device-b",
			DisplayName: "desktop-b",
			TailscaleIP: "100.109.251.97",
		},
		{
			ID:          "device-c",
			DisplayName: "desktop-c",
			TailscaleIP: "100.109.251.98",
		},
	}); err != nil {
		t.Fatal(err)
	}
	authorizer := &fakeAccessAuthorizer{}
	bridge := &fakeLocalhostBridge{}
	m := New(Config{
		Tailscale: fakeTailscale{report: tailscale.ReadyReport{
			Ready:  true,
			Code:   tailscale.CodeOK,
			Status: tailscale.Status{LocalIPv4: "100.79.83.104"},
		}},
		PeerStore:        mem,
		AccessAuthorizer: authorizer,
		LocalhostBridge:  bridge,
	})

	if err := m.RemoveTrustedPeer(context.Background(), "device-b"); err != nil {
		t.Fatal(err)
	}

	revokeCalls := authorizer.RevokeCalls()
	if len(revokeCalls) != 1 {
		t.Fatalf("expected one firewall revoke, got %+v", revokeCalls)
	}
	if revokeCalls[0].RulePrefix != "portshare" ||
		revokeCalls[0].LocalTailscaleIP != "100.79.83.104" ||
		revokeCalls[0].PeerTailscaleIP != "100.109.251.97" ||
		revokeCalls[0].PeerID != "device-b" ||
		revokeCalls[0].PeerName != "desktop-b" {
		t.Fatalf("unexpected revoke access: %+v", revokeCalls[0])
	}

	peers := mem.Peers()
	if len(peers) != 1 || peers[0].ID != "device-c" {
		t.Fatalf("expected remaining peer to be saved, got %+v", peers)
	}
	localIP, allowedPeers, refresh, _ := bridge.Snapshot()
	if localIP != "100.79.83.104" || len(allowedPeers) != 1 || allowedPeers[0] != "100.109.251.98" || refresh == 0 {
		t.Fatalf("unexpected bridge refresh after removal: localIP=%q peers=%+v refresh=%d", localIP, allowedPeers, refresh)
	}
}

func TestRemoveTrustedPeerRejectsInvalidRequests(t *testing.T) {
	if err := New(Config{PeerStore: NewMemoryPeerStore()}).RemoveTrustedPeer(context.Background(), " "); err == nil {
		t.Fatal("expected empty peer id to fail")
	}
	if err := New(Config{}).RemoveTrustedPeer(context.Background(), "device-b"); err == nil {
		t.Fatal("expected missing peer store to fail")
	}

	mem := NewMemoryPeerStore()
	if err := mem.SavePeers([]store.TrustedPeer{{ID: "device-b"}}); err != nil {
		t.Fatal(err)
	}
	if err := New(Config{PeerStore: mem}).RemoveTrustedPeer(context.Background(), "missing"); err == nil {
		t.Fatal("expected missing trusted peer to fail")
	}
	if peers := mem.Peers(); len(peers) != 1 || peers[0].ID != "device-b" {
		t.Fatalf("missing peer removal should not mutate store, got %+v", peers)
	}
}

func TestNetworkPathDelegatesToDiagnostics(t *testing.T) {
	diag := &fakeNetworkDiagnostics{report: netdiag.PeerPathReport{
		PeerTailscaleIP: "100.109.251.97",
		Status:          netdiag.PathDirectProxy,
		EndpointIP:      "115.233.222.82",
	}}
	m := New(Config{NetworkDiagnostics: diag})

	report, err := m.NetworkPath(context.Background(), "100.109.251.97")
	if err != nil {
		t.Fatal(err)
	}
	if diag.reportPeer != "100.109.251.97" || report.EndpointIP != "115.233.222.82" {
		t.Fatalf("unexpected network path delegation: peer=%q report=%+v", diag.reportPeer, report)
	}
}

func TestApplyAndClearNetworkBypassStoresActiveRoute(t *testing.T) {
	diag := &fakeNetworkDiagnostics{}
	m := New(Config{NetworkDiagnostics: diag})
	request := netdiag.BypassRequest{
		PeerTailscaleIP: "100.109.251.97",
		EndpointIP:      "115.233.222.82",
		Candidate: netdiag.EgressCandidate{
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
		},
	}

	active, err := m.ApplyNetworkBypass(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if active.EndpointIP != "115.233.222.82" || diag.applyRequest.EndpointIP != "115.233.222.82" {
		t.Fatalf("unexpected active bypass: active=%+v request=%+v", active, diag.applyRequest)
	}
	stored, ok := m.ActiveNetworkBypass()
	if !ok || stored.EndpointIP != active.EndpointIP {
		t.Fatalf("expected active bypass to be stored, got ok=%v stored=%+v", ok, stored)
	}

	if err := m.ClearNetworkBypass(context.Background()); err != nil {
		t.Fatal(err)
	}
	if diag.cleared.EndpointIP != "115.233.222.82" {
		t.Fatalf("expected stored route to be cleared, got %+v", diag.cleared)
	}
	if _, ok := m.ActiveNetworkBypass(); ok {
		t.Fatal("expected active bypass to be cleared")
	}
}

func TestOptimizeLinkAppliesRecommendedBypassForHighLatencyTUN(t *testing.T) {
	diag := &fakeNetworkDiagnostics{report: netdiag.PeerPathReport{
		PeerTailscaleIP: "100.109.251.97",
		Status:          netdiag.PathDirectProxy,
		RouteType:       netdiag.RouteDirect,
		EndpointIP:      "115.233.222.82",
		Latency:         "249ms",
		Candidates: []netdiag.EgressCandidate{{
			InterfaceAlias: "Ethernet",
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
			Recommended:    true,
		}},
	}}
	m := New(Config{NetworkDiagnostics: diag})

	result, err := m.OptimizeLink(context.Background(), "100.109.251.97", linkguardian.Options{AutoBypass: true})
	if err != nil {
		t.Fatal(err)
	}

	if result.Decision.Action != linkguardian.ActionApplyBypass || result.Decision.Status != linkguardian.StatusBypassApplied {
		t.Fatalf("unexpected optimization result: %+v", result)
	}
	if diag.applyRequest.EndpointIP != "115.233.222.82" || diag.applyRequest.Candidate.InterfaceIndex != 15 {
		t.Fatalf("unexpected bypass request: %+v", diag.applyRequest)
	}
	if stored, ok := m.ActiveNetworkBypass(); !ok || stored.EndpointIP != "115.233.222.82" {
		t.Fatalf("expected active bypass to be stored, ok=%v stored=%+v", ok, stored)
	}
}

func TestClashEgressDelegatesDiscoveryAndNodeSelection(t *testing.T) {
	egress := &fakeClashEgress{report: clash.DiscoveryReport{
		Control: clash.ControlEndpoint{Kind: clash.ControlNamedPipe, Address: `\\.\pipe\verge-mihomo`},
		Nodes: []clash.ProxyNode{{
			GroupName: "GLOBAL",
			Name:      "上海 01",
			Region:    "上海",
		}},
	}}
	m := New(Config{ClashEgress: egress})

	report, err := m.DetectClash(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Control.Kind != clash.ControlNamedPipe {
		t.Fatalf("unexpected discovery report: %+v", report)
	}

	nodes, err := m.RefreshClashNodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes.Nodes) != 1 || nodes.Nodes[0].Name != "上海 01" {
		t.Fatalf("unexpected nodes: %+v", nodes.Nodes)
	}

	result, err := m.ApplyClashNode(context.Background(), clash.ApplyRequest{
		PeerTailscaleIP: "100.109.251.97",
		GroupName:       "GLOBAL",
		NodeName:        "上海 01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if egress.applyRequest.NodeName != "上海 01" || result.Latency != "25ms" {
		t.Fatalf("unexpected apply result: request=%+v result=%+v", egress.applyRequest, result)
	}
	if err := m.RestoreClashNode(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !egress.restored {
		t.Fatal("expected restore to be delegated")
	}
}

func TestProbeTCPConnectLatencyMeasuresReachableListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	latency, err := probeTCPConnectLatency(context.Background(), listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if latency <= 0 {
		t.Fatalf("expected positive latency, got %s", latency)
	}
	<-done
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
