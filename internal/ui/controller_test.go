package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
)

func TestControllerRefreshPopulatesServicesAndShareState(t *testing.T) {
	ctx := context.Background()
	svc := domain.LocalService{ID: "svc-3000", Name: "Vite App", Scheme: "http", Host: "127.0.0.1", Port: 3000}
	mgr := &fakeUIManager{
		status: []domain.Share{{
			ID:        "tailnet-3000",
			ServiceID: "svc-3000",
			Mode:      domain.ModeTailnet,
			PublicURL: "https://vite.tailnet",
			Status:    domain.ShareActive,
		}},
	}
	ctrl := NewController(Dependencies{
		Manager:   mgr,
		Discovery: &fakeDiscovery{scan: []domain.LocalService{svc}},
	})

	if err := ctrl.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	if len(state.Services) != 1 {
		t.Fatalf("expected one service, got %+v", state.Services)
	}
	if state.Services[0].Name != "Vite App" {
		t.Fatalf("expected service name, got %+v", state.Services[0])
	}
	if state.Services[0].TailnetURL != "https://vite.tailnet" {
		t.Fatalf("expected tailnet URL, got %+v", state.Services[0])
	}
	if !state.HasSelection || state.Selected != 0 {
		t.Fatalf("expected first service selected, got %+v", state)
	}
}

func TestControllerAddManualNormalizesAndSelectsService(t *testing.T) {
	ctx := context.Background()
	discovery := &fakeDiscovery{
		probeService: domain.LocalService{ID: "manual-5173", Name: "Manual", Scheme: "http", Host: "127.0.0.1", Port: 5173},
	}
	ctrl := NewController(Dependencies{
		Manager:   &fakeUIManager{},
		Discovery: discovery,
	})

	if err := ctrl.AddManual(ctx, "127.0.0.1:5173"); err != nil {
		t.Fatal(err)
	}

	if discovery.probedURL != "http://127.0.0.1:5173" {
		t.Fatalf("expected normalized URL, got %q", discovery.probedURL)
	}
	state := ctrl.State()
	if len(state.Services) != 1 || !state.HasSelection || state.Selected != 0 {
		t.Fatalf("expected added service selected, got %+v", state)
	}
}

