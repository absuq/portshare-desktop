# portshare Direct Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the new `portshare` direct-mode MVP where two Tailscale-connected desktop clients pair with a shared secret and create local TCP forwarding entries to each other.

**Architecture:** Direct mode is a new subsystem beside the existing Tailscale Serve provider. It uses a Tailscale diagnostic adapter, an authenticated control protocol on `<tailscale-ip>:17890`, trusted peer persistence, and local TCP forwarding. The Fyne UI becomes centered on pairing and forwarding; legacy Serve/Funnel behavior can remain in code but is no longer the main path.

**Tech Stack:** Go 1.23+, Fyne v2, standard `net`, `crypto/hmac`, `crypto/sha256`, JSON persistence, Tailscale CLI adapter, Go tests with fake runners and loopback integration.

---

## File Structure

- Modify `internal/i18n/i18n.go`: visible product name and new direct-mode strings.
- Modify `internal/i18n/i18n_test.go`: assert `portshare` title and direct-mode strings.
- Create `internal/tailscale/runner.go`: command runner abstraction for `tailscale`.
- Create `internal/tailscale/status.go`: parse `tailscale status --json`, local IP, peer metadata.
- Create `internal/tailscale/diagnostics.go`: readiness checks, peer ping parsing, DNS/shields-up fix hints.
- Create `internal/tailscale/status_test.go`: status parsing tests.
- Create `internal/tailscale/diagnostics_test.go`: fake runner diagnostics tests.
- Create `internal/direct/protocol/frame.go`: length-prefixed JSON frame helpers.
- Create `internal/direct/protocol/auth.go`: shared-secret challenge response helpers.
- Create `internal/direct/protocol/messages.go`: protocol message types and version constants.
- Create `internal/direct/protocol/protocol_test.go`: framing and HMAC tests.
- Create `internal/direct/store/store.go`: trusted peer JSON persistence.
- Create `internal/direct/store/store_test.go`: persistence and no-plaintext-secret tests.
- Create `internal/direct/server.go`: authenticated control listener and open TCP handler.
- Create `internal/direct/client.go`: pair and open authenticated peer sessions.
- Create `internal/direct/server_client_test.go`: loopback pairing success/failure tests.
- Create `internal/direct/forward/forward.go`: local TCP listener, remote open, bidirectional copy.
- Create `internal/direct/forward/forward_test.go`: loopback forward integration tests.
- Create `internal/direct/manager/manager.go`: orchestration for readiness, pairing, trusted peers, forwards.
- Create `internal/direct/manager/manager_test.go`: fake Tailscale and fake direct client tests.
- Modify `internal/config/store.go`: add direct-mode config fields for control port and trusted peer path if needed.
- Modify `internal/config/store_test.go`: config round-trip for direct-mode fields.
- Modify `internal/ui/app.go`: inject direct manager and use `portshare` app title.
- Modify `internal/ui/main_window.go`: replace main screen with direct-mode panels.
- Create `internal/ui/direct_controller.go`: UI state/actions for diagnostics, pairing, and forwarding.
- Create `internal/ui/direct_controller_test.go`: controller behavior tests.
- Modify `internal/ui/tray.go`: tray title and direct-mode quick actions.
- Modify `cmd/portshare/main.go`: wire Tailscale adapter, direct manager, direct server.
- Modify `README.md`: describe new direct-mode MVP.
- Modify `docs/manual-verification.md`: update manual two-machine verification.
- Modify `AGENTS.md`: record new progress and direct-mode constraints.

---

### Task 1: Rename Visible Product To `portshare`

**Files:**
- Modify: `internal/i18n/i18n.go`
- Modify: `internal/i18n/i18n_test.go`
- Modify: `internal/ui/main_window.go`
- Modify: `internal/ui/tray.go`
- Modify: `README.md`
- Modify: `docs/manual-verification.md`

- [ ] **Step 1: Write failing i18n test**

Update `internal/i18n/i18n_test.go` so the app title must be `portshare` in both languages:

```go
func TestAppTitleIsPortshareInAllLanguages(t *testing.T) {
	zh := NewCatalog(domain.LanguageChinese)
	if got := zh.T(KeyAppTitle); got != "portshare" {
		t.Fatalf("expected Chinese app title to stay portshare, got %q", got)
	}
	en := NewCatalog(domain.LanguageEnglish)
	if got := en.T(KeyAppTitle); got != "portshare" {
		t.Fatalf("expected English app title to stay portshare, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/i18n
```

Expected: fail because Chinese title is still `端口发布器`.

- [ ] **Step 3: Update title strings**

Change `internal/i18n/i18n.go`:

```go
var zh = map[Key]string{
	KeyAppTitle: "portshare",
	// keep existing Chinese UI strings here
}

var en = map[Key]string{
	KeyAppTitle: "portshare",
	// keep existing English UI strings here
}
```

- [ ] **Step 4: Update hard-coded UI titles**

Change window and tray titles:

```go
// internal/ui/main_window.go
w := a.fyneApp.NewWindow("portshare")
```

```go
// internal/ui/tray.go
menu := fyne.NewMenu("portshare", ...)
```

- [ ] **Step 5: Update docs wording**

In `README.md` and `docs/manual-verification.md`, replace the product heading and product references with `portshare`. Keep Chinese explanatory prose.

- [ ] **Step 6: Run tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/i18n ./internal/ui
```

Expected: pass.

- [ ] **Step 7: Commit**

```powershell
git add internal/i18n internal/ui/main_window.go internal/ui/tray.go README.md docs/manual-verification.md
git commit -m "chore: rename product to portshare"
```

---

### Task 2: Add Tailscale Status And Diagnostics Adapter

**Files:**
- Create: `internal/tailscale/runner.go`
- Create: `internal/tailscale/status.go`
- Create: `internal/tailscale/diagnostics.go`
- Create: `internal/tailscale/status_test.go`
- Create: `internal/tailscale/diagnostics_test.go`

- [ ] **Step 1: Write status parsing tests**

Create `internal/tailscale/status_test.go`:

```go
package tailscale

import "testing"

func TestParseStatusReturnsLocalIPv4AndDNS(t *testing.T) {
	raw := []byte(`{
		"BackendState":"Running",
		"TailscaleIPs":["100.79.83.104","fd7a:115c:a1e0::1"],
		"Self":{"HostName":"abs_u_q","DNSName":"abs-u-q.tail51fe78.ts.net."},
		"CurrentTailnet":{"MagicDNSEnabled":true,"MagicDNSSuffix":"tail51fe78.ts.net"}
	}`)
	status, err := ParseStatus(raw)
	if err != nil {
		t.Fatal(err)
	}
	if status.BackendState != "Running" {
		t.Fatalf("unexpected backend state: %q", status.BackendState)
	}
	if status.LocalIPv4 != "100.79.83.104" {
		t.Fatalf("expected IPv4, got %q", status.LocalIPv4)
	}
	if status.SelfDNSName != "abs-u-q.tail51fe78.ts.net" {
		t.Fatalf("expected trimmed DNS name, got %q", status.SelfDNSName)
	}
	if !status.MagicDNSEnabled {
		t.Fatalf("expected MagicDNS enabled")
	}
}
```

- [ ] **Step 2: Write diagnostics tests**

Create `internal/tailscale/diagnostics_test.go`:

```go
package tailscale

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	outputs map[string][]byte
	errs    map[string]error
}

