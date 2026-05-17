# Clash/Mihomo Egress Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Clash/Mihomo TUN discovery, dynamic controller detection, node listing, node delay display, and Tailscale direct verification after selecting a proxy egress node.

**Architecture:** Introduce `internal/clash` as the focused package for Clash/Mihomo config parsing, Windows discovery, controller transports, proxy node parsing, region inference, and egress application. Wire it into `internal/direct/manager`, then expose it in `internal/ui` beside the existing network-path diagnostics. Keep existing `internal/netdiag` responsible for Tailscale route/ping diagnostics and reuse it for post-switch verification.

**Tech Stack:** Go 1.26, Fyne, Windows PowerShell networking cmdlets, Clash/Mihomo external-controller HTTP API, Windows named pipe transport, existing `internal/winexec`, `gopkg.in/yaml.v3`.

---

### Task 1: Clash Domain Model, Config Parsing, And Region Rules

**Files:**
- Create: `internal/clash/types.go`
- Create: `internal/clash/config.go`
- Create: `internal/clash/region.go`
- Test: `internal/clash/config_test.go`
- Test: `internal/clash/region_test.go`

- [ ] **Step 1: Write failing config and region tests**

Add tests that parse a Clash Verge generated YAML containing:

```yaml
mixed-port: 7897
socks-port: 7898
port: 7899
external-controller: 127.0.0.1:9097
external-controller-pipe: \\.\pipe\verge-mihomo
secret: test-secret
tun:
  enable: true
```

Expected parsed fields:

```go
cfg.MixedPort == 7897
cfg.SocksPort == 7898
cfg.HTTPPort == 7899
cfg.ExternalController == "127.0.0.1:9097"
cfg.ExternalControllerPipe == `\\.\pipe\verge-mihomo`
cfg.Secret == "test-secret"
cfg.TUNEnabled == true
```

Add region tests:

```go
InferRegion("上海 01") == "上海"
InferRegion("杭州-移动") == "杭州"
InferRegion("HK Premium") == "香港"
InferRegion("JP Tokyo") == "日本/东京"
InferRegion("plain-node") == "未知地区"
```

Run: `go test ./internal/clash`

Expected: build fails because package does not exist.

- [ ] **Step 2: Implement types and parsers**

Create `ClashConfig`, `ProxyPort`, `ControlEndpoint`, `DiscoveryReport`, `ProxyNode`, `ProxyGroup`, `NodeDelay`, `ApplyRequest`, `ApplyResult`.

Implement:

```go
func ParseConfigYAML(raw []byte) (ClashConfig, error)
func InferRegion(name string) string
func MaskSecret(secret string) string
```

`MaskSecret("")` returns `""`; non-empty values return `"***"`.

- [ ] **Step 3: Verify parsing tests pass**

Run: `go test ./internal/clash`

Expected: pass.

### Task 2: Windows Clash/Mihomo Discovery

**Files:**
- Create: `internal/clash/discovery.go`
- Create: `internal/clash/runner_windows.go`
- Create: `internal/clash/runner_default.go`
- Test: `internal/clash/discovery_test.go`

- [ ] **Step 1: Write failing discovery tests**

Use a fake runner and fake filesystem roots. Verify that discovery:

- reads common Clash Verge config files;
- marks `mixed-port: 7897`, `socks-port: 7898`, and `port: 7899` as proxy entry ports;
- does not treat those ports as controller ports;
- prefers `external-controller-pipe` when it exists;
- reports TUN interface `Meta` from adapter JSON.

Run: `go test ./internal/clash`

Expected: fails because discovery APIs are missing.

- [ ] **Step 2: Implement discovery service**

Implement:

```go
type Runner interface {
    Run(context.Context, string, ...string) ([]byte, error)
}

type Service struct {
    runner Runner
    roots  []string
    client ControllerClient
}

func NewService(runner Runner, roots []string) *Service
func (s *Service) Discover(ctx context.Context) (DiscoveryReport, error)
```

Windows runner uses `winexec.NewCommand` so child PowerShell windows stay hidden.

PowerShell queries:

```powershell
Get-NetAdapter | Select-Object Name,InterfaceDescription,ifIndex,Status | ConvertTo-Json -Compress
Get-NetTCPConnection -State Listen | Select-Object LocalAddress,LocalPort,OwningProcess | ConvertTo-Json -Compress
```

Discovery must parse config files with priority:

1. `clash-verge.yaml`
2. `clash-verge-check.yaml`
3. `config.yaml`
4. `profiles/*.yaml`

- [ ] **Step 3: Verify discovery tests pass**

Run: `go test ./internal/clash`

Expected: pass.

### Task 3: Controller Transports And Proxy Parsing

**Files:**
- Create: `internal/clash/controller.go`
- Create: `internal/clash/namedpipe_windows.go`
- Create: `internal/clash/namedpipe_default.go`
- Test: `internal/clash/controller_test.go`

- [ ] **Step 1: Write failing controller tests**

Use `httptest.Server` to verify HTTP controller behavior:

- `Version(ctx)` calls `/version`;
- `Proxies(ctx)` parses selector group `GLOBAL` with `all`, `now`, node type, and history delay;
- `Delay(ctx, "上海 01", url, timeout)` calls `/proxies/{name}/delay`;
- `Select(ctx, "GLOBAL", "上海 01")` sends `PUT /proxies/GLOBAL` with JSON `{"name":"上海 01"}`;
- `Authorization: Bearer <secret>` is present when secret is configured.

Use a fake named-pipe round tripper to verify the same request formatting without requiring Windows pipe access.

Run: `go test ./internal/clash`

Expected: fails because controller APIs are missing.

- [ ] **Step 2: Implement controller client**

Implement:

