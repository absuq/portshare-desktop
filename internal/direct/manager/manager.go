package manager

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	direct "github.com/absuq/portshare-desktop/internal/direct"
	"github.com/absuq/portshare-desktop/internal/direct/store"
	"github.com/absuq/portshare-desktop/internal/tailscale"
)

type Tailscale interface {
	CheckReady(context.Context) tailscale.ReadyReport
	PingPeer(context.Context, string) (tailscale.PeerRoute, error)
}

type Config struct {
	Tailscale        Tailscale
	PairClient       PairClient
	PeerStore        PeerStore
	AccessAuthorizer AccessAuthorizer
	LocalhostBridge  LocalhostBridge
	SecretLabel      string
	DeviceID         string
	DeviceName       string
}

type Manager struct {
	tailscale        Tailscale
	pairClient       PairClient
	peerStore        PeerStore
	accessAuthorizer AccessAuthorizer
	localhostBridge  LocalhostBridge
	secretLabel      string
	deviceID         string
	deviceName       string

	authMu          sync.RWMutex
	controlMu       sync.Mutex
	controlServer   *direct.Server
	controlListener net.Listener
	bridgeCancel    context.CancelFunc
	peerMu          sync.Mutex
}

type PairedPeer = direct.PairedPeer

type PairClient interface {
	Pair(context.Context, string) (direct.PairedPeer, error)
}

type AccessAuthorizer interface {
	AllowTrustedPeer(context.Context, TrustedPeerAccess) error
}

type LocalhostBridge interface {
	SetLocalTailscaleIP(string)
	SetAllowedPeers([]string)
	Refresh(context.Context) error
	ActivePorts() []int
	ConflictPorts() []int
	Close() error
}

type TrustedPeerAccess struct {
	RulePrefix       string
	LocalTailscaleIP string
	PeerTailscaleIP  string
	PeerID           string
	PeerName         string
}

type PeerStore interface {
	LoadPeers() ([]store.TrustedPeer, error)
	SavePeers([]store.TrustedPeer) error
}

type TrustedPeer = store.TrustedPeer

type ReadyState struct {
	Ready            bool
	LocalTailscaleIP string
	Code             tailscale.DiagnosticCode
	Message          string
}

func New(config Config) *Manager {
	return &Manager{
		tailscale:        config.Tailscale,
		pairClient:       config.PairClient,
		peerStore:        config.PeerStore,
		accessAuthorizer: config.AccessAuthorizer,
		localhostBridge:  config.LocalhostBridge,
		secretLabel:      config.SecretLabel,
		deviceID:         config.DeviceID,
		deviceName:       config.DeviceName,
	}
}

func (m *Manager) Ready(ctx context.Context) ReadyState {
	if m.tailscale == nil {
		return ReadyState{
			Code:    tailscale.CodeTailscaleUnavailable,
			Message: "Tailscale client is not configured.",
		}
	}

	report := m.tailscale.CheckReady(ctx)
	return ReadyState{
		Ready:            report.Ready,
		LocalTailscaleIP: report.Status.LocalIPv4,
		Code:             report.Code,
		Message:          report.Message,
	}
}