func (r fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	if err := r.errs[key]; err != nil {
		return nil, err
	}
	return r.outputs[key], nil
}

func TestCheckReadyReportsMissingCLI(t *testing.T) {
	client := NewClient(fakeRunner{errs: map[string]error{
		"tailscale status --json": errors.New("executable file not found"),
	}})
	report := client.CheckReady(context.Background())
	if report.Ready {
		t.Fatalf("expected not ready")
	}
	if report.Code != CodeTailscaleUnavailable {
		t.Fatalf("unexpected code: %s", report.Code)
	}
	if report.FixCommand != "" {
		t.Fatalf("missing CLI should not have automatic fix command")
	}
}

func TestPingPeerParsesDirectRoute(t *testing.T) {
	client := NewClient(fakeRunner{outputs: map[string][]byte{
		"tailscale ping --c 1 100.109.251.97": []byte("pong from peer via 115.233.222.82:41641 in 15ms"),
	}})
	route, err := client.PingPeer(context.Background(), "100.109.251.97")
	if err != nil {
		t.Fatal(err)
	}
	if route.Type != RouteDirect || route.Latency != "15ms" {
		t.Fatalf("unexpected route: %+v", route)
	}
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/tailscale
```

Expected: fail because package does not exist.

- [ ] **Step 4: Implement runner and types**

Create `internal/tailscale/runner.go`:

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
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
```

Create `internal/tailscale/status.go`:

```go
package tailscale

import (
	"encoding/json"
	"net"
	"strings"
)

type Status struct {
	BackendState     string
	LocalIPv4        string
	SelfHostName     string
	SelfDNSName      string
	MagicDNSEnabled  bool
	MagicDNSSuffix   string
}

func ParseStatus(data []byte) (Status, error) {
	var raw struct {
		BackendState string   `json:"BackendState"`
		TailscaleIPs []string `json:"TailscaleIPs"`
		Self struct {
			HostName string `json:"HostName"`
			DNSName  string `json:"DNSName"`
		} `json:"Self"`
		CurrentTailnet struct {
			MagicDNSEnabled bool   `json:"MagicDNSEnabled"`
			MagicDNSSuffix  string `json:"MagicDNSSuffix"`
		} `json:"CurrentTailnet"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Status{}, err
	}
	status := Status{
		BackendState:    raw.BackendState,
		SelfHostName:    raw.Self.HostName,
		SelfDNSName:     strings.TrimSuffix(raw.Self.DNSName, "."),
		MagicDNSEnabled: raw.CurrentTailnet.MagicDNSEnabled,
		MagicDNSSuffix:  raw.CurrentTailnet.MagicDNSSuffix,
	}
	for _, ip := range raw.TailscaleIPs {
		parsed := net.ParseIP(ip)
		if parsed != nil && parsed.To4() != nil {
			status.LocalIPv4 = ip
			break
		}
	}
	return status, nil
}
```

- [ ] **Step 5: Implement diagnostics client**

Create `internal/tailscale/diagnostics.go`:

```go
package tailscale

import (
	"context"
	"regexp"
	"strings"
)

type DiagnosticCode string

const (
	CodeOK                   DiagnosticCode = "ok"
	CodeTailscaleUnavailable DiagnosticCode = "tailscale.unavailable"
	CodeTailscaleStopped     DiagnosticCode = "tailscale.stopped"
	CodeNoTailscaleIP        DiagnosticCode = "tailscale.no_ip"
	CodePeerUnreachable      DiagnosticCode = "peer.unreachable"
	CodeDNSNotAccepted       DiagnosticCode = "dns.not_accepted"
)

type ReadyReport struct {
	Ready      bool
	Code       DiagnosticCode
	Message    string
	FixCommand string
	Status     Status
}

type RouteType string

const (
	RouteUnknown RouteType = "unknown"
	RouteDirect  RouteType = "direct"
	RouteDERP    RouteType = "derp"
)

type PeerRoute struct {
	Type    RouteType
	Via     string
	Latency string
	Raw     string
}

type Client struct {
	runner Runner
}

func NewClient(runner Runner) Client {
	if runner == nil {
		runner = ExecRunner{}
	}
	return Client{runner: runner}
}

func (c Client) CheckReady(ctx context.Context) ReadyReport {
	output, err := c.runner.Run(ctx, "tailscale", "status", "--json")
	if err != nil {
		return ReadyReport{Code: CodeTailscaleUnavailable, Message: err.Error()}
	}
	status, err := ParseStatus(output)
	if err != nil {
		return ReadyReport{Code: CodeTailscaleUnavailable, Message: err.Error()}
	}
	if status.BackendState != "Running" {
		return ReadyReport{Code: CodeTailscaleStopped, Message: "Tailscale 未运行", Status: status}
	}
	if status.LocalIPv4 == "" {
		return ReadyReport{Code: CodeNoTailscaleIP, Message: "没有检测到 Tailscale IPv4", Status: status}
	}
	return ReadyReport{Ready: true, Code: CodeOK, Message: "Tailscale ready", Status: status}
}

func (c Client) PingPeer(ctx context.Context, peer string) (PeerRoute, error) {
	output, err := c.runner.Run(ctx, "tailscale", "ping", "--c", "1", peer)
	if err != nil {
		return PeerRoute{Type: RouteUnknown, Raw: string(output)}, err
	}
	raw := string(output)
	route := PeerRoute{Type: RouteUnknown, Raw: raw}
	if strings.Contains(raw, "via DERP(") {
		route.Type = RouteDERP
	} else if strings.Contains(raw, " via ") {
		route.Type = RouteDirect
	}
	viaMatch := regexp.MustCompile(`via ([^ ]+)`).FindStringSubmatch(raw)
	if len(viaMatch) == 2 {
		route.Via = viaMatch[1]
	}
	latencyMatch := regexp.MustCompile(`in ([0-9.]+[a-z]+)`).FindStringSubmatch(raw)
	if len(latencyMatch) == 2 {
		route.Latency = latencyMatch[1]
	}
	return route, nil
}
```

- [ ] **Step 6: Run tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/tailscale
```

Expected: pass.

- [ ] **Step 7: Commit**

```powershell
git add internal/tailscale
git commit -m "feat: add tailscale diagnostics"
```

---

### Task 3: Add Direct Protocol Framing And Shared-Secret Auth

**Files:**
- Create: `internal/direct/protocol/messages.go`
- Create: `internal/direct/protocol/frame.go`
- Create: `internal/direct/protocol/auth.go`
- Create: `internal/direct/protocol/protocol_test.go`

- [ ] **Step 1: Write protocol tests**

Create `internal/direct/protocol/protocol_test.go`:

