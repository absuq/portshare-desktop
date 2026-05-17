package provider

import (
	"context"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
)

type Capabilities struct {
	Tailnet       bool
	Public        bool
	Expiry        bool
	MultiplePorts bool
	CustomDomain  bool
	StatusQuery   bool
	StopOne       bool
}

type PublishOptions struct {
	ExpiresAt   *time.Time
	LongRunning bool
}

type Provider interface {
	Name() string
	Capabilities(context.Context) (Capabilities, error)
	Health(context.Context) error
	Publish(context.Context, domain.LocalService, domain.ShareMode, *PublishOptions) (domain.Share, error)
	Stop(context.Context, string) error
	StopAll(context.Context, domain.ShareMode) error
	Status(context.Context) ([]domain.Share, error)
}
