package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	s := New(path)
	peer := TrustedPeer{
		ID:            "peer-1",
		DisplayName:   "desktop-b",
		TailscaleIP:   "100.109.251.97",
		FirstPairedAt: time.Now().UTC(),
		LastSeenAt:    time.Now().UTC(),
		LastRoute:     "https://desktop-b.tailnet.example",
		SecretLabel:   "sha256:abc",
	}
	if err := s.SavePeers([]TrustedPeer{peer}); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("unexpected peers: %+v", got)
	}
	if got[0].ID != peer.ID {
		t.Fatalf("ID = %q, want %q", got[0].ID, peer.ID)
	}
	if got[0].DisplayName != peer.DisplayName {
		t.Fatalf("DisplayName = %q, want %q", got[0].DisplayName, peer.DisplayName)
	}
	if got[0].TailscaleIP != peer.TailscaleIP {
		t.Fatalf("TailscaleIP = %q, want %q", got[0].TailscaleIP, peer.TailscaleIP)
	}
	if !got[0].FirstPairedAt.Equal(peer.FirstPairedAt) {
		t.Fatalf("FirstPairedAt = %v, want %v", got[0].FirstPairedAt, peer.FirstPairedAt)
	}
	if !got[0].LastSeenAt.Equal(peer.LastSeenAt) {
		t.Fatalf("LastSeenAt = %v, want %v", got[0].LastSeenAt, peer.LastSeenAt)
	}
	if got[0].LastRoute != peer.LastRoute {
		t.Fatalf("LastRoute = %q, want %q", got[0].LastRoute, peer.LastRoute)
	}
	if got[0].SecretLabel != peer.SecretLabel {
		t.Fatalf("SecretLabel = %q, want %q", got[0].SecretLabel, peer.SecretLabel)
	}
	if got[0].FirstPairedAt.Location() != time.UTC || got[0].LastSeenAt.Location() != time.UTC {
		t.Fatalf("stored times are not UTC: %+v", got[0])
	}
}

func TestSavePeersReplacesExistingFileWithDefaultReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	s := New(path)
	oldPeer := TrustedPeer{
		ID:          "old-peer",
		TailscaleIP: "100.64.0.1",
		SecretLabel: DeriveSecretLabel("old-secret"),
	}
	if err := s.SavePeers([]TrustedPeer{oldPeer}); err != nil {
		t.Fatal(err)
	}

	newPeer := TrustedPeer{
		ID:          "new-peer",
		TailscaleIP: "100.64.0.2",
		SecretLabel: DeriveSecretLabel("new-secret"),
	}
	if err := s.SavePeers([]TrustedPeer{newPeer}); err != nil {
		t.Fatal(err)
	}

	got, err := s.LoadPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("unexpected peers: %+v", got)
	}
	if got[0].ID != newPeer.ID || got[0].TailscaleIP != newPeer.TailscaleIP || got[0].SecretLabel != newPeer.SecretLabel {
		t.Fatalf("LoadPeers = %+v, want only %+v", got, newPeer)
	}
}

func TestStoreDoesNotContainPlainSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	s := New(path)
	if err := s.SavePeers([]TrustedPeer{{
		ID:          "peer-1",
		TailscaleIP: "100.109.251.97",
		SecretLabel: DeriveSecretLabel("super-secret-value"),
	}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "super-secret-value") {
		t.Fatalf("stored peer file contains plain secret: %s", data)
	}
}

func TestLoadPeersMissingFileReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "peers.json")
	got, err := New(path).LoadPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("LoadPeers returned %+v, want empty", got)
	}
}

func TestLoadPeersCorruptJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path).LoadPeers(); err == nil {
		t.Fatal("LoadPeers returned nil error for corrupt JSON")
	}
}

func TestDeriveSecretLabelIsStableAndOpaque(t *testing.T) {
	const secret = "super-secret-value"
	first := DeriveSecretLabel(secret)
	second := DeriveSecretLabel(secret)
	if first != second {
		t.Fatalf("DeriveSecretLabel is not stable: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("DeriveSecretLabel = %q, want sha256: prefix", first)
	}
	if strings.Contains(first, secret) {
		t.Fatalf("DeriveSecretLabel leaked original secret: %q", first)
	}
}

func TestSavePeersReplaceFailureKeepsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	s := New(path)
	oldPeer := TrustedPeer{
		ID:          "old-peer",
		TailscaleIP: "100.64.0.1",
		SecretLabel: DeriveSecretLabel("old-secret"),
	}
	if err := s.SavePeers([]TrustedPeer{oldPeer}); err != nil {
		t.Fatal(err)
	}

	originalReplace := replaceFile
	t.Cleanup(func() {
		replaceFile = originalReplace
	})
	replaceFile = func(oldpath, newpath string) error {
		return errors.New("replace failed")
	}

	err := s.SavePeers([]TrustedPeer{{
		ID:          "new-peer",
		TailscaleIP: "100.64.0.2",
		SecretLabel: DeriveSecretLabel("new-secret"),
	}})
	if err == nil {
		t.Fatal("SavePeers returned nil error when replace failed")
	}

	got, err := s.LoadPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != oldPeer.ID || got[0].TailscaleIP != oldPeer.TailscaleIP {
		t.Fatalf("existing file was not preserved: %+v", got)
	}
}