func TestControllerPublishesSelectedService(t *testing.T) {
	ctx := context.Background()
	svc := domain.LocalService{ID: "svc-3000", Name: "Vite App", Scheme: "http", Host: "127.0.0.1", Port: 3000}
	mgr := &fakeUIManager{
		tailnetShare: domain.Share{ID: "tailnet-3000", ServiceID: "svc-3000", Mode: domain.ModeTailnet, PublicURL: "https://vite.tailnet", Status: domain.ShareActive},
		publicShare:  domain.Share{ID: "public-3000", ServiceID: "svc-3000", Mode: domain.ModePublic, PublicURL: "https://vite.example", Status: domain.ShareActive},
	}
	ctrl := NewController(Dependencies{
		Manager:   mgr,
		Discovery: &fakeDiscovery{scan: []domain.LocalService{svc}},
	})
	if err := ctrl.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := ctrl.PublishTailnet(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.PublishPublic(ctx, PublicChoice{TTL: 10 * time.Minute}); err != nil {
		t.Fatal(err)
	}

	if mgr.tailnetService.ID != "svc-3000" {
		t.Fatalf("expected tailnet publish to use selected service, got %+v", mgr.tailnetService)
	}
	if mgr.publicService.ID != "svc-3000" || mgr.publicTTL != 10*time.Minute || mgr.publicLongRunning {
		t.Fatalf("expected public publish choice to be forwarded, got svc=%+v ttl=%s long=%v", mgr.publicService, mgr.publicTTL, mgr.publicLongRunning)
	}
	state := ctrl.State()
	if state.Services[0].TailnetURL != "https://vite.tailnet" || state.Services[0].PublicURL != "https://vite.example" {
		t.Fatalf("expected published URLs in state, got %+v", state.Services[0])
	}
}

func TestControllerRefreshPreservesCurrentSessionSharesWhenStatusIsEmpty(t *testing.T) {
	ctx := context.Background()
	svc := domain.LocalService{ID: "svc-3000", Name: "Vite App", Scheme: "http", Host: "127.0.0.1", Port: 3000}
	mgr := &fakeUIManager{
		tailnetShare: domain.Share{ID: "tailnet-3000", ServiceID: "svc-3000", Mode: domain.ModeTailnet, PublicURL: "https://vite.tailnet", Status: domain.ShareActive},
	}
	ctrl := NewController(Dependencies{
		Manager:   mgr,
		Discovery: &fakeDiscovery{scan: []domain.LocalService{svc}},
	})
	if err := ctrl.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.PublishTailnet(ctx); err != nil {
		t.Fatal(err)
	}

	if err := ctrl.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	if state.Services[0].TailnetURL != "https://vite.tailnet" {
		t.Fatalf("expected current-session share to remain visible, got %+v", state.Services[0])
	}
}

func TestControllerStopSelectedStopsAllSharesForService(t *testing.T) {
	ctx := context.Background()
	svc := domain.LocalService{ID: "svc-3000", Name: "Vite App", Scheme: "http", Host: "127.0.0.1", Port: 3000}
	mgr := &fakeUIManager{
		status: []domain.Share{
			{ID: "tailnet-3000", ServiceID: "svc-3000", Mode: domain.ModeTailnet, PublicURL: "https://vite.tailnet", Status: domain.ShareActive},
			{ID: "public-3000", ServiceID: "svc-3000", Mode: domain.ModePublic, PublicURL: "https://vite.example", Status: domain.ShareActive},
		},
	}
	ctrl := NewController(Dependencies{
		Manager:   mgr,
		Discovery: &fakeDiscovery{scan: []domain.LocalService{svc}},
	})
	if err := ctrl.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	if err := ctrl.StopSelected(ctx); err != nil {
		t.Fatal(err)
	}

	if len(mgr.stopped) != 2 {
		t.Fatalf("expected both shares stopped, got %+v", mgr.stopped)
	}
	state := ctrl.State()
	if state.Services[0].TailnetURL != "" || state.Services[0].PublicURL != "" {
		t.Fatalf("expected stopped shares removed from state, got %+v", state.Services[0])
	}
}

func TestControllerRequiresSelectedService(t *testing.T) {
	ctrl := NewController(Dependencies{
		Manager:   &fakeUIManager{},
		Discovery: &fakeDiscovery{},
	})

	if _, err := ctrl.PublishTailnet(context.Background()); !errors.Is(err, ErrNoServiceSelected) {
		t.Fatalf("expected ErrNoServiceSelected, got %v", err)
	}
}

type fakeDiscovery struct {
	scan         []domain.LocalService
	probedURL    string
	probeService domain.LocalService
	probeErr     error
}

func (d *fakeDiscovery) ScanCommon(time.Duration) []domain.LocalService {
	return append([]domain.LocalService(nil), d.scan...)
}

func (d *fakeDiscovery) Probe(rawURL string, _ time.Duration) (domain.LocalService, error) {
	d.probedURL = rawURL
	return d.probeService, d.probeErr
}

type fakeUIManager struct {
	status            []domain.Share
	tailnetShare      domain.Share
	publicShare       domain.Share
	tailnetService    domain.LocalService
	publicService     domain.LocalService
	publicTTL         time.Duration
	publicLongRunning bool
	stopped           []domain.Share
	stopAllPublic     bool
	stopAll           bool
}

func (m *fakeUIManager) PublishTailnet(_ context.Context, svc domain.LocalService) (domain.Share, error) {
	m.tailnetService = svc
	return m.tailnetShare, nil
}

func (m *fakeUIManager) PublishPublic(_ context.Context, svc domain.LocalService, ttl time.Duration, longRunning bool) (domain.Share, error) {
	m.publicService = svc
	m.publicTTL = ttl
	m.publicLongRunning = longRunning
	return m.publicShare, nil
}

func (m *fakeUIManager) Stop(_ context.Context, share domain.Share, _ string) error {
	m.stopped = append(m.stopped, share)
	for i := 0; i < len(m.status); i++ {
		if m.status[i].ID == share.ID {
			m.status = append(m.status[:i], m.status[i+1:]...)
			i--
		}
	}
	return nil
}

func (m *fakeUIManager) StopAllPublic(context.Context) error {
	m.stopAllPublic = true
	return nil
}

func (m *fakeUIManager) StopAll(context.Context) error {
	m.stopAll = true
	return nil
}

func (m *fakeUIManager) Status(context.Context) ([]domain.Share, error) {
	return append([]domain.Share(nil), m.status...), nil
}
