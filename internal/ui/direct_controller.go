package ui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/tailscale"
)

const defaultDirectControlPort = "17890"

var (
	ErrDirectManagerRequired       = errors.New("direct manager is not configured")
	ErrDirectSecretRequired        = errors.New("shared secret is required")
	ErrDirectListenAddressRequired = errors.New("listen address is required")
	ErrDirectPeerAddressRequired   = errors.New("peer tailscale address is required")
	ErrDirectPeerAddressInvalid    = errors.New("peer tailscale address is invalid")
	ErrDirectPeerRequired          = errors.New("trusted peer is required")
	ErrDirectTargetPortRequired    = errors.New("target port is required")
	ErrDirectForwardRequired       = errors.New("forward ID is required")
)

type DirectManager interface {
	Ready(context.Context) directmanager.ReadyState
	StartControlServer(context.Context, string, string) error
	StopControlServer(context.Context) error
	PairPeer(context.Context, string) (directmanager.PairedPeer, error)
	TrustedPeers(context.Context) ([]directmanager.TrustedPeer, error)
	CreateForward(context.Context, directmanager.ForwardRequest) (directmanager.RunningForward, error)
	StopForward(context.Context, string) error
}

type DirectController struct {
	manager  DirectManager
	state    DirectState
	forwards []directmanager.RunningForward
}

type DirectState struct {
	Ready            bool
	LocalTailscaleIP string
	DiagnosticCode   tailscale.DiagnosticCode
	Message          string
	Peers            []directmanager.TrustedPeer
	Forwards         []directmanager.RunningForward
}

func NewDirectController(manager DirectManager) *DirectController {
	return &DirectController{manager: manager}
}

func (c *DirectController) StartDirectMode(ctx context.Context, secret string, listenAddress string) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		c.state.Message = "请输入共享密钥"
		return ErrDirectSecretRequired
	}
	listenAddress = strings.TrimSpace(listenAddress)
	if listenAddress == "" {
		c.state.Message = "缺少直连监听地址"
		return ErrDirectListenAddressRequired
	}
	if err := c.manager.StartControlServer(ctx, listenAddress, secret); err != nil {
		c.state.Message = "启动直连监听失败：" + err.Error()
		return err
	}
	c.state.Message = "直连监听已启动"
	if err := c.Refresh(ctx); err != nil {
		c.state.Message = "直连监听已启动，但状态刷新失败：" + err.Error()
	}
	return nil
}

func (c *DirectController) StopDirectMode(ctx context.Context) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	if err := c.manager.StopControlServer(ctx); err != nil {
		c.state.Message = "停止直连监听失败：" + err.Error()
		return err
	}
	c.state.Message = "直连监听已停止"
	return nil
}

func (c *DirectController) Refresh(ctx context.Context) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	ready := c.manager.Ready(ctx)
	peers, err := c.manager.TrustedPeers(ctx)
	if err != nil {
		c.state.Message = "读取可信设备失败：" + err.Error()
		return err
	}
	c.state.Ready = ready.Ready
	c.state.LocalTailscaleIP = ready.LocalTailscaleIP
	c.state.DiagnosticCode = ready.Code
	c.state.Peers = copyTrustedPeers(peers)
	c.state.Forwards = copyRunningForwards(c.forwards)
	if ready.Ready {
		c.state.Message = "Tailscale 已就绪"
	} else {
		c.state.Message = ready.Message
	}
	return nil
}

func (c *DirectController) PairPeer(ctx context.Context, peerAddress string) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	address, err := normalizePeerControlAddress(peerAddress)
	if err != nil {
		c.state.Message = "对方 Tailscale 地址无效：" + err.Error()
		return err
	}
	peer, err := c.manager.PairPeer(ctx, address)
	if err != nil {
		c.state.Message = "配对失败：" + err.Error()
		return err
	}
	c.state.Message = "已配对：" + displayPeerName(peer.DeviceName, peer.DeviceID)
	if err := c.Refresh(ctx); err != nil {
		c.state.Message = "已配对：" + displayPeerName(peer.DeviceName, peer.DeviceID) + "；状态刷新失败：" + err.Error()
	}
	return nil
}

