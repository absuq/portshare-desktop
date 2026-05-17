package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type PublicChoice struct {
	TTL         time.Duration
	LongRunning bool
}

func ShowPublicConfirm(parent fyne.Window, onConfirm func(PublicChoice)) {
	duration := widget.NewSelect([]string{"10 分钟", "30 分钟", "1 小时", "长期开放"}, nil)
	duration.SetSelected("30 分钟")
	dialog.ShowForm("确认开启公网", "确认", "取消", []*widget.FormItem{
		widget.NewFormItem("风险提示", widget.NewLabel("公网开放会让非 tailnet 设备访问该服务，请确认服务本身已有保护。")),
		widget.NewFormItem("开放时长", duration),
	}, func(ok bool) {
		if !ok {
			return
		}
		choice := PublicChoice{TTL: 30 * time.Minute}
		switch duration.Selected {
		case "10 分钟":
			choice.TTL = 10 * time.Minute
		case "1 小时":
			choice.TTL = time.Hour
		case "长期开放":
			choice.LongRunning = true
			choice.TTL = 0
		}
		onConfirm(choice)
	}, parent)
}
