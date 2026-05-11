# Tailscale Link Guardian Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Tailscale link guardian that reuses the existing home-page peer latency detector, actively nudges Tailscale direct path discovery, and applies only endpoint-scoped temporary host routes when the current direct path is slow and routed through suspected TUN/proxy interfaces.

**Architecture:** Add a focused `internal/linkguardian` policy/service package for decisions and orchestration. Extend `internal/netdiag` with small, testable Tailscale action helpers for `restun`/`rebind`, then expose the guardian through `internal/direct/manager` and `internal/ui/direct_controller.go`. Keep UI changes compact by adding guardian status/actions to the existing network tab and reusing `PeerLatencies`.

**Tech Stack:** Go, Fyne, existing `internal/netdiag`, existing direct manager/controller patterns, table-driven Go tests.

---

### Task 1: Link Guardian Policy

**Files:**
- Create: `internal/linkguardian/types.go`
- Create: `internal/linkguardian/policy.go`
- Test: `internal/linkguardian/policy_test.go`

- [ ] **Step 1: Write failing policy tests**

```go
func TestEvaluateKeepsLowLatencyDirectWithoutBypass(t *testing.T) {
	report := netdiag.PeerPathReport{Status: netdiag.PathDirectTUNOptimized, Latency: "15ms"}
	decision := Evaluate(EvaluateInput{Path: report, AutoBypass: true})
	if decision.Action != ActionWatch || decision.Status != StatusOptimized {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestEvaluateAppliesBypassForHighLatencyTUNDirect(t *testing.T) {
	report := netdiag.PeerPathReport{
		Status:     netdiag.PathDirectProxy,
		EndpointIP: "115.233.222.82",
		Latency:    "249ms",
		Candidates: []netdiag.EgressCandidate{{InterfaceIndex: 15, NextHop: "192.168.1.1", Recommended: true}},
	}
	decision := Evaluate(EvaluateInput{Path: report, AutoBypass: true})
	if decision.Action != ActionApplyBypass || decision.Candidate.InterfaceIndex != 15 {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}
```

Run: `go test ./internal/linkguardian`
Expected: FAIL because package/functions do not exist.

- [ ] **Step 2: Implement minimal policy**

```go
type Status string
const (
	StatusIdle Status = "idle"
	StatusWarming Status = "warming"
	StatusOptimized Status = "optimized"
	StatusTUNUsable Status = "tun-usable"
	StatusBypassReady Status = "bypass-ready"
	StatusBypassApplied Status = "bypass-applied"
	StatusRelay Status = "relay"
	StatusFailed Status = "failed"
)

type Action string
const (
	ActionWatch Action = "watch"
	ActionRestun Action = "restun"
	ActionApplyBypass Action = "apply-bypass"
	ActionClearBypass Action = "clear-bypass"
)

func Evaluate(input EvaluateInput) Decision {
	if input.Path.Status == netdiag.PathDirectTUNOptimized || input.Path.Status == netdiag.PathDirectNormal {
		return Decision{Status: StatusOptimized, Action: ActionWatch, Message: "当前已经是低延迟直连"}
	}
	if input.Path.Status == netdiag.PathDERP {
		return Decision{Status: StatusRelay, Action: ActionRestun, Message: "当前仍在中继，先重新探测"}
	}
	if input.Path.Status == netdiag.PathDirectProxy {
		candidate, ok := RecommendedCandidate(input.Path.Candidates)
		if input.AutoBypass && input.Path.EndpointIP != "" && ok {
			return Decision{Status: StatusBypassReady, Action: ActionApplyBypass, Candidate: candidate, Message: "当前直连疑似被 TUN 绕路，准备精确绕过"}
		}
		return Decision{Status: StatusBypassReady, Action: ActionWatch, Message: "当前直连疑似被 TUN 绕路"}
	}
	return Decision{Status: StatusFailed, Action: ActionWatch, Message: "链路状态未知或检测失败"}
}
```

- [ ] **Step 3: Run package tests**

Run: `go test ./internal/linkguardian`
Expected: PASS.

### Task 2: Tailscale Reprobe Actions

**Files:**
- Modify: `internal/netdiag/types.go`
- Modify: `internal/netdiag/service.go`
- Test: `internal/netdiag/service_test.go`

- [ ] **Step 1: Write failing tests for action support**

```go
func TestServiceReprobeRunsRestunThenRebind(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]byte{
		"tailscale debug restun": []byte("ok"),
		"tailscale debug rebind": []byte("ok"),
	}}
	service := NewService(runner)
	result := service.Reprobe(context.Background(), ReprobeRequest{Restun: true, Rebind: true})
	if !result.RestunAttempted || !result.RebindAttempted || result.RestunError != "" || result.RebindError != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
```

Run: `go test ./internal/netdiag -run TestServiceReprobeRunsRestunThenRebind`
Expected: FAIL because `Reprobe` does not exist.

- [ ] **Step 2: Implement minimal `Reprobe`**