```go
package protocol

import (
	"bytes"
	"testing"
)

func TestProofMatchesWithSameSecret(t *testing.T) {
	nonceA := []byte("initiator-nonce")
	nonceB := []byte("responder-nonce")
	proof := ComputeProof("shared-secret", "device-a", "device-b", nonceA, nonceB)
	if !VerifyProof("shared-secret", "device-a", "device-b", nonceA, nonceB, proof) {
		t.Fatalf("expected proof to verify")
	}
}

func TestProofFailsWithDifferentSecret(t *testing.T) {
	nonceA := []byte("initiator-nonce")
	nonceB := []byte("responder-nonce")
	proof := ComputeProof("right-secret", "device-a", "device-b", nonceA, nonceB)
	if VerifyProof("wrong-secret", "device-a", "device-b", nonceA, nonceB, proof) {
		t.Fatalf("expected wrong secret to fail")
	}
}

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	msg := ControlMessage{Type: TypeHello, Version: Version, DeviceID: "device-a"}
	if err := WriteFrame(&buf, msg); err != nil {
		t.Fatal(err)
	}
	var got ControlMessage
	if err := ReadFrame(&buf, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != TypeHello || got.DeviceID != "device-a" {
		t.Fatalf("unexpected message: %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/protocol
```

Expected: fail because package does not exist.

- [ ] **Step 3: Implement message types**

Create `internal/direct/protocol/messages.go`:

```go
package protocol

const Version = 1

type MessageType string

const (
	TypeHello        MessageType = "hello"
	TypeHelloResp    MessageType = "hello_response"
	TypeAuthProof    MessageType = "auth_proof"
	TypeAuthOK       MessageType = "auth_ok"
	TypeOpenTCP      MessageType = "open_tcp"
	TypeOpenTCPOK    MessageType = "open_tcp_ok"
	TypeOpenTCPError MessageType = "open_tcp_error"
)

type ControlMessage struct {
	Type       MessageType `json:"type"`
	Version    int         `json:"version"`
	DeviceID   string      `json:"device_id,omitempty"`
	DeviceName string      `json:"device_name,omitempty"`
	Nonce      []byte      `json:"nonce,omitempty"`
	Proof      []byte      `json:"proof,omitempty"`
	TargetHost string      `json:"target_host,omitempty"`
	TargetPort int         `json:"target_port,omitempty"`
	Error      string      `json:"error,omitempty"`
}
```

- [ ] **Step 4: Implement frame helpers**

Create `internal/direct/protocol/frame.go`:

```go
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const MaxFrameSize = 1 << 20

func WriteFrame(w io.Writer, msg ControlMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if len(data) > MaxFrameSize {
		return fmt.Errorf("frame too large: %d", len(data))
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func ReadFrame(r io.Reader, msg *ControlMessage) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > MaxFrameSize {
		return fmt.Errorf("invalid frame size: %d", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	return json.Unmarshal(data, msg)
}
```

- [ ] **Step 5: Implement auth helpers**

Create `internal/direct/protocol/auth.go`:

```go
package protocol

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"
)

func NewNonce() ([]byte, error) {
	nonce := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, nonce)
	return nonce, err
}

func ComputeProof(secret, fromDevice, toDevice string, nonceA, nonceB []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("portshare-direct-v1"))
	mac.Write([]byte(fromDevice))
	mac.Write([]byte{0})
	mac.Write([]byte(toDevice))
	mac.Write([]byte{0})
	mac.Write(nonceA)
	mac.Write(nonceB)
	return mac.Sum(nil)
}

func VerifyProof(secret, fromDevice, toDevice string, nonceA, nonceB, proof []byte) bool {
	expected := ComputeProof(secret, fromDevice, toDevice, nonceA, nonceB)
	return hmac.Equal(expected, proof)
}
```

- [ ] **Step 6: Run tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/protocol
```

Expected: pass.

- [ ] **Step 7: Commit**

```powershell
git add internal/direct/protocol
git commit -m "feat: add direct protocol auth"
```

---

### Task 4: Add Trusted Peer Store

**Files:**
- Create: `internal/direct/store/store.go`
- Create: `internal/direct/store/store_test.go`

- [ ] **Step 1: Write store tests**

Create `internal/direct/store/store_test.go`:

```go
package store

import (
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
		SecretLabel:   "sha256:abc",
	}
	if err := s.SavePeers([]TrustedPeer{peer}); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TailscaleIP != peer.TailscaleIP {
		t.Fatalf("unexpected peers: %+v", got)
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
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/store
```

Expected: fail because package does not exist.

- [ ] **Step 3: Implement store**

Create `internal/direct/store/store.go`:

```go
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
	SecretLabel   string    `json:"secret_label"`
}

type Store struct {
	path string
}

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
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(peers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
```

- [ ] **Step 4: Run tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/store
```

Expected: pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/direct/store
git commit -m "feat: persist trusted peers"
```

---

### Task 5: Add Direct Server And Client Pairing

**Files:**
- Create: `internal/direct/server.go`
- Create: `internal/direct/client.go`
- Create: `internal/direct/server_client_test.go`

- [ ] **Step 1: Write loopback pairing tests**

Create `internal/direct/server_client_test.go`:

```go
package direct

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestPairingSucceedsWithMatchingSecret(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	server := NewServer(ServerConfig{
		DeviceID:   "device-b",
		DeviceName: "desktop-b",
		Secret:     "shared",
	})
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	client := NewClient(ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	peer, err := client.Pair(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if peer.DeviceID != "device-b" || peer.DeviceName != "desktop-b" {
		t.Fatalf("unexpected peer: %+v", peer)
	}
}

func TestPairingFailsWithWrongSecret(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	server := NewServer(ServerConfig{DeviceID: "device-b", DeviceName: "desktop-b", Secret: "right"})
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	client := NewClient(ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "wrong"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.Pair(ctx, listener.Addr().String()); err == nil {
		t.Fatalf("expected pairing to fail")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct
```

Expected: fail because direct package does not exist.

- [ ] **Step 3: Implement server API**

Create `internal/direct/server.go`:

```go
package direct

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/absuq/portshare-desktop/internal/direct/protocol"
)

type ServerConfig struct {
	DeviceID   string
	DeviceName string
	Secret     string
}

type Server struct {
	config ServerConfig
	closed chan struct{}
	once   sync.Once
}

func NewServer(config ServerConfig) *Server {
	return &Server{config: config, closed: make(chan struct{})}
}

func (s *Server) Serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return nil
			default:
				return err
			}
		}
		go s.handle(conn)
	}
}

func (s *Server) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	var hello protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &hello); err != nil || hello.Type != protocol.TypeHello {
		return
	}
	responderNonce, err := protocol.NewNonce()
	if err != nil {
		return
	}
	resp := protocol.ControlMessage{
		Type:       protocol.TypeHelloResp,
		Version:    protocol.Version,
		DeviceID:   s.config.DeviceID,
		DeviceName: s.config.DeviceName,
		Nonce:      responderNonce,
		Proof: protocol.ComputeProof(
			s.config.Secret,
			s.config.DeviceID,
			hello.DeviceID,
			hello.Nonce,
			responderNonce,
		),
	}
	if err := protocol.WriteFrame(conn, resp); err != nil {
		return
	}
	var proof protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &proof); err != nil || proof.Type != protocol.TypeAuthProof {
		return
	}
	if !protocol.VerifyProof(s.config.Secret, hello.DeviceID, s.config.DeviceID, hello.Nonce, responderNonce, proof.Proof) {
		_ = protocol.WriteFrame(conn, protocol.ControlMessage{Type: protocol.TypeOpenTCPError, Version: protocol.Version, Error: "authentication failed"})
		return
	}
	_ = protocol.WriteFrame(conn, protocol.ControlMessage{Type: protocol.TypeAuthOK, Version: protocol.Version, DeviceID: s.config.DeviceID, DeviceName: s.config.DeviceName})
}

