package ui

import "fyne.io/fyne/v2"

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
		fyne.NewMenuItem("暂停所有公网", func() {}),
		fyne.NewMenuItem("停止全部发布", func() {}),
		fyne.NewMenuItem("退出", func() {
			a.fyneApp.Quit()
		}),
	)
	desktop.SetSystemTrayMenu(menu)
}