```go
type ReprobeRequest struct {
	Restun bool
	Rebind bool
}

type ReprobeResult struct {
	RestunAttempted bool
	RebindAttempted bool
	RestunError string
	RebindError string
}

func (s *Service) Reprobe(ctx context.Context, request ReprobeRequest) ReprobeResult {
	var result ReprobeResult
	if request.Restun {
		result.RestunAttempted = true
		if _, err := s.runner.Run(ctx, "tailscale", "debug", "restun"); err != nil {
			result.RestunError = err.Error()
		}
	}
	if request.Rebind {
		result.RebindAttempted = true
		if _, err := s.runner.Run(ctx, "tailscale", "debug", "rebind"); err != nil {
			result.RebindError = err.Error()
		}
	}
	return result
}
```

- [ ] **Step 3: Run netdiag tests**

Run: `go test ./internal/netdiag`
Expected: PASS.

### Task 3: Manager Orchestration

**Files:**
- Modify: `internal/direct/manager/manager.go`
- Test: `internal/direct/manager/manager_test.go`

- [ ] **Step 1: Write failing manager tests**

```go
func TestOptimizeLinkAppliesRecommendedBypassForHighLatencyTUN(t *testing.T) {
	diag := &fakeNetworkDiagnostics{report: netdiag.PeerPathReport{
		PeerTailscaleIP: "100.109.251.97",
		Status: netdiag.PathDirectProxy,
		EndpointIP: "115.233.222.82",
		Latency: "249ms",
		Candidates: []netdiag.EgressCandidate{{InterfaceIndex: 15, NextHop: "192.168.1.1", Recommended: true}},
	}}
	m := New(Config{NetworkDiagnostics: diag})
	result, err := m.OptimizeLink(context.Background(), "100.109.251.97", linkguardian.Options{AutoBypass: true})
	if err != nil { t.Fatal(err) }
	if result.Decision.Action != linkguardian.ActionApplyBypass || diag.applyRequest.EndpointIP != "115.233.222.82" {
		t.Fatalf("unexpected optimization: result=%+v request=%+v", result, diag.applyRequest)
	}
}
```

Run: `go test ./internal/direct/manager -run TestOptimizeLink`
Expected: FAIL because `OptimizeLink` does not exist.

- [ ] **Step 2: Implement orchestration**

Add `Reprobe(context.Context, netdiag.ReprobeRequest) netdiag.ReprobeResult` to `NetworkDiagnostics`. Implement `OptimizeLink` so it diagnoses, evaluates, optionally reprobes, applies bypass, stores `activeBypass`, and returns a `linkguardian.Result`.

- [ ] **Step 3: Run manager tests**

Run: `go test ./internal/direct/manager`
Expected: PASS.

### Task 4: UI Controller And Existing Latency Reuse

**Files:**
- Modify: `internal/ui/direct_controller.go`
- Modify: `internal/ui/main_window.go`
- Test: `internal/ui/direct_controller_test.go`

- [ ] **Step 1: Write failing controller tests**

```go
func TestDirectControllerOptimizeLinkUsesSelectedPeerLatencySample(t *testing.T) {
	mgr := &fakeDirectManager{peers: []directmanager.TrustedPeer{{ID: "device-b", TailscaleIP: "100.109.251.97"}}}
	ctrl := NewDirectController(mgr)
	_ = ctrl.Refresh(context.Background())
	ctrl.state.PeerLatencies = map[string]PeerLatency{"device-b": {Latency: 23 * time.Millisecond, Updated: true}}
	if err := ctrl.OptimizeLink(context.Background(), "100.109.251.97", true); err != nil { t.Fatal(err) }
	if mgr.guardianOptions.LatestLatency != 23*time.Millisecond {
		t.Fatalf("expected latest latency sample to be reused, got %s", mgr.guardianOptions.LatestLatency)
	}
}
```

Run: `go test ./internal/ui -run TestDirectControllerOptimizeLink`
Expected: FAIL because controller API does not exist.

- [ ] **Step 2: Implement controller and UI wiring**

Add `LinkGuardian` to `DirectState`, add `OptimizeLink` to `DirectController`, extend `DirectManager` interface, add a compact network-tab status label, an `自动精确绕过` checkbox, and a `重新优化` button. Keep `peerLatencyRefreshInterval` at `200ms`, which satisfies the spec’s `<= 1s` limit.

- [ ] **Step 3: Run UI tests**

Run: `go test ./internal/ui`
Expected: PASS.

### Task 5: Verification And Build

**Files:**
- Modify: `docs/manual-verification.md`

- [ ] **Step 1: Run focused tests**

Run:
```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/linkguardian ./internal/netdiag ./internal/direct/manager ./internal/ui
```

Expected: all packages PASS.

- [ ] **Step 2: Run full verification**

Run:
```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED='1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File '.\scripts\build-windows.ps1'
```

Expected: tests pass, vet passes, build creates the Windows executable.
