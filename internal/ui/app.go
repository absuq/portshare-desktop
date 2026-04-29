package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

type App struct {
	fyneApp fyne.App
	window  fyne.Window
}

func New() *App {
	a := app.NewWithID("com.absuq.portshare")
	return &App{fyneApp: a}
}

func (a *App) Run() {
	a.configureTray()
	a.window = a.buildMainWindow()
	a.window.SetCloseIntercept(func() {
		a.window.Hide()
	})
	a.window.ShowAndRun()
}
