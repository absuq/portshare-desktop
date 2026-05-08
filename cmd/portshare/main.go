package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/absuq/portshare-desktop/internal/audit"
	"github.com/absuq/portshare-desktop/internal/config"
	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
	directstore "github.com/absuq/portshare-desktop/internal/direct/store"
	"github.com/absuq/portshare-desktop/internal/firewall"
	tailscalediag "github.com/absuq/portshare-desktop/internal/tailscale"
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
	deviceName, err := os.Hostname()
	if err != nil || deviceName == "" {
		deviceName = "portshare-device"
	}
	peersPath := cfg.DirectPeersPath
	if peersPath == "" {
		peersPath = filepath.Join(filepath.Dir(cfgPath), "direct-peers.json")
	}
	directMgr := directmanager.New(directmanager.Config{
		Tailscale:        tailscalediag.NewClient(nil),
		PeerStore:        directstore.New(peersPath),
		AccessAuthorizer: managerFirewallAuthorizer{inner: firewall.NewAuthorizer(nil)},
		DeviceID:         deviceName,
		DeviceName:       deviceName,
	})
	ui.New(ui.Dependencies{
		DirectManager: directMgr,
	}).Run()
}
