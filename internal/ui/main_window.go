package ui

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/absuq/portshare-desktop/internal/clash"
	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/netdiag"
)

const (
	peerLatencyRefreshInterval = 500 * time.Millisecond
	peerLatencyProbeTimeout    = 150 * time.Millisecond
)

func (a *App) buildMainWindow() fyne.Window {
	w := a.fyneApp.NewWindow("portshare")
	w.Resize(fyne.NewSize(1040, 680))

	var state DirectState
	var selectedPeerID string
	var hasExplicitPeerSelection bool
	var selectedCandidateIndex = -1
	var candidateOptions []string
	var selectedClashNodeIndex = -1
	var clashOptions []string
	var render func()

	summaryLabel := widget.NewLabel("Tailscale：未检测 · IP - · 未监听")
	summaryLabel.TextStyle = fyne.TextStyle{Bold: true}
	summaryLabel.Wrapping = fyne.TextWrapOff
	summaryLabel.Truncation = fyne.TextTruncateEllipsis
	statusLabel := widget.NewLabel("Tailscale：未检测")
	ipLabel := widget.NewLabel("本机 IP：-")
	controlLabel := widget.NewLabel("portshare：未启用")
	bridgeLabel := widget.NewLabel("localhost 桥接：无")
	bridgeConflictLabel := widget.NewLabel("localhost 冲突：无")
	networkPathLabel := widget.NewLabel("网络路径：未检测")
	networkRouteLabel := widget.NewLabel("当前出口：-")
	bypassLabel := widget.NewLabel("临时路由：未启用")
	linkGuardianLabel := widget.NewLabel("链路守护：待优化")
	clashTunLabel := widget.NewLabel("TUN：未检测")
	clashPortsLabel := widget.NewLabel("代理入口：未检测")
	clashControlLabel := widget.NewLabel("控制接口：未检测")
	clashResultLabel := widget.NewLabel("出口优化：未应用")
	messageLabel := widget.NewLabel("准备就绪")
	messageLabel.Wrapping = fyne.TextWrapOff
	messageLabel.Truncation = fyne.TextTruncateEllipsis
	for _, label := range []*widget.Label{statusLabel, ipLabel, controlLabel, bridgeLabel, bridgeConflictLabel, networkPathLabel, networkRouteLabel, bypassLabel, linkGuardianLabel, clashTunLabel, clashPortsLabel, clashControlLabel, clashResultLabel, messageLabel} {
		label.Wrapping = fyne.TextWrapWord
	}
	messageLabel.Wrapping = fyne.TextWrapOff

	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetPlaceHolder("共享密钥")
	peerEntry := widget.NewEntry()
	peerEntry.SetPlaceHolder("对方 Tailscale IP 或 MagicDNS")
	candidateSelect := widget.NewSelect(nil, func(value string) {
		selectedCandidateIndex = indexOfString(candidateOptions, value)
	})
	candidateSelect.PlaceHolder = "选择公网出口"
	autoBypassCheck := widget.NewCheck("自动精确绕过", nil)
	autoBypassCheck.SetChecked(true)
	clashNodeSelect := widget.NewSelect(nil, func(value string) {
		selectedClashNodeIndex = indexOfString(clashOptions, value)
	})
	clashNodeSelect.PlaceHolder = "选择代理出口节点"

	peers := widget.NewList(
		func() int { return len(state.Peers) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("可信设备")
			name.TextStyle = fyne.TextStyle{Bold: true}
			meta := widget.NewLabel("Tailscale IP")
			meta.Wrapping = fyne.TextWrapWord
			return container.NewVBox(name, meta)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(state.Peers) {
				return
			}
			peer := state.Peers[id]
			box := obj.(*fyne.Container)
			name := box.Objects[0].(*widget.Label)
			meta := box.Objects[1].(*widget.Label)
			name.SetText(peerDisplayName(peer))
			meta.SetText(peerDisplayMeta(peer, state.PeerLatencies[peerLatencyKey(peer)]))
		},
	)
	peers.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(state.Peers) {
			selectedPeerID = ""
			hasExplicitPeerSelection = false
			return
		}
		selectedPeerID = state.Peers[id].ID
		hasExplicitPeerSelection = true
		render()
	}

	withTimeout := func(fn func(context.Context) error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := fn(ctx); err != nil {
			dialog.ShowError(err, w)
		}
		render()
	}

	refreshButton := widget.NewButton("检测 Tailscale", func() {
		withTimeout(func(ctx context.Context) error {
			return a.directCtrl.Refresh(ctx)
		})
	})
	startButton := widget.NewButton("启用直连密钥", func() {
		withTimeout(func(ctx context.Context) error {
			current := a.directCtrl.State()
			if current.LocalTailscaleIP == "" {
				return errors.New("请先检测 Tailscale")
			}
			return a.directCtrl.StartDirectMode(ctx, secretEntry.Text, net.JoinHostPort(current.LocalTailscaleIP, defaultDirectControlPort))
		})
	})
	pairButton := widget.NewButton("配对设备", func() {
		withTimeout(func(ctx context.Context) error {
			if _, err := normalizePeerControlAddress(peerEntry.Text); err != nil {
				return err
			}
			if strings.TrimSpace(secretEntry.Text) == "" {
				secret, err := generatePairingSecret()
				if err != nil {
					return err
				}
				secretEntry.SetText(secret)
				dialog.ShowInformation(
					"配对密钥",
					"已生成配对密钥：\n\n"+secret+"\n\n请让对方也输入这个密钥并点击“启用直连密钥”，然后再次点击“配对设备”。",
					w,
				)
				return nil
			}
			current := a.directCtrl.State()
			if current.LocalTailscaleIP == "" {
				if err := a.directCtrl.Refresh(ctx); err != nil {
					return err
				}
				current = a.directCtrl.State()
			}
			if current.LocalTailscaleIP == "" {
				return errors.New("请先检测 Tailscale，确认本机 IP 后再配对")
			}
			if err := a.directCtrl.PairPeerWithSecret(
				ctx,
				peerEntry.Text,
				secretEntry.Text,
				net.JoinHostPort(current.LocalTailscaleIP, defaultDirectControlPort),
			); err != nil {
				return err
			}
			dialog.ShowInformation("配对成功", pairSuccessDialogMessage(a.directCtrl.State()), w)
			return nil
		})
	})
	removePeerButton := widget.NewButton("删除可信设备", func() {
		peerID := selectedPeerID
		if !canRemoveSelectedPeer(state.Peers, peerID, hasExplicitPeerSelection) {
			dialog.ShowInformation("删除可信设备", "请先选择一个可信设备。", w)
			return
		}
		peerName := peerDisplayNameByID(state.Peers, peerID)
		dialog.ShowConfirm("删除可信设备", "确定删除 "+peerName+" 并撤销防火墙授权吗？", func(confirm bool) {
			if !confirm {
				return
			}
			withTimeout(func(ctx context.Context) error {
				return a.directCtrl.RemoveTrustedPeer(ctx, peerID)
			})
		}, w)
	})
	detectNetworkButton := widget.NewButton("检测网络路径", func() {
		withTimeout(func(ctx context.Context) error {
			peerIP := selectedPeerTailscaleIP(state.Peers, selectedPeerID, peerEntry.Text)
			if peerIP == "" {
				return errors.New("请先选择可信设备或输入对方 Tailscale IP")
			}
			return a.directCtrl.DetectNetworkPath(ctx, peerIP)
		})
	})
	applyBypassButton := widget.NewButton("临时绕过代理", func() {
		withTimeout(func(ctx context.Context) error {
			return a.directCtrl.ApplyNetworkBypass(ctx, selectedCandidateIndex)
		})
	})
	clearBypassButton := widget.NewButton("撤销绕过", func() {
		withTimeout(func(ctx context.Context) error {
			return a.directCtrl.ClearNetworkBypass(ctx)
		})
	})
	optimizeLinkButton := widget.NewButton("重新优化", func() {
		withTimeout(func(ctx context.Context) error {
			peerIP := selectedPeerTailscaleIP(state.Peers, selectedPeerID, peerEntry.Text)
			if peerIP == "" {
				return errors.New("请先选择可信设备或输入对方 Tailscale IP")
			}
			return a.directCtrl.OptimizeLink(ctx, peerIP, autoBypassCheck.Checked)
		})
	})
	detectClashButton := widget.NewButton("检测代理/TUN", func() {
		withTimeout(func(ctx context.Context) error {
			return a.directCtrl.DetectClash(ctx)
		})
	})
	refreshClashNodesButton := widget.NewButton("刷新节点延迟", func() {
		withTimeout(func(ctx context.Context) error {
			return a.directCtrl.RefreshClashNodes(ctx)
		})
	})
	applyClashNodeButton := widget.NewButton("应用出口节点", func() {
		withTimeout(func(ctx context.Context) error {
			peerIP := selectedPeerTailscaleIP(state.Peers, selectedPeerID, peerEntry.Text)
			return a.directCtrl.ApplyClashNode(ctx, peerIP, selectedClashNodeIndex)
		})
	})
	restoreClashNodeButton := widget.NewButton("恢复原节点", func() {
		withTimeout(func(ctx context.Context) error {
			return a.directCtrl.RestoreClashNode(ctx)
		})
	})

	render = func() {
		state = a.directCtrl.State()
		selectedPeerID, hasExplicitPeerSelection = reconcileSelectedPeer(state.Peers, selectedPeerID, hasExplicitPeerSelection)
		if state.Ready {
			statusLabel.SetText("Tailscale：ready")
		} else {
			statusLabel.SetText("Tailscale：未就绪")
		}
		ipLabel.SetText("本机 IP：" + valueOrDash(state.LocalTailscaleIP))
		if state.ControlListening {
			controlLabel.SetText("portshare：监听中 " + valueOrDash(state.ControlAddress))
		} else {
			controlLabel.SetText("portshare：未启用")
		}
		bridgeLabel.SetText(localhostBridgeStatusText(state))
		bridgeConflictLabel.SetText(localhostBridgeConflictStatusText(state))
		networkPathLabel.SetText(networkPathStatusText(state))
		networkRouteLabel.SetText(networkRouteDetailText(state))
		bypassLabel.SetText(activeBypassStatusText(state))
		linkGuardianLabel.SetText(linkGuardianStatusText(state))
		clashTunLabel.SetText(clashTUNStatusText(state))
		clashPortsLabel.SetText(clashProxyPortsText(state))
		clashControlLabel.SetText(clashControlText(state))
		clashResultLabel.SetText(clashApplyResultText(state))
		summaryLabel.SetText(compactStatusSummaryText(state))

		options := egressCandidateOptions(state.NetworkPath.Candidates)
		candidateOptions = options
		candidateSelect.SetOptions(options)
		if len(options) == 0 {
			selectedCandidateIndex = -1
			candidateSelect.ClearSelected()
			candidateSelect.Disable()
		} else {
			candidateSelect.Enable()
			if selectedCandidateIndex < 0 || selectedCandidateIndex >= len(options) {
				selectedCandidateIndex = recommendedCandidateIndex(state.NetworkPath.Candidates)
			}
			if selectedCandidateIndex >= 0 && selectedCandidateIndex < len(options) {
				candidateSelect.SetSelected(options[selectedCandidateIndex])
			}
		}
		clashOptions = clashNodeOptions(state.ClashReport.Nodes)
		clashNodeSelect.SetOptions(clashOptions)
		if len(clashOptions) == 0 {
			selectedClashNodeIndex = -1
			clashNodeSelect.ClearSelected()
			clashNodeSelect.Disable()
		} else {
			clashNodeSelect.Enable()
			if selectedClashNodeIndex < 0 || selectedClashNodeIndex >= len(clashOptions) {
				selectedClashNodeIndex = currentClashNodeIndex(state.ClashReport.Nodes)
			}
			if selectedClashNodeIndex >= 0 && selectedClashNodeIndex < len(clashOptions) {
				clashNodeSelect.SetSelected(clashOptions[selectedClashNodeIndex])
			}
		}
		if state.Message != "" {
			messageLabel.SetText(state.Message)
		}
		if canRemoveSelectedPeer(state.Peers, selectedPeerID, hasExplicitPeerSelection) {
			removePeerButton.Enable()
		} else {
			removePeerButton.Disable()
		}
		peers.Refresh()
	}
	a.refreshUI = render

	statusBand := container.NewVBox(summaryLabel, messageLabel)
	connectionPage := scrollPage(container.NewVBox(
		widget.NewLabel("直连密钥"),
		secretEntry,
		startButton,
		refreshButton,
		widget.NewSeparator(),
		widget.NewLabel("配对"),
		peerEntry,
		pairButton,
	))
	networkPage := scrollPage(container.NewVBox(
		widget.NewLabel("网络路径"),
		networkPathLabel,
		networkRouteLabel,
		bypassLabel,
		linkGuardianLabel,
		detectNetworkButton,
		autoBypassCheck,
		optimizeLinkButton,
		candidateSelect,
		applyBypassButton,
		clearBypassButton,
	))
	egressPage := scrollPage(container.NewVBox(
		widget.NewLabel("出口优化"),
		clashResultLabel,
		clashTunLabel,
		clashPortsLabel,
		clashControlLabel,
		detectClashButton,
		refreshClashNodesButton,
		clashNodeSelect,
		applyClashNodeButton,
		restoreClashNodeButton,
	))
	statusPage := scrollPage(container.NewVBox(
		widget.NewLabel("运行状态"),
		statusLabel,
		ipLabel,
		controlLabel,
		bridgeLabel,
		bridgeConflictLabel,
	))
	setupPanel := container.NewAppTabs(
		container.NewTabItem("连接", connectionPage),
		container.NewTabItem("网络", networkPage),
		container.NewTabItem("出口", egressPage),
		container.NewTabItem("状态", statusPage),
	)
	setupPanel.SetTabLocation(container.TabLocationTop)
	peerPanel := container.NewBorder(
		container.NewVBox(widget.NewLabel("可信设备"), removePeerButton),
		nil,
		nil,
		nil,
		peers,
	)

	main := container.NewHSplit(setupPanel, peerPanel)
	main.Offset = 0.32
	w.SetContent(container.NewBorder(statusBand, nil, nil, nil, main))
	render()
	refreshLatencies := func(ctx context.Context) {
		_ = a.directCtrl.RefreshPeerLatencies(ctx)
		render()
	}
	a.startWindowRefresh = func() {
		a.latencyRefresh.Start(peerLatencyRefreshInterval, peerLatencyProbeTimeout, refreshLatencies)
	}
	a.stopWindowRefresh = a.latencyRefresh.Stop
	w.SetOnClosed(a.stopDirectLatencyRefresh)
	a.startDirectLatencyRefresh()
	return w
}