var ErrAuthFailed = errors.New("authentication failed")

func unusedContext(context.Context) {}
```

- [ ] **Step 4: Implement client API**

Create `internal/direct/client.go`:

```go
package direct

import (
	"context"
	"fmt"
	"net"

	"github.com/absuq/portshare-desktop/internal/direct/protocol"
)

type ClientConfig struct {
	DeviceID   string
	DeviceName string
	Secret     string
}

type Client struct {
	config ClientConfig
}

type PairedPeer struct {
	DeviceID   string
	DeviceName string
	Address    string
}

func NewClient(config ClientConfig) Client {
	return Client{config: config}
}

func (c Client) Pair(ctx context.Context, address string) (PairedPeer, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return PairedPeer{}, err
	}
	defer conn.Close()

	initiatorNonce, err := protocol.NewNonce()
	if err != nil {
		return PairedPeer{}, err
	}
	hello := protocol.ControlMessage{
		Type:       protocol.TypeHello,
		Version:    protocol.Version,
		DeviceID:   c.config.DeviceID,
		DeviceName: c.config.DeviceName,
		Nonce:      initiatorNonce,
	}
	if err := protocol.WriteFrame(conn, hello); err != nil {
		return PairedPeer{}, err
	}
	var resp protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &resp); err != nil {
		return PairedPeer{}, err
	}
	if resp.Type != protocol.TypeHelloResp {
		return PairedPeer{}, fmt.Errorf("unexpected response: %s", resp.Type)
	}
	if !protocol.VerifyProof(c.config.Secret, resp.DeviceID, c.config.DeviceID, initiatorNonce, resp.Nonce, resp.Proof) {
		return PairedPeer{}, ErrAuthFailed
	}
	proof := protocol.ControlMessage{
		Type:    protocol.TypeAuthProof,
		Version: protocol.Version,
		Proof: protocol.ComputeProof(c.config.Secret, c.config.DeviceID, resp.DeviceID, initiatorNonce, resp.Nonce),
	}
	if err := protocol.WriteFrame(conn, proof); err != nil {
		return PairedPeer{}, err
	}
	var ok protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &ok); err != nil {
		return PairedPeer{}, err
	}
	if ok.Type != protocol.TypeAuthOK {
		return PairedPeer{}, ErrAuthFailed
	}
	return PairedPeer{DeviceID: resp.DeviceID, DeviceName: resp.DeviceName, Address: address}, nil
}
```

- [ ] **Step 5: Run pairing tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct ./internal/direct/protocol
```

Expected: pass.

- [ ] **Step 6: Commit**

```powershell
git add internal/direct
git commit -m "feat: pair direct peers"
```

---

### Task 6: Add TCP Open Protocol And Local Forwarding

**Files:**
- Modify: `internal/direct/server.go`
- Modify: `internal/direct/client.go`
- Create: `internal/direct/forward/forward.go`
- Create: `internal/direct/forward/forward_test.go`

- [ ] **Step 1: Write forward integration test**

Create `internal/direct/forward/forward_test.go`:

```go
package forward

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	direct "github.com/absuq/portshare-desktop/internal/direct"
)

func TestLocalForwardReachesPeerTarget(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello through portshare"))
	}))
	defer target.Close()
	targetHost, targetPort := splitHostPort(t, target.Listener.Addr().String())

	control, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()

	server := direct.NewServer(direct.ServerConfig{DeviceID: "device-b", DeviceName: "desktop-b", Secret: "shared"})
	go func() { _ = server.Serve(control) }()
	defer server.Close()

	client := direct.NewClient(direct.ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	fwd := New(Options{
		LocalAddress:  "127.0.0.1:0",
		PeerAddress:   control.Addr().String(),
		TargetHost:    targetHost,
		TargetPort:    targetPort,
		DirectClient:  client,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := fwd.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer fwd.Stop()

	resp, err := http.Get("http://" + fwd.LocalAddress())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello through portshare" {
		t.Fatalf("unexpected body: %q", body)
	}
}
```

Add helper in the same test file:

```go
func splitHostPort(t *testing.T, address string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}
```

Import `strconv` in the test file.

- [ ] **Step 2: Run test to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/forward
```

Expected: fail because package does not exist and direct client cannot open TCP streams yet.

- [ ] **Step 3: Extend direct client with OpenTCP**

Add this method to `internal/direct/client.go`:

```go
func (c Client) OpenTCP(ctx context.Context, peerAddress, targetHost string, targetPort int) (net.Conn, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", peerAddress)
	if err != nil {
		return nil, err
	}
	if err := c.authenticate(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	req := protocol.ControlMessage{
		Type:       protocol.TypeOpenTCP,
		Version:    protocol.Version,
		TargetHost: targetHost,
		TargetPort: targetPort,
	}
	if err := protocol.WriteFrame(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	var resp protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &resp); err != nil {
		conn.Close()
		return nil, err
	}
	if resp.Type != protocol.TypeOpenTCPOK {
		conn.Close()
		if resp.Error != "" {
			return nil, fmt.Errorf(resp.Error)
		}
		return nil, fmt.Errorf("open tcp failed")
	}
	return conn, nil
}
```

Refactor `Pair` to use a private `authenticate(ctx, conn)` helper that performs the hello/proof/auth_ok exchange and returns peer metadata. Keep existing pairing tests green.

- [ ] **Step 4: Extend server to handle open_tcp**

In `internal/direct/server.go`, after authentication succeeds, read one more frame. If the frame is `open_tcp`, dial the requested target and pipe bytes:

```go
func (s *Server) handleOpenTCP(conn net.Conn, msg protocol.ControlMessage) {
	target, err := net.Dial("tcp", net.JoinHostPort(msg.TargetHost, strconv.Itoa(msg.TargetPort)))
	if err != nil {
		_ = protocol.WriteFrame(conn, protocol.ControlMessage{Type: protocol.TypeOpenTCPError, Version: protocol.Version, Error: err.Error()})
		return
	}
	defer target.Close()
	if err := protocol.WriteFrame(conn, protocol.ControlMessage{Type: protocol.TypeOpenTCPOK, Version: protocol.Version}); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(target, conn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(conn, target); done <- struct{}{} }()
	<-done
}
```

Import `io`, `strconv`, and use `net.JoinHostPort`.

- [ ] **Step 5: Implement local forward**

Create `internal/direct/forward/forward.go`:

```go
package forward

import (
	"context"
	"io"
	"net"
	"sync"

	direct "github.com/absuq/portshare-desktop/internal/direct"
)

type Options struct {
	LocalAddress string
	PeerAddress  string
	TargetHost   string
	TargetPort   int
	DirectClient direct.Client
}

type Forward struct {
	options  Options
	listener net.Listener
	mu       sync.Mutex
}

func New(options Options) *Forward {
	return &Forward{options: options}
}

func (f *Forward) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", f.options.LocalAddress)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.listener = ln
	f.mu.Unlock()
	go f.accept(ctx, ln)
	return nil
}

