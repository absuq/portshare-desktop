package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
)

type Config struct {
	Language          domain.Language `json:"language"`
	AuditRetention    time.Duration   `json:"audit_retention"`
	MinimizeToTray    bool            `json:"minimize_to_tray"`
	ConfirmOnExit     bool            `json:"confirm_on_exit"`
	DefaultPublicTTL  time.Duration   `json:"default_public_ttl"`
	DirectControlPort int             `json:"direct_control_port"`
	DirectPeersPath   string          `json:"direct_peers_path"`
}

func Default() Config {
	return Config{
		Language:          domain.LanguageChinese,
		AuditRetention:    365 * 24 * time.Hour,
		MinimizeToTray:    true,
		ConfirmOnExit:     true,
		DefaultPublicTTL:  30 * time.Minute,
		DirectControlPort: 17890,
	}
}

type Store struct {
	path string
}

func NewStore(path string) Store {
	return Store{path: path}
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "PortShare", "config.json"), nil
}

func (s Store) Load() (Config, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (s Store) Save(cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