type peerLatencyRefreshControl struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (c *peerLatencyRefreshControl) Start(interval time.Duration, timeout time.Duration, refresh func(context.Context)) {
	if c == nil || refresh == nil {
		return
	}
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.mu.Unlock()

	startPeerLatencyRefresh(ctx, interval, timeout, refresh)
}

func (c *peerLatencyRefreshControl) Stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func startPeerLatencyRefresh(ctx context.Context, interval time.Duration, timeout time.Duration, refresh func(context.Context)) {
	if interval <= 0 || refresh == nil {
		return
	}
	if timeout <= 0 || timeout > interval {
		timeout = interval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tickCtx, cancel := context.WithTimeout(ctx, timeout)
				refresh(tickCtx)
				cancel()
			}
		}
	}()
}

func scrollPage(content fyne.CanvasObject) *container.Scroll {
	scroll := container.NewScroll(content)
	scroll.SetMinSize(fyne.NewSize(300, 220))
	return scroll
}

func compactStatusSummaryText(state DirectState) string {
	parts := make([]string, 0, 4)
	if state.Ready {
		parts = append(parts, "Tailscale：ready")
	} else {
		parts = append(parts, "Tailscale：未就绪")
	}
	parts = append(parts, "IP "+valueOrDash(state.LocalTailscaleIP))
	if state.ControlListening {
		parts = append(parts, "监听中")
	} else {
		parts = append(parts, "未监听")
	}
	if state.ClashApplyResult.NodeName != "" {
		egress := "出口 " + state.ClashApplyResult.NodeName
		if state.ClashApplyResult.RouteType != "" {
			egress += " " + state.ClashApplyResult.RouteType
		}
		if state.ClashApplyResult.Latency != "" {
			egress += " " + state.ClashApplyResult.Latency
		}
		parts = append(parts, egress)
	}
	return strings.Join(parts, " · ")
}

const pairingSecretAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func generatePairingSecret() (string, error) {
	randomBytes := make([]byte, 20)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("生成配对密钥失败：%w", err)
	}

	var builder strings.Builder
	builder.Grow(24)
	for i, value := range randomBytes {
		if i > 0 && i%4 == 0 {
			builder.WriteByte('-')
		}
		builder.WriteByte(pairingSecretAlphabet[int(value)%len(pairingSecretAlphabet)])
	}
	return builder.String(), nil
}

func peerDisplayName(peer directmanager.TrustedPeer) string {
	if peer.DisplayName != "" {
		return peer.DisplayName
	}
	return peer.ID
}

func peerDisplayNameByID(peers []directmanager.TrustedPeer, id string) string {
	for _, peer := range peers {
		if peer.ID == id {
			return peerDisplayName(peer)
		}
	}
	return id
}

func peerDisplayMeta(peer directmanager.TrustedPeer, latency PeerLatency) string {
	parts := []string{valueOrDash(peer.TailscaleIP)}
	if !peer.AccessAuthorizedAt.IsZero() {
		parts = append(parts, "已授权全端口")
	}
	if peer.LastRoute != "" {
		parts = append(parts, peer.LastRoute)
	}
	if latency.Updated {
		parts = append(parts, peerLatencyText(latency))
	}
	return strings.Join(parts, " · ")
}

