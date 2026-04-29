package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
	"github.com/absuq/portshare-desktop/internal/provider"
)

type Provider struct {
	name   string
	mu     sync.Mutex
	shares map[string]domain.Share
}

func New(name string) *Provider {
	return &Provider{name: name, shares: map[string]domain.Share{}}
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Capabilities(context.Context) (provider.Capabilities, error) {
	return provider.Capabilities{Tailnet: true, Public: true, Expiry: true, MultiplePorts: true, StatusQuery: true, StopOne: true}, nil
}

func (p *Provider) Health(context.Context) error {
	return nil
}

func (p *Provider) Publish(_ context.Context, svc domain.LocalService, mode domain.ShareMode, opts *provider.PublishOptions) (domain.Share, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := fmt.Sprintf("%s-%s-%d", svc.ID, mode, len(p.shares)+1)
	expiresAt := (*time.Time)(nil)
	longRunning := false
	if opts != nil {
		expiresAt = opts.ExpiresAt
		longRunning = opts.LongRunning
	}
	share := domain.Share{
		ID:          id,
		ServiceID:   svc.ID,
		Provider:    p.name,
		Mode:        mode,
		LocalURL:    fmt.Sprintf("%s://%s:%d", svc.Scheme, svc.Host, svc.Port),
		PublicURL:   fmt.Sprintf("https://%s.example/%d", mode, svc.Port),
		Status:      domain.ShareActive,
		StartedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		LongRunning: longRunning,
	}
	p.shares[id] = share
	return share, nil
}

func (p *Provider) Stop(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.shares, id)
	return nil
}

func (p *Provider) StopAll(_ context.Context, mode domain.ShareMode) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, share := range p.shares {
		if share.Mode == mode {
			delete(p.shares, id)
		}
	}
	return nil
}

func (p *Provider) Status(context.Context) ([]domain.Share, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	shares := make([]domain.Share, 0, len(p.shares))
	for _, share := range p.shares {
		shares = append(shares, share)
	}
	return shares, nil
}
