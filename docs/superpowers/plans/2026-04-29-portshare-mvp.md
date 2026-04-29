# PortShare MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first usable Go + Fyne desktop app for publishing local HTTP/HTTPS services to tailnet and optionally to public internet through a provider abstraction.

**Architecture:** The app is split into domain models, provider abstraction, local service discovery, share manager, config/audit storage, and Fyne UI. Tailscale is the first provider, isolated behind a `Provider` interface so future self-hosted relay providers can be added without rewriting UI or manager code.

**Tech Stack:** Go 1.23+, Fyne v2, standard `net/http`, JSON config/audit files, Tailscale CLI adapter behind an interface, Go tests with fake providers and fake command runners.

---

## File Structure

- Create `go.mod`: module definition and dependencies.
- Create `cmd/portshare/main.go`: application entrypoint.
- Create `internal/domain/types.go`: shared domain types for services, shares, providers, modes, durations, and languages.
- Create `internal/i18n/i18n.go`: Chinese-default string catalog with English switching.
- Create `internal/config/store.go`: JSON config read/write under `os.UserConfigDir`.
- Create `internal/config/store_test.go`: config persistence tests.
- Create `internal/audit/log.go`: JSONL audit log with retention cleanup.
- Create `internal/audit/log_test.go`: audit append and retention tests.
- Create `internal/provider/provider.go`: provider interface and capability model.
- Create `internal/provider/fake/fake.go`: deterministic fake provider for manager and UI tests.
- Create `internal/provider/tailscale/runner.go`: command runner abstraction.
- Create `internal/provider/tailscale/provider.go`: Tailscale provider implementation.
- Create `internal/provider/tailscale/provider_test.go`: command construction and parsing tests.
- Create `internal/discovery/discovery.go`: local HTTP/HTTPS service discovery and probing.
- Create `internal/discovery/discovery_test.go`: probe tests using `httptest`.
- Create `internal/manager/manager.go`: publish/stop/status orchestration, timers, and audit events.
- Create `internal/manager/manager_test.go`: tailnet/public/timer behavior tests.
- Create `internal/ui/app.go`: Fyne app bootstrap and dependency wiring.
- Create `internal/ui/main_window.go`: service list and action panel.
- Create `internal/ui/dialogs.go`: public exposure confirmation and language/settings dialogs.
- Create `internal/ui/tray.go`: tray menu actions and window-close behavior.
- Create `docs/manual-verification.md`: manual validation steps for two machines and real Tailscale.

---

### Task 1: Scaffold Go Module and Entrypoint

**Files:**
- Create: `go.mod`
- Create: `cmd/portshare/main.go`
- Create: `internal/domain/types.go`

- [ ] **Step 1: Write the initial domain compile test**

Create `internal/domain/types_test.go`:

```go
package domain

import "testing"

func TestShareModeStrings(t *testing.T) {
	if ModeTailnet.String() != "tailnet" {
		t.Fatalf("expected tailnet, got %q", ModeTailnet.String())
	}
	if ModePublic.String() != "public" {
		t.Fatalf("expected public, got %q", ModePublic.String())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/domain`

Expected: failure because `go.mod` and domain types do not exist.

- [ ] **Step 3: Create the module and domain types**

Create `go.mod`:

```go
module github.com/absuq/portshare-desktop

go 1.23

require fyne.io/fyne/v2 v2.5.3
```

Create `internal/domain/types.go`:

```go
package domain

import "time"

type ShareMode string

const (
	ModeTailnet ShareMode = "tailnet"
	ModePublic  ShareMode = "public"
)

func (m ShareMode) String() string {
	return string(m)
}

type Language string

const (
	LanguageChinese Language = "zh-CN"
	LanguageEnglish Language = "en-US"
)

type LocalService struct {
	ID          string
	Name        string
	Scheme      string
	Host        string
	Port        int
	Title       string
	Discovered  bool
	LastChecked time.Time
}

type ShareStatus string

const (
	ShareStopped ShareStatus = "stopped"
	ShareStarting ShareStatus = "starting"
	ShareActive   ShareStatus = "active"
	ShareError    ShareStatus = "error"
)

type Share struct {
	ID          string
	ServiceID   string
	Provider    string
	Mode        ShareMode
	LocalURL    string
	PublicURL   string
	Status      ShareStatus
	StartedAt   time.Time
	ExpiresAt   *time.Time
	LastError   string
	LongRunning bool
}
```

