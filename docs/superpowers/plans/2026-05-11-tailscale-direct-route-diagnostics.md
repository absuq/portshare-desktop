# Tailscale Direct Route Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add network-path diagnostics and user-confirmed temporary `/32` route bypass so Tailscale peer direct traffic can bypass proxy/TUN interfaces without changing other application routing.

**Architecture:** Introduce `internal/netdiag` as a focused Windows diagnostics and route-control package. Wire it into `internal/direct/manager` and `internal/ui` so the direct page can diagnose the selected trusted peer, list physical egress candidates, apply a temporary endpoint route, and remove it.

**Tech Stack:** Go 1.26, Fyne, Windows PowerShell networking cmdlets, Tailscale CLI, existing hidden child-process helper `internal/winexec`.

---

### Task 1: netdiag Domain Model And Pure Parsing

**Files:**
- Create: `internal/netdiag/types.go`
- Create: `internal/netdiag/parse.go`
- Test: `internal/netdiag/parse_test.go`

- [ ] **Step 1: Write failing parsing tests**

Add tests for direct ping parsing, DERP ping parsing, endpoint extraction, public IPv4 validation, proxy interface classification, and route summary parsing.

Run: `go test ./internal/netdiag`

Expected: build fails because `netdiag` package does not exist.

- [ ] **Step 2: Implement minimal domain and parsing code**

Create exported types:

```go
type PathStatus string

const (
	PathUnknown      PathStatus = "unknown"
	PathDirectNormal PathStatus = "direct-normal"
	PathDirectProxy  PathStatus = "direct-proxy"
	PathDERP         PathStatus = "derp"
	PathFailed       PathStatus = "failed"
)

type PeerPathReport struct {
	PeerTailscaleIP string
	Status          PathStatus
	RouteType       string
	Endpoint        string
	EndpointIP      string
	Latency         string
	CurrentRoute    RouteInfo
	Candidates      []EgressCandidate
	Message         string
}

type RouteInfo struct {
	InterfaceAlias string
	InterfaceIndex int
	NextHop        string
	IPAddress      string
}

type EgressCandidate struct {
	InterfaceAlias string
	InterfaceIndex int
	InterfaceIP    string
	NextHop        string
	RouteMetric    int
	InterfaceMetric int
	SuspectedProxy bool
	Recommended    bool
}
```

Implement pure helpers:

```go
func ParsePingRoute(raw []byte) (routeType, endpoint, latency string)
func EndpointIP(endpoint string) string
func IsPublicIPv4(ip string) bool
func IsSuspectedProxyInterface(alias string) bool
func ClassifyPath(routeType string, latency string, current RouteInfo) PathStatus
```

- [ ] **Step 3: Verify parsing tests pass**

Run: `go test ./internal/netdiag`

Expected: all tests in `internal/netdiag` pass.

### Task 2: Windows Network Diagnostics Runner

**Files:**
- Create: `internal/netdiag/service.go`
- Create: `internal/netdiag/service_windows.go`
- Create: `internal/netdiag/service_default.go`
- Test: `internal/netdiag/service_test.go`

- [ ] **Step 1: Write failing service tests with fake runner**

Test that `DiagnosePeer(ctx, "100.109.251.97")`:

- runs `tailscale ping --until-direct=false --c 3 100.109.251.97`
- reads the route for the endpoint IP
- returns `PathDirectProxy` when route interface is `Meta`
- returns physical candidates sorted before suspected proxy interfaces

Run: `go test ./internal/netdiag`

Expected: fails because service APIs are missing.

- [ ] **Step 2: Implement service and command abstraction**

Create:

```go
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type Service struct {
	runner Runner
	routes RouteProvider
}

func NewService(runner Runner) *Service
func (s *Service) DiagnosePeer(ctx context.Context, peerTailscaleIP string) (PeerPathReport, error)
```

Windows command execution must use `winexec.NewCommand` through a package runner so PowerShell windows stay hidden.

- [ ] **Step 3: Implement Windows PowerShell readers**

Implement Windows-only functions that run PowerShell JSON:

```powershell
Find-NetRoute -RemoteIPAddress <endpoint-ip> |
  Select-Object InterfaceAlias,InterfaceIndex,NextHop,RouteMetric,InterfaceMetric,IPAddress |
  ConvertTo-Json -Compress

Get-NetRoute -DestinationPrefix 0.0.0.0/0 |
  Select-Object InterfaceAlias,InterfaceIndex,NextHop,RouteMetric |
  ConvertTo-Json -Compress
```

Also read interface IP and adapter data with `Get-NetIPInterface`, `Get-NetIPAddress`, and `Get-NetAdapter` where needed.

- [ ] **Step 4: Verify service tests pass**

Run: `go test ./internal/netdiag`

Expected: all tests pass.

### Task 3: Temporary Route Apply And Remove

**Files:**
- Modify: `internal/netdiag/service.go`
- Modify: `internal/netdiag/service_windows.go`
- Test: `internal/netdiag/route_test.go`

- [ ] **Step 1: Write failing route command tests**

Test `ApplyBypass(ctx, request)` builds:

```powershell
New-NetRoute -DestinationPrefix 115.233.222.82/32 -InterfaceIndex 15 -NextHop 192.168.1.1 -PolicyStore ActiveStore
```