func peerLatencyText(latency PeerLatency) string {
	if latency.Latency > 0 {
		return fmt.Sprintf("%dms", latency.Latency.Milliseconds())
	}
	return "延迟 -"
}

func hasPeer(peers []directmanager.TrustedPeer, id string) bool {
	for _, peer := range peers {
		if peer.ID == id {
			return true
		}
	}
	return false
}

func reconcileSelectedPeer(peers []directmanager.TrustedPeer, selectedPeerID string, hasExplicitPeerSelection bool) (string, bool) {
	if selectedPeerID != "" && !hasPeer(peers, selectedPeerID) {
		selectedPeerID = ""
		hasExplicitPeerSelection = false
	}
	if selectedPeerID == "" && len(peers) > 0 {
		selectedPeerID = peers[0].ID
	}
	return selectedPeerID, hasExplicitPeerSelection
}

func canRemoveSelectedPeer(peers []directmanager.TrustedPeer, selectedPeerID string, hasExplicitPeerSelection bool) bool {
	return hasExplicitPeerSelection && selectedPeerID != "" && hasPeer(peers, selectedPeerID)
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func pairSuccessDialogMessage(state DirectState) string {
	if strings.HasPrefix(state.Message, "已配对并授权全端口访问：") {
		return state.Message
	}
	return "配对成功，已授权对方 Tailscale IP 访问本机全端口。"
}

func localhostBridgeStatusText(state DirectState) string {
	if len(state.LocalhostBridgePorts) == 0 {
		return "localhost 桥接：无"
	}
	ports := append([]int(nil), state.LocalhostBridgePorts...)
	sort.Ints(ports)
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, strconv.Itoa(port))
	}
	return "localhost 桥接：" + strings.Join(parts, ", ")
}

