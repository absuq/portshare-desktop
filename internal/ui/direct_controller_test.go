package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/absuq/portshare-desktop/internal/clash"
	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/direct/store"
	"github.com/absuq/portshare-desktop/internal/netdiag"
	"github.com/absuq/portshare-desktop/internal/tailscale"
)

type fakeDirectManager struct {
	ready              directmanager.ReadyState
	readyCalls         int
	peers              []directmanager.TrustedPeer
	started            bool
	startListenAddress string
	startSecret        string
	pairAddress        string
	trustedErr         error
	pairErr            error
	networkReport      netdiag.PeerPathReport
	networkPeer        string
	applyRequest       netdiag.BypassRequest
	activeBypass       netdiag.ActiveBypass
	clearBypassCalled  bool
	clashReport        clash.DiscoveryReport
	clashApplyRequest  clash.ApplyRequest
	clashApplyResult   clash.ApplyResult
	clashRestored      bool
	peerLatencies      map[string]time.Duration
	peerLatencyErr     error
}

func (f *fakeDirectManager) Ready(context.Context) directmanager.ReadyState {
	f.readyCalls++
	return f.ready
}

func (f *fakeDirectManager) StartControlServer(_ context.Context, listenAddress string, secret string) error {
	f.started = true
	f.startListenAddress = listenAddress
	f.startSecret = secret
	return nil
}

func (f *fakeDirectManager) StopControlServer(context.Context) error {
	f.started = false
	return nil
}

func (f *fakeDirectManager) ControlAddress() string {
	if !f.started {
		return ""
	}
	return f.startListenAddress
}

func (f *fakeDirectManager) PairPeer(_ context.Context, address string) (directmanager.PairedPeer, error) {
	f.pairAddress = address
	if f.pairErr != nil {
		return directmanager.PairedPeer{}, f.pairErr
	}
	peer := directmanager.TrustedPeer{ID: "device-b", DisplayName: "desktop-b", TailscaleIP: "100.109.251.97"}
	f.peers = upsertFakePeer(f.peers, peer)
	return directmanager.PairedPeer{DeviceID: "device-b", DeviceName: "desktop-b", Address: address}, nil
}

func (f *fakeDirectManager) TrustedPeers(context.Context) ([]directmanager.TrustedPeer, error) {
	if f.trustedErr != nil {
		return nil, f.trustedErr
	}
	return append([]directmanager.TrustedPeer(nil), f.peers...), nil
}

func (f *fakeDirectManager) LocalhostBridgePorts() []int {
	return []int{18789, 3000}
}

func (f *fakeDirectManager) LocalhostBridgeConflictPorts() []int {
	return []int{3000}
}

func (f *fakeDirectManager) NetworkPath(_ context.Context, peer string) (netdiag.PeerPathReport, error) {
	f.networkPeer = peer
	return f.networkReport, nil
}

func (f *fakeDirectManager) ApplyNetworkBypass(_ context.Context, request netdiag.BypassRequest) (netdiag.ActiveBypass, error) {
	f.applyRequest = request
	if f.activeBypass.EndpointIP == "" {
		f.activeBypass = netdiag.ActiveBypass{
			PeerTailscaleIP: request.PeerTailscaleIP,
			EndpointIP:      request.EndpointIP,
			InterfaceIndex:  request.Candidate.InterfaceIndex,
			NextHop:         request.Candidate.NextHop,
			CreatedAt:       time.Now().UTC(),
		}
	}
	return f.activeBypass, nil
}

func (f *fakeDirectManager) ClearNetworkBypass(context.Context) error {
	f.clearBypassCalled = true
	f.activeBypass = netdiag.ActiveBypass{}
	return nil
}

func (f *fakeDirectManager) ActiveNetworkBypass() (netdiag.ActiveBypass, bool) {
	if f.activeBypass.EndpointIP == "" {
		return netdiag.ActiveBypass{}, false
	}
	return f.activeBypass, true
}

func (f *fakeDirectManager) ProbePeerLatency(_ context.Context, peerIP string) (time.Duration, error) {
	if f.peerLatencyErr != nil {
		return 0, f.peerLatencyErr
	}
	if f.peerLatencies != nil {
		if latency, ok := f.peerLatencies[peerIP]; ok {
			return latency, nil
		}
	}
	return 0, errors.New("latency unavailable")
}

