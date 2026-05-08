package ui

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
)

func (a *App) buildMainWindow() fyne.Window {
	w := a.fyneApp.NewWindow("portshare")
	w.Resize(fyne.NewSize(1040, 680))

	var state DirectState
	var selectedPeerID string
	var selectedForwardID string
	var render func()

	statusLabel := widget.NewLabel("Tailscale：未检测")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	ipLabel := widget.NewLabel("本机 IP：-")
	messageLabel := widget.NewLabel("准备就绪")
	for _, label := range []*widget.Label{statusLabel, ipLabel, messageLabel} {
		label.Wrapping = fyne.TextWrapWord
	}

	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetPlaceHolder("共享密钥")
	peerEntry := widget.NewEntry()
	peerEntry.SetPlaceHolder("对方 Tailscale IP 或 MagicDNS")
	targetHostEntry := widget.NewEntry()
	targetHostEntry.SetPlaceHolder("远端 host")
	targetHostEntry.SetText("127.0.0.1")
	targetPortEntry := widget.NewEntry()
	targetPortEntry.SetPlaceHolder("远端端口")
	localPortEntry := widget.NewEntry()
	localPortEntry.SetPlaceHolder("本地端口，留空自动分配")

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

	forwards := widget.NewList(
		func() int { return len(state.Forwards) },
		func() fyne.CanvasObject {
			local := widget.NewLabel("本地入口")
			local.TextStyle = fyne.TextStyle{Bold: true}
			target := widget.NewLabel("远端目标")
			target.Wrapping = fyne.TextWrapWord
			return container.NewVBox(local, target)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(state.Forwards) {
				return
			}
			forward := state.Forwards[id]
			box := obj.(*fyne.Container)
			local := box.Objects[0].(*widget.Label)
			target := box.Objects[1].(*widget.Label)
			local.SetText(forward.LocalAddress)
			target.SetText(forward.Target)
		},
	)
	forwards.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(state.Forwards) {
			selectedForwardID = ""
			return
		}
		selectedForwardID = state.Forwards[id].ID
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
			return a.directCtrl.PairPeerWithSecret(
				ctx,
				peerEntry.Text,
				secretEntry.Text,
				net.JoinHostPort(current.LocalTailscaleIP, defaultDirectControlPort),
			)
		})
	})

	var stopForwardButton *widget.Button
	forwardButton := widget.NewButton("创建本地转发", func() {
		withTimeout(func(ctx context.Context) error {
			peerID := selectedPeerID
			current := a.directCtrl.State()
			if peerID == "" && len(current.Peers) > 0 {
				peerID = current.Peers[0].ID
			}
			remotePort, err := strconv.Atoi(strings.TrimSpace(targetPortEntry.Text))
			if err != nil {
				return fmt.Errorf("远端端口无效：%w", err)
			}
			if remotePort <= 0 || remotePort > 65535 {
				return errors.New("远端端口必须在 1-65535 之间")
			}
			localAddress, err := localListenAddress(localPortEntry.Text)
			if err != nil {
				return err
			}
			return a.directCtrl.CreateForward(
				ctx,
				peerID,
				targetHostEntry.Text,
				remotePort,
				localAddress,
			)
		})
	})
	stopForwardButton = widget.NewButton("停止选中转发", func() {
		withTimeout(func(ctx context.Context) error {
			if selectedForwardID == "" {
				return errors.New("请先选择一个转发")
			}
			return a.directCtrl.StopForward(ctx, selectedForwardID)
		})
	})

	render = func() {
		state = a.directCtrl.State()
		if state.Ready {
			statusLabel.SetText("Tailscale：ready")
		} else {
			statusLabel.SetText("Tailscale：未就绪")
		}
		ipLabel.SetText("本机 IP：" + valueOrDash(state.LocalTailscaleIP))
		if state.Message != "" {
			messageLabel.SetText(state.Message)
		}
		peers.Refresh()
		forwards.Refresh()
		if selectedPeerID == "" && len(state.Peers) > 0 {
			selectedPeerID = state.Peers[0].ID
		}
		if !hasPeer(state.Peers, selectedPeerID) {
			selectedPeerID = ""
		}
		if !hasForward(state.Forwards, selectedForwardID) {
			selectedForwardID = ""
		}
		if len(state.Peers) == 0 {
			forwardButton.Disable()
		} else {
			forwardButton.Enable()
		}
		if selectedForwardID == "" {
			stopForwardButton.Disable()
		} else {
			stopForwardButton.Enable()
		}
	}
	a.refreshUI = render

	statusBand := container.NewVBox(statusLabel, ipLabel, messageLabel)
	setupPanel := container.NewVBox(
		widget.NewLabel("直连密钥"),
		secretEntry,
		startButton,
		refreshButton,
		widget.NewSeparator(),
		widget.NewLabel("配对"),
		peerEntry,
		pairButton,
	)
	peerPanel := container.NewBorder(
		container.NewVBox(widget.NewLabel("可信设备")),
		nil,
		nil,
		nil,
		peers,
	)
	forwardPanel := container.NewBorder(
		container.NewVBox(
			widget.NewLabel("本地转发"),
			targetHostEntry,
			targetPortEntry,
			localPortEntry,
			forwardButton,
			stopForwardButton,
		),
		nil,
		nil,
		nil,
		forwards,
	)

	lists := container.NewHSplit(peerPanel, forwardPanel)
	lists.Offset = 0.45
	main := container.NewHSplit(setupPanel, lists)
	main.Offset = 0.28
	w.SetContent(container.NewBorder(statusBand, nil, nil, nil, main))
	render()
	return w
}

func localListenAddress(portText string) (string, error) {
	portText = strings.TrimSpace(portText)
	if portText == "" {
		return "127.0.0.1:0", nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", errors.New("本地端口必须在 1-65535 之间")
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil
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

func hasForward(forwards []directmanager.RunningForward, id string) bool {
	for _, forward := range forwards {
		if forward.ID == id {
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
