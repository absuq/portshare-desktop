package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (a *App) buildMainWindow() fyne.Window {
	w := a.fyneApp.NewWindow("portshare")
	w.Resize(fyne.NewSize(980, 640))
	services := widget.NewList(
		func() int { return 0 },
		func() fyne.CanvasObject { return widget.NewLabel("本地服务") },
		func(widget.ListItemID, fyne.CanvasObject) {},
	)
	actions := container.NewVBox(
		widget.NewEntry(),
		widget.NewButton("添加服务", func() {}),
		widget.NewButton("刷新发现", func() {}),
		widget.NewButton("开放到 tailnet", func() {}),
		widget.NewButton("开启公网", func() {
			ShowPublicConfirm(w, func(PublicChoice) {})
		}),
	)
	w.SetContent(container.NewBorder(nil, nil, nil, actions, services))
	return w
}
