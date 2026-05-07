package manager

import (
	"context"

	"github.com/absuq/portshare-desktop/internal/tailscale"
)

type Tailscale interface {
	CheckReady(context.Context) tailscale.ReadyReport
	PingPeer(context.Context, string) (tailscale.PeerRoute, error)
}

type Config struct {
	Tailscale Tailscale
}

type Manager struct {
	tailscale Tailscale
}

type ReadyState struct {
	Ready            bool
	LocalTailscaleIP string
	Code             tailscale.DiagnosticCode
	Message          string
}

func New(config Config) *Manager {
	return &Manager{tailscale: config.Tailscale}
}

func (m *Manager) Ready(ctx context.Context) ReadyState {
	if m.tailscale == nil {
		return ReadyState{
			Code:    tailscale.CodeTailscaleUnavailable,
			Message: "Tailscale client is not configured.",
		}
	}

	report := m.tailscale.CheckReady(ctx)
	return ReadyState{
		Ready:            report.Ready,
		LocalTailscaleIP: report.Status.LocalIPv4,
		Code:             report.Code,
		Message:          report.Message,
	}
}
