package main

import (
	"context"
	"testing"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/firewall"
)

type fakeFirewallAuthorizer struct {
	access firewall.TrustedPeerAccess
}

func (f *fakeFirewallAuthorizer) AllowTrustedPeer(_ context.Context, access firewall.TrustedPeerAccess) error {
	f.access = access
	return nil
}

func TestManagerFirewallAuthorizerMapsDirectAccess(t *testing.T) {
	inner := &fakeFirewallAuthorizer{}
	adapter := managerFirewallAuthorizer{inner: inner}

	err := adapter.AllowTrustedPeer(context.Background(), directmanager.TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: "100.79.83.104",
		PeerTailscaleIP:  "100.109.251.97",
		PeerID:           "device-b",
		PeerName:         "desktop-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inner.access.LocalTailscaleIP != "100.79.83.104" ||
		inner.access.PeerTailscaleIP != "100.109.251.97" ||
		inner.access.PeerID != "device-b" ||
		inner.access.PeerName != "desktop-b" {
		t.Fatalf("unexpected mapped access: %+v", inner.access)
	}
}
