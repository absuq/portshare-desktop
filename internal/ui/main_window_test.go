package ui

import (
	"testing"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
)

func TestPeerSelectionKeepsImplicitSelectionNonDestructive(t *testing.T) {
	peers := []directmanager.TrustedPeer{{ID: "device-a"}}

	selectedPeerID, explicit := reconcileSelectedPeer(peers, "", false)

	if selectedPeerID != "device-a" {
		t.Fatalf("expected first peer to remain available for non-destructive actions, got %q", selectedPeerID)
	}
	if explicit {
		t.Fatal("expected automatic peer selection to remain non-explicit")
	}
	if canRemoveSelectedPeer(peers, selectedPeerID, explicit) {
		t.Fatal("expected delete to be disabled until the user explicitly selects a peer")
	}
}

func TestPeerSelectionClearsExplicitStateWhenSelectedPeerDisappears(t *testing.T) {
	peers := []directmanager.TrustedPeer{{ID: "device-a"}}

	selectedPeerID, explicit := reconcileSelectedPeer(peers, "device-b", true)

	if selectedPeerID != "device-a" {
		t.Fatalf("expected selection to fall back to first peer for non-destructive actions, got %q", selectedPeerID)
	}
	if explicit {
		t.Fatal("expected explicit delete selection to be cleared when selected peer disappears")
	}
	if canRemoveSelectedPeer(peers, selectedPeerID, explicit) {
		t.Fatal("expected delete to be disabled after the explicit peer disappears")
	}
}

func TestCanRemoveSelectedPeerRequiresExplicitValidSelection(t *testing.T) {
	peers := []directmanager.TrustedPeer{{ID: "device-a"}}

	if !canRemoveSelectedPeer(peers, "device-a", true) {
		t.Fatal("expected explicit valid peer selection to allow delete")
	}
	if canRemoveSelectedPeer(peers, "device-missing", true) {
		t.Fatal("expected missing peer selection to block delete")
	}
}
