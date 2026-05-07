package ui

import (
	"context"
	"errors"
	"testing"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/direct/store"
	"github.com/absuq/portshare-desktop/internal/tailscale"
)

type fakeDirectManager struct {
	ready              directmanager.ReadyState
	readyCalls         int
	peers              []directmanager.TrustedPeer
	forward            directmanager.RunningForward
	forwards           []directmanager.RunningForward
	started            bool
	startListenAddress string
	startSecret        string
	pairAddress        string
	createRequest      directmanager.ForwardRequest
	stopForwardID      string
	trustedErr         error
	pairErr            error
	createErr          error
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

func (f *fakeDirectManager) CreateForward(_ context.Context, req directmanager.ForwardRequest) (directmanager.RunningForward, error) {
	f.createRequest = req
	if f.createErr != nil {
		return directmanager.RunningForward{}, f.createErr
	}
	f.forward = directmanager.RunningForward{
		ID:           "fwd-1",
		PeerID:       req.PeerID,
		LocalAddress: "127.0.0.1:18080",
		Target:       "127.0.0.1:3000",
	}
	f.forwards = append(f.forwards, f.forward)
	return f.forward, nil
}

func (f *fakeDirectManager) StopForward(_ context.Context, id string) error {
	f.stopForwardID = id
	for i := 0; i < len(f.forwards); i++ {
		if f.forwards[i].ID == id {
			f.forwards = append(f.forwards[:i], f.forwards[i+1:]...)
			i--
		}
	}
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
	if len(state.Peers) != 0 || len(state.Forwards) != 0 {
		t.Fatalf("expected empty peers and forwards, got %+v", state)
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

func TestDirectControllerCreateAndStopForward(t *testing.T) {
	mgr := &fakeDirectManager{peers: []directmanager.TrustedPeer{
		{ID: "device-b", DisplayName: "desktop-b", TailscaleIP: "100.109.251.97"},
	}}
	ctrl := NewDirectController(mgr)
	if err := ctrl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := ctrl.CreateForward(context.Background(), "device-b", "127.0.0.1", 3000, "127.0.0.1:18080"); err != nil {
		t.Fatal(err)
	}

	if mgr.createRequest.PeerID != "device-b" || mgr.createRequest.TargetPort != 3000 {
		t.Fatalf("unexpected forward request: %+v", mgr.createRequest)
	}
	state := ctrl.State()
	if len(state.Forwards) != 1 || state.Forwards[0].LocalAddress != "127.0.0.1:18080" {
		t.Fatalf("unexpected forwards: %+v", state.Forwards)
	}

	if err := ctrl.StopForward(context.Background(), "fwd-1"); err != nil {
		t.Fatal(err)
	}
	if mgr.stopForwardID != "fwd-1" {
		t.Fatalf("expected stop call for fwd-1, got %q", mgr.stopForwardID)
	}
	if len(ctrl.State().Forwards) != 0 {
		t.Fatalf("expected stopped forward removed, got %+v", ctrl.State().Forwards)
	}
}

func TestDirectControllerCreateForwardKeepsForwardVisibleWhenRefreshFails(t *testing.T) {
	mgr := &fakeDirectManager{
		peers: []directmanager.TrustedPeer{{ID: "device-b", DisplayName: "desktop-b", TailscaleIP: "100.109.251.97"}},
	}
	ctrl := NewDirectController(mgr)
	if err := ctrl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	mgr.trustedErr = errors.New("store unavailable")
	if err := ctrl.CreateForward(context.Background(), "device-b", "127.0.0.1", 3000, "127.0.0.1:18080"); err != nil {
		t.Fatal(err)
	}
	state := ctrl.State()
	if len(state.Forwards) != 1 || state.Forwards[0].ID != "fwd-1" {
		t.Fatalf("expected running forward to stay visible after refresh failure, got %+v", state.Forwards)
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

func TestDirectControllerStateIsImmutableSnapshot(t *testing.T) {
	mgr := &fakeDirectManager{
		peers: []directmanager.TrustedPeer{{ID: "device-b", DisplayName: "desktop-b", TailscaleIP: "100.109.251.97"}},
	}
	ctrl := NewDirectController(mgr)
	if err := ctrl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.CreateForward(context.Background(), "device-b", "127.0.0.1", 3000, "127.0.0.1:18080"); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	state.Peers[0].ID = "mutated-peer"
	state.Forwards[0].ID = "mutated-forward"

	next := ctrl.State()
	if next.Peers[0].ID != "device-b" || next.Forwards[0].ID != "fwd-1" {
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
	if err := ctrl.CreateForward(context.Background(), "", "127.0.0.1", 3000, "127.0.0.1:0"); !errors.Is(err, ErrDirectPeerRequired) {
		t.Fatalf("expected ErrDirectPeerRequired, got %v", err)
	}
	if err := ctrl.CreateForward(context.Background(), "device-b", "127.0.0.1", 0, "127.0.0.1:0"); !errors.Is(err, ErrDirectTargetPortRequired) {
		t.Fatalf("expected ErrDirectTargetPortRequired, got %v", err)
	}
	if err := ctrl.CreateForward(context.Background(), "device-b", "127.0.0.1", 70000, "127.0.0.1:0"); !errors.Is(err, ErrDirectTargetPortRequired) {
		t.Fatalf("expected ErrDirectTargetPortRequired for oversized port, got %v", err)
	}
}

func TestLocalListenAddressAcceptsOnlyLocalPort(t *testing.T) {
	got, err := localListenAddress("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "127.0.0.1:0" {
		t.Fatalf("unexpected automatic local address: %s", got)
	}
	got, err = localListenAddress("18080")
	if err != nil {
		t.Fatal(err)
	}
	if got != "127.0.0.1:18080" {
		t.Fatalf("unexpected local address: %s", got)
	}
	for _, input := range []string{"localhost:18080", ":18080", "abc", "70000", "0"} {
		if _, err := localListenAddress(input); err == nil {
			t.Fatalf("expected localListenAddress(%q) to fail", input)
		}
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
