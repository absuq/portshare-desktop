package ui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/absuq/portshare-desktop/internal/domain"
)

type Dependencies struct {
	Manager interface {
		Status(context.Context) ([]domain.Share, error)
	}
}

type App struct {
	fyneApp fyne.App
	window  fyne.Window
	deps    Dependencies
}

func New(deps Dependencies) *App {
	a := app.NewWithID("com.absuq.portshare")
	return &App{fyneApp: a, deps: deps}
}

func (a *App) Run() {
	a.configureTray()
	a.window = a.buildMainWindow()
	a.window.SetCloseIntercept(func() {
		a.window.Hide()
	})
	a.window.ShowAndRun()
}
