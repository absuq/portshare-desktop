package ui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/absuq/portshare-desktop/internal/clash"
	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/netdiag"
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
	LocalhostBridgePorts() []int
	LocalhostBridgeConflictPorts() []int
	NetworkPath(context.Context, string) (netdiag.PeerPathReport, error)
	ApplyNetworkBypass(context.Context, netdiag.BypassRequest) (netdiag.ActiveBypass, error)
	ClearNetworkBypass(context.Context) error
	ActiveNetworkBypass() (netdiag.ActiveBypass, bool)
	ProbePeerLatency(context.Context, string) (time.Duration, error)
	DetectClash(context.Context) (clash.DiscoveryReport, error)
	RefreshClashNodes(context.Context) (clash.DiscoveryReport, error)
	ApplyClashNode(context.Context, clash.ApplyRequest) (clash.ApplyResult, error)
	RestoreClashNode(context.Context) error
	PairPeer(context.Context, string) (directmanager.PairedPeer, error)
	TrustedPeers(context.Context) ([]directmanager.TrustedPeer, error)
}

type DirectController struct {
	manager DirectManager
	state   DirectState
}

type DirectState struct {
	Ready                        bool
	LocalTailscaleIP             string
	ControlListening             bool
	ControlAddress               string
	LocalhostBridgePorts         []int
	LocalhostBridgeConflictPorts []int
	NetworkPath                  netdiag.PeerPathReport
	ActiveBypass                 netdiag.ActiveBypass
	HasActiveBypass              bool
	ClashReport                  clash.DiscoveryReport
	ClashApplyResult             clash.ApplyResult
	DiagnosticCode               tailscale.DiagnosticCode
	Message                      string
	Peers                        []directmanager.TrustedPeer
	PeerLatencies                map[string]PeerLatency
}

type PeerLatency struct {
	Latency time.Duration
	Error   string
	Updated bool
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
	c.state.LocalhostBridgePorts = copyInts(c.manager.LocalhostBridgePorts())
	c.state.LocalhostBridgeConflictPorts = copyInts(c.manager.LocalhostBridgeConflictPorts())
	c.state.ActiveBypass, c.state.HasActiveBypass = c.manager.ActiveNetworkBypass()
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

func (c *DirectController) DetectNetworkPath(ctx context.Context, peerIP string) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	peerIP = strings.TrimSpace(peerIP)
	if peerIP == "" {
		err := errors.New("请选择一个可信设备")
		c.state.Message = err.Error()
		return err
	}
	report, err := c.manager.NetworkPath(ctx, peerIP)
	if err != nil {
		c.state.NetworkPath = report
		c.state.Message = "网络路径检测失败：" + err.Error()
		return err
	}
	c.state.NetworkPath = copyNetworkPathReport(report)
	c.state.Message = networkPathStatusText(c.state)
	return nil
}

func (c *DirectController) ApplyNetworkBypass(ctx context.Context, candidateIndex int) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	report := c.state.NetworkPath
	if report.EndpointIP == "" {
		err := errors.New("请先检测网络路径，确认 Tailscale 直连 endpoint")
		c.state.Message = err.Error()
		return err
	}
	if candidateIndex < 0 || candidateIndex >= len(report.Candidates) {
		err := errors.New("请选择一个公网出口")
		c.state.Message = err.Error()
		return err
	}
	active, err := c.manager.ApplyNetworkBypass(ctx, netdiag.BypassRequest{
		PeerTailscaleIP: report.PeerTailscaleIP,
		EndpointIP:      report.EndpointIP,
		Candidate:       report.Candidates[candidateIndex],
	})
	if err != nil {
		c.state.Message = "临时绕过代理失败：" + err.Error()
		return err
	}
	c.state.ActiveBypass = active
	c.state.HasActiveBypass = true
	c.state.Message = "已临时绕过代理：" + active.EndpointIP
	return nil
}

func (c *DirectController) ClearNetworkBypass(ctx context.Context) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	if err := c.manager.ClearNetworkBypass(ctx); err != nil {
		c.state.Message = "撤销绕过失败：" + err.Error()
		return err
	}
	c.state.ActiveBypass = netdiag.ActiveBypass{}
	c.state.HasActiveBypass = false
	c.state.Message = "已撤销临时绕过"
	return nil
}

func (c *DirectController) RefreshPeerLatencies(ctx context.Context) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	if c.state.PeerLatencies == nil {
		c.state.PeerLatencies = map[string]PeerLatency{}
	}
	for _, peer := range c.state.Peers {
		key := peerLatencyKey(peer)
		peerIP := strings.TrimSpace(peer.TailscaleIP)
		if key == "" || peerIP == "" {
			continue
		}
		latency, err := c.manager.ProbePeerLatency(ctx, peerIP)
		if err != nil {
			c.state.PeerLatencies[key] = PeerLatency{Error: err.Error(), Updated: true}
			continue
		}
		c.state.PeerLatencies[key] = PeerLatency{Latency: latency, Updated: true}
	}
	return nil
}