func (m *Manager) StartControlServer(ctx context.Context, listenAddress string, secret string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(secret) == "" {
		return errors.New("shared secret is required")
	}
	if strings.TrimSpace(listenAddress) == "" {
		return errors.New("listen address is required")
	}

	var sameAddressServer *direct.Server
	var sameAddressListener net.Listener
	m.controlMu.Lock()
	if m.controlListener != nil && m.controlListener.Addr().String() == listenAddress {
		sameAddressServer = m.controlServer
		sameAddressListener = m.controlListener
		m.controlServer = nil
		m.controlListener = nil
	}
	m.controlMu.Unlock()
	if sameAddressServer != nil {
		_ = sameAddressServer.Close()
	}
	if sameAddressListener != nil {
		_ = closeListener(sameAddressListener)
	}

	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return err
	}
	server := direct.NewServer(direct.ServerConfig{
		DeviceID:   m.deviceID,
		DeviceName: m.deviceName,
		Secret:     secret,
		OnAuthenticated: func(peer direct.PairedPeer) {
			_ = m.TrustAuthenticatedPeer(context.Background(), peer)
		},
	})
	client := direct.NewClient(direct.ClientConfig{
		DeviceID:   m.deviceID,
		DeviceName: m.deviceName,
		Secret:     secret,
	})

	m.controlMu.Lock()
	oldServer := m.controlServer
	oldListener := m.controlListener
	oldBridgeCancel := m.bridgeCancel
	m.controlServer = server
	m.controlListener = listener
	m.bridgeCancel = nil
	m.controlMu.Unlock()

	if oldBridgeCancel != nil {
		oldBridgeCancel()
	}
	if oldServer != nil {
		_ = oldServer.Close()
	}
	if oldListener != nil {
		_ = closeListener(oldListener)
	}

	m.authMu.Lock()
	m.pairClient = client
	m.secretLabel = store.DeriveSecretLabel(secret)
	m.authMu.Unlock()

	m.startLocalhostBridgePolling(ctx, hostFromAddress(listener.Addr().String()))
	go func() { _ = server.Serve(listener) }()
	return nil
}

func (m *Manager) StopControlServer(ctx context.Context) error {
	_ = ctx
	m.controlMu.Lock()
	server := m.controlServer
	listener := m.controlListener
	bridgeCancel := m.bridgeCancel
	m.controlServer = nil
	m.controlListener = nil
	m.bridgeCancel = nil
	m.controlMu.Unlock()

	if bridgeCancel != nil {
		bridgeCancel()
	}
	if m.localhostBridge != nil {
		_ = m.localhostBridge.Close()
	}
	if server != nil {
		_ = server.Close()
	}
	if listener != nil {
		return closeListener(listener)
	}
	return nil
}

func closeListener(listener net.Listener) error {
	err := listener.Close()
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (m *Manager) ControlAddress() string {
	m.controlMu.Lock()
	defer m.controlMu.Unlock()
	if m.controlListener == nil {
		return ""
	}
	return m.controlListener.Addr().String()
}

func (m *Manager) PairPeer(ctx context.Context, address string) (PairedPeer, error) {
	m.authMu.RLock()
	pairClient := m.pairClient
	secretLabel := m.secretLabel
	m.authMu.RUnlock()

	if pairClient == nil {
		return PairedPeer{}, errors.New("pair client is not configured")
	}
	if m.peerStore == nil {
		return PairedPeer{}, errors.New("peer store is not configured")
	}

	peer, err := pairClient.Pair(ctx, address)
	if err != nil {
		return PairedPeer{}, err
	}

	m.peerMu.Lock()
	defer m.peerMu.Unlock()

	trusted, err := m.buildAndAuthorizeTrustedPeer(ctx, peer, address, secretLabel)
	if err != nil {
		return PairedPeer{}, err
	}

	peers, err := m.peerStore.LoadPeers()
	if err != nil {
		return PairedPeer{}, err
	}

	peers = upsertTrustedPeer(peers, trusted)

	if err := m.peerStore.SavePeers(peers); err != nil {
		return PairedPeer{}, err
	}
	_ = m.refreshLocalhostBridge(ctx)
	return peer, nil
}

func (m *Manager) TrustAuthenticatedPeer(ctx context.Context, peer direct.PairedPeer) error {
	if m.peerStore == nil {
		return errors.New("peer store is not configured")
	}

	m.authMu.RLock()
	secretLabel := m.secretLabel
	m.authMu.RUnlock()

	m.peerMu.Lock()
	defer m.peerMu.Unlock()

	trusted, err := m.buildAndAuthorizeTrustedPeer(ctx, peer, peer.Address, secretLabel)
	if err != nil {
		return err
	}
	peers, err := m.peerStore.LoadPeers()
	if err != nil {
		return err
	}
	peers = upsertTrustedPeer(peers, trusted)
	if err := m.peerStore.SavePeers(peers); err != nil {
		return err
	}
	return m.refreshLocalhostBridge(ctx)
}

func (m *Manager) TrustedPeers(ctx context.Context) ([]TrustedPeer, error) {
	_ = ctx
	if m.peerStore == nil {
		return nil, errors.New("peer store is not configured")
	}
	m.peerMu.Lock()
	defer m.peerMu.Unlock()
	return m.peerStore.LoadPeers()
}

func (m *Manager) LocalhostBridgePorts() []int {
	if m.localhostBridge == nil {
		return nil
	}
	return m.localhostBridge.ActivePorts()
}

func (m *Manager) LocalhostBridgeConflictPorts() []int {
	if m.localhostBridge == nil {
		return nil
	}
	return m.localhostBridge.ConflictPorts()
}

func (m *Manager) startLocalhostBridgePolling(ctx context.Context, localIP string) {
	if m.localhostBridge == nil {
		return
	}
	bridgeCtx, cancel := context.WithCancel(context.Background())
	m.controlMu.Lock()
	m.bridgeCancel = cancel
	m.controlMu.Unlock()

	m.localhostBridge.SetLocalTailscaleIP(localIP)
	_ = m.refreshLocalhostBridge(ctx)

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-bridgeCtx.Done():
				return
			case <-ticker.C:
				_ = m.refreshLocalhostBridge(bridgeCtx)
			}
		}
	}()
}

