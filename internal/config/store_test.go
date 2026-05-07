package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "config.json"))
	cfg := Config{
		Language:         domain.LanguageEnglish,
		AuditRetention:   90 * 24 * time.Hour,
		MinimizeToTray:   true,
		ConfirmOnExit:    true,
		DefaultPublicTTL: 30 * time.Minute,
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Language != domain.LanguageEnglish || got.AuditRetention != 90*24*time.Hour {
		t.Fatalf("unexpected config: %+v", got)
	}
}

func TestStoreRoundTripDirectModeFields(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "config.json"))
	cfg := Default()
	cfg.DirectControlPort = 19001
	cfg.DirectPeersPath = filepath.Join(dir, "peers.json")
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.DirectControlPort != cfg.DirectControlPort || got.DirectPeersPath != cfg.DirectPeersPath {
		t.Fatalf("unexpected direct config: %+v", got)
	}
}