Create `cmd/portshare/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("PortShare desktop app bootstrap")
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./...`

Expected: `ok github.com/absuq/portshare-desktop/internal/domain`.

- [ ] **Step 5: Commit**

```bash
git add go.mod cmd internal/domain
git commit -m "chore: scaffold go module"
```

---

### Task 2: Add Chinese-Default Internationalization

**Files:**
- Create: `internal/i18n/i18n.go`
- Create: `internal/i18n/i18n_test.go`

- [ ] **Step 1: Write language catalog tests**

Create `internal/i18n/i18n_test.go`:

```go
package i18n

import (
	"testing"

	"github.com/absuq/portshare-desktop/internal/domain"
)

func TestDefaultChineseStrings(t *testing.T) {
	c := NewCatalog(domain.LanguageChinese)
	if got := c.T(KeyAppTitle); got != "端口发布器" {
		t.Fatalf("expected Chinese app title, got %q", got)
	}
}

func TestEnglishSwitch(t *testing.T) {
	c := NewCatalog(domain.LanguageEnglish)
	if got := c.T(KeyPublicShare); got != "Public share" {
		t.Fatalf("expected English text, got %q", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/i18n`

Expected: failure because package does not exist.

- [ ] **Step 3: Implement the catalog**

Create `internal/i18n/i18n.go`:

```go
package i18n

import "github.com/absuq/portshare-desktop/internal/domain"

type Key string

const (
	KeyAppTitle       Key = "app.title"
	KeyServices       Key = "services"
	KeyHistory        Key = "history"
	KeySettings       Key = "settings"
	KeyAddService     Key = "add.service"
	KeyRefresh        Key = "refresh"
	KeyTailnetShare   Key = "tailnet.share"
	KeyPublicShare    Key = "public.share"
	KeyStopShare      Key = "stop.share"
	KeyStopAll        Key = "stop.all"
	KeyPausePublic    Key = "pause.public"
	KeyPublicWarning  Key = "public.warning"
	KeyLongRunWarning Key = "longrun.warning"
)

type Catalog struct {
	lang domain.Language
}

func NewCatalog(lang domain.Language) Catalog {
	if lang == "" {
		lang = domain.LanguageChinese
	}
	return Catalog{lang: lang}
}

func (c Catalog) T(key Key) string {
	if c.lang == domain.LanguageEnglish {
		if v, ok := en[key]; ok {
			return v
		}
	}
	if v, ok := zh[key]; ok {
		return v
	}
	return string(key)
}

var zh = map[Key]string{
	KeyAppTitle:       "端口发布器",
	KeyServices:       "服务",
	KeyHistory:        "历史",
	KeySettings:       "设置",
	KeyAddService:     "添加服务",
	KeyRefresh:        "刷新发现",
	KeyTailnetShare:   "开放到 tailnet",
	KeyPublicShare:    "开启公网",
	KeyStopShare:      "关闭发布",
	KeyStopAll:        "停止全部发布",
	KeyPausePublic:    "暂停所有公网",
	KeyPublicWarning:  "公网开放会让非 tailnet 设备访问该服务，请确认服务本身已有保护。",
	KeyLongRunWarning: "长期开放不会自动关闭，请确认你愿意持续暴露该公网入口。",
}

var en = map[Key]string{
	KeyAppTitle:       "PortShare",
	KeyServices:       "Services",
	KeyHistory:        "History",
	KeySettings:       "Settings",
	KeyAddService:     "Add service",
	KeyRefresh:        "Refresh discovery",
	KeyTailnetShare:   "Share to tailnet",
	KeyPublicShare:    "Public share",
	KeyStopShare:      "Stop share",
	KeyStopAll:        "Stop all shares",
	KeyPausePublic:    "Pause public shares",
	KeyPublicWarning:  "Public sharing allows non-tailnet devices to access this service. Confirm the service is protected.",
	KeyLongRunWarning: "Long-term public sharing does not close automatically. Confirm you want to keep this public entry open.",
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/i18n`

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add internal/i18n
git commit -m "feat: add language catalog"
```

---

### Task 3: Implement Config and Audit Storage

**Files:**
- Create: `internal/config/store.go`
- Create: `internal/config/store_test.go`
- Create: `internal/audit/log.go`
- Create: `internal/audit/log_test.go`

- [ ] **Step 1: Write config and audit tests**

Create `internal/config/store_test.go`:

```go
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
```

Create `internal/audit/log_test.go`:

```go
package audit

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	log := NewLog(path)
	old := Event{At: time.Now().Add(-400 * 24 * time.Hour), Action: "public.start", Service: "127.0.0.1:3000"}
	recent := Event{At: time.Now(), Action: "public.stop", Service: "127.0.0.1:3000"}
	if err := log.Append(old); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(recent); err != nil {
		t.Fatal(err)
	}
	if err := log.Cleanup(365 * 24 * time.Hour); err != nil {
		t.Fatal(err)
	}
	events, err := log.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "public.stop" {
		t.Fatalf("unexpected events: %+v", events)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/config ./internal/audit`

Expected: failure because packages do not exist.

- [ ] **Step 3: Implement config store**

Create `internal/config/store.go`:

```go
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
	Language         domain.Language `json:"language"`
	AuditRetention   time.Duration   `json:"audit_retention"`
	MinimizeToTray   bool            `json:"minimize_to_tray"`
	ConfirmOnExit    bool            `json:"confirm_on_exit"`
	DefaultPublicTTL time.Duration   `json:"default_public_ttl"`
}