func (f *Forward) LocalAddress() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listener == nil {
		return ""
	}
	return f.listener.Addr().String()
}

func (f *Forward) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listener == nil {
		return nil
	}
	err := f.listener.Close()
	f.listener = nil
	return err
}

func (f *Forward) accept(ctx context.Context, ln net.Listener) {
	for {
		local, err := ln.Accept()
		if err != nil {
			return
		}
		go f.handle(ctx, local)
	}
}

func (f *Forward) handle(ctx context.Context, local net.Conn) {
	defer local.Close()
	remote, err := f.options.DirectClient.OpenTCP(ctx, f.options.PeerAddress, f.options.TargetHost, f.options.TargetPort)
	if err != nil {
		return
	}
	defer remote.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(remote, local); done <- struct{}{} }()
	go func() { _, _ = io.Copy(local, remote); done <- struct{}{} }()
	<-done
}
```

- [ ] **Step 6: Run forward tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/... 
```

Expected: pass.

- [ ] **Step 7: Commit**

```powershell
git add internal/direct
git commit -m "feat: forward tcp through direct peers"
```

---

### Task 7: Add Direct Manager And Config Wiring

**Files:**
- Create: `internal/direct/manager/manager.go`
- Create: `internal/direct/manager/manager_test.go`
- Modify: `internal/config/store.go`
- Modify: `internal/config/store_test.go`

- [ ] **Step 1: Write config test**

Extend `internal/config/store_test.go`:

```go
func TestStoreRoundTripDirectModeFields(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "config.json"))
	cfg := Default()
	cfg.DirectControlPort = 17890
	cfg.DirectPeersPath = filepath.Join(dir, "peers.json")
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.DirectControlPort != 17890 || got.DirectPeersPath == "" {
		t.Fatalf("unexpected direct config: %+v", got)
	}
}
```

- [ ] **Step 2: Run config test to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/config
```

Expected: fail because direct fields do not exist.

- [ ] **Step 3: Add config fields**

Modify `internal/config/store.go`:

```go
type Config struct {
	Language          domain.Language `json:"language"`
	AuditRetention    time.Duration   `json:"audit_retention"`
	MinimizeToTray    bool            `json:"minimize_to_tray"`
	ConfirmOnExit     bool            `json:"confirm_on_exit"`
	DefaultPublicTTL   time.Duration   `json:"default_public_ttl"`
	DirectControlPort int             `json:"direct_control_port"`
	DirectPeersPath    string          `json:"direct_peers_path"`
}
```

Update `Default()`:

```go
DirectControlPort: 17890,
```

In `cmd/portshare/main.go`, if `DirectPeersPath` is empty after load, default it to `filepath.Join(filepath.Dir(cfgPath), "direct-peers.json")`.

- [ ] **Step 4: Write manager tests**

Create `internal/direct/manager/manager_test.go`:

```go
package manager

import (
	"context"
	"testing"

	"github.com/absuq/portshare-desktop/internal/tailscale"
)

type fakeTailscale struct {
	report tailscale.ReadyReport
	route  tailscale.PeerRoute
}

func (f fakeTailscale) CheckReady(context.Context) tailscale.ReadyReport { return f.report }
func (f fakeTailscale) PingPeer(context.Context, string) (tailscale.PeerRoute, error) {
	return f.route, nil
}

func TestReadyUsesTailscaleReport(t *testing.T) {
	m := New(Config{Tailscale: fakeTailscale{report: tailscale.ReadyReport{
		Ready: true,
		Code:  tailscale.CodeOK,
		Status: tailscale.Status{LocalIPv4: "100.79.83.104"},
	}}})
	state := m.Ready(context.Background())
	if !state.Ready || state.LocalTailscaleIP != "100.79.83.104" {
		t.Fatalf("unexpected state: %+v", state)
	}
}
```

- [ ] **Step 5: Run manager tests to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager
```

Expected: fail because manager package does not exist.

- [ ] **Step 6: Implement direct manager skeleton**

Create `internal/direct/manager/manager.go`:

```go
package manager

import (
	"context"

	"github.com/absuq/portshare-desktop/internal/tailscale"
)

type Tailscale interface {
	CheckReady(context.Context) tailscale.ReadyReport
	PingPeer(context.Context, string) (tailscale.PeerRoute, error)
}

type Config struct {
	Tailscale Tailscale
}

type Manager struct {
	tailscale Tailscale
}

type ReadyState struct {
	Ready            bool
	LocalTailscaleIP string
	Code             tailscale.DiagnosticCode
	Message          string
}

func New(config Config) *Manager {
	return &Manager{tailscale: config.Tailscale}
}

func (m *Manager) Ready(ctx context.Context) ReadyState {
	report := m.tailscale.CheckReady(ctx)
	return ReadyState{
		Ready:            report.Ready,
		LocalTailscaleIP: report.Status.LocalIPv4,
		Code:             report.Code,
		Message:          report.Message,
	}
}
```

This baseline is intentionally limited to readiness. Task 8 adds peer and forward APIs, and Task 11 adds control server lifecycle.

- [ ] **Step 7: Run tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/config ./internal/direct/manager
```

Expected: pass.

- [ ] **Step 8: Commit**

```powershell
git add internal/config internal/direct/manager
git commit -m "feat: add direct manager baseline"
```

---

### Task 8: Extend Direct Manager With Pairing And Forwards

**Files:**
- Modify: `internal/direct/manager/manager.go`
- Modify: `internal/direct/manager/manager_test.go`

- [ ] **Step 1: Write manager pair test**

Add to `internal/direct/manager/manager_test.go`:

```go
type fakePairClient struct {
	peerID string
}

func (f fakePairClient) Pair(ctx context.Context, address string) (PairedPeer, error) {
	return PairedPeer{DeviceID: f.peerID, DeviceName: "desktop-b", Address: address}, nil
}

