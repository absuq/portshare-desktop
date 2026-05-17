package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/absuq/portshare-desktop/internal/audit"
	"github.com/absuq/portshare-desktop/internal/domain"
	"github.com/absuq/portshare-desktop/internal/provider"
)

type Manager struct {
	provider provider.Provider
	audit    audit.Log
	mu       sync.Mutex
	timers   map[string]*time.Timer
}

func New(p provider.Provider, log audit.Log) *Manager {
	return &Manager{provider: p, audit: log, timers: map[string]*time.Timer{}}
}

func (m *Manager) PublishTailnet(ctx context.Context, svc domain.LocalService) (domain.Share, error) {
	share, err := m.provider.Publish(ctx, svc, domain.ModeTailnet, nil)
	m.record("tailnet.start", svc, share, err, "")
	return share, err
}

func (m *Manager) PublishPublic(ctx context.Context, svc domain.LocalService, ttl time.Duration, longRunning bool) (domain.Share, error) {
	var expiresAt *time.Time
	if !longRunning {
		at := time.Now().Add(ttl)
		expiresAt = &at
	}
	share, err := m.provider.Publish(ctx, svc, domain.ModePublic, &provider.PublishOptions{ExpiresAt: expiresAt, LongRunning: longRunning})
	m.record("public.start", svc, share, err, "")
	if err == nil && expiresAt != nil {
		m.scheduleExpiry(share.ID, time.Until(*expiresAt))
	}
	return share, err
}

func (m *Manager) Stop(ctx context.Context, share domain.Share, reason string) error {
	err := m.provider.Stop(ctx, share.ID)
	m.record("share.stop", domain.LocalService{}, share, err, reason)
	m.cancelTimer(share.ID)
	return err
}

func (m *Manager) StopAllPublic(ctx context.Context) error {
	err := m.provider.StopAll(ctx, domain.ModePublic)
	_ = m.audit.Append(audit.Event{At: time.Now(), Action: "public.stop.all", Mode: string(domain.ModePublic), Reason: "manual"})
	m.clearTimers()
	return err
}

func (m *Manager) Status(ctx context.Context) ([]domain.Share, error) {
	return m.provider.Status(ctx)
}

func (m *Manager) scheduleExpiry(id string, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old := m.timers[id]; old != nil {
		old.Stop()
	}
	m.timers[id] = time.AfterFunc(delay, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := m.provider.Stop(ctx, id)
		_ = m.audit.Append(audit.Event{At: time.Now(), Action: "public.expired", Mode: string(domain.ModePublic), Reason: "expired", Error: errorString(err)})
		m.cancelTimer(id)
	})
}

func (m *Manager) cancelTimer(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if timer := m.timers[id]; timer != nil {
		timer.Stop()
		delete(m.timers, id)
	}
}

func (m *Manager) clearTimers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, timer := range m.timers {
		timer.Stop()
		delete(m.timers, id)
	}
}

func (m *Manager) record(action string, svc domain.LocalService, share domain.Share, err error, reason string) {
	event := audit.Event{
		At:       time.Now(),
		Action:   action,
		Service:  fmt.Sprintf("%s://%s:%d", svc.Scheme, svc.Host, svc.Port),
		Mode:     string(share.Mode),
		URL:      share.PublicURL,
		Provider: share.Provider,
		Reason:   reason,
		Error:    errorString(err),
	}
	_ = m.audit.Append(event)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
