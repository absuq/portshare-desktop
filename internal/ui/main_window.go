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
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/netdiag"
)

func (a *App) buildMainWindow() fyne.Window {
	w := a.fyneApp.NewWindow("portshare")
	w.Resize(fyne.NewSize(1040, 680))

	var state DirectState
	var selectedPeerID string
	var selectedCandidateIndex = -1
	var candidateOptions []string
	var render func()

	statusLabel := widget.NewLabel("Tailscale：未检测")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	ipLabel := widget.NewLabel("本机 IP：-")
	controlLabel := widget.NewLabel("portshare：未启用")
	bridgeLabel := widget.NewLabel("localhost 桥接：无")
	bridgeConflictLabel := widget.NewLabel("localhost 冲突：无")
	networkPathLabel := widget.NewLabel("网络路径：未检测")
	networkRouteLabel := widget.NewLabel("当前出口：-")
	bypassLabel := widget.NewLabel("临时绕过：未启用")
	messageLabel := widget.NewLabel("准备就绪")
	for _, label := range []*widget.Label{statusLabel, ipLabel, controlLabel, bridgeLabel, bridgeConflictLabel, networkPathLabel, networkRouteLabel, bypassLabel, messageLabel} {
		label.Wrapping = fyne.TextWrapWord
	}

	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetPlaceHolder("共享密钥")
	peerEntry := widget.NewEntry()
	peerEntry.SetPlaceHolder("对方 Tailscale IP 或 MagicDNS")
	candidateSelect := widget.NewSelect(nil, func(value string) {
		selectedCandidateIndex = indexOfString(candidateOptions, value)
	})
	candidateSelect.PlaceHolder = "选择公网出口"

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
			meta.SetText(peerDisplayMeta(peer))
		},
	)
	peers.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(state.Peers) {
			selectedPeerID = ""
			return
		}
		selectedPeerID = state.Peers[id].ID
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

	render = func() {
		state = a.directCtrl.State()
		if selectedPeerID == "" && len(state.Peers) > 0 {
			selectedPeerID = state.Peers[0].ID
		}
		if !hasPeer(state.Peers, selectedPeerID) {
			selectedPeerID = ""
		}
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
		if state.Message != "" {
			messageLabel.SetText(state.Message)
		}
		peers.Refresh()
	}
	a.refreshUI = render

	statusBand := container.NewVBox(statusLabel, ipLabel, controlLabel, bridgeLabel, bridgeConflictLabel, networkPathLabel, networkRouteLabel, bypassLabel, messageLabel)
	setupPanel := container.NewVBox(
		widget.NewLabel("直连密钥"),
		secretEntry,
		startButton,
		refreshButton,
		widget.NewSeparator(),
		widget.NewLabel("配对"),
		peerEntry,
		pairButton,
		widget.NewSeparator(),
		widget.NewLabel("网络路径"),
		detectNetworkButton,
		candidateSelect,
		applyBypassButton,
		clearBypassButton,
	)
	peerPanel := container.NewBorder(
		container.NewVBox(widget.NewLabel("可信设备")),
		nil,
		nil,
		nil,
		peers,
	)

	main := container.NewHSplit(setupPanel, peerPanel)
	main.Offset = 0.32
	w.SetContent(container.NewBorder(statusBand, nil, nil, nil, main))
	render()
	return w
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

func peerDisplayMeta(peer directmanager.TrustedPeer) string {
	parts := []string{valueOrDash(peer.TailscaleIP)}
	if !peer.AccessAuthorizedAt.IsZero() {
		parts = append(parts, "已授权全端口")
	}
	if peer.LastRoute != "" {
		parts = append(parts, peer.LastRoute)
	}
	return strings.Join(parts, " · ")
}

func hasPeer(peers []directmanager.TrustedPeer, id string) bool {
	for _, peer := range peers {
		if peer.ID == id {
			return true
		}
	}
	return false
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
		return "临时绕过：未启用"
	}
	return "临时绕过：" + state.ActiveBypass.EndpointIP + " -> " + state.ActiveBypass.NextHop
}

func egressCandidateOptions(candidates []netdiag.EgressCandidate) []string {
	options := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		label := candidate.InterfaceAlias + " -> " + candidate.NextHop
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