func TestPairPeerStoresTrustedPeer(t *testing.T) {
	mem := NewMemoryPeerStore()
	m := New(Config{
		Tailscale: fakeTailscale{report: tailscale.ReadyReport{Ready: true, Code: tailscale.CodeOK}},
		PairClient: fakePairClient{peerID: "device-b"},
		PeerStore: mem,
	})
	peer, err := m.PairPeer(context.Background(), "100.109.251.97:17890")
	if err != nil {
		t.Fatal(err)
	}
	if peer.DeviceID != "device-b" {
		t.Fatalf("unexpected peer: %+v", peer)
	}
	if len(mem.Peers()) != 1 {
		t.Fatalf("expected one stored peer")
	}
}
```

Define the `fakePairClient` shown above and a `MemoryPeerStore` helper in the test package. Keep production interfaces small:

```go
type PairClient interface {
	Pair(context.Context, string) (PairedPeer, error)
}
```

- [ ] **Step 2: Write manager forward test**

Add:

```go
func TestCreateForwardRejectsUnknownPeer(t *testing.T) {
	m := New(Config{PeerStore: NewMemoryPeerStore()})
	_, err := m.CreateForward(context.Background(), ForwardRequest{
		PeerID: "missing",
		TargetHost: "127.0.0.1",
		TargetPort: 3000,
		LocalAddress: "127.0.0.1:0",
	})
	if err == nil {
		t.Fatalf("expected unknown peer error")
	}
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager
```

Expected: fail because manager does not expose pairing/forward APIs yet.

- [ ] **Step 4: Implement peer and forward APIs**

Extend `internal/direct/manager/manager.go` with:

```go
type PairedPeer struct {
	DeviceID   string
	DeviceName string
	Address    string
}

type PairClient interface {
	Pair(context.Context, string) (PairedPeer, error)
}

type PeerStore interface {
	LoadPeers() ([]TrustedPeer, error)
	SavePeers([]TrustedPeer) error
}

type TrustedPeer struct {
	ID          string
	DisplayName string
	Address     string
}

type ForwardRequest struct {
	PeerID       string
	TargetHost   string
	TargetPort   int
	LocalAddress string
}

type RunningForward struct {
	ID           string
	PeerID       string
	LocalAddress string
	Target       string
}
```

Implement:

```go
func (m *Manager) PairPeer(ctx context.Context, address string) (PairedPeer, error)
func (m *Manager) TrustedPeers(ctx context.Context) ([]TrustedPeer, error)
func (m *Manager) CreateForward(ctx context.Context, req ForwardRequest) (RunningForward, error)
func (m *Manager) StopForward(ctx context.Context, id string) error
```

Use simple IDs such as peer device ID and forward IDs derived from an incrementing counter guarded by a mutex. Return a clear error for unknown peer.

- [ ] **Step 5: Add memory store for tests**

Add this helper to `manager.go` only if it is useful outside tests; otherwise put it in `manager_test.go`:

```go
type MemoryPeerStore struct {
	peers []TrustedPeer
}

func NewMemoryPeerStore() *MemoryPeerStore { return &MemoryPeerStore{} }
func (s *MemoryPeerStore) LoadPeers() ([]TrustedPeer, error) { return append([]TrustedPeer(nil), s.peers...), nil }
func (s *MemoryPeerStore) SavePeers(peers []TrustedPeer) error {
	s.peers = append([]TrustedPeer(nil), peers...)
	return nil
}
func (s *MemoryPeerStore) Peers() []TrustedPeer { return append([]TrustedPeer(nil), s.peers...) }
```

- [ ] **Step 6: Run manager tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager
```

Expected: pass.

- [ ] **Step 7: Commit**

```powershell
git add internal/direct/manager
git commit -m "feat: manage direct peers and forwards"
```

---

### Task 9: Add Direct UI Controller

**Files:**
- Create: `internal/ui/direct_controller.go`
- Create: `internal/ui/direct_controller_test.go`

- [ ] **Step 1: Write controller tests**

Create `internal/ui/direct_controller_test.go`:

```go
package ui

import (
	"context"
	"testing"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
)

type fakeDirectManager struct {
	ready   directmanager.ReadyState
	peers   []directmanager.TrustedPeer
	forward directmanager.RunningForward
	started bool
}

func (f *fakeDirectManager) Ready(context.Context) directmanager.ReadyState { return f.ready }
func (f *fakeDirectManager) StartControlServer(context.Context, string, string) error {
	f.started = true
	return nil
}
func (f *fakeDirectManager) StopControlServer(context.Context) error { f.started = false; return nil }
func (f *fakeDirectManager) PairPeer(context.Context, string) (directmanager.PairedPeer, error) {
	f.peers = append(f.peers, directmanager.TrustedPeer{ID: "device-b", DisplayName: "desktop-b", Address: "100.109.251.97:17890"})
	return directmanager.PairedPeer{DeviceID: "device-b", DeviceName: "desktop-b", Address: "100.109.251.97:17890"}, nil
}
func (f *fakeDirectManager) TrustedPeers(context.Context) ([]directmanager.TrustedPeer, error) { return f.peers, nil }
func (f *fakeDirectManager) CreateForward(context.Context, directmanager.ForwardRequest) (directmanager.RunningForward, error) {
	f.forward = directmanager.RunningForward{ID: "fwd-1", PeerID: "device-b", LocalAddress: "127.0.0.1:18080", Target: "127.0.0.1:3000"}
	return f.forward, nil
}
func (f *fakeDirectManager) StopForward(context.Context, string) error { f.forward = directmanager.RunningForward{}; return nil }

func TestDirectControllerRefreshShowsReadyState(t *testing.T) {
	mgr := &fakeDirectManager{ready: directmanager.ReadyState{Ready: true, LocalTailscaleIP: "100.79.83.104"}}
	ctrl := NewDirectController(mgr)
	if err := ctrl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := ctrl.State()
	if !state.Ready || state.LocalTailscaleIP != "100.79.83.104" {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestDirectControllerPairAndForward(t *testing.T) {
	mgr := &fakeDirectManager{}
	ctrl := NewDirectController(mgr)
	if err := ctrl.PairPeer(context.Background(), "100.109.251.97"); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.CreateForward(context.Background(), "device-b", "127.0.0.1", 3000, "127.0.0.1:18080"); err != nil {
		t.Fatal(err)
	}
	state := ctrl.State()
	if len(state.Peers) != 1 || len(state.Forwards) != 1 {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestDirectControllerStartsControlServerWithSecret(t *testing.T) {
	mgr := &fakeDirectManager{}
	ctrl := NewDirectController(mgr)
	if err := ctrl.StartDirectMode(context.Background(), "shared-secret", "100.79.83.104:17890"); err != nil {
		t.Fatal(err)
	}
	if !mgr.started {
		t.Fatalf("expected control server to start")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/ui
```

Expected: fail because direct controller does not exist.

- [ ] **Step 3: Implement controller**

Create `internal/ui/direct_controller.go`:

```go
package ui

import (
	"context"
	"fmt"

	directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
)

type DirectManager interface {
	Ready(context.Context) directmanager.ReadyState
	StartControlServer(context.Context, string, string) error
	StopControlServer(context.Context) error
	PairPeer(context.Context, string) (directmanager.PairedPeer, error)
	TrustedPeers(context.Context) ([]directmanager.TrustedPeer, error)
	CreateForward(context.Context, directmanager.ForwardRequest) (directmanager.RunningForward, error)
	StopForward(context.Context, string) error
}

type DirectController struct {
	manager  DirectManager
	state    DirectState
	forwards []directmanager.RunningForward
}

type DirectState struct {
	Ready            bool
	LocalTailscaleIP string
	Message          string
	Peers            []directmanager.TrustedPeer
	Forwards         []directmanager.RunningForward
}

func NewDirectController(manager DirectManager) *DirectController {
	return &DirectController{manager: manager}
}

func (c *DirectController) StartDirectMode(ctx context.Context, secret string, listenAddress string) error {
	if err := c.manager.StartControlServer(ctx, listenAddress, secret); err != nil {
		c.state.Message = "启动直连监听失败：" + err.Error()
		return err
	}
	c.state.Message = "直连监听已启动"
	return c.Refresh(ctx)
}

func (c *DirectController) Refresh(ctx context.Context) error {
	ready := c.manager.Ready(ctx)
	peers, err := c.manager.TrustedPeers(ctx)
	if err != nil {
		c.state.Message = "读取可信设备失败：" + err.Error()
		return err
	}
	c.state.Ready = ready.Ready
	c.state.LocalTailscaleIP = ready.LocalTailscaleIP
	c.state.Peers = peers
	c.state.Forwards = append([]directmanager.RunningForward(nil), c.forwards...)
	if ready.Ready {
		c.state.Message = "Tailscale 与 direct mode 已就绪"
	} else {
		c.state.Message = ready.Message
	}
	return nil
}

func (c *DirectController) PairPeer(ctx context.Context, peerIP string) error {
	peer, err := c.manager.PairPeer(ctx, peerIP+":17890")
	if err != nil {
		c.state.Message = "配对失败：" + err.Error()
		return err
	}
	c.state.Message = "已配对：" + peer.DeviceName
	return c.Refresh(ctx)
}

func (c *DirectController) CreateForward(ctx context.Context, peerID, targetHost string, targetPort int, localAddress string) error {
	fwd, err := c.manager.CreateForward(ctx, directmanager.ForwardRequest{
		PeerID:       peerID,
		TargetHost:   targetHost,
		TargetPort:   targetPort,
		LocalAddress: localAddress,
	})
	if err != nil {
		c.state.Message = "创建转发失败：" + err.Error()
		return err
	}
	c.forwards = append(c.forwards, fwd)
	c.state.Message = fmt.Sprintf("已创建转发：%s -> %s", fwd.LocalAddress, fwd.Target)
	return c.Refresh(ctx)
}

func (c *DirectController) State() DirectState {
	return c.state
}
```

- [ ] **Step 4: Run UI controller tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/ui
```

Expected: pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/ui/direct_controller.go internal/ui/direct_controller_test.go
git commit -m "feat: add direct ui controller"
```

---

### Task 10: Replace Main Window With Direct-Mode UI

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/main_window.go`
- Modify: `internal/ui/tray.go`

- [ ] **Step 1: Extend dependencies**

Modify `internal/ui/app.go`:

```go
type Dependencies struct {
	Manager       Manager
	Discovery     Discovery
	DirectManager DirectManager
	Timeout       time.Duration
}

type App struct {
	fyneApp    fyne.App
	window     fyne.Window
	deps       Dependencies
	ctrl       *Controller
	directCtrl *DirectController
	refreshUI  func()
}

func New(deps Dependencies) *App {
	a := app.NewWithID("com.absuq.portshare")
	return &App{
		fyneApp:    a,
		deps:       deps,
		ctrl:       NewController(deps),
		directCtrl: NewDirectController(deps.DirectManager),
	}
}
```

- [ ] **Step 2: Replace main title and top-level content**

In `internal/ui/main_window.go`, set:

```go
w := a.fyneApp.NewWindow("portshare")
```

Build these UI sections:

```go
statusLabel := widget.NewLabel("Tailscale：检测中")
ipLabel := widget.NewLabel("本机 IP：-")
secretEntry := widget.NewPasswordEntry()
secretEntry.SetPlaceHolder("输入共享密钥")
peerEntry := widget.NewEntry()
peerEntry.SetPlaceHolder("对方 Tailscale IP，例如 100.109.251.97")
targetPortEntry := widget.NewEntry()
targetPortEntry.SetPlaceHolder("远端端口，例如 3000")
localPortEntry := widget.NewEntry()
localPortEntry.SetPlaceHolder("本地端口，例如 18080，留空自动分配")
messageLabel := widget.NewLabel("")
```

The secret field must start the direct-mode control listener through `DirectController.StartDirectMode`; do not store the shared secret in config or logs.

- [ ] **Step 3: Add direct actions**

Wire buttons:

```go
refreshButton := widget.NewButton("检测 Tailscale", func() {
	withTimeout(func(ctx context.Context) error {
		return a.directCtrl.Refresh(ctx)
	})
})

startButton := widget.NewButton("启用直连密钥", func() {
	withTimeout(func(ctx context.Context) error {
		state := a.directCtrl.State()
		if state.LocalTailscaleIP == "" {
			return errors.New("请先检测 Tailscale")
		}
		return a.directCtrl.StartDirectMode(ctx, secretEntry.Text, state.LocalTailscaleIP+":17890")
	})
})

pairButton := widget.NewButton("配对设备", func() {
	withTimeout(func(ctx context.Context) error {
		return a.directCtrl.PairPeer(ctx, peerEntry.Text)
	})
})

forwardButton := widget.NewButton("创建本地转发", func() {
	withTimeout(func(ctx context.Context) error {
		remotePort, err := strconv.Atoi(strings.TrimSpace(targetPortEntry.Text))
		if err != nil {
			return err
		}
		localPort := strings.TrimSpace(localPortEntry.Text)
		localAddress := "127.0.0.1:0"
		if localPort != "" {
			localAddress = "127.0.0.1:" + localPort
		}
		state := a.directCtrl.State()
		if len(state.Peers) == 0 {
			return errors.New("请先配对设备")
		}
		return a.directCtrl.CreateForward(ctx, state.Peers[0].ID, "127.0.0.1", remotePort, localAddress)
	})
})
```

Import `errors`, `strconv`, and `strings`.

- [ ] **Step 4: Render direct state**

In `render`, update labels:

```go
state := a.directCtrl.State()
if state.Ready {
	statusLabel.SetText("Tailscale：ready")
} else {
	statusLabel.SetText("Tailscale：未就绪")
}
ipLabel.SetText("本机 IP：" + valueOrDash(state.LocalTailscaleIP))
messageLabel.SetText(state.Message)
```

Show peers and forwards with `widget.NewList` or labels. The first MVP can use compact lists as long as it shows peer address, local forward address, target, and status.

- [ ] **Step 5: Update tray title**

Change tray menu title in `internal/ui/tray.go` to `portshare`.

- [ ] **Step 6: Run UI tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/ui
```

Expected: pass. Full application build happens after `cmd/portshare/main.go` is wired in Task 11.

- [ ] **Step 7: Commit**

```powershell
git add internal/ui
git commit -m "feat: show direct mode ui"
```

---

### Task 11: Wire Direct Mode In Entrypoint

**Files:**
- Modify: `cmd/portshare/main.go`
- Modify: `internal/direct/manager/manager.go`

- [ ] **Step 1: Add server lifecycle to manager**

Extend `internal/direct/manager/manager.go` with:

```go
func (m *Manager) StartControlServer(ctx context.Context, listenAddress string, secret string) error
func (m *Manager) StopControlServer(ctx context.Context) error
```

These methods create a `net.Listener` on the Tailscale IP and control port, then start `direct.Server`. Store the listener/server in the manager so shutdown can close it.

- [ ] **Step 2: Write lifecycle test**

Add to `internal/direct/manager/manager_test.go`:

```go
func TestStartControlServerRejectsEmptySecret(t *testing.T) {
	m := New(Config{})
	err := m.StartControlServer(context.Background(), "127.0.0.1:0", "")
	if err == nil {
		t.Fatalf("expected empty secret to be rejected")
	}
}
```

- [ ] **Step 3: Run lifecycle test to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager
```

Expected: fail because lifecycle methods do not exist.

- [ ] **Step 4: Implement lifecycle methods**

Implement:

```go
func (m *Manager) StartControlServer(ctx context.Context, listenAddress string, secret string) error {
	if strings.TrimSpace(secret) == "" {
		return errors.New("共享密钥不能为空")
	}
	ln, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return err
	}
	server := direct.NewServer(direct.ServerConfig{
		DeviceID:   m.deviceID,
		DeviceName: m.deviceName,
		Secret:     secret,
	})
	m.mu.Lock()
	m.listener = ln
	m.server = server
	m.mu.Unlock()
	go func() { _ = server.Serve(ln) }()
	return nil
}
```

Add required manager fields: `mu`, `listener`, `server`, `deviceID`, `deviceName`.

- [ ] **Step 5: Wire main**

Modify `cmd/portshare/main.go` to create:

```go
tsClient := tailscale.NewClient(nil)
directMgr := directmanager.New(directmanager.Config{
	Tailscale: tsClient,
})
ui.New(ui.Dependencies{
	Manager: mgr,
	Discovery: ui.DiscoveryFuncs{
		ScanCommonFunc: discovery.ScanCommon,
		ProbeFunc:      discovery.Probe,
	},
	DirectManager: directMgr,
}).Run()
```

Use import aliases to avoid collision between old `provider/tailscale` and new `internal/tailscale`:

```go
tailscalediag "github.com/absuq/portshare-desktop/internal/tailscale"
directmanager "github.com/absuq/portshare-desktop/internal/direct/manager"
```

- [ ] **Step 6: Run full tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
```

