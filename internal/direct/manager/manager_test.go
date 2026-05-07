package manager

import (
	"context"
	"testing"

	"github.com/absuq/portshare-desktop/internal/tailscale"
)

type fakeTailscale struct {
	report tailscale.ReadyReport
	route  tailscale.PeerRoute
}

func (f fakeTailscale) CheckReady(context.Context) tailscale.ReadyReport { return f.report }
func (f fakeTailscale) PingPeer(context.Context, string) (tailscale.PeerRoute, error) {
	return f.route, nil
}

func TestReadyUsesTailscaleReport(t *testing.T) {
	m := New(Config{Tailscale: fakeTailscale{report: tailscale.ReadyReport{
		Ready:  true,
		Code:   tailscale.CodeOK,
		Status: tailscale.Status{LocalIPv4: "100.79.83.104"},
	}}})
	state := m.Ready(context.Background())
	if !state.Ready || state.LocalTailscaleIP != "100.79.83.104" {
		t.Fatalf("unexpected state: %+v", state)
	}
}
