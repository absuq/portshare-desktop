package manager

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	direct "github.com/absuq/portshare-desktop/internal/direct"
	directforward "github.com/absuq/portshare-desktop/internal/direct/forward"
	"github.com/absuq/portshare-desktop/internal/direct/store"
	"github.com/absuq/portshare-desktop/internal/tailscale"
)

const defaultDirectControlPort = 17890

type Tailscale interface {
	CheckReady(context.Context) tailscale.ReadyReport
	PingPeer(context.Context, string) (tailscale.PeerRoute, error)
}

type Config struct {
	Tailscale         Tailscale
	PairClient        PairClient
	PeerStore         PeerStore
	DirectClient      directforward.DirectClient
	ForwardFactory    ForwardFactory
	DirectControlPort int
	SecretLabel       string
	DeviceID          string
	DeviceName        string
}

type Manager struct {
	tailscale         Tailscale
	pairClient        PairClient
	peerStore         PeerStore
	directClient      directforward.DirectClient
	forwardFactory    ForwardFactory
	directControlPort int
	secretLabel       string
	deviceID          string
	deviceName        string

	authMu          sync.RWMutex
	controlMu       sync.Mutex
	controlServer   *direct.Server
	controlListener net.Listener
	peerMu          sync.Mutex
	forwardMu       sync.Mutex
	nextForwardSeq  int
	forwards        map[string]runningForward
}

type PairedPeer = direct.PairedPeer

type PairClient interface {
	Pair(context.Context, string) (direct.PairedPeer, error)
}

type PeerStore interface {
	LoadPeers() ([]store.TrustedPeer, error)
	SavePeers([]store.TrustedPeer) error
}

type TrustedPeer = store.TrustedPeer

type Forward interface {
	Start(context.Context) error
	Stop()
	LocalAddress() string
}

type ForwardFactory interface {
	New(directforward.Options) Forward
}

type realForwardFactory struct{}

func (realForwardFactory) New(options directforward.Options) Forward {
	return directforward.New(options)
}

type ForwardRequest struct {
	PeerID       string
	TargetHost   string
	TargetPort   int
	LocalAddress string
}

type RunningForward struct {
	ID           string
	PeerID       string
	LocalAddress string
	Target       string
}

type runningForward struct {
	info   RunningForward
	handle Forward
	cancel context.CancelFunc
}

type ReadyState struct {
	Ready            bool
	LocalTailscaleIP string
	Code             tailscale.DiagnosticCode
	Message          string
}

func New(config Config) *Manager {
	forwardFactory := config.ForwardFactory
	if forwardFactory == nil {
		forwardFactory = realForwardFactory{}
	}
	directControlPort := config.DirectControlPort
	if directControlPort == 0 {
		directControlPort = defaultDirectControlPort
	}
	return &Manager{
		tailscale:         config.Tailscale,
		pairClient:        config.PairClient,
		peerStore:         config.PeerStore,
		directClient:      config.DirectClient,
		forwardFactory:    forwardFactory,
		directControlPort: directControlPort,
		secretLabel:       config.SecretLabel,
		deviceID:          config.DeviceID,
		deviceName:        config.DeviceName,
		forwards:          make(map[string]runningForward),
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
	})
	client := direct.NewClient(direct.ClientConfig{
		DeviceID:   m.deviceID,
		DeviceName: m.deviceName,
		Secret:     secret,
	})

	m.controlMu.Lock()
	oldServer := m.controlServer
	oldListener := m.controlListener
	m.controlServer = server
	m.controlListener = listener
	m.controlMu.Unlock()

	if oldServer != nil {
		_ = oldServer.Close()
	}
	if oldListener != nil {
		_ = closeListener(oldListener)
	}

	m.authMu.Lock()
	m.pairClient = client
	m.directClient = client
	m.secretLabel = store.DeriveSecretLabel(secret)
	m.authMu.Unlock()

	go func() { _ = server.Serve(listener) }()
	return nil
}

