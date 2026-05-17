package main

import (
	"context"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	"github.com/absuq/portshare-desktop/internal/firewall"
)

type firewallAccessAuthorizer interface {
	AllowTrustedPeer(context.Context, firewall.TrustedPeerAccess) error
}

type managerFirewallAuthorizer struct {
	inner firewallAccessAuthorizer
}

func (a managerFirewallAuthorizer) AllowTrustedPeer(ctx context.Context, access directmanager.TrustedPeerAccess) error {
	return a.inner.AllowTrustedPeer(ctx, firewall.TrustedPeerAccess{
		RulePrefix:       access.RulePrefix,
		LocalTailscaleIP: access.LocalTailscaleIP,
		PeerTailscaleIP:  access.PeerTailscaleIP,
		PeerID:           access.PeerID,
		PeerName:         access.PeerName,
	})
}