func (m *Manager) refreshLocalhostBridge(ctx context.Context) error {
	if m.localhostBridge == nil {
		return nil
	}
	m.localhostBridge.SetLocalTailscaleIP(m.localTailscaleIP(ctx))
	m.localhostBridge.SetAllowedPeers(m.trustedPeerIPs(ctx))
	return m.localhostBridge.Refresh(ctx)
}

func (m *Manager) trustedPeerIPs(ctx context.Context) []string {
	_ = ctx
	if m.peerStore == nil {
		return nil
	}
	peers, err := m.peerStore.LoadPeers()
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	var ips []string
	for _, peer := range peers {
		ip := strings.TrimSpace(peer.TailscaleIP)
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}
	return ips
}

func (m *Manager) buildAndAuthorizeTrustedPeer(ctx context.Context, peer direct.PairedPeer, address string, secretLabel string) (store.TrustedPeer, error) {
	trusted := trustedPeerFromPair(peer, address, secretLabel)
	if m.accessAuthorizer == nil {
		return trusted, nil
	}
	if err := m.accessAuthorizer.AllowTrustedPeer(ctx, TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: m.localTailscaleIP(ctx),
		PeerTailscaleIP:  trusted.TailscaleIP,
		PeerID:           trusted.ID,
		PeerName:         trusted.DisplayName,
	}); err != nil {
		return store.TrustedPeer{}, err
	}
	trusted.AccessAuthorizedAt = time.Now().UTC()
	return trusted, nil
}

func (m *Manager) localTailscaleIP(ctx context.Context) string {
	if host := hostFromAddress(m.ControlAddress()); host != "" {
		return host
	}
	if m.tailscale == nil {
		return ""
	}
	return m.Ready(ctx).LocalTailscaleIP
}

func trustedPeerFromPair(peer direct.PairedPeer, address string, secretLabel string) store.TrustedPeer {
	now := time.Now().UTC()
	if peer.Address == "" {
		peer.Address = address
	}
	return store.TrustedPeer{
		ID:            peer.DeviceID,
		DisplayName:   peer.DeviceName,
		TailscaleIP:   hostFromAddress(peer.Address),
		FirstPairedAt: now,
		LastSeenAt:    now,
		SecretLabel:   secretLabel,
	}
}

func upsertTrustedPeer(peers []store.TrustedPeer, trusted store.TrustedPeer) []store.TrustedPeer {
	for i := range peers {
		if peers[i].ID != trusted.ID {
			continue
		}
		if !peers[i].FirstPairedAt.IsZero() {
			trusted.FirstPairedAt = peers[i].FirstPairedAt
		}
		if trusted.SecretLabel == "" {
			trusted.SecretLabel = peers[i].SecretLabel
		}
		if trusted.LastRoute == "" {
			trusted.LastRoute = peers[i].LastRoute
		}
		if trusted.AccessAuthorizedAt.IsZero() {
			trusted.AccessAuthorizedAt = peers[i].AccessAuthorizedAt
		}
		peers[i] = trusted
		return peers
	}
	return append(peers, trusted)
}

func hostFromAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}