func (f *fakeDirectManager) DetectClash(context.Context) (clash.DiscoveryReport, error) {
	return f.clashReport, nil
}

func (f *fakeDirectManager) RefreshClashNodes(context.Context) (clash.DiscoveryReport, error) {
	return f.clashReport, nil
}

func (f *fakeDirectManager) ApplyClashNode(_ context.Context, request clash.ApplyRequest) (clash.ApplyResult, error) {
	f.clashApplyRequest = request
	if f.clashApplyResult.NodeName == "" {
		f.clashApplyResult = clash.ApplyResult{
			GroupName: request.GroupName,
			NodeName:  request.NodeName,
			RouteType: "direct",
			Latency:   "25ms",
		}
	}
	return f.clashApplyResult, nil
}

func (f *fakeDirectManager) RestoreClashNode(context.Context) error {
	f.clashRestored = true
	return nil
}

func TestDirectControllerRefreshShowsReadyState(t *testing.T) {
	mgr := &fakeDirectManager{ready: directmanager.ReadyState{Ready: true, LocalTailscaleIP: "100.79.83.104", Code: tailscale.CodeOK}}
	ctrl := NewDirectController(mgr)

	if err := ctrl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	if !state.Ready || state.LocalTailscaleIP != "100.79.83.104" {
		t.Fatalf("unexpected state: %+v", state)
	}
	if len(state.Peers) != 0 {
		t.Fatalf("expected empty peers, got %+v", state)
	}
	if len(state.LocalhostBridgePorts) != 2 || state.LocalhostBridgePorts[0] != 18789 {
		t.Fatalf("expected localhost bridge ports in state, got %+v", state.LocalhostBridgePorts)
	}
	if len(state.LocalhostBridgeConflictPorts) != 1 || state.LocalhostBridgeConflictPorts[0] != 3000 {
		t.Fatalf("expected localhost bridge conflict ports in state, got %+v", state.LocalhostBridgeConflictPorts)
	}
}

func TestDirectControllerDetectNetworkPathUpdatesState(t *testing.T) {
	mgr := &fakeDirectManager{networkReport: netdiag.PeerPathReport{
		PeerTailscaleIP: "100.109.251.97",
		Status:          netdiag.PathDirectProxy,
		Endpoint:        "115.233.222.82:41641",
		EndpointIP:      "115.233.222.82",
		Latency:         "249ms",
		CurrentRoute:    netdiag.RouteInfo{InterfaceAlias: "Meta", NextHop: "198.18.0.2"},
		Candidates: []netdiag.EgressCandidate{{
			InterfaceAlias: "以太网",
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
			Recommended:    true,
		}},
	}}
	ctrl := NewDirectController(mgr)

	if err := ctrl.DetectNetworkPath(context.Background(), "100.109.251.97"); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	if mgr.networkPeer != "100.109.251.97" || state.NetworkPath.Status != netdiag.PathDirectProxy {
		t.Fatalf("unexpected network state: peer=%q state=%+v", mgr.networkPeer, state.NetworkPath)
	}
	if !strings.Contains(state.Message, "直连但疑似代理绕路") {
		t.Fatalf("expected proxy warning message, got %q", state.Message)
	}
}