Expected: pass.

- [ ] **Step 7: Commit**

```powershell
git add cmd/portshare/main.go internal/direct/manager
git commit -m "feat: wire direct mode"
```

---

### Task 12: Update Documentation And Manual Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/manual-verification.md`
- Modify: `AGENTS.md`
- Modify: `docs/NEXT_SESSION.md`

- [ ] **Step 1: Update README**

Change README summary to:

```markdown
# portshare

`portshare` 是一个 Go + Fyne 桌面工具，用于让同一 Tailscale tailnet 内的两台电脑通过共享密钥配对，并按需创建本地 TCP 转发入口。
```

Include run commands and note that direct mode uses `<tailscale-ip>:17890` internally.

- [ ] **Step 2: Update manual verification**

Replace `docs/manual-verification.md` with direct-mode steps:

```markdown
# 手动验收

## 准备

1. 两台 Windows 电脑登录同一个 Tailscale tailnet。
2. 两台电脑都运行 `portshare`。
3. 在电脑 B 启动测试服务：`python -m http.server 3000 --bind 127.0.0.1`。

## 直连配对验收

1. 两台电脑都输入同一个共享密钥。
2. 电脑 A 输入电脑 B 的 Tailscale IP。
3. 点击“配对设备”。
4. 确认 UI 显示配对成功，并显示 direct 或 DERP 路径与延迟。

## TCP 转发验收

1. 在电脑 A 创建转发：远端端口 `3000`，本地端口 `18080`。
2. 在电脑 A 访问 `http://127.0.0.1:18080/`。
3. 确认返回电脑 B 的测试页面。
4. 停止转发。
5. 再次访问 `http://127.0.0.1:18080/`，确认不可访问。

