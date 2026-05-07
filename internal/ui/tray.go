package ui

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
)

func (a *App) configureTray() {
	desktop, ok := a.fyneApp.(interface {
		SetSystemTrayMenu(*fyne.Menu)
	})
	if !ok {
		return
	}
	menu := fyne.NewMenu("portshare",
		fyne.NewMenuItem("打开主界面", func() {
			if a.window != nil {
				a.window.Show()
			}
		}),
		fyne.NewMenuItem("检测 Tailscale", func() {
			a.runTrayAction(func(ctx context.Context) error {
				return a.directCtrl.Refresh(ctx)
			})
		}),
		fyne.NewMenuItem("停止直连监听", func() {
			a.runTrayAction(func(ctx context.Context) error {
				return a.directCtrl.StopDirectMode(ctx)
			})
		}),
		fyne.NewMenuItem("退出", func() {
			a.fyneApp.Quit()
		}),
	)
	desktop.SetSystemTrayMenu(menu)
}

func (a *App) runTrayAction(fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = fn(ctx)
	if a.refreshUI != nil {
		a.refreshUI()
	}
}
