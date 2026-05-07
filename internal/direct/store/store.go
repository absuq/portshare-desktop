package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type TrustedPeer struct {
	ID            string    `json:"id"`
	DisplayName   string    `json:"display_name"`
	TailscaleIP   string    `json:"tailscale_ip"`
	FirstPairedAt time.Time `json:"first_paired_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	LastRoute     string    `json:"last_route"`
	// SecretLabel is for display/matching only; it is not a secret and cannot authenticate peers.
	SecretLabel string `json:"secret_label"`
}

type Store struct {
	path string
}

var replaceFile = replaceExistingFile

func New(path string) Store {
	return Store{path: path}
}

func DeriveSecretLabel(secret string) string {
	sum := sha256.Sum256([]byte("portshare-secret-label:" + secret))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func (s Store) LoadPeers() ([]TrustedPeer, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var peers []TrustedPeer
	if err := json.Unmarshal(data, &peers); err != nil {
		return nil, err
	}
	return peers, nil
}

func (s Store) SavePeers(peers []TrustedPeer) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".peers-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	if err := replaceFile(tmpPath, s.path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