## DNS 诊断验收

1. 在一台电脑执行 `tailscale set --accept-dns=false`。
2. 在 `portshare` 中输入对方 MagicDNS 名称。
3. 确认 UI 提示 DNS 未接管，并给出 `tailscale set --accept-dns=true` 修复命令。
```

- [ ] **Step 3: Update AGENTS and NEXT_SESSION**

Record:

- New active branch: `codex/portshare-direct-mode`.
- New MVP direction: shared-secret pairing and TCP forwarding.
- Tailscale Serve/Funnel is legacy, not the main MVP path.
- Direct mode requires Tailscale diagnostics.

- [ ] **Step 4: Commit docs**

```powershell
git add README.md docs/manual-verification.md AGENTS.md docs/NEXT_SESSION.md
git commit -m "docs: update direct mode verification"
```

---

### Task 13: Final Verification

**Files:**
- Modify only files needed to fix verification failures.

- [ ] **Step 1: Run full test suite**

Run:

```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
```

Expected: all packages pass.

- [ ] **Step 2: Run vet**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
```

Expected: no output and exit code 0.

- [ ] **Step 3: Build desktop app**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' build -o .superpowers\tmp\portshare-direct.exe ./cmd/portshare
```

Expected: executable is built.

- [ ] **Step 4: Manual two-machine direct-mode validation**

Run the updated `docs/manual-verification.md` direct-mode steps on two Windows computers.

Expected:

- Both apps show Tailscale ready.
- Pairing succeeds only with matching shared secret.
- `tailscale ping` route information is shown.
- `127.0.0.1:<local-port>` on one computer reaches the other computer's TCP service.
- Stopping the forward makes the local entry unreachable.

- [ ] **Step 5: Check git status**

Run:

```powershell
git status --short --branch
```

Expected: clean except ignored `.superpowers` artifacts.

- [ ] **Step 6: Push branch**

Run:

```powershell
git push origin codex/portshare-direct-mode
```

Expected: branch updates on GitHub.

---

## Self-Review

- Spec coverage: tasks cover product renaming, Tailscale diagnostics, control listener, shared-secret HMAC pairing, trusted peer storage, arbitrary TCP forwarding, UI main-flow replacement, docs, tests, and manual verification.
- Scope control: UDP, public sharing, transparent routing, and per-port authorization remain out of scope.
- TDD coverage: each implementation task starts with a failing test and expected failure command before production code.
- Migration: legacy Tailscale Serve/Funnel provider remains available in code but is no longer the main direct-mode flow.