func (c *DirectController) CreateForward(ctx context.Context, peerID, targetHost string, targetPort int, localAddress string) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		c.state.Message = "请先选择可信设备"
		return ErrDirectPeerRequired
	}
	targetHost = strings.TrimSpace(targetHost)
	if targetHost == "" {
		targetHost = "127.0.0.1"
	}
	if targetPort <= 0 {
		c.state.Message = "请输入远端目标端口"
		return ErrDirectTargetPortRequired
	}
	localAddress = strings.TrimSpace(localAddress)
	if localAddress == "" {
		localAddress = "127.0.0.1:0"
	}

	fwd, err := c.manager.CreateForward(ctx, directmanager.ForwardRequest{
		PeerID:       peerID,
		TargetHost:   targetHost,
		TargetPort:   targetPort,
		LocalAddress: localAddress,
	})
	if err != nil {
		c.state.Message = "创建本地转发失败：" + err.Error()
		return err
	}
	c.forwards = upsertRunningForward(c.forwards, fwd)
	c.state.Forwards = copyRunningForwards(c.forwards)
	c.state.Message = fmt.Sprintf("已创建转发：%s -> %s", fwd.LocalAddress, fwd.Target)
	if err := c.Refresh(ctx); err != nil {
		c.state.Forwards = copyRunningForwards(c.forwards)
		c.state.Message = fmt.Sprintf("已创建转发：%s -> %s；状态刷新失败：%s", fwd.LocalAddress, fwd.Target, err.Error())
	}
	return nil
}

func (c *DirectController) StopForward(ctx context.Context, id string) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		c.state.Message = "缺少转发 ID"
		return ErrDirectForwardRequired
	}
	if err := c.manager.StopForward(ctx, id); err != nil {
		c.state.Message = "停止转发失败：" + err.Error()
		return err
	}
	c.forwards = removeRunningForward(c.forwards, id)
	c.state.Forwards = copyRunningForwards(c.forwards)
	c.state.Message = "转发已停止"
	return nil
}

func (c *DirectController) State() DirectState {
	state := c.state
	state.Peers = copyTrustedPeers(state.Peers)
	state.Forwards = copyRunningForwards(state.Forwards)
	return state
}

func (c *DirectController) requireManager() error {
	if c.manager == nil {
		c.state.Message = "直连管理器未配置"
		return ErrDirectManagerRequired
	}
	return nil
}

func normalizePeerControlAddress(address string) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", ErrDirectPeerAddressRequired
	}
	host, port, err := net.SplitHostPort(address)
	if err == nil {
		if err := validatePeerHost(host); err != nil {
			return "", err
		}
		if err := validateTCPPort(port); err != nil {
			return "", err
		}
		return net.JoinHostPort(host, port), nil
	}
	if strings.HasPrefix(address, "[") || strings.Contains(address, "]:") {
		return "", ErrDirectPeerAddressInvalid
	}
	if hasSingleColon(address) {
		host, port, _ = strings.Cut(address, ":")
		if host == "" || port == "" {
			return "", ErrDirectPeerAddressInvalid
		}
		if err := validatePeerHost(host); err != nil {
			return "", err
		}
		if err := validateTCPPort(port); err != nil {
			return "", err
		}
		return net.JoinHostPort(host, port), nil
	}
	if err := validatePeerHost(address); err != nil {
		return "", err
	}
	return net.JoinHostPort(address, defaultDirectControlPort), nil
}

func validatePeerHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return ErrDirectPeerAddressInvalid
	}
	if strings.ContainsAny(host, " \t\r\n/") {
		return ErrDirectPeerAddressInvalid
	}
	return nil
}

func validateTCPPort(port string) error {
	value, err := strconv.Atoi(port)
	if err != nil || value <= 0 || value > 65535 {
		return ErrDirectPeerAddressInvalid
	}
	return nil
}

func hasSingleColon(value string) bool {
	return strings.Count(value, ":") == 1
}

func displayPeerName(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return id
}

func copyTrustedPeers(peers []directmanager.TrustedPeer) []directmanager.TrustedPeer {
	return append([]directmanager.TrustedPeer(nil), peers...)
}

func copyRunningForwards(forwards []directmanager.RunningForward) []directmanager.RunningForward {
	return append([]directmanager.RunningForward(nil), forwards...)
}

func upsertRunningForward(forwards []directmanager.RunningForward, fwd directmanager.RunningForward) []directmanager.RunningForward {
	for i := range forwards {
		if forwards[i].ID == fwd.ID {
			forwards[i] = fwd
			return forwards
		}
	}
	return append(forwards, fwd)
}

func removeRunningForward(forwards []directmanager.RunningForward, id string) []directmanager.RunningForward {
	for i := 0; i < len(forwards); i++ {
		if forwards[i].ID == id {
			forwards = append(forwards[:i], forwards[i+1:]...)
			i--
		}
	}
	return forwards
}