Test `ClearBypass(ctx, bypass)` builds:

```powershell
Remove-NetRoute -DestinationPrefix 115.233.222.82/32 -InterfaceIndex 15 -NextHop 192.168.1.1 -Confirm:$false
```

Run: `go test ./internal/netdiag`

Expected: fails because route mutation APIs are missing.

- [ ] **Step 2: Implement route mutation APIs**

Add:

```go
type BypassRequest struct {
	PeerTailscaleIP string
	EndpointIP      string
	Candidate       EgressCandidate
}

type ActiveBypass struct {
	PeerTailscaleIP string
	EndpointIP      string
	InterfaceIndex  int
	NextHop         string
	CreatedAt       time.Time
}

func (s *Service) ApplyBypass(ctx context.Context, request BypassRequest) (ActiveBypass, error)
func (s *Service) ClearBypass(ctx context.Context, bypass ActiveBypass) error
```

Validation must reject empty endpoint, non-public IPv4, empty gateway, and zero interface index.

- [ ] **Step 3: Verify route tests pass**

Run: `go test ./internal/netdiag`

Expected: all route tests pass.

### Task 4: Direct Manager Integration

**Files:**
- Modify: `internal/direct/manager/manager.go`
- Modify: `internal/direct/manager/manager_test.go`

- [ ] **Step 1: Write failing manager tests**

Add a fake `NetworkDiagnostics` dependency and test:

- `NetworkPath(ctx, "100.109.251.97")` delegates to diagnostics.
- `ApplyNetworkBypass(ctx, request)` stores the active bypass.
- `ClearNetworkBypass(ctx)` clears the active bypass.

Run: `go test ./internal/direct/manager`

Expected: fails because manager methods are missing.

- [ ] **Step 2: Implement manager methods**

Add dependency to `Config`:

```go
NetworkDiagnostics NetworkDiagnostics
```

Add interface and methods:

```go
type NetworkDiagnostics interface {
	DiagnosePeer(context.Context, string) (netdiag.PeerPathReport, error)
	ApplyBypass(context.Context, netdiag.BypassRequest) (netdiag.ActiveBypass, error)
	ClearBypass(context.Context, netdiag.ActiveBypass) error
}

func (m *Manager) NetworkPath(context.Context, string) (netdiag.PeerPathReport, error)
func (m *Manager) ApplyNetworkBypass(context.Context, netdiag.BypassRequest) (netdiag.ActiveBypass, error)
func (m *Manager) ClearNetworkBypass(context.Context) error
func (m *Manager) ActiveNetworkBypass() (netdiag.ActiveBypass, bool)
```

- [ ] **Step 3: Verify manager tests pass**

Run: `go test ./internal/direct/manager`

Expected: pass.

### Task 5: UI Controller And Main Window

**Files:**
- Modify: `internal/ui/direct_controller.go`
- Modify: `internal/ui/main_window.go`
- Modify: `internal/ui/direct_controller_test.go`

- [ ] **Step 1: Write failing UI controller tests**

Extend fake manager and assert:

- `DetectNetworkPath` updates `DirectState.NetworkPath`.
- `ApplyNetworkBypass` uses selected candidate and current endpoint.
- `ClearNetworkBypass` clears state.
- Status text shows `直连但疑似代理绕路` for `PathDirectProxy`.

Run: `go test ./internal/ui`

Expected: fails because UI state and methods are missing.

- [ ] **Step 2: Implement controller state and actions**

Add to `DirectState`:

```go
NetworkPath       netdiag.PeerPathReport
SelectedCandidate int
ActiveBypass      netdiag.ActiveBypass
HasActiveBypass   bool
```

Add methods:

```go
func (c *DirectController) DetectNetworkPath(ctx context.Context, peerIP string) error
func (c *DirectController) ApplyNetworkBypass(ctx context.Context, candidateIndex int) error
func (c *DirectController) ClearNetworkBypass(ctx context.Context) error
```

- [ ] **Step 3: Add UI controls**

In `buildMainWindow`, add a “网络路径” section with labels for status, endpoint, latency, route, a select widget for candidates, and buttons:

- `检测网络路径`
- `临时绕过代理`
- `撤销绕过`

Disable route apply by returning friendly errors when peer is missing, endpoint is missing, or candidate is invalid.

- [ ] **Step 4: Verify UI tests pass**

Run: `go test ./internal/ui`

Expected: pass.

### Task 6: Wire Production Dependencies And Verify

**Files:**
- Modify: `cmd/portshare/main.go`
- Test: full repo

- [ ] **Step 1: Wire netdiag service**

In `cmd/portshare/main.go`, pass:

```go
NetworkDiagnostics: netdiag.NewService(nil),
```

to `directmanager.Config`.

- [ ] **Step 2: Run full verification**

Run:

```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

Expected: tests, vet, and build all exit 0.

- [ ] **Step 3: Manual smoke test**

Run built app, select the trusted peer, click `检测网络路径`, confirm it shows current `Meta` route when proxy/TUN is active, apply temporary bypass to the physical Ethernet candidate, and verify:

```powershell
Find-NetRoute -RemoteIPAddress 115.233.222.82
tailscale ping 100.109.251.97
```

Expected: route points to physical interface and Tailscale latency drops without changing the default route.
