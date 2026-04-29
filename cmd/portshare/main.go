package main

import (
	"log"
	"path/filepath"

	"github.com/absuq/portshare-desktop/internal/audit"
	"github.com/absuq/portshare-desktop/internal/config"
	"github.com/absuq/portshare-desktop/internal/manager"
	"github.com/absuq/portshare-desktop/internal/provider/tailscale"
	"github.com/absuq/portshare-desktop/internal/ui"
)

func main() {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	store := config.NewStore(cfgPath)
	cfg, err := store.Load()
	if err != nil {
		log.Fatal(err)
	}
	logPath := filepath.Join(filepath.Dir(cfgPath), "audit.jsonl")
	auditLog := audit.NewLog(logPath)
	_ = auditLog.Cleanup(cfg.AuditRetention)
	mgr := manager.New(tailscale.New(nil), auditLog)
	ui.New(ui.Dependencies{Manager: mgr}).Run()
}
