package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/direct/store"
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
