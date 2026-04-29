package tailscale

import (
	"context"
	"fmt"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
	"github.com/absuq/portshare-desktop/internal/provider"
)

type Provider struct {
	runner Runner
}

func New(runner Runner) *Provider {
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Provider{runner: runner}
}

func (p *Provider) Name() string {
	return "Tailscale"
}

func (p *Provider) Capabilities(context.Context) (provider.Capabilities, error) {
	return provider.Capabilities{Tailnet: true, Public: true, Expiry: false, MultiplePorts: true, StatusQuery: true, StopOne: true}, nil
}

func (p *Provider) Health(ctx context.Context) error {
	_, err := p.runner.Run(ctx, "tailscale", "status", "--json")
	return err
}

func (p *Provider) Publish(ctx context.Context, svc domain.LocalService, mode domain.ShareMode, opts *provider.PublishOptions) (domain.Share, error) {
	target := fmt.Sprintf("%s://%s:%d", svc.Scheme, svc.Host, svc.Port)
	var args []string
	if mode == domain.ModePublic {
		args = []string{"funnel", "--bg", "--https=443", target}
	} else {
		args = []string{"serve", "--bg", "--http=" + fmt.Sprint(svc.Port), target}
	}
	if _, err := p.runner.Run(ctx, "tailscale", args...); err != nil {
		return domain.Share{Status: domain.ShareError, LastError: err.Error()}, err
	}
	expiresAt := (*time.Time)(nil)
	longRunning := false
	if opts != nil {
		expiresAt = opts.ExpiresAt
		longRunning = opts.LongRunning
	}
	return domain.Share{
		ID:          fmt.Sprintf("tailscale-%s-%d", mode, svc.Port),
		ServiceID:   svc.ID,
		Provider:    p.Name(),
		Mode:        mode,
		LocalURL:    target,
		PublicURL:   fmt.Sprintf("%s://%s:%d", svc.Scheme, "localhost", svc.Port),
		Status:      domain.ShareActive,
		StartedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		LongRunning: longRunning,
	}, nil
}

func (p *Provider) Stop(ctx context.Context, id string) error {
	_, err := p.runner.Run(ctx, "tailscale", "serve", "reset")
	return err
}

func (p *Provider) StopAll(ctx context.Context, mode domain.ShareMode) error {
	if mode == domain.ModePublic {
		_, err := p.runner.Run(ctx, "tailscale", "funnel", "reset")
		return err
	}
	_, err := p.runner.Run(ctx, "tailscale", "serve", "reset")
	return err
}

func (p *Provider) Status(ctx context.Context) ([]domain.Share, error) {
	_, err := p.runner.Run(ctx, "tailscale", "serve", "status", "--json")
	if err != nil {
		return nil, err
	}
	return nil, nil
}