func TestDirectControllerApplyAndClearNetworkBypass(t *testing.T) {
	mgr := &fakeDirectManager{}
	ctrl := NewDirectController(mgr)
	ctrl.state.NetworkPath = netdiag.PeerPathReport{
		PeerTailscaleIP: "100.109.251.97",
		EndpointIP:      "115.233.222.82",
		Candidates: []netdiag.EgressCandidate{{
			InterfaceAlias: "以太网",
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
		}},
	}

	if err := ctrl.ApplyNetworkBypass(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if mgr.applyRequest.EndpointIP != "115.233.222.82" || mgr.applyRequest.Candidate.InterfaceIndex != 15 {
		t.Fatalf("unexpected bypass request: %+v", mgr.applyRequest)
	}
	if !ctrl.State().HasActiveBypass {
		t.Fatal("expected active bypass in state")
	}

	if err := ctrl.ClearNetworkBypass(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !mgr.clearBypassCalled || ctrl.State().HasActiveBypass {
		t.Fatalf("expected bypass to be cleared, called=%v state=%+v", mgr.clearBypassCalled, ctrl.State())
	}
}

func TestDirectControllerClashActionsUpdateState(t *testing.T) {
	mgr := &fakeDirectManager{clashReport: clash.DiscoveryReport{
		Control: clash.ControlEndpoint{Kind: clash.ControlNamedPipe, Address: `\\.\pipe\verge-mihomo`},
		Nodes: []clash.ProxyNode{{
			GroupName: "GLOBAL",
			Name:      "上海 01",
			Region:    "上海",
			Delay:     23 * time.Millisecond,
			Current:   true,
		}},
	}}
	ctrl := NewDirectController(mgr)

	if err := ctrl.DetectClash(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ctrl.State().ClashReport.Control.Kind != clash.ControlNamedPipe {
		t.Fatalf("expected clash report in state, got %+v", ctrl.State().ClashReport)
	}
	if err := ctrl.RefreshClashNodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(ctrl.State().ClashReport.Nodes) != 1 {
		t.Fatalf("expected node list in state, got %+v", ctrl.State().ClashReport.Nodes)
	}
	if err := ctrl.ApplyClashNode(context.Background(), "100.109.251.97", 0); err != nil {
		t.Fatal(err)
	}
	if mgr.clashApplyRequest.NodeName != "上海 01" || ctrl.State().ClashApplyResult.Latency != "25ms" {
		t.Fatalf("unexpected clash apply: request=%+v state=%+v", mgr.clashApplyRequest, ctrl.State().ClashApplyResult)
	}
	if err := ctrl.RestoreClashNode(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !mgr.clashRestored {
		t.Fatal("expected restore call")
	}
}

func TestDirectControllerApplyClashNodeRequiresPeer(t *testing.T) {
	ctrl := NewDirectController(&fakeDirectManager{clashReport: clash.DiscoveryReport{Nodes: []clash.ProxyNode{{GroupName: "GLOBAL", Name: "上海 01"}}}})

	if err := ctrl.ApplyClashNode(context.Background(), "", 0); err == nil {
		t.Fatal("expected missing peer to fail")
	}
}

func TestDirectControllerPairNormalizesAddressAndRefreshesPeers(t *testing.T) {
	mgr := &fakeDirectManager{}
	ctrl := NewDirectController(mgr)

	if err := ctrl.PairPeer(context.Background(), "100.109.251.97"); err != nil {
		t.Fatal(err)
	}

	if mgr.pairAddress != "100.109.251.97:17890" {
		t.Fatalf("expected default control port, got %q", mgr.pairAddress)
	}
	state := ctrl.State()
	if len(state.Peers) != 1 || state.Peers[0].ID != "device-b" {
		t.Fatalf("unexpected peers: %+v", state.Peers)
	}
}

func TestDirectControllerPairKeepsExplicitPort(t *testing.T) {
	mgr := &fakeDirectManager{}
	ctrl := NewDirectController(mgr)

	if err := ctrl.PairPeer(context.Background(), "100.109.251.97:19000"); err != nil {
		t.Fatal(err)
	}

	if mgr.pairAddress != "100.109.251.97:19000" {
		t.Fatalf("expected explicit port to be preserved, got %q", mgr.pairAddress)
	}
}

func TestDirectControllerPairAcceptsHostnameAndIPv6(t *testing.T) {
	cases := map[string]string{
		"desktop-b.tail51fe78.ts.net": "desktop-b.tail51fe78.ts.net:17890",
		"fd7a:115c:a1e0::1":           "[fd7a:115c:a1e0::1]:17890",
		"[fd7a:115c:a1e0::1]:19000":   "[fd7a:115c:a1e0::1]:19000",
	}
	for input, want := range cases {
		mgr := &fakeDirectManager{}
		ctrl := NewDirectController(mgr)

		if err := ctrl.PairPeer(context.Background(), input); err != nil {
			t.Fatalf("PairPeer(%q) returned error: %v", input, err)
		}
		if mgr.pairAddress != want {
			t.Fatalf("PairPeer(%q) address = %q, want %q", input, mgr.pairAddress, want)
		}
	}
}

func TestDirectControllerPairRejectsBadPort(t *testing.T) {
	for _, input := range []string{"100.109.251.97:abc", "100.109.251.97:", "100.109.251.97:70000"} {
		ctrl := NewDirectController(&fakeDirectManager{})
		if err := ctrl.PairPeer(context.Background(), input); !errors.Is(err, ErrDirectPeerAddressInvalid) {
			t.Fatalf("PairPeer(%q) expected ErrDirectPeerAddressInvalid, got %v", input, err)
		}
	}
}

func TestDirectControllerStartsControlServerWithSecret(t *testing.T) {
	mgr := &fakeDirectManager{ready: directmanager.ReadyState{Ready: true, LocalTailscaleIP: "100.79.83.104"}}
	ctrl := NewDirectController(mgr)

	if err := ctrl.StartDirectMode(context.Background(), "shared-secret", "100.79.83.104:17890"); err != nil {
		t.Fatal(err)
	}

	if !mgr.started || mgr.startListenAddress != "100.79.83.104:17890" || mgr.startSecret == "" {
		t.Fatalf("expected control server to start, got started=%v address=%q secretPresent=%v", mgr.started, mgr.startListenAddress, mgr.startSecret != "")
	}
}

func TestDirectControllerStartDirectModeShowsListeningState(t *testing.T) {
	mgr := &fakeDirectManager{ready: directmanager.ReadyState{Ready: true, LocalTailscaleIP: "100.79.83.104"}}
	ctrl := NewDirectController(mgr)

	if err := ctrl.StartDirectMode(context.Background(), "shared-secret", "100.79.83.104:17890"); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	if !state.ControlListening || state.ControlAddress != "100.79.83.104:17890" {
		t.Fatalf("expected listening state, got listening=%v address=%q", state.ControlListening, state.ControlAddress)
	}
	if !strings.Contains(state.Message, "直连监听已启动：100.79.83.104:17890") {
		t.Fatalf("expected direct listening success message, got %q", state.Message)
	}
}

func TestDirectControllerStopDirectModeClearsListeningState(t *testing.T) {
	mgr := &fakeDirectManager{ready: directmanager.ReadyState{Ready: true, LocalTailscaleIP: "100.79.83.104"}}
	ctrl := NewDirectController(mgr)

	if err := ctrl.StartDirectMode(context.Background(), "shared-secret", "100.79.83.104:17890"); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.StopDirectMode(context.Background()); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	if state.ControlListening || state.ControlAddress != "" {
		t.Fatalf("expected listening state to be cleared, got listening=%v address=%q", state.ControlListening, state.ControlAddress)
	}
}

func TestDirectControllerStartDirectModeSucceedsWhenRefreshFails(t *testing.T) {
	mgr := &fakeDirectManager{
		ready:      directmanager.ReadyState{Ready: true, LocalTailscaleIP: "100.79.83.104"},
		trustedErr: errors.New("store unavailable"),
	}
	ctrl := NewDirectController(mgr)

	if err := ctrl.StartDirectMode(context.Background(), "shared-secret", "100.79.83.104:17890"); err != nil {
		t.Fatal(err)
	}
	if !mgr.started {
		t.Fatal("expected control server to be started")
	}
	if ctrl.State().Message == "" {
		t.Fatal("expected warning or success message after refresh failure")
	}
}

func TestDirectControllerPairPeerSucceedsWhenRefreshFails(t *testing.T) {
	mgr := &fakeDirectManager{trustedErr: errors.New("store unavailable")}
	ctrl := NewDirectController(mgr)

	if err := ctrl.PairPeer(context.Background(), "100.109.251.97"); err != nil {
		t.Fatal(err)
	}
	if mgr.pairAddress == "" {
		t.Fatal("expected pair to be attempted")
	}
	if ctrl.State().Message == "" {
		t.Fatal("expected warning or success message after refresh failure")
	}
}

func TestDirectControllerPairPeerKeepsAuthorizedSuccessMessageAfterRefresh(t *testing.T) {
	mgr := &fakeDirectManager{ready: directmanager.ReadyState{Ready: true, LocalTailscaleIP: "100.79.83.104"}}
	ctrl := NewDirectController(mgr)

	if err := ctrl.PairPeer(context.Background(), "100.109.251.97"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(ctrl.State().Message, "已配对并授权全端口访问：desktop-b") {
		t.Fatalf("expected authorized pairing success message to remain visible, got %q", ctrl.State().Message)
	}
}

func TestDirectControllerPairPeerExplainsConnectionRefused(t *testing.T) {
	mgr := &fakeDirectManager{
		pairErr: errors.New("dial tcp 100.79.83.104:17890: connectex: No connection could be made because the target machine actively refused it"),
	}
	ctrl := NewDirectController(mgr)

	err := ctrl.PairPeer(context.Background(), "100.79.83.104")
	if err == nil {
		t.Fatal("expected pairing error")
	}
	if !strings.Contains(err.Error(), "对方 100.79.83.104:17890 没有接受 portshare 直连连接") {
		t.Fatalf("expected actionable connection refused message, got %v", err)
	}
	if !strings.Contains(err.Error(), "启用直连密钥") {
		t.Fatalf("expected message to mention enabling direct key, got %v", err)
	}
	if !strings.Contains(ctrl.State().Message, "配对失败：对方 100.79.83.104:17890") {
		t.Fatalf("expected state message to include friendly pairing failure, got %q", ctrl.State().Message)
	}
}

func TestDirectControllerPairPeerWithSecretStartsControlServerBeforePairing(t *testing.T) {
	mgr := &fakeDirectManager{}
	ctrl := NewDirectController(mgr)

	if err := ctrl.PairPeerWithSecret(context.Background(), "100.109.251.97", "shared-secret", "100.79.83.104:17890"); err != nil {
		t.Fatal(err)
	}

	if !mgr.started {
		t.Fatal("expected direct mode to be started before pairing")
	}
	if mgr.startListenAddress != "100.79.83.104:17890" || mgr.startSecret != "shared-secret" {
		t.Fatalf("unexpected direct start request: address=%q secret=%q", mgr.startListenAddress, mgr.startSecret)
	}
	if mgr.pairAddress != "100.109.251.97:17890" {
		t.Fatalf("expected pair to use default control port, got %q", mgr.pairAddress)
	}
}

func TestDirectControllerPairPeerWithSecretRejectsMissingSecretBeforePairing(t *testing.T) {
	mgr := &fakeDirectManager{}
	ctrl := NewDirectController(mgr)

	err := ctrl.PairPeerWithSecret(context.Background(), "100.109.251.97", " ", "100.79.83.104:17890")
	if !errors.Is(err, ErrDirectSecretRequired) {
		t.Fatalf("expected ErrDirectSecretRequired, got %v", err)
	}
	if mgr.started || mgr.pairAddress != "" {
		t.Fatalf("expected no start or pair without secret, started=%v pairAddress=%q", mgr.started, mgr.pairAddress)
	}
}

func TestDirectControllerStateIsImmutableSnapshot(t *testing.T) {
	mgr := &fakeDirectManager{
		peers: []directmanager.TrustedPeer{{ID: "device-b", DisplayName: "desktop-b", TailscaleIP: "100.109.251.97"}},
	}
	ctrl := NewDirectController(mgr)
	if err := ctrl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	state.Peers[0].ID = "mutated-peer"

	next := ctrl.State()
	if next.Peers[0].ID != "device-b" {
		t.Fatalf("state mutation leaked into controller: %+v", next)
	}
}

func TestAppNewInitializesDirectController(t *testing.T) {
	mgr := &fakeDirectManager{}
	app := New(Dependencies{DirectManager: mgr})

	if app.directCtrl == nil {
		t.Fatal("expected direct controller to be initialized")
	}
	if app.ctrl == nil {
		t.Fatal("expected legacy controller to remain available")
	}
}

func TestDependenciesRequireDirectManagerForApp(t *testing.T) {
	var _ DirectManager = &fakeDirectManager{}
	var _ DirectManager = directmanager.New(directmanager.Config{})
}

func TestDirectControllerRejectsMissingInputs(t *testing.T) {
	ctrl := NewDirectController(&fakeDirectManager{})

	if err := ctrl.StartDirectMode(context.Background(), "", "100.79.83.104:17890"); !errors.Is(err, ErrDirectSecretRequired) {
		t.Fatalf("expected ErrDirectSecretRequired, got %v", err)
	}
	if err := ctrl.PairPeer(context.Background(), " "); !errors.Is(err, ErrDirectPeerAddressRequired) {
		t.Fatalf("expected ErrDirectPeerAddressRequired, got %v", err)
	}
}

func TestGeneratePairingSecretReturnsShareableSecret(t *testing.T) {
	secret, err := generatePairingSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != 24 {
		t.Fatalf("expected grouped 20 character secret, got %q", secret)
	}
	if secret[4] != '-' || secret[9] != '-' || secret[14] != '-' || secret[19] != '-' {
		t.Fatalf("expected hyphenated secret, got %q", secret)
	}
	for _, r := range secret {
		if r == '-' {
			continue
		}
		if r < 'A' || r > 'Z' {
			t.Fatalf("expected uppercase shareable characters, got %q in %q", r, secret)
		}
	}
}

func TestPairSuccessDialogMessageUsesPairedState(t *testing.T) {
	state := DirectState{Message: "已配对并授权全端口访问：desktop-b"}
	if got := pairSuccessDialogMessage(state); got != "已配对并授权全端口访问：desktop-b" {
		t.Fatalf("expected paired state message, got %q", got)
	}
}

func TestPairSuccessDialogMessageFallsBack(t *testing.T) {
	state := DirectState{Message: "Tailscale 已就绪"}
	if got := pairSuccessDialogMessage(state); got != "配对成功，已授权对方 Tailscale IP 访问本机全端口。" {
		t.Fatalf("expected fallback success message, got %q", got)
	}
}

func TestCompactStatusSummaryTextKeepsTopBarShort(t *testing.T) {
	state := DirectState{
		Ready:            true,
		LocalTailscaleIP: "100.79.83.104",
		ControlListening: true,
		ControlAddress:   "100.79.83.104:17890",
		HasActiveBypass:  true,
		ActiveBypass:     netdiag.ActiveBypass{EndpointIP: "115.233.222.82", NextHop: "192.168.1.1"},
		ClashApplyResult: clash.ApplyResult{NodeName: "上海 01", RouteType: "direct", Latency: "25ms"},
	}

	got := compactStatusSummaryText(state)
	for _, verbose := range []string{"localhost", "代理入口", "控制接口", "endpoint"} {
		if strings.Contains(got, verbose) {
			t.Fatalf("top summary should not include verbose diagnostics %q: %q", verbose, got)
		}
	}
	for _, want := range []string{"Tailscale：ready", "IP 100.79.83.104", "监听中", "出口 上海 01 direct 25ms"} {
		if !strings.Contains(got, want) {
			t.Fatalf("top summary missing %q: %q", want, got)
		}
	}
}

func TestActiveBypassStatusTextShowsIPv6HostRoute(t *testing.T) {
	state := DirectState{
		HasActiveBypass: true,
		ActiveBypass: netdiag.ActiveBypass{
			EndpointIP:    "2401:b60:1b::1033",
			AddressFamily: netdiag.AddressFamilyIPv6,
			NextHop:       "fe80::1",
		},
	}

	got := activeBypassStatusText(state)
	for _, want := range []string{"IPv6", "2401:b60:1b::1033/128", "fe80::1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected active bypass text to contain %q, got %q", want, got)
		}
	}
}

func TestScrollPageAllowsBothAxisOverflowInsideTabs(t *testing.T) {
	page := scrollPage(widget.NewLabel(strings.Repeat("very-long-node-name-", 20)))

	if page.Direction != container.ScrollBoth {
		t.Fatalf("expected tab page to contain both vertical and horizontal overflow, got %v", page.Direction)
	}
	if page.MinSize().Width > 320 {
		t.Fatalf("scroll page min width should stay compact, got %v", page.MinSize())
	}
}

func TestPeerLatencyRefreshIntervalIsTwoHundredMilliseconds(t *testing.T) {
	if peerLatencyRefreshInterval != 200*time.Millisecond {
		t.Fatalf("expected peer latency refresh interval to be 200ms, got %s", peerLatencyRefreshInterval)
	}
	if peerLatencyProbeTimeout >= peerLatencyRefreshInterval {
		t.Fatalf("probe timeout should stay below refresh interval, timeout=%s interval=%s", peerLatencyProbeTimeout, peerLatencyRefreshInterval)
	}
}

func TestPeerDisplayMetaShowsFullAccessAuthorization(t *testing.T) {
	peer := directmanager.TrustedPeer{
		TailscaleIP:        "100.109.251.97",
		AccessAuthorizedAt: time.Now().UTC(),
	}
	if got := peerDisplayMeta(peer, PeerLatency{}); !strings.Contains(got, "已授权全端口") {
		t.Fatalf("expected full access authorization in peer meta, got %q", got)
	}
}

func TestPeerDisplayMetaAppendsLatency(t *testing.T) {
	peer := directmanager.TrustedPeer{
		TailscaleIP:        "100.109.251.97",
		AccessAuthorizedAt: time.Now().UTC(),
	}
	latency := PeerLatency{Latency: 23 * time.Millisecond, Updated: true}

	if got := peerDisplayMeta(peer, latency); got != "100.109.251.97 · 已授权全端口 · 23ms" {
		t.Fatalf("unexpected peer meta with latency: %q", got)
	}
}

func TestDirectControllerRefreshPeerLatenciesStoresLatencyByPeer(t *testing.T) {
	mgr := &fakeDirectManager{
		peers:         []directmanager.TrustedPeer{{ID: "device-b", DisplayName: "desktop-b", TailscaleIP: "100.109.251.97"}},
		peerLatencies: map[string]time.Duration{"100.109.251.97": 23 * time.Millisecond},
	}
	ctrl := NewDirectController(mgr)
	if err := ctrl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := ctrl.RefreshPeerLatencies(context.Background()); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	latency, ok := state.PeerLatencies["device-b"]
	if !ok || latency.Latency != 23*time.Millisecond || !latency.Updated {
		t.Fatalf("expected latency stored by peer id, got %+v", state.PeerLatencies)
	}
}

func TestLocalhostBridgeStatusTextShowsActivePorts(t *testing.T) {
	state := DirectState{LocalhostBridgePorts: []int{18789, 3000}}
	if got := localhostBridgeStatusText(state); got != "localhost 桥接：3000, 18789" {
		t.Fatalf("unexpected localhost bridge status: %q", got)
	}
}

func TestLocalhostBridgeStatusTextShowsNone(t *testing.T) {
	if got := localhostBridgeStatusText(DirectState{}); got != "localhost 桥接：无" {
		t.Fatalf("unexpected localhost bridge status: %q", got)
	}
}

func TestLocalhostBridgeConflictStatusTextShowsConflictPorts(t *testing.T) {
	state := DirectState{LocalhostBridgeConflictPorts: []int{3000}}
	if got := localhostBridgeConflictStatusText(state); got != "localhost 冲突：3000 原生监听，未桥接" {
		t.Fatalf("unexpected localhost bridge conflict status: %q", got)
	}
}

func TestLocalhostBridgeConflictStatusTextShowsNone(t *testing.T) {
	if got := localhostBridgeConflictStatusText(DirectState{}); got != "localhost 冲突：无" {
		t.Fatalf("unexpected localhost bridge conflict status: %q", got)
	}
}

func TestNetworkPathStatusTextShowsProxyWarning(t *testing.T) {
	state := DirectState{NetworkPath: netdiag.PeerPathReport{
		Status:       netdiag.PathDirectProxy,
		Endpoint:     "115.233.222.82:41641",
		Latency:      "249ms",
		CurrentRoute: netdiag.RouteInfo{InterfaceAlias: "Meta", NextHop: "198.18.0.2"},
	}}
	got := networkPathStatusText(state)
	if !strings.Contains(got, "直连但疑似代理绕路") || !strings.Contains(got, "249ms") {
		t.Fatalf("unexpected network status text: %q", got)
	}
}

func TestNetworkPathStatusTextShowsOptimizedTUNDirect(t *testing.T) {
	state := DirectState{NetworkPath: netdiag.PeerPathReport{
		Status:       netdiag.PathDirectTUNOptimized,
		Endpoint:     "115.233.222.82:52477",
		Latency:      "15ms",
		CurrentRoute: netdiag.RouteInfo{InterfaceAlias: "Meta", NextHop: "198.18.0.2"},
	}}
	got := networkPathStatusText(state)
	if !strings.Contains(got, "TUN") || !strings.Contains(got, "15ms") {
		t.Fatalf("unexpected optimized TUN status text: %q", got)
	}
}

func TestEgressCandidateOptionsShowPublicMappingFirst(t *testing.T) {
	options := egressCandidateOptions([]netdiag.EgressCandidate{{
		InterfaceAlias: "以太网",
		InterfaceIP:    "192.168.1.11",
		NextHop:        "192.168.1.1",
		PublicIPv4:     "112.10.189.69:1142",
		Recommended:    true,
	}})
	if len(options) != 1 {
		t.Fatalf("expected one option, got %+v", options)
	}
	if !strings.Contains(options[0], "公网 112.10.189.69:1142") || !strings.Contains(options[0], "本机 192.168.1.11") {
		t.Fatalf("expected option to show public mapping and local IP, got %q", options[0])
	}
}

func TestEgressCandidateOptionsShowIPv6FamilyAndPublicMapping(t *testing.T) {
	options := egressCandidateOptions([]netdiag.EgressCandidate{{
		InterfaceAlias: "以太网",
		AddressFamily:  netdiag.AddressFamilyIPv6,
		InterfaceIP:    "2409:8a28:127d:e2f0:e431:c739:7833:d9b5",
		NextHop:        "fe80::1",
		PublicIPv6:     "[2409:8a28:127d:e2f0::100]:41641",
		Recommended:    true,
	}})
	if len(options) != 1 {
		t.Fatalf("expected one option, got %+v", options)
	}
	for _, want := range []string{"IPv6", "公网IPv6 [2409:8a28:127d:e2f0::100]:41641", "本机 2409:8a28:127d:e2f0:e431:c739:7833:d9b5", "推荐"} {
		if !strings.Contains(options[0], want) {
			t.Fatalf("expected option to contain %q, got %q", want, options[0])
		}
	}
}

func TestClashStatusTextHelpers(t *testing.T) {
	report := clash.DiscoveryReport{
		TUNInterfaces: []clash.TUNInterface{{Name: "Meta", Status: "Up"}},
		ProxyPorts: []clash.ProxyPort{
			{Kind: "mixed", Port: 7897},
			{Kind: "socks", Port: 7898},
			{Kind: "http", Port: 7899},
		},
		Control: clash.ControlEndpoint{Kind: clash.ControlNamedPipe, Address: `\\.\pipe\verge-mihomo`},
		Nodes: []clash.ProxyNode{{
			GroupName: "GLOBAL",
			Name:      "上海 01",
			Region:    "上海",
			Delay:     23 * time.Millisecond,
			Current:   true,
		}},
	}
	state := DirectState{ClashReport: report}

	if got := clashTUNStatusText(state); got != "TUN：Meta 已启用" {
		t.Fatalf("unexpected TUN text: %q", got)
	}
	if got := clashProxyPortsText(state); got != "代理入口：mixed 7897 / socks 7898 / http 7899" {
		t.Fatalf("unexpected proxy ports text: %q", got)
	}
	if got := clashControlText(state); got != `控制接口：named pipe \\.\pipe\verge-mihomo` {
		t.Fatalf("unexpected control text: %q", got)
	}
	options := clashNodeOptions(report.Nodes)
	if len(options) != 1 || options[0] != "上海 · 上海 01 · 23ms · 当前" {
		t.Fatalf("unexpected node options: %+v", options)
	}
}

func TestDirectControllerRefreshKeepsPreviousStateOnPeerLoadFailure(t *testing.T) {
	mgr := &fakeDirectManager{
		ready: directmanager.ReadyState{Ready: true, LocalTailscaleIP: "100.79.83.104"},
		peers: []directmanager.TrustedPeer{{ID: "device-b", TailscaleIP: "100.109.251.97"}},
	}
	ctrl := NewDirectController(mgr)
	if err := ctrl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	mgr.trustedErr = errors.New("store unavailable")
	if err := ctrl.Refresh(context.Background()); err == nil {
		t.Fatal("expected trusted peer load error")
	}

	state := ctrl.State()
	if len(state.Peers) != 1 {
		t.Fatalf("expected previous peers to remain visible, got %+v", state.Peers)
	}
}

func upsertFakePeer(peers []directmanager.TrustedPeer, peer store.TrustedPeer) []directmanager.TrustedPeer {
	for i := range peers {
		if peers[i].ID == peer.ID {
			peers[i] = peer
			return peers
		}
	}
	return append(peers, peer)
}
