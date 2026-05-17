package main

import (
	"context"
	"testing"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/firewall"
)

type fakeFirewallAuthorizer struct {
	access       firewall.TrustedPeerAccess
	revoked      firewall.TrustedPeerAccess
	revokeCalled bool
}

func (f *fakeFirewallAuthorizer) AllowTrustedPeer(_ context.Context, access firewall.TrustedPeerAccess) error {
	f.access = access
	return nil
}

func (f *fakeFirewallAuthorizer) RevokeTrustedPeer(_ context.Context, access firewall.TrustedPeerAccess) error {
	f.revoked = access
	f.revokeCalled = true
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

func TestManagerFirewallAuthorizerMapsDirectRevoke(t *testing.T) {
	inner := &fakeFirewallAuthorizer{}
	adapter := managerFirewallAuthorizer{inner: inner}

	err := adapter.RevokeTrustedPeer(context.Background(), directmanager.TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: "100.79.83.104",
		PeerTailscaleIP:  "100.109.251.97",
		PeerID:           "device-b",
		PeerName:         "desktop-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !inner.revokeCalled {
		t.Fatal("expected revoke to be called")
	}
	if inner.revoked.LocalTailscaleIP != "100.79.83.104" ||
		inner.revoked.PeerTailscaleIP != "100.109.251.97" ||
		inner.revoked.PeerID != "device-b" ||
		inner.revoked.PeerName != "desktop-b" {
		t.Fatalf("unexpected mapped revoke: %+v", inner.revoked)
	}
}
