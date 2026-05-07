package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

type Dependencies struct {
	Manager       Manager
	Discovery     Discovery
	DirectManager DirectManager
	Timeout       time.Duration
}

type App struct {
	fyneApp    fyne.App
	window     fyne.Window
	deps       Dependencies
	ctrl       *Controller
	directCtrl *DirectController
	refreshUI  func()
}

func New(deps Dependencies) *App {
	a := app.NewWithID("com.absuq.portshare")
	return &App{
		fyneApp:    a,
		deps:       deps,
		ctrl:       NewController(deps),
		directCtrl: NewDirectController(deps.DirectManager),
	}
}

func (a *App) Run() {
	a.configureTray()
	a.window = a.buildMainWindow()
	a.window.SetCloseIntercept(func() {
		a.window.Hide()
	})
	a.window.ShowAndRun()
}
