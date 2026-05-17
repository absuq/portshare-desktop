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

	latencyRefresh     peerLatencyRefreshControl
	startWindowRefresh func()
	stopWindowRefresh  func()
}

func New(deps Dependencies) *App {
	a := app.NewWithID("com.absuq.portshare")
	a.SetIcon(portshareIconResource())
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
		a.stopDirectLatencyRefresh()
		a.window.Hide()
	})
	a.window.ShowAndRun()
}

func (a *App) startDirectLatencyRefresh() {
	if a.startWindowRefresh != nil {
		a.startWindowRefresh()
	}
}

func (a *App) stopDirectLatencyRefresh() {
	if a.stopWindowRefresh != nil {
		a.stopWindowRefresh()
	}
}