func (m *Manager) StopControlServer(ctx context.Context) error {
	_ = ctx
	m.controlMu.Lock()
	server := m.controlServer
	listener := m.controlListener
	m.controlServer = nil
	m.controlListener = nil
	m.controlMu.Unlock()

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

	peers, err := m.peerStore.LoadPeers()
	if err != nil {
		return PairedPeer{}, err
	}

	peers = upsertTrustedPeer(peers, trustedPeerFromPair(peer, address, secretLabel))

	if err := m.peerStore.SavePeers(peers); err != nil {
		return PairedPeer{}, err
	}
	return peer, nil
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

func (m *Manager) CreateForward(ctx context.Context, req ForwardRequest) (RunningForward, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return RunningForward{}, err
	}
	if m.peerStore == nil {
		return RunningForward{}, errors.New("peer store is not configured")
	}
	m.authMu.RLock()
	directClient := m.directClient
	m.authMu.RUnlock()
	if directClient == nil {
		return RunningForward{}, errors.New("direct client is not configured")
	}
	if req.PeerID == "" {
		return RunningForward{}, errors.New("peer ID is required")
	}
	if req.TargetHost == "" {
		return RunningForward{}, errors.New("target host is required")
	}
	if req.TargetPort <= 0 {
		return RunningForward{}, errors.New("target port is required")
	}
	if strings.TrimSpace(req.LocalAddress) == "" {
		req.LocalAddress = "127.0.0.1:0"
	}

	m.peerMu.Lock()
	peers, err := m.peerStore.LoadPeers()
	m.peerMu.Unlock()
	if err != nil {
		return RunningForward{}, err
	}
	peer, ok := findTrustedPeer(peers, req.PeerID)
	if !ok {
		return RunningForward{}, fmt.Errorf("unknown trusted peer %q", req.PeerID)
	}
	peerAddress := peerControlAddress(peer, m.directControlPort)
	if peerAddress == "" {
		return RunningForward{}, fmt.Errorf("trusted peer %q has no tailscale address", req.PeerID)
	}

	handle := m.forwardFactory.New(directforward.Options{
		LocalAddress: req.LocalAddress,
		PeerAddress:  peerAddress,
		TargetHost:   req.TargetHost,
		TargetPort:   req.TargetPort,
		DirectClient: directClient,
	})
	if handle == nil {
		return RunningForward{}, errors.New("forward factory returned nil")
	}
	forwardCtx, cancel := context.WithCancel(context.Background())
	if err := handle.Start(forwardCtx); err != nil {
		cancel()
		return RunningForward{}, err
	}
	localAddress := handle.LocalAddress()
	if localAddress == "" {
		localAddress = req.LocalAddress
	}

	m.forwardMu.Lock()
	defer m.forwardMu.Unlock()

	m.nextForwardSeq++
	running := RunningForward{
		ID:           fmt.Sprintf("forward-%d", m.nextForwardSeq),
		PeerID:       req.PeerID,
		LocalAddress: localAddress,
		Target:       net.JoinHostPort(req.TargetHost, strconv.Itoa(req.TargetPort)),
	}
	m.forwards[running.ID] = runningForward{info: running, handle: handle, cancel: cancel}
	return running, nil
}

func (m *Manager) StopForward(ctx context.Context, id string) error {
	_ = ctx
	m.forwardMu.Lock()
	forward, ok := m.forwards[id]
	if ok {
		delete(m.forwards, id)
	}
	m.forwardMu.Unlock()
	if !ok {
		return fmt.Errorf("unknown forward %q", id)
	}
	if forward.cancel != nil {
		forward.cancel()
	}
	forward.handle.Stop()
	return nil
}

func findTrustedPeer(peers []store.TrustedPeer, id string) (store.TrustedPeer, bool) {
	for _, peer := range peers {
		if peer.ID == id {
			return peer, true
		}
	}
	return store.TrustedPeer{}, false
}

func trustedPeerFromPair(peer direct.PairedPeer, address string, secretLabel string) store.TrustedPeer {
	now := time.Now()
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

func peerControlAddress(peer store.TrustedPeer, defaultPort int) string {
	address := strings.TrimSpace(peer.TailscaleIP)
	if address == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(address); err == nil {
		return address
	}
	if defaultPort <= 0 {
		return address
	}
	return net.JoinHostPort(address, strconv.Itoa(defaultPort))
}
