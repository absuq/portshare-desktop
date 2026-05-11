package manager

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/absuq/portshare-desktop/internal/clash"
	direct "github.com/absuq/portshare-desktop/internal/direct"
	"github.com/absuq/portshare-desktop/internal/direct/store"
	"github.com/absuq/portshare-desktop/internal/netdiag"
	"github.com/absuq/portshare-desktop/internal/tailscale"
)

type Tailscale interface {
	CheckReady(context.Context) tailscale.ReadyReport
	PingPeer(context.Context, string) (tailscale.PeerRoute, error)
}

type Config struct {
	Tailscale          Tailscale
	PairClient         PairClient
	PeerStore          PeerStore
	AccessAuthorizer   AccessAuthorizer
	LocalhostBridge    LocalhostBridge
	NetworkDiagnostics NetworkDiagnostics
	ClashEgress        ClashEgress
	SecretLabel        string
	DeviceID           string
	DeviceName         string
}

type Manager struct {
	tailscale          Tailscale
	pairClient         PairClient
	peerStore          PeerStore
	accessAuthorizer   AccessAuthorizer
	localhostBridge    LocalhostBridge
	networkDiagnostics NetworkDiagnostics
	clashEgress        ClashEgress
	secretLabel        string
	deviceID           string
	deviceName         string

	authMu          sync.RWMutex
	controlMu       sync.Mutex
	controlServer   *direct.Server
	controlListener net.Listener
	bridgeCancel    context.CancelFunc
	peerMu          sync.Mutex
	networkMu       sync.Mutex
	activeBypass    netdiag.ActiveBypass
	hasActiveBypass bool
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

type NetworkDiagnostics interface {
	DiagnosePeer(context.Context, string) (netdiag.PeerPathReport, error)
	ApplyBypass(context.Context, netdiag.BypassRequest) (netdiag.ActiveBypass, error)
	ClearBypass(context.Context, netdiag.ActiveBypass) error
}

type ClashEgress interface {
	Discover(context.Context) (clash.DiscoveryReport, error)
	RefreshNodes(context.Context) (clash.DiscoveryReport, error)
	ApplyNode(context.Context, clash.ApplyRequest) (clash.ApplyResult, error)
	RestoreNode(context.Context) error
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
		tailscale:          config.Tailscale,
		pairClient:         config.PairClient,
		peerStore:          config.PeerStore,
		accessAuthorizer:   config.AccessAuthorizer,
		localhostBridge:    config.LocalhostBridge,
		networkDiagnostics: config.NetworkDiagnostics,
		clashEgress:        config.ClashEgress,
		secretLabel:        config.SecretLabel,
		deviceID:           config.DeviceID,
		deviceName:         config.DeviceName,
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

func (m *Manager) NetworkPath(ctx context.Context, peerTailscaleIP string) (netdiag.PeerPathReport, error) {
	if m.networkDiagnostics == nil {
		return netdiag.PeerPathReport{}, errors.New("network diagnostics is not configured")
	}
	return m.networkDiagnostics.DiagnosePeer(ctx, peerTailscaleIP)
}

func (m *Manager) ApplyNetworkBypass(ctx context.Context, request netdiag.BypassRequest) (netdiag.ActiveBypass, error) {
	if m.networkDiagnostics == nil {
		return netdiag.ActiveBypass{}, errors.New("network diagnostics is not configured")
	}
	active, err := m.networkDiagnostics.ApplyBypass(ctx, request)
	if err != nil {
		return netdiag.ActiveBypass{}, err
	}
	m.networkMu.Lock()
	m.activeBypass = active
	m.hasActiveBypass = true
	m.networkMu.Unlock()
	return active, nil
}

func (m *Manager) ClearNetworkBypass(ctx context.Context) error {
	if m.networkDiagnostics == nil {
		return errors.New("network diagnostics is not configured")
	}
	m.networkMu.Lock()
	active := m.activeBypass
	hasActive := m.hasActiveBypass
	m.networkMu.Unlock()
	if !hasActive {
		return nil
	}
	if err := m.networkDiagnostics.ClearBypass(ctx, active); err != nil {
		return err
	}
	m.networkMu.Lock()
	m.activeBypass = netdiag.ActiveBypass{}
	m.hasActiveBypass = false
	m.networkMu.Unlock()
	return nil
}

func (m *Manager) ActiveNetworkBypass() (netdiag.ActiveBypass, bool) {
	m.networkMu.Lock()
	defer m.networkMu.Unlock()
	return m.activeBypass, m.hasActiveBypass
}

func (m *Manager) ProbePeerLatency(ctx context.Context, peerIP string) (time.Duration, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	peerIP = strings.TrimSpace(peerIP)
	if peerIP == "" {
		return 0, errors.New("peer tailscale ip is required")
	}
	return probeTCPConnectLatency(ctx, net.JoinHostPort(peerIP, "17890"), 150*time.Millisecond)
}

func probeTCPConnectLatency(ctx context.Context, address string, timeout time.Duration) (time.Duration, error) {
	dialer := net.Dialer{Timeout: 150 * time.Millisecond}
	if timeout > 0 {
		dialer.Timeout = timeout
	}
	started := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return time.Since(started), nil
}

func (m *Manager) DetectClash(ctx context.Context) (clash.DiscoveryReport, error) {
	if m.clashEgress == nil {
		return clash.DiscoveryReport{}, errors.New("clash egress is not configured")
	}
	return m.clashEgress.Discover(ctx)
}

func (m *Manager) RefreshClashNodes(ctx context.Context) (clash.DiscoveryReport, error) {
	if m.clashEgress == nil {
		return clash.DiscoveryReport{}, errors.New("clash egress is not configured")
	}
	return m.clashEgress.RefreshNodes(ctx)
}

func (m *Manager) ApplyClashNode(ctx context.Context, request clash.ApplyRequest) (clash.ApplyResult, error) {
	if m.clashEgress == nil {
		return clash.ApplyResult{}, errors.New("clash egress is not configured")
	}
	return m.clashEgress.ApplyNode(ctx, request)
}

func (m *Manager) RestoreClashNode(ctx context.Context) error {
	if m.clashEgress == nil {
		return errors.New("clash egress is not configured")
	}
	return m.clashEgress.RestoreNode(ctx)
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