func (c *DirectController) DetectClash(ctx context.Context) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	report, err := c.manager.DetectClash(ctx)
	if err != nil {
		c.state.Message = "检测代理/TUN 失败：" + err.Error()
		return err
	}
	c.state.ClashReport = copyClashReport(report)
	c.state.Message = "已检测代理/TUN"
	return nil
}

func (c *DirectController) RefreshClashNodes(ctx context.Context) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	report, err := c.manager.RefreshClashNodes(ctx)
	if err != nil {
		c.state.Message = "刷新节点延迟失败：" + err.Error()
		return err
	}
	c.state.ClashReport = copyClashReport(report)
	c.state.Message = "已刷新节点延迟"
	return nil
}

func (c *DirectController) ApplyClashNode(ctx context.Context, peerIP string, nodeIndex int) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	peerIP = strings.TrimSpace(peerIP)
	if peerIP == "" {
		err := errors.New("请选择可信设备或输入对方 Tailscale IP")
		c.state.Message = err.Error()
		return err
	}
	if nodeIndex < 0 || nodeIndex >= len(c.state.ClashReport.Nodes) {
		err := errors.New("请选择一个代理出口节点")
		c.state.Message = err.Error()
		return err
	}
	node := c.state.ClashReport.Nodes[nodeIndex]
	result, err := c.manager.ApplyClashNode(ctx, clash.ApplyRequest{
		PeerTailscaleIP: peerIP,
		GroupName:       node.GroupName,
		NodeName:        node.Name,
		PreviousNode:    currentNodeInGroup(c.state.ClashReport.Nodes, node.GroupName),
	})
	c.state.ClashApplyResult = result
	if err != nil {
		c.state.Message = "应用出口节点失败：" + err.Error()
		return err
	}
	c.state.Message = "已应用出口节点：" + node.Name + " · " + result.Latency
	return nil
}

func (c *DirectController) RestoreClashNode(ctx context.Context) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	if err := c.manager.RestoreClashNode(ctx); err != nil {
		c.state.Message = "恢复原节点失败：" + err.Error()
		return err
	}
	c.state.Message = "已恢复原节点"
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
	successMessage := "已配对并授权全端口访问：" + displayPeerName(peer.DeviceName, peer.DeviceID)
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
	state.LocalhostBridgePorts = copyInts(state.LocalhostBridgePorts)
	state.LocalhostBridgeConflictPorts = copyInts(state.LocalhostBridgeConflictPorts)
	state.NetworkPath = copyNetworkPathReport(state.NetworkPath)
	state.ClashReport = copyClashReport(state.ClashReport)
	state.PeerLatencies = copyPeerLatencies(state.PeerLatencies)
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

func copyPeerLatencies(latencies map[string]PeerLatency) map[string]PeerLatency {
	if len(latencies) == 0 {
		return nil
	}
	result := make(map[string]PeerLatency, len(latencies))
	for key, value := range latencies {
		result[key] = value
	}
	return result
}

func peerLatencyKey(peer directmanager.TrustedPeer) string {
	if strings.TrimSpace(peer.ID) != "" {
		return peer.ID
	}
	return strings.TrimSpace(peer.TailscaleIP)
}

func copyInts(values []int) []int {
	return append([]int(nil), values...)
}

func copyNetworkPathReport(report netdiag.PeerPathReport) netdiag.PeerPathReport {
	report.Candidates = append([]netdiag.EgressCandidate(nil), report.Candidates...)
	return report
}

func copyClashReport(report clash.DiscoveryReport) clash.DiscoveryReport {
	report.TUNInterfaces = append([]clash.TUNInterface(nil), report.TUNInterfaces...)
	report.ProxyPorts = append([]clash.ProxyPort(nil), report.ProxyPorts...)
	report.Nodes = append([]clash.ProxyNode(nil), report.Nodes...)
	return report
}

func currentNodeInGroup(nodes []clash.ProxyNode, groupName string) string {
	for _, node := range nodes {
		if node.GroupName == groupName && node.Current {
			return node.Name
		}
	}
	return ""
}

func networkPathStatusText(state DirectState) string {
	report := state.NetworkPath
	switch report.Status {
	case netdiag.PathDirectNormal:
		return networkPathSummary("网络路径：直连正常", report)
	case netdiag.PathDirectTUNOptimized:
		return networkPathSummary("网络路径：TUN 接管但低延迟直连", report)
	case netdiag.PathDirectProxy:
		return networkPathSummary("网络路径：直连但疑似代理绕路", report)
	case netdiag.PathDERP:
		return networkPathSummary("网络路径：DERP 中继", report)
	case netdiag.PathFailed:
		return networkPathSummary("网络路径：检测失败", report)
	default:
		return "网络路径：未检测"
	}
}

func networkPathSummary(prefix string, report netdiag.PeerPathReport) string {
	parts := []string{prefix}
	if report.Endpoint != "" {
		parts = append(parts, report.Endpoint)
	}
	if report.Latency != "" {
		parts = append(parts, report.Latency)
	}
	if report.CurrentRoute.InterfaceAlias != "" {
		route := report.CurrentRoute.InterfaceAlias
		if report.CurrentRoute.NextHop != "" {
			route += " -> " + report.CurrentRoute.NextHop
		}
		parts = append(parts, route)
	}
	return strings.Join(parts, " · ")
}