func localhostBridgeConflictStatusText(state DirectState) string {
	if len(state.LocalhostBridgeConflictPorts) == 0 {
		return "localhost 冲突：无"
	}
	ports := append([]int(nil), state.LocalhostBridgeConflictPorts...)
	sort.Ints(ports)
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, strconv.Itoa(port))
	}
	return "localhost 冲突：" + strings.Join(parts, ", ") + " 原生监听，未桥接"
}

func networkRouteDetailText(state DirectState) string {
	report := state.NetworkPath
	if report.Endpoint == "" && report.CurrentRoute.InterfaceAlias == "" {
		return "当前出口：-"
	}
	parts := []string{}
	if report.Endpoint != "" {
		parts = append(parts, "endpoint "+report.Endpoint)
	}
	if report.CurrentRoute.InterfaceAlias != "" {
		route := report.CurrentRoute.InterfaceAlias
		if report.CurrentRoute.NextHop != "" {
			route += " -> " + report.CurrentRoute.NextHop
		}
		parts = append(parts, route)
	}
	return "当前出口：" + strings.Join(parts, " · ")
}

func activeBypassStatusText(state DirectState) string {
	if !state.HasActiveBypass {
		return "临时路由：未启用"
	}
	family := state.ActiveBypass.AddressFamily
	if family == "" {
		family = addressFamilyLabel(state.ActiveBypass.EndpointIP)
	}
	return "临时路由：" + family + " " + hostRoutePrefix(state.ActiveBypass.EndpointIP, family) + " -> " + state.ActiveBypass.NextHop
}

