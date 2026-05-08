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
)

type DirectManager interface {
	Ready(context.Context) directmanager.ReadyState
	StartControlServer(context.Context, string, string) error
	StopControlServer(context.Context) error
	ControlAddress() string
	PairPeer(context.Context, string) (directmanager.PairedPeer, error)
	TrustedPeers(context.Context) ([]directmanager.TrustedPeer, error)
}

type DirectController struct {
	manager DirectManager
	state   DirectState
}

type DirectState struct {
	Ready            bool
	LocalTailscaleIP string
	ControlListening bool
	ControlAddress   string
	DiagnosticCode   tailscale.DiagnosticCode
	Message          string
	Peers            []directmanager.TrustedPeer
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
	c.updateControlState()
	if c.state.ControlAddress == "" {
		c.state.ControlAddress = listenAddress
		c.state.ControlListening = true
	}
	successMessage := controlListeningMessage(c.state.ControlAddress)
	c.state.Message = successMessage
	if err := c.Refresh(ctx); err != nil {
		c.state.Message = successMessage + "，但状态刷新失败：" + err.Error()
		return nil
	}
	c.state.Message = controlListeningMessage(c.state.ControlAddress)
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
	c.updateControlState()
	c.state.Message = "直连监听已停止"
	return nil
}

func (c *DirectController) Refresh(ctx context.Context) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	ready := c.manager.Ready(ctx)
	c.state.Ready = ready.Ready
	c.state.LocalTailscaleIP = ready.LocalTailscaleIP
	c.state.DiagnosticCode = ready.Code
	c.updateControlState()
	peers, err := c.manager.TrustedPeers(ctx)
	if err != nil {
		c.state.Message = "读取可信设备失败：" + err.Error()
		return err
	}
	c.state.Peers = copyTrustedPeers(peers)
	if ready.Ready {
		if c.state.ControlListening {
			c.state.Message = controlListeningMessage(c.state.ControlAddress)
		} else {
			c.state.Message = "Tailscale 已就绪"
		}
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
	return c.pairNormalizedPeer(ctx, address)
}

func (c *DirectController) PairPeerWithSecret(ctx context.Context, peerAddress string, secret string, listenAddress string) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	address, err := normalizePeerControlAddress(peerAddress)
	if err != nil {
		c.state.Message = "对方 Tailscale 地址无效：" + err.Error()
		return err
	}
	if err := c.StartDirectMode(ctx, secret, listenAddress); err != nil {
		return err
	}
	return c.pairNormalizedPeer(ctx, address)
}

func (c *DirectController) pairNormalizedPeer(ctx context.Context, address string) error {
	peer, err := c.manager.PairPeer(ctx, address)
	if err != nil {
		err = describePairError(address, err)
		c.state.Message = "配对失败：" + err.Error()
		return err
	}
	successMessage := "已配对：" + displayPeerName(peer.DeviceName, peer.DeviceID)
	c.state.Message = successMessage
	if err := c.Refresh(ctx); err != nil {
		c.state.Message = successMessage + "；状态刷新失败：" + err.Error()
		return nil
	}
	c.state.Message = successMessage
	return nil
}

func describePairError(address string, err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "actively refused") || strings.Contains(message, "connection refused") {
		return fmt.Errorf("对方 %s 没有接受 portshare 直连连接。请确认对方电脑也运行新版 portshare，输入同一个直连密钥，并点击“启用直连密钥”；如果已经启用，请检查 Tailscale Shields Up 或 Windows 防火墙是否拦截 17890。原始错误：%w", address, err)
	}
	return err
}

func (c *DirectController) State() DirectState {
	state := c.state
	state.Peers = copyTrustedPeers(state.Peers)
	return state
}

func (c *DirectController) requireManager() error {
	if c.manager == nil {
		c.state.Message = "直连管理器未配置"
		return ErrDirectManagerRequired
	}
	return nil
}

func (c *DirectController) updateControlState() {
	address := strings.TrimSpace(c.manager.ControlAddress())
	c.state.ControlAddress = address
	c.state.ControlListening = address != ""
}

func controlListeningMessage(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return "直连监听已启动"
	}
	return "直连监听已启动：" + address
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