func Default() Config {
	return Config{
		Language:         domain.LanguageChinese,
		AuditRetention:   365 * 24 * time.Hour,
		MinimizeToTray:   true,
		ConfirmOnExit:    true,
		DefaultPublicTTL: 30 * time.Minute,
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
```

- [ ] **Step 4: Implement audit log**

Create `internal/audit/log.go`:

```go
package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Event struct {
	At       time.Time `json:"at"`
	Action   string    `json:"action"`
	Service  string    `json:"service"`
	Mode     string    `json:"mode,omitempty"`
	URL      string    `json:"url,omitempty"`
	Provider string    `json:"provider,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	Error    string    `json:"error,omitempty"`
}

type Log struct {
	path string
}

func NewLog(path string) Log {
	return Log{path: path}
}

func (l Log) Append(event Event) error {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(event)
}

func (l Log) ReadAll() ([]Event, error) {
	f, err := os.Open(l.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

func (l Log) Cleanup(retention time.Duration) error {
	events, err := l.ReadAll()
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-retention)
	var kept []Event
	for _, event := range events {
		if event.At.After(cutoff) || event.At.Equal(cutoff) {
			kept = append(kept, event)
		}
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, event := range kept {
		if err := enc.Encode(event); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/config ./internal/audit`

Expected: pass.

```bash
git add internal/config internal/audit
git commit -m "feat: add config and audit storage"
```

---

### Task 4: Define Provider Interface and Fake Provider

**Files:**
- Create: `internal/provider/provider.go`
- Create: `internal/provider/fake/fake.go`
- Create: `internal/provider/fake/fake_test.go`

- [ ] **Step 1: Write fake provider test**

Create `internal/provider/fake/fake_test.go`:

```go
package fake

import (
	"context"
	"testing"

	"github.com/absuq/portshare-desktop/internal/domain"
)

func TestPublishAndStop(t *testing.T) {
	p := New("fake")
	share, err := p.Publish(context.Background(), domain.LocalService{ID: "svc", Scheme: "http", Host: "127.0.0.1", Port: 3000}, domain.ModeTailnet, nil)
	if err != nil {
		t.Fatal(err)
	}
	if share.Status != domain.ShareActive || share.PublicURL == "" {
		t.Fatalf("unexpected share: %+v", share)
	}
	if err := p.Stop(context.Background(), share.ID); err != nil {
		t.Fatal(err)
	}
	shares, err := p.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 0 {
		t.Fatalf("expected no shares, got %+v", shares)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/provider/...`

Expected: failure because provider packages do not exist.

- [ ] **Step 3: Implement provider interface**

Create `internal/provider/provider.go`:

```go
package provider

import (
	"context"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
)

type Capabilities struct {
	Tailnet       bool
	Public        bool
	Expiry        bool
	MultiplePorts bool
	CustomDomain  bool
	StatusQuery   bool
	StopOne       bool
}

type PublishOptions struct {
	ExpiresAt   *time.Time
	LongRunning bool
}

type Provider interface {
	Name() string
	Capabilities(context.Context) (Capabilities, error)
	Health(context.Context) error
	Publish(context.Context, domain.LocalService, domain.ShareMode, *PublishOptions) (domain.Share, error)
	Stop(context.Context, string) error
	StopAll(context.Context, domain.ShareMode) error
	Status(context.Context) ([]domain.Share, error)
}
```

- [ ] **Step 4: Implement fake provider**

Create `internal/provider/fake/fake.go`:

```go
package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
	"github.com/absuq/portshare-desktop/internal/provider"
)

type Provider struct {
	name   string
	mu     sync.Mutex
	shares map[string]domain.Share
}

func New(name string) *Provider {
	return &Provider{name: name, shares: map[string]domain.Share{}}
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Capabilities(context.Context) (provider.Capabilities, error) {
	return provider.Capabilities{Tailnet: true, Public: true, Expiry: true, MultiplePorts: true, StatusQuery: true, StopOne: true}, nil
}

func (p *Provider) Health(context.Context) error {
	return nil
}

func (p *Provider) Publish(_ context.Context, svc domain.LocalService, mode domain.ShareMode, opts *provider.PublishOptions) (domain.Share, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := fmt.Sprintf("%s-%s-%d", svc.ID, mode, len(p.shares)+1)
	expiresAt := (*time.Time)(nil)
	longRunning := false
	if opts != nil {
		expiresAt = opts.ExpiresAt
		longRunning = opts.LongRunning
	}
	share := domain.Share{
		ID:          id,
		ServiceID:   svc.ID,
		Provider:    p.name,
		Mode:        mode,
		LocalURL:    fmt.Sprintf("%s://%s:%d", svc.Scheme, svc.Host, svc.Port),
		PublicURL:   fmt.Sprintf("https://%s.example/%d", mode, svc.Port),
		Status:      domain.ShareActive,
		StartedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		LongRunning: longRunning,
	}
	p.shares[id] = share
	return share, nil
}

func (p *Provider) Stop(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.shares, id)
	return nil
}

func (p *Provider) StopAll(_ context.Context, mode domain.ShareMode) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, share := range p.shares {
		if share.Mode == mode {
			delete(p.shares, id)
		}
	}
	return nil
}

func (p *Provider) Status(context.Context) ([]domain.Share, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	shares := make([]domain.Share, 0, len(p.shares))
	for _, share := range p.shares {
		shares = append(shares, share)
	}
	return shares, nil
}
```

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/provider/...`

Expected: pass.

```bash
git add internal/provider
git commit -m "feat: define provider abstraction"
```

---

### Task 5: Implement Share Manager With Public Expiry

**Files:**
- Create: `internal/manager/manager.go`
- Create: `internal/manager/manager_test.go`

- [ ] **Step 1: Write manager behavior tests**

Create `internal/manager/manager_test.go`:

```go
package manager

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/absuq/portshare-desktop/internal/audit"
	"github.com/absuq/portshare-desktop/internal/domain"
	"github.com/absuq/portshare-desktop/internal/provider/fake"
)

func TestPublicShareExpires(t *testing.T) {
	ctx := context.Background()
	p := fake.New("fake")
	log := audit.NewLog(filepath.Join(t.TempDir(), "audit.jsonl"))
	m := New(p, log)
	svc := domain.LocalService{ID: "svc", Scheme: "http", Host: "127.0.0.1", Port: 3000}
	share, err := m.PublishPublic(ctx, svc, 20*time.Millisecond, false)
	if err != nil {
		t.Fatal(err)
	}
	if share.ExpiresAt == nil {
		t.Fatal("expected expiry")
	}
	time.Sleep(80 * time.Millisecond)
	shares, err := p.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 0 {
		t.Fatalf("expected expired share to stop, got %+v", shares)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/manager`

Expected: failure because manager package does not exist.

- [ ] **Step 3: Implement manager**

Create `internal/manager/manager.go`:

```go
package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/absuq/portshare-desktop/internal/audit"
	"github.com/absuq/portshare-desktop/internal/domain"
	"github.com/absuq/portshare-desktop/internal/provider"
)

type Manager struct {
	provider provider.Provider
	audit    audit.Log
	mu       sync.Mutex
	timers   map[string]*time.Timer
}

func New(p provider.Provider, log audit.Log) *Manager {
	return &Manager{provider: p, audit: log, timers: map[string]*time.Timer{}}
}

func (m *Manager) PublishTailnet(ctx context.Context, svc domain.LocalService) (domain.Share, error) {
	share, err := m.provider.Publish(ctx, svc, domain.ModeTailnet, nil)
	m.record("tailnet.start", svc, share, err, "")
	return share, err
}

func (m *Manager) PublishPublic(ctx context.Context, svc domain.LocalService, ttl time.Duration, longRunning bool) (domain.Share, error) {
	var expiresAt *time.Time
	if !longRunning {
		at := time.Now().Add(ttl)
		expiresAt = &at
	}
	share, err := m.provider.Publish(ctx, svc, domain.ModePublic, &provider.PublishOptions{ExpiresAt: expiresAt, LongRunning: longRunning})
	m.record("public.start", svc, share, err, "")
	if err == nil && expiresAt != nil {
		m.scheduleExpiry(share.ID, time.Until(*expiresAt))
	}
	return share, err
}

func (m *Manager) Stop(ctx context.Context, share domain.Share, reason string) error {
	err := m.provider.Stop(ctx, share.ID)
	m.record("share.stop", domain.LocalService{}, share, err, reason)
	m.cancelTimer(share.ID)
	return err
}

func (m *Manager) StopAllPublic(ctx context.Context) error {
	err := m.provider.StopAll(ctx, domain.ModePublic)
	_ = m.audit.Append(audit.Event{At: time.Now(), Action: "public.stop.all", Mode: string(domain.ModePublic), Reason: "manual"})
	m.clearTimers()
	return err
}

func (m *Manager) Status(ctx context.Context) ([]domain.Share, error) {
	return m.provider.Status(ctx)
}

func (m *Manager) scheduleExpiry(id string, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old := m.timers[id]; old != nil {
		old.Stop()
	}
	m.timers[id] = time.AfterFunc(delay, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := m.provider.Stop(ctx, id)
		_ = m.audit.Append(audit.Event{At: time.Now(), Action: "public.expired", Mode: string(domain.ModePublic), Reason: "expired", Error: errorString(err)})
		m.cancelTimer(id)
	})
}

func (m *Manager) cancelTimer(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if timer := m.timers[id]; timer != nil {
		timer.Stop()
		delete(m.timers, id)
	}
}

func (m *Manager) clearTimers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, timer := range m.timers {
		timer.Stop()
		delete(m.timers, id)
	}
}

func (m *Manager) record(action string, svc domain.LocalService, share domain.Share, err error, reason string) {
	event := audit.Event{
		At:       time.Now(),
		Action:   action,
		Service:  fmt.Sprintf("%s://%s:%d", svc.Scheme, svc.Host, svc.Port),
		Mode:     string(share.Mode),
		URL:      share.PublicURL,
		Provider: share.Provider,
		Reason:   reason,
		Error:    errorString(err),
	}
	_ = m.audit.Append(event)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
```

- [ ] **Step 4: Run tests and commit**

Run: `go test ./internal/manager`

Expected: pass.

```bash
git add internal/manager
git commit -m "feat: add share manager"
```

---

### Task 6: Add Local HTTP/HTTPS Discovery

**Files:**
- Create: `internal/discovery/discovery.go`
- Create: `internal/discovery/discovery_test.go`

- [ ] **Step 1: Write HTTP probe test**

Create `internal/discovery/discovery_test.go`:

```go
package discovery

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeReadsTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><head><title>Vite App</title></head><body></body></html>"))
	}))
	defer server.Close()
	svc, err := Probe(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Title != "Vite App" {
		t.Fatalf("expected title, got %q", svc.Title)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/discovery`

Expected: failure because package does not exist.

- [ ] **Step 3: Implement probing**

Create `internal/discovery/discovery.go`:

```go
package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
)

var titlePattern = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func Probe(rawURL string, timeout time.Duration) (domain.LocalService, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return domain.LocalService{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return domain.LocalService{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return domain.LocalService{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	host := parsed.Hostname()
	port, _ := strconv.Atoi(parsed.Port())
	title := extractTitle(string(body))
	if title == "" {
		title = fmt.Sprintf("本地服务 %d", port)
	}
	return domain.LocalService{
		ID:          fmt.Sprintf("%s-%s-%d", parsed.Scheme, host, port),
		Name:        title,
		Scheme:      parsed.Scheme,
		Host:        host,
		Port:        port,
		Title:       title,
		Discovered:  true,
		LastChecked: time.Now(),
	}, nil
}

func extractTitle(html string) string {
	matches := titlePattern.FindStringSubmatch(html)
	if len(matches) < 2 {
		return ""
	}
	title := strings.TrimSpace(matches[1])
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\t", " ")
	return title
}
```

- [ ] **Step 4: Add listener scanning adapter**

Extend `internal/discovery/discovery.go` with a conservative first scan that probes common development ports. This avoids platform-specific listener parsing in the first pass while still meeting common workflow needs.

```go
func ScanCommon(timeout time.Duration) []domain.LocalService {
	ports := []int{3000, 5173, 8080, 8000, 5000, 4200, 8443}
	var services []domain.LocalService
	for _, port := range ports {
		for _, scheme := range []string{"http", "https"} {
			svc, err := Probe(fmt.Sprintf("%s://127.0.0.1:%d", scheme, port), timeout)
			if err == nil {
				services = append(services, svc)
				break
			}
		}
	}
	return services
}
```

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/discovery`

Expected: pass.

```bash
git add internal/discovery
git commit -m "feat: discover local web services"
```

---

### Task 7: Implement Tailscale Provider Adapter

**Files:**
- Create: `internal/provider/tailscale/runner.go`
- Create: `internal/provider/tailscale/provider.go`
- Create: `internal/provider/tailscale/provider_test.go`

- [ ] **Step 1: Write command construction tests**

Create `internal/provider/tailscale/provider_test.go`:

```go
package tailscale

import (
	"context"
	"strings"
	"testing"

	"github.com/absuq/portshare-desktop/internal/domain"
)

type recordingRunner struct {
	commands []string
	output   string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	return []byte(r.output), nil
}

func TestPublishTailnetUsesServe(t *testing.T) {
	r := &recordingRunner{output: `{"TCP":{}}`}
	p := New(r)
	_, err := p.Publish(context.Background(), domain.LocalService{ID: "svc", Scheme: "http", Host: "127.0.0.1", Port: 3000}, domain.ModeTailnet, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.commands[0], "tailscale serve --bg --http=3000 http://127.0.0.1:3000") {
		t.Fatalf("unexpected command: %+v", r.commands)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/provider/tailscale`

Expected: failure because package does not exist.

- [ ] **Step 3: Implement command runner**

Create `internal/provider/tailscale/runner.go`:

```go
package tailscale

import (
	"context"
	"os/exec"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}
```

- [ ] **Step 4: Implement provider commands**

Create `internal/provider/tailscale/provider.go`:

```go
package tailscale

import (
	"context"
	"fmt"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
	"github.com/absuq/portshare-desktop/internal/provider"
)

type Provider struct {
	runner Runner
}

func New(runner Runner) *Provider {
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Provider{runner: runner}
}

func (p *Provider) Name() string {
	return "Tailscale"
}

func (p *Provider) Capabilities(context.Context) (provider.Capabilities, error) {
	return provider.Capabilities{Tailnet: true, Public: true, Expiry: false, MultiplePorts: true, StatusQuery: true, StopOne: true}, nil
}

func (p *Provider) Health(ctx context.Context) error {
	_, err := p.runner.Run(ctx, "tailscale", "status", "--json")
	return err
}

func (p *Provider) Publish(ctx context.Context, svc domain.LocalService, mode domain.ShareMode, opts *provider.PublishOptions) (domain.Share, error) {
	target := fmt.Sprintf("%s://%s:%d", svc.Scheme, svc.Host, svc.Port)
	var args []string
	if mode == domain.ModePublic {
		args = []string{"funnel", "--bg", "--https=443", target}
	} else {
		args = []string{"serve", "--bg", "--http=" + fmt.Sprint(svc.Port), target}
	}
	if _, err := p.runner.Run(ctx, "tailscale", args...); err != nil {
		return domain.Share{Status: domain.ShareError, LastError: err.Error()}, err
	}
	expiresAt := (*time.Time)(nil)
	longRunning := false
	if opts != nil {
		expiresAt = opts.ExpiresAt
		longRunning = opts.LongRunning
	}
	return domain.Share{
		ID:          fmt.Sprintf("tailscale-%s-%d", mode, svc.Port),
		ServiceID:   svc.ID,
		Provider:    p.Name(),
		Mode:        mode,
		LocalURL:    target,
		PublicURL:   fmt.Sprintf("%s://%s:%d", svc.Scheme, "localhost", svc.Port),
		Status:      domain.ShareActive,
		StartedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		LongRunning: longRunning,
	}, nil
}

func (p *Provider) Stop(ctx context.Context, id string) error {
	_, err := p.runner.Run(ctx, "tailscale", "serve", "reset")
	return err
}

func (p *Provider) StopAll(ctx context.Context, mode domain.ShareMode) error {
	if mode == domain.ModePublic {
		_, err := p.runner.Run(ctx, "tailscale", "funnel", "reset")
		return err
	}
	_, err := p.runner.Run(ctx, "tailscale", "serve", "reset")
	return err
}

func (p *Provider) Status(ctx context.Context) ([]domain.Share, error) {
	_, err := p.runner.Run(ctx, "tailscale", "serve", "status", "--json")
	if err != nil {
		return nil, err
	}
	return nil, nil
}
```

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/provider/tailscale`

Expected: pass.

```bash
git add internal/provider/tailscale
git commit -m "feat: add tailscale provider adapter"
```

---

### Task 8: Build Fyne App Shell and Main Window

**Files:**
- Create: `internal/ui/app.go`
- Create: `internal/ui/main_window.go`
- Modify: `cmd/portshare/main.go`

- [ ] **Step 1: Add UI bootstrap**

Create `internal/ui/app.go`:

```go
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

type App struct {
	fyneApp fyne.App
	window  fyne.Window
}

func New() *App {
	a := app.NewWithID("com.absuq.portshare")
	return &App{fyneApp: a}
}

func (a *App) Run() {
	a.window = a.buildMainWindow()
	a.window.ShowAndRun()
}
```

- [ ] **Step 2: Add main window**

Create `internal/ui/main_window.go`:

```go
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (a *App) buildMainWindow() fyne.Window {
	w := a.fyneApp.NewWindow("端口发布器")
	w.Resize(fyne.NewSize(980, 640))
	services := widget.NewList(
		func() int { return 0 },
		func() fyne.CanvasObject { return widget.NewLabel("本地服务") },
		func(widget.ListItemID, fyne.CanvasObject) {},
	)
	actions := container.NewVBox(
		widget.NewEntry(),
		widget.NewButton("添加服务", func() {}),
		widget.NewButton("刷新发现", func() {}),
		widget.NewButton("开放到 tailnet", func() {}),
		widget.NewButton("开启公网", func() {}),
	)
	w.SetContent(container.NewBorder(nil, nil, nil, actions, services))
	return w
}
```

- [ ] **Step 3: Wire entrypoint**

Modify `cmd/portshare/main.go`:

```go
package main

import "github.com/absuq/portshare-desktop/internal/ui"

func main() {
	ui.New().Run()
}
```

- [ ] **Step 4: Run compile check**

Run: `go test ./...`

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add cmd internal/ui go.mod go.sum
git commit -m "feat: add fyne app shell"
```

---

### Task 9: Add Public Confirmation Dialogs and Tray Controls

**Files:**
- Create: `internal/ui/dialogs.go`
- Create: `internal/ui/tray.go`
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/main_window.go`

- [ ] **Step 1: Add public confirmation dialog**

Create `internal/ui/dialogs.go`:

```go
package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type PublicChoice struct {
	TTL         time.Duration
	LongRunning bool
}

func ShowPublicConfirm(parent fyne.Window, onConfirm func(PublicChoice)) {
	duration := widget.NewSelect([]string{"10 分钟", "30 分钟", "1 小时", "长期开放"}, nil)
	duration.SetSelected("30 分钟")
	dialog.ShowForm("确认开启公网", "确认", "取消", []*widget.FormItem{
		widget.NewFormItem("风险提示", widget.NewLabel("公网开放会让非 tailnet 设备访问该服务，请确认服务本身已有保护。")),
		widget.NewFormItem("开放时长", duration),
	}, func(ok bool) {
		if !ok {
			return
		}
		choice := PublicChoice{TTL: 30 * time.Minute}
		switch duration.Selected {
		case "10 分钟":
			choice.TTL = 10 * time.Minute
		case "1 小时":
			choice.TTL = time.Hour
		case "长期开放":
			choice.LongRunning = true
			choice.TTL = 0
		}
		onConfirm(choice)
	}, parent)
}
```

- [ ] **Step 2: Add tray menu**

Create `internal/ui/tray.go`:

```go
package ui

import "fyne.io/fyne/v2"

func (a *App) configureTray() {
	desktop, ok := a.fyneApp.(interface {
		SetSystemTrayMenu(*fyne.Menu)
	})
	if !ok {
		return
	}
	menu := fyne.NewMenu("端口发布器",
		fyne.NewMenuItem("打开主界面", func() {
			if a.window != nil {
				a.window.Show()
			}
		}),
		fyne.NewMenuItem("暂停所有公网", func() {}),
		fyne.NewMenuItem("停止全部发布", func() {}),
		fyne.NewMenuItem("退出", func() {
			a.fyneApp.Quit()
		}),
	)
	desktop.SetSystemTrayMenu(menu)
}
```

- [ ] **Step 3: Wire tray during app startup**

Modify `internal/ui/app.go`:

```go
func (a *App) Run() {
	a.configureTray()
	a.window = a.buildMainWindow()
	a.window.SetCloseIntercept(func() {
		a.window.Hide()
	})
	a.window.ShowAndRun()
}
```

- [ ] **Step 4: Run compile check and commit**

Run: `go test ./...`

Expected: pass.

```bash
git add internal/ui
git commit -m "feat: add public confirmation and tray controls"
```

---

### Task 10: Wire Real Dependencies Into UI

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/main_window.go`
- Modify: `cmd/portshare/main.go`

- [ ] **Step 1: Extend UI app dependencies**

Modify `internal/ui/app.go`:

```go
type Dependencies struct {
	Manager interface {
		Status(context.Context) ([]domain.Share, error)
	}
}

type App struct {
	fyneApp fyne.App
	window  fyne.Window
	deps    Dependencies
}

func New(deps Dependencies) *App {
	a := app.NewWithID("com.absuq.portshare")
	return &App{fyneApp: a, deps: deps}
}
```

- [ ] **Step 2: Wire config, audit, manager, provider in entrypoint**

Modify `cmd/portshare/main.go`:

```go
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
```

- [ ] **Step 3: Run compile check and commit**

Run: `go test ./...`

Expected: pass.

```bash
git add cmd internal/ui
git commit -m "feat: wire app dependencies"
```

---

### Task 11: Add Manual Verification Guide

**Files:**
- Create: `docs/manual-verification.md`
- Modify: `README.md`

- [ ] **Step 1: Add manual verification document**

Create `docs/manual-verification.md`:

```markdown
# 手动验收

## 准备

1. 在电脑 B 启动本地网页服务：`npm run dev -- --host 127.0.0.1 --port 3000`
2. 在电脑 B 运行端口发布器。
3. 在电脑 A 登录同一个 Tailscale tailnet。

## tailnet 验收

1. 在端口发布器中刷新发现。
2. 选择 `127.0.0.1:3000`。
3. 点击“开放到 tailnet”。
4. 在电脑 A 浏览器访问界面显示的 MagicDNS 地址。
5. 关闭发布后再次访问，确认不可访问。

## 公网验收

1. 点击“开启公网”。
2. 确认看到强确认风险提示。
3. 选择 10 分钟。
4. 确认公网 URL 可访问。
5. 等到倒计时结束，确认公网 URL 不再可访问。

## 托盘验收

1. 开启公网发布。
2. 关闭窗口，确认程序仍在托盘。
3. 从托盘选择“暂停所有公网”。
4. 确认公网入口关闭。
```

- [ ] **Step 2: Link guide from README**

Add to `README.md`:

```markdown
手动验收步骤见：

- `docs/manual-verification.md`
```

- [ ] **Step 3: Commit**

```bash
git add README.md docs/manual-verification.md
git commit -m "docs: add manual verification guide"
```

---

### Task 12: Final MVP Verification

**Files:**
- Modify only files needed to fix verification failures.

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`

Expected: all packages pass.

- [ ] **Step 2: Run app locally**

Run: `go run ./cmd/portshare`

Expected: Fyne window opens with Chinese title “端口发布器”.

- [ ] **Step 3: Verify repository status**

Run: `git status --short --branch`

Expected: clean working tree.

- [ ] **Step 4: Push branch**

Run: `git push origin main`

Expected: branch updates on GitHub. If Git HTTPS is unavailable in the current network, use GitHub Contents API only for documentation commits and retry Git push from a network that allows Git HTTPS.

---

## Self-Review

- Spec coverage: This plan covers Go + Fyne, Chinese default with English switching, provider abstraction, Tailscale first provider, HTTP/HTTPS-only MVP, multiple ports, auto discovery, tray behavior, public confirmation, expiry, long-running public exposure, audit retention, and manual two-machine verification.
- Scope: Raw TCP, token authentication, self-hosted relay provider, custom domains, and additional provider backends remain future work as specified.
- Placeholder scan: The plan avoids unresolved placeholder markers and gives exact files, commands, and code snippets for each implementation task.
- Type consistency: Shared domain types are introduced before provider, manager, discovery, and UI tasks use them.