```go
type ControllerClient interface {
    Version(context.Context) (Version, error)
    Proxies(context.Context) (ProxySnapshot, error)
    Delay(context.Context, string, string, int) (time.Duration, error)
    Select(context.Context, string, string) error
}

func NewHTTPController(baseURL string, secret string) ControllerClient
func NewPipeController(pipePath string, secret string) ControllerClient
```

Named pipe transport writes minimal HTTP/1.1 requests to `\\.\pipe\verge-mihomo` and parses `http.ReadResponse`.

- [ ] **Step 3: Verify controller tests pass**

Run: `go test ./internal/clash`

Expected: pass.

### Task 4: Node Listing, Delay Refresh, And Egress Application

**Files:**
- Modify: `internal/clash/service.go` or create it if discovery code lives elsewhere
- Test: `internal/clash/service_test.go`

- [ ] **Step 1: Write failing service tests**

With fake controller and fake Tailscale verifier, test:

- `RefreshNodes(ctx)` returns selectable nodes with group name, region, current node, and Clash delay.
- `ApplyNode(ctx, request)` records the previous group/node, selects the target node, calls Tailscale restun/ping verifier, and returns direct latency.
- If verifier reports DERP or worse latency, `ApplyNode` selects the previous node again and returns an error.
- `RestoreNode(ctx)` selects the previously recorded node.

Run: `go test ./internal/clash`

Expected: fails because service methods are missing.

- [ ] **Step 2: Implement node service behavior**

Add:

```go
func (s *Service) RefreshNodes(ctx context.Context) (DiscoveryReport, error)
func (s *Service) ApplyNode(ctx context.Context, request ApplyRequest) (ApplyResult, error)
func (s *Service) RestoreNode(ctx context.Context) error
```

Use `https://www.gstatic.com/generate_204` as the default Clash delay test URL and `5000` ms timeout.

- [ ] **Step 3: Verify service tests pass**

Run: `go test ./internal/clash`

Expected: pass.

### Task 5: Manager And UI Controller Integration

**Files:**
- Modify: `internal/direct/manager/manager.go`
- Modify: `internal/direct/manager/manager_test.go`
- Modify: `internal/ui/direct_controller.go`
- Modify: `internal/ui/direct_controller_test.go`

- [ ] **Step 1: Write failing manager/controller tests**

Add fake `ClashEgress` dependency and verify:

- manager delegates `DetectClash`, `RefreshClashNodes`, `ApplyClashNode`, and `RestoreClashNode`;
- UI controller stores `ClashReport`, `ClashApplyResult`, and message text;
- applying a node requires a peer Tailscale IP;
- state snapshots deep-copy node lists.

Run:

```powershell
go test ./internal/direct/manager ./internal/ui
```

Expected: fails because interfaces and controller methods are missing.

- [ ] **Step 2: Implement manager/controller integration**

Add `ClashEgress` to `directmanager.Config`:

```go
ClashEgress ClashEgress
```

Expose methods:

```go
DetectClash(context.Context) (clash.DiscoveryReport, error)
RefreshClashNodes(context.Context) (clash.DiscoveryReport, error)
ApplyClashNode(context.Context, clash.ApplyRequest) (clash.ApplyResult, error)
RestoreClashNode(context.Context) error
```

Add matching `DirectController` methods and `DirectState` fields.

- [ ] **Step 3: Verify manager/controller tests pass**

Run:

```powershell
go test ./internal/direct/manager ./internal/ui
```

Expected: pass.

### Task 6: Main Window UI And Production Wiring

**Files:**
- Modify: `internal/ui/main_window.go`
- Modify: `cmd/portshare/main.go`
- Test: `internal/ui/direct_controller_test.go`

- [ ] **Step 1: Write failing UI text helper tests**

Test helper output for:

- TUN status: `TUN：Meta 已启用`
- proxy entries: `代理入口：mixed 7897 / socks 7898 / http 7899`
- controller: `控制接口：named pipe \\.\pipe\verge-mihomo`
- node option: `上海 · 上海 01 · 23ms · 当前`

Run: `go test ./internal/ui`

Expected: fails until helpers are implemented.

- [ ] **Step 2: Implement UI section**

Replace the current “临时绕过代理” wording with “出口优化”. Add buttons:

- `检测代理/TUN`
- `刷新节点延迟`
- `应用出口节点`
- `恢复原节点`

Add a node select widget. Keep existing network path labels visible.

- [ ] **Step 3: Wire production dependency**

In `cmd/portshare/main.go`, pass:

```go
ClashEgress: clash.NewService(nil, nil),
```

to `directmanager.Config`.

- [ ] **Step 4: Verify UI and production build compile**

Run:

```powershell
go test ./internal/ui ./cmd/portshare
```

Expected: pass.

### Task 7: Full Verification, Build, Commit, And Push

**Files:**
- All touched files

- [ ] **Step 1: Run full test suite**

```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
```

Expected: all tests pass.

- [ ] **Step 2: Run vet**

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
```

Expected: exit 0.

- [ ] **Step 3: Build Windows artifact**

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

Expected: `.superpowers\tmp\portshare-direct.exe` exists.

- [ ] **Step 4: Verify executable properties**

```powershell
objdump -x '.\.superpowers\tmp\portshare-direct.exe' | Select-String -Pattern 'Subsystem'
strings '.\.superpowers\tmp\portshare-direct.exe' | Select-String -Pattern 'requireAdministrator'
```

Expected: `Windows GUI` and `requireAdministrator`.

- [ ] **Step 5: Commit and push**

```powershell
git status --short
git add docs/superpowers/plans/2026-05-11-clash-mihomo-egress-selection.md internal/clash internal/direct/manager internal/ui cmd/portshare
git commit -m "feat: add clash mihomo egress selection"
git push origin codex/portshare-direct-mode
```

Expected: branch pushed.