func egressCandidateOptions(candidates []netdiag.EgressCandidate) []string {
	options := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		label := candidate.InterfaceAlias
		if candidate.AddressFamily != "" {
			label += " " + candidate.AddressFamily
		}
		label += " -> " + candidate.NextHop
		details := []string{}
		if candidate.PublicIPv4 != "" {
			details = append(details, "公网 "+candidate.PublicIPv4)
		}
		if candidate.PublicIPv6 != "" {
			details = append(details, "公网IPv6 "+candidate.PublicIPv6)
		}
		if candidate.InterfaceIP != "" {
			details = append(details, "本机 "+candidate.InterfaceIP)
		}
		if candidate.NetcheckError != "" {
			details = append(details, "公网检测失败")
		}
		if len(details) > 0 {
			label += " (" + strings.Join(details, " / ") + ")"
		}
		if candidate.Recommended {
			label += " 推荐"
		}
		if candidate.SuspectedProxy {
			label += " 疑似代理"
		}
		options = append(options, label)
	}
	return options
}

func addressFamilyLabel(ip string) string {
	if strings.Contains(ip, ":") {
		return netdiag.AddressFamilyIPv6
	}
	return netdiag.AddressFamilyIPv4
}

func hostRoutePrefix(ip string, family string) string {
	if family == netdiag.AddressFamilyIPv6 || strings.Contains(ip, ":") {
		return ip + "/128"
	}
	return ip + "/32"
}

func recommendedCandidateIndex(candidates []netdiag.EgressCandidate) int {
	for i, candidate := range candidates {
		if candidate.Recommended {
			return i
		}
	}
	if len(candidates) == 0 {
		return -1
	}
	return 0
}

func indexOfString(values []string, value string) int {
	for i, item := range values {
		if item == value {
			return i
		}
	}
	return -1
}

func selectedPeerTailscaleIP(peers []directmanager.TrustedPeer, selectedPeerID string, fallback string) string {
	for _, peer := range peers {
		if peer.ID == selectedPeerID {
			return strings.TrimSpace(peer.TailscaleIP)
		}
	}
	address, err := normalizePeerControlAddress(fallback)
	if err != nil {
		return strings.TrimSpace(fallback)
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return strings.TrimSpace(fallback)
	}
	return strings.Trim(host, "[]")
}

func clashTUNStatusText(state DirectState) string {
	if len(state.ClashReport.TUNInterfaces) == 0 {
		return "TUN：未检测"
	}
	names := make([]string, 0, len(state.ClashReport.TUNInterfaces))
	for _, item := range state.ClashReport.TUNInterfaces {
		if strings.EqualFold(item.Status, "up") || item.Status == "Up" {
			names = append(names, item.Name+" 已启用")
		} else {
			names = append(names, item.Name+" "+item.Status)
		}
	}
	return "TUN：" + strings.Join(names, " / ")
}

func clashProxyPortsText(state DirectState) string {
	if len(state.ClashReport.ProxyPorts) == 0 {
		return "代理入口：未检测"
	}
	parts := make([]string, 0, len(state.ClashReport.ProxyPorts))
	for _, port := range state.ClashReport.ProxyPorts {
		parts = append(parts, fmt.Sprintf("%s %d", port.Kind, port.Port))
	}
	return "代理入口：" + strings.Join(parts, " / ")
}

func clashControlText(state DirectState) string {
	control := state.ClashReport.Control
	switch control.Kind {
	case clash.ControlNamedPipe:
		return "控制接口：named pipe " + control.Address
	case clash.ControlHTTP:
		return "控制接口：" + control.Address
	default:
		return "控制接口：未检测"
	}
}

func clashApplyResultText(state DirectState) string {
	result := state.ClashApplyResult
	if result.NodeName == "" {
		return "出口优化：未应用"
	}
	parts := []string{result.NodeName}
	if result.RouteType != "" {
		parts = append(parts, result.RouteType)
	}
	if result.Latency != "" {
		parts = append(parts, result.Latency)
	}
	return "出口优化：" + strings.Join(parts, " · ")
}

func clashNodeOptions(nodes []clash.ProxyNode) []string {
	options := make([]string, 0, len(nodes))
	for _, node := range nodes {
		parts := []string{node.Region, node.Name}
		if node.Delay > 0 {
			parts = append(parts, fmt.Sprintf("%dms", node.Delay.Milliseconds()))
		}
		if node.TailscaleLatency != "" {
			parts = append(parts, "Tailscale "+node.TailscaleLatency)
		}
		if node.Current {
			parts = append(parts, "当前")
		}
		options = append(options, strings.Join(parts, " · "))
	}
	return options
}

func currentClashNodeIndex(nodes []clash.ProxyNode) int {
	for i, node := range nodes {
		if node.Current {
			return i
		}
	}
	if len(nodes) == 0 {
		return -1
	}
	return 0
}
