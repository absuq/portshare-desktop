# portshare MVP Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修正当前 MVP 的文档与产品方向不一致问题，并补齐可信设备撤权、localhost 桥接控制、诊断提示、CI 与发布验收能力。

**Architecture:** 继续保持当前 direct-mode 主线：`internal/direct/manager` 负责编排可信设备、防火墙授权和 localhost bridge；`internal/ui/direct_controller.go` 负责状态与用户可见提示；`internal/firewall` 提供可测试的 Windows 防火墙 allow/revoke 原语。公网转发不纳入本计划，避免和当前“只优化两台 Tailscale 设备直连体验”的目标混在一起。

**Tech Stack:** Go 1.23+、Fyne v2、Windows `netsh advfirewall`、GitHub Actions Windows runner、现有 `scripts/build-windows.ps1`。

---

## File Structure

- Modify: `README.md`  
  修正文档中“本地 TCP 转发入口/创建转发”的旧描述，改为“配对、全端口授权、localhost-only TCP 自动桥接、链路诊断与优化”。

- Modify: `AGENTS.md`  
  修正项目交接说明，删除已不存在的 `internal/direct/forward` 和“停止关联转发”说法。

- Modify: `docs/NEXT_SESSION.md`  
  更新下一步列表，把本计划拆成明确的验收项，并记录 release `v0.1.0` 后的实际状态。

- Modify: `internal/firewall/firewall.go`  
  为可信设备增加撤权能力：删除 `BuildTrustedPeerRules` 生成的 TCP/UDP 防火墙规则。

- Modify: `internal/firewall/firewall_test.go`  
  测试撤权只执行 TCP/UDP 规则删除，不执行新增规则。

- Modify: `cmd/portshare/firewall_adapter.go`  
  把 direct manager 的撤权请求映射到 `internal/firewall.Authorizer`。

- Modify: `cmd/portshare/firewall_adapter_test.go`  
  覆盖 adapter 的 revoke 映射。

- Modify: `internal/direct/manager/manager.go`  
  增加 `RemoveTrustedPeer` 和 `SetLocalhostBridgeEnabled`。删除可信设备时同步撤销防火墙规则并刷新 localhost bridge 允许列表。

- Modify: `internal/direct/manager/manager_test.go`  
  覆盖删除可信设备、撤销规则、刷新 bridge、禁用/启用 localhost bridge。

- Modify: `internal/ui/direct_controller.go`  
  增加可信设备删除动作、localhost bridge 启停动作、配对错误分类提示。

- Modify: `internal/ui/direct_controller_test.go`  
  覆盖删除设备、bridge 启停、DNS/超时/密钥不匹配/拒绝连接提示。

- Modify: `internal/ui/main_window.go`  
  在可信设备区域增加“删除可信设备”按钮；在状态页或网络页增加“自动 localhost 桥接”开关。

- Modify: `internal/ui/tray.go`  
  如需从托盘退出时清理 bridge 状态，复用 `StopDirectMode` 现有路径；不要在托盘里增加复杂业务入口。

- Create: `.github/workflows/ci.yml`  
  增加 Windows CI：`go test ./...`、`go vet ./...`、`scripts/build-windows.ps1`。

- Create: `docs/release-checklist.md`  
  记录 release 前必须完成的本地验证、PR 检查、手动双机验收和资产 hash。

---

### Task 0: Branch Hygiene

**Files:**
- No code files

- [ ] **Step 1: Fetch latest remote state**

Run:

```powershell
cd D:\developsoftweare\portshare-desktop
git fetch origin --prune
git status --short --branch
git worktree list
```

Expected:

```text
origin/main exists and contains the merged PR #2.
No uncommitted changes in the target worktree.
```

- [ ] **Step 2: Create a fresh hardening branch from latest main**

Run:

```powershell
cd D:\developsoftweare\portshare-desktop
git switch main
git pull --ff-only origin main
git switch -c codex/portshare-hardening
```

Expected:

```text
branch 'codex/portshare-hardening' set up from the latest local main.
```

- [ ] **Step 3: Verify the baseline**

Run:

```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
```

Expected:

```text
All Go packages pass.
```

- [ ] **Step 4: Commit status checkpoint**

No commit is created in this task. Continue only if the baseline is clean.

Run:

```powershell
git status --short --branch
```

Expected:

```text
## codex/portshare-hardening
```

---

### Task 1: Documentation Direction Correction

**Files:**
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `docs/NEXT_SESSION.md`

- [ ] **Step 1: Write the failing documentation scan**

Run:

```powershell
rg -n "本地 TCP 转发入口|创建/停止本地转发|internal/direct/forward|停止关联转发|授权后可以创建本地 TCP 转发" README.md AGENTS.md docs/NEXT_SESSION.md
```

Expected before the fix:

```text
README.md and AGENTS.md contain stale forwarding wording.
```

- [ ] **Step 2: Update `README.md` product summary**

Replace the opening paragraph and current status list with this content:

```markdown
# portshare

`portshare` 是一个 Go + Fyne 桌面工具，用于让同一 Tailscale tailnet 内的两台 Windows 电脑通过共享密钥配对，建立可信设备关系，并尽量获得低延迟、类似局域网的互访体验。

当前 MVP 的主路径不是 Tailscale Serve/Funnel 端口发布，也不是业务端口转发，而是：

```text
电脑 A portshare
  -> 共享密钥握手
  -> 电脑 B portshare
  -> 双方写入可信设备
  -> Windows 防火墙仅允许对方 Tailscale IP 访问本机 Tailscale IP 的 TCP/UDP 全端口
  -> localhost-only TCP 服务按需自动桥接到本机 Tailscale IP 同端口
  -> Tailscale 链路诊断和链路守护器优化直连路径
```

Tailscale 仍是必需的私有网络底座。`portshare` 会使用 Tailscale CLI 做本机状态检测、Tailscale IP 识别、peer 延迟检测、直连 endpoint 诊断和临时 host route 优化，但不会通过 `tailscale serve` 或 `tailscale funnel` 暴露业务端口。

## 当前状态

- 可见产品名已改为 `portshare`。
- 已实现 Tailscale ready 诊断、MagicDNS/路由提示基础能力。
- 已实现共享密钥 HMAC 握手协议，不保存明文密钥。
- 已实现可信设备 JSON 持久化。
- 已实现 Windows 防火墙可信设备 TCP/UDP 全端口授权，规则限定为对方 Tailscale IP。
- 已实现自动 localhost-only TCP 桥接：可信设备可通过本机 Tailscale IP 同端口访问只监听 `127.0.0.1` 的 TCP 服务。
- 已实现 localhost 冲突提示：同端口已有 `0.0.0.0` 或本机 Tailscale IP 原生监听时不桥接。
- 已实现 Tailscale 网络路径检测、Clash/Mihomo/TUN 出口识别、IPv4 `/32` 与 IPv6 `/128` endpoint 精确绕过。
- 已实现链路守护器：复用主页延迟样本，支持 `tailscale debug restun/rebind` 重探，并在确认需要时应用临时 host route。
- 旧 Serve/Funnel provider 代码仍保留为 legacy，不是当前 MVP 主路径。
```

- [ ] **Step 3: Update `AGENTS.md` current direction**

Replace the “当前产品方向” and stale completed bullets with this wording:

```markdown
## 当前产品方向

`portshare` 的 MVP 已从“端口发布器”调整为“两台电脑之间的 Tailscale direct-mode 工具”：

- 两端都运行 `portshare`。
- 两端输入同一个共享密钥。
- 任意一方输入对方 Tailscale IP 或 MagicDNS 名称完成配对。
- 配对成功后，双方保存可信设备。
- `portshare` 为可信设备写入 Windows 防火墙规则，仅允许对方 Tailscale IP 访问本机 Tailscale IP 的 TCP/UDP 全端口。
- 对只监听 `127.0.0.1` 的 TCP 服务，`portshare` 自动创建本机 Tailscale IP 同端口桥接。
- 业务端口不通过 Tailscale Serve/Funnel 直接发布。
- `internal/provider/tailscale` 的 Serve/Funnel 代码保留为 legacy，不是当前主路径。

## 已完成

- 产品可见名统一为 `portshare`。
- 新增 `internal/tailscale`：Tailscale CLI runner、status 解析、ready 诊断、peer ping 解析。
- 新增 `internal/direct/protocol`：length-prefixed JSON frame 和 HMAC 共享密钥认证。
- 新增 `internal/direct/store`：可信设备 JSON 存储，不保存明文共享密钥。
- 新增 `internal/direct` server/client：配对和认证控制协议。
- 新增 `internal/direct/manager`：ready、控制监听、配对、可信设备、防火墙授权、localhost bridge、网络优化编排。
- 新增 `internal/localhostbridge`：扫描 loopback-only TCP 服务，并为可信设备桥接到本机 Tailscale IP 同端口。
- 新增 `internal/netdiag` 和 `internal/linkguardian`：Tailscale endpoint 路由诊断、临时绕过和链路守护。
- 主窗口已切换为 direct-mode UI。
- `cmd/portshare/main.go` 已注入真实 direct manager。
- 文档已更新为 direct-mode 规格、计划和手动验收。
```

- [ ] **Step 4: Update `docs/NEXT_SESSION.md` release status**

Add this near the top under “当前分支”:

```markdown
- 已发布版本：`v0.1.0`
- Release 地址：`https://github.com/absuq/portshare-desktop/releases/tag/v0.1.0`
- 当前主线已合并 PR：`#2 [codex] add tailscale link guardian`
```

Replace “下一步” with:

```markdown
## 下一步

1. 修正 README 和 AGENTS.md 中残留的旧“本地 TCP 转发”描述。
2. 增加可信设备删除，并同步删除对应 Windows 防火墙规则。
3. 增加 localhost bridge 暂停/恢复开关。
4. 增加更清晰的 Tailscale DNS、Shields Up、Windows 防火墙、对端未运行 portshare、密钥不匹配提示。
5. 增加 GitHub Actions CI：测试、vet、Windows 构建。
6. 按 `docs/manual-verification.md` 做 release 版真实双机配对、全端口访问、localhost 桥接和链路守护验收。
7. 单独设计下一阶段“公网转发”能力。
```

- [ ] **Step 5: Run documentation scan again**

Run:

```powershell
rg -n "本地 TCP 转发入口|创建/停止本地转发|internal/direct/forward|停止关联转发|授权后可以创建本地 TCP 转发" README.md AGENTS.md docs/NEXT_SESSION.md
```

Expected:

```text
No matches.
```

- [ ] **Step 6: Commit documentation correction**

Run:

```powershell
git add README.md AGENTS.md docs/NEXT_SESSION.md
git commit -m "docs: align mvp direction"
```

Expected:

```text
[codex/portshare-hardening <sha>] docs: align mvp direction
```

---

### Task 2: Firewall Revoke API

**Files:**
- Modify: `internal/firewall/firewall.go`
- Modify: `internal/firewall/firewall_test.go`
- Modify: `cmd/portshare/firewall_adapter.go`
- Modify: `cmd/portshare/firewall_adapter_test.go`

- [ ] **Step 1: Write failing firewall revoke test**

Append this test to `internal/firewall/firewall_test.go`:

```go
func TestAuthorizerRevokesTCPAndUDPRules(t *testing.T) {
	runner := &recordingRunner{}
	authorizer := NewAuthorizer(runner)

	err := authorizer.RevokeTrustedPeer(context.Background(), TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: "100.79.83.104",
		PeerTailscaleIP:  "100.109.251.97",
		PeerID:           "desktop-bgpql0r",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(runner.commands) != 2 {
		t.Fatalf("expected delete commands for TCP and UDP, got %+v", runner.commands)
	}
	for _, command := range runner.commands {
		if command.name != "netsh" {
			t.Fatalf("expected netsh command, got %+v", command)
		}
		if !containsArg(command.args, "delete") || !containsArgPrefix(command.args, "name=portshare") {
			t.Fatalf("expected delete rule command, got %+v", command)
		}
		if containsArg(command.args, "add") {
			t.Fatalf("revoke must not add firewall rules, got %+v", command)
		}
	}
}
```

- [ ] **Step 2: Run firewall test to verify failure**

Run:

```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/firewall -run TestAuthorizerRevokesTCPAndUDPRules
```

Expected:

```text
FAIL because Authorizer.RevokeTrustedPeer is undefined.
```

- [ ] **Step 3: Implement `RevokeTrustedPeer`**

Add this method to `internal/firewall/firewall.go` after `AllowTrustedPeer`:

```go
func (a *Authorizer) RevokeTrustedPeer(ctx context.Context, access TrustedPeerAccess) error {
	if a == nil {
		return errors.New("firewall authorizer is not configured")
	}
	rules, err := BuildTrustedPeerRules(access)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		output, err := a.runner.Run(ctx, "netsh", deleteRuleArgs(rule)...)
		if err != nil {
			return describeDeleteRuleError(rule, output, err)
		}
	}
	return nil
}

func describeDeleteRuleError(rule Rule, output []byte, err error) error {
	details := strings.TrimSpace(string(output))
	text := strings.ToLower(details + " " + err.Error())
	if strings.Contains(text, "elevat") || strings.Contains(text, "administrator") || strings.Contains(text, "access is denied") || strings.Contains(text, "拒绝访问") {
		return fmt.Errorf("删除 Windows 防火墙规则 %q 失败：请以管理员身份运行 portshare 后重试：%w", rule.Name, err)
	}
	if details == "" {
		return fmt.Errorf("删除 Windows 防火墙规则 %q 失败：%w", rule.Name, err)
	}
	return fmt.Errorf("删除 Windows 防火墙规则 %q 失败：%s：%w", rule.Name, details, err)
}
```

- [ ] **Step 4: Run firewall test to verify pass**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/firewall -run TestAuthorizerRevokesTCPAndUDPRules
```

Expected:

```text
ok github.com/absuq/portshare-desktop/internal/firewall
```

- [ ] **Step 5: Write failing adapter revoke test**

Update `cmd/portshare/firewall_adapter_test.go`:

```go
type fakeFirewallAuthorizer struct {
	access       firewall.TrustedPeerAccess
	revoked      firewall.TrustedPeerAccess
	revokeCalled bool
}

func (f *fakeFirewallAuthorizer) AllowTrustedPeer(_ context.Context, access firewall.TrustedPeerAccess) error {
	f.access = access
	return nil
}

func (f *fakeFirewallAuthorizer) RevokeTrustedPeer(_ context.Context, access firewall.TrustedPeerAccess) error {
	f.revoked = access
	f.revokeCalled = true
	return nil
}

func TestManagerFirewallAuthorizerMapsDirectRevoke(t *testing.T) {
	inner := &fakeFirewallAuthorizer{}
	adapter := managerFirewallAuthorizer{inner: inner}

	err := adapter.RevokeTrustedPeer(context.Background(), directmanager.TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: "100.79.83.104",
		PeerTailscaleIP:  "100.109.251.97",
		PeerID:           "device-b",
		PeerName:         "desktop-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !inner.revokeCalled {
		t.Fatal("expected revoke to be called")
	}
	if inner.revoked.LocalTailscaleIP != "100.79.83.104" ||
		inner.revoked.PeerTailscaleIP != "100.109.251.97" ||
		inner.revoked.PeerID != "device-b" ||
		inner.revoked.PeerName != "desktop-b" {
		t.Fatalf("unexpected mapped revoke: %+v", inner.revoked)
	}
}
```

- [ ] **Step 6: Run adapter test to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./cmd/portshare -run TestManagerFirewallAuthorizerMapsDirectRevoke
```

Expected:

```text
FAIL because managerFirewallAuthorizer.RevokeTrustedPeer is undefined or interface lacks revoke.
```

- [ ] **Step 7: Implement adapter revoke mapping**

Update `cmd/portshare/firewall_adapter.go`:

```go
type firewallAccessAuthorizer interface {
	AllowTrustedPeer(context.Context, firewall.TrustedPeerAccess) error
	RevokeTrustedPeer(context.Context, firewall.TrustedPeerAccess) error
}

func (a managerFirewallAuthorizer) RevokeTrustedPeer(ctx context.Context, access directmanager.TrustedPeerAccess) error {
	return a.inner.RevokeTrustedPeer(ctx, firewall.TrustedPeerAccess{
		RulePrefix:       access.RulePrefix,
		LocalTailscaleIP: access.LocalTailscaleIP,
		PeerTailscaleIP:  access.PeerTailscaleIP,
		PeerID:           access.PeerID,
		PeerName:         access.PeerName,
	})
}
```

- [ ] **Step 8: Run task tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/firewall ./cmd/portshare
```

Expected:

```text
ok github.com/absuq/portshare-desktop/internal/firewall
ok github.com/absuq/portshare-desktop/cmd/portshare
```

- [ ] **Step 9: Commit firewall revoke API**

Run:

```powershell
git add internal/firewall/firewall.go internal/firewall/firewall_test.go cmd/portshare/firewall_adapter.go cmd/portshare/firewall_adapter_test.go
git commit -m "feat: revoke trusted peer firewall rules"
```

Expected:

```text
[codex/portshare-hardening <sha>] feat: revoke trusted peer firewall rules
```

---

### Task 3: Trusted Peer Deletion and Revocation

**Files:**
- Modify: `internal/direct/manager/manager.go`
- Modify: `internal/direct/manager/manager_test.go`
- Modify: `internal/ui/direct_controller.go`
- Modify: `internal/ui/direct_controller_test.go`
- Modify: `internal/ui/main_window.go`

- [ ] **Step 1: Write failing manager test for peer removal**

Add this test to `internal/direct/manager/manager_test.go`:

```go
func TestRemoveTrustedPeerRevokesFirewallAndRefreshesBridge(t *testing.T) {
	store := &fakePeerStore{peers: []store.TrustedPeer{
		{
			ID:                 "device-b",
			DisplayName:        "desktop-b",
			TailscaleIP:        "100.109.251.97",
			AccessAuthorizedAt: time.Now().UTC(),
		},
	}}
	authorizer := &fakeAccessAuthorizer{}
	bridge := &fakeLocalhostBridge{}
	m := New(Config{
		Tailscale:        fakeTailscale{ready: true, ip: "100.79.83.104"},
		PeerStore:        store,
		AccessAuthorizer: authorizer,
		LocalhostBridge:  bridge,
	})

	if err := m.RemoveTrustedPeer(context.Background(), "device-b"); err != nil {
		t.Fatal(err)
	}
	if len(store.peers) != 0 {
		t.Fatalf("expected peer to be removed, got %+v", store.peers)
	}
	if authorizer.revoked.PeerTailscaleIP != "100.109.251.97" || authorizer.revoked.PeerID != "device-b" {
		t.Fatalf("expected firewall revoke for device-b, got %+v", authorizer.revoked)
	}
	if len(bridge.allowedPeerIPs) != 0 {
		t.Fatalf("expected localhost bridge allowed peers to be refreshed empty, got %+v", bridge.allowedPeerIPs)
	}
}
```

Update the existing manager test fake:

```go
type fakeAccessAuthorizer struct {
	access  TrustedPeerAccess
	revoked TrustedPeerAccess
	err     error
}

func (f *fakeAccessAuthorizer) RevokeTrustedPeer(_ context.Context, access TrustedPeerAccess) error {
	f.revoked = access
	return f.err
}
```

- [ ] **Step 2: Run manager test to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager -run TestRemoveTrustedPeerRevokesFirewallAndRefreshesBridge
```

Expected:

```text
FAIL because Manager.RemoveTrustedPeer is undefined or AccessAuthorizer lacks RevokeTrustedPeer.
```

- [ ] **Step 3: Extend manager interfaces and implement removal**

Update `internal/direct/manager/manager.go`:

```go
type AccessAuthorizer interface {
	AllowTrustedPeer(context.Context, TrustedPeerAccess) error
	RevokeTrustedPeer(context.Context, TrustedPeerAccess) error
}

func (m *Manager) RemoveTrustedPeer(ctx context.Context, peerID string) error {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return errors.New("trusted peer id is required")
	}
	if m.peerStore == nil {
		return errors.New("peer store is not configured")
	}

	m.peerMu.Lock()
	defer m.peerMu.Unlock()

	peers, err := m.peerStore.LoadPeers()
	if err != nil {
		return err
	}

	index := -1
	var removed store.TrustedPeer
	for i, peer := range peers {
		if peer.ID == peerID {
			index = i
			removed = peer
			break
		}
	}
	if index < 0 {
		return errors.New("trusted peer not found")
	}

	if m.accessAuthorizer != nil && strings.TrimSpace(removed.TailscaleIP) != "" {
		if err := m.accessAuthorizer.RevokeTrustedPeer(ctx, TrustedPeerAccess{
			RulePrefix:       "portshare",
			LocalTailscaleIP: m.localTailscaleIP(ctx),
			PeerTailscaleIP:  removed.TailscaleIP,
			PeerID:           removed.ID,
			PeerName:         removed.DisplayName,
		}); err != nil {
			return err
		}
	}

	peers = append(peers[:index], peers[index+1:]...)
	if err := m.peerStore.SavePeers(peers); err != nil {
		return err
	}
	return m.refreshLocalhostBridge(ctx)
}
```

- [ ] **Step 4: Run manager test to verify pass**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager -run TestRemoveTrustedPeerRevokesFirewallAndRefreshesBridge
```

Expected:

```text
ok github.com/absuq/portshare-desktop/internal/direct/manager
```

- [ ] **Step 5: Write failing controller removal test**

Update `internal/ui/direct_controller_test.go` fake manager with:

```go
removedPeerID string
removeErr     error

func (f *fakeDirectManager) RemoveTrustedPeer(_ context.Context, peerID string) error {
	f.removedPeerID = peerID
	if f.removeErr != nil {
		return f.removeErr
	}
	var remaining []directmanager.TrustedPeer
	for _, peer := range f.peers {
		if peer.ID != peerID {
			remaining = append(remaining, peer)
		}
	}
	f.peers = remaining
	return nil
}
```

Add this test:

```go
func TestDirectControllerRemoveTrustedPeerUpdatesState(t *testing.T) {
	mgr := &fakeDirectManager{peers: []directmanager.TrustedPeer{
		{ID: "device-b", DisplayName: "desktop-b", TailscaleIP: "100.109.251.97"},
	}}
	ctrl := NewDirectController(mgr)
	if err := ctrl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := ctrl.RemoveTrustedPeer(context.Background(), "device-b"); err != nil {
		t.Fatal(err)
	}

	state := ctrl.State()
	if mgr.removedPeerID != "device-b" {
		t.Fatalf("expected manager removal for device-b, got %q", mgr.removedPeerID)
	}
	if len(state.Peers) != 0 {
		t.Fatalf("expected peer list to be empty, got %+v", state.Peers)
	}
	if !strings.Contains(state.Message, "已删除可信设备") {
		t.Fatalf("expected delete success message, got %q", state.Message)
	}
}
```

- [ ] **Step 6: Run controller test to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/ui -run TestDirectControllerRemoveTrustedPeerUpdatesState
```

Expected:

```text
FAIL because DirectController.RemoveTrustedPeer or DirectManager.RemoveTrustedPeer is undefined.
```

- [ ] **Step 7: Implement controller removal**

Update `internal/ui/direct_controller.go`:

```go
type DirectManager interface {
	Ready(context.Context) directmanager.ReadyState
	StartControlServer(context.Context, string, string) error
	StopControlServer(context.Context) error
	ControlAddress() string
	LocalhostBridgePorts() []int
	LocalhostBridgeConflictPorts() []int
	NetworkPath(context.Context, string) (netdiag.PeerPathReport, error)
	ApplyNetworkBypass(context.Context, netdiag.BypassRequest) (netdiag.ActiveBypass, error)
	ClearNetworkBypass(context.Context) error
	ActiveNetworkBypass() (netdiag.ActiveBypass, bool)
	OptimizeLink(context.Context, string, linkguardian.Options) (linkguardian.Result, error)
	ProbePeerLatency(context.Context, string) (time.Duration, error)
	DetectClash(context.Context) (clash.DiscoveryReport, error)
	RefreshClashNodes(context.Context) (clash.DiscoveryReport, error)
	ApplyClashNode(context.Context, clash.ApplyRequest) (clash.ApplyResult, error)
	RestoreClashNode(context.Context) error
	PairPeer(context.Context, string) (directmanager.PairedPeer, error)
	TrustedPeers(context.Context) ([]directmanager.TrustedPeer, error)
	RemoveTrustedPeer(context.Context, string) error
}

func (c *DirectController) RemoveTrustedPeer(ctx context.Context, peerID string) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		err := errors.New("请选择要删除的可信设备")
		c.state.Message = err.Error()
		return err
	}
	if err := c.manager.RemoveTrustedPeer(ctx, peerID); err != nil {
		c.state.Message = "删除可信设备失败：" + err.Error()
		return err
	}
	c.state.Message = "已删除可信设备并撤销防火墙授权"
	if err := c.Refresh(ctx); err != nil {
		c.state.Message = "已删除可信设备，但状态刷新失败：" + err.Error()
		return nil
	}
	c.state.Message = "已删除可信设备并撤销防火墙授权"
	return nil
}
```

- [ ] **Step 8: Add UI delete button**

In `internal/ui/main_window.go`, add this button near the peer panel:

```go
deletePeerButton := widget.NewButton("删除可信设备", func() {
	peerID := selectedPeerID
	if peerID == "" {
		dialog.ShowInformation("删除可信设备", "请先选择一个可信设备。", w)
		return
	}
	dialog.ShowConfirm("删除可信设备", "将删除该可信设备，并撤销对应 Windows 防火墙授权。确认继续？", func(ok bool) {
		if !ok {
			return
		}
		withTimeout(func(ctx context.Context) error {
			return a.directCtrl.RemoveTrustedPeer(ctx, peerID)
		})
	}, w)
})
```

Change peer panel construction:

```go
peerPanel := container.NewBorder(
	container.NewVBox(widget.NewLabel("可信设备"), deletePeerButton),
	nil,
	nil,
	nil,
	peers,
)
```

- [ ] **Step 9: Run task tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager ./internal/ui ./cmd/portshare ./internal/firewall
```

Expected:

```text
All four packages pass.
```

- [ ] **Step 10: Commit trusted peer deletion**

Run:

```powershell
git add internal/direct/manager/manager.go internal/direct/manager/manager_test.go internal/ui/direct_controller.go internal/ui/direct_controller_test.go internal/ui/main_window.go
git commit -m "feat: delete trusted peers"
```

Expected:

```text
[codex/portshare-hardening <sha>] feat: delete trusted peers
```

---

### Task 4: Localhost Bridge Pause and Resume

**Files:**
- Modify: `internal/direct/manager/manager.go`
- Modify: `internal/direct/manager/manager_test.go`
- Modify: `internal/ui/direct_controller.go`
- Modify: `internal/ui/direct_controller_test.go`
- Modify: `internal/ui/main_window.go`

- [ ] **Step 1: Write failing manager bridge toggle test**

Add this test to `internal/direct/manager/manager_test.go`:

```go
func TestSetLocalhostBridgeEnabledClosesAndRestartsBridge(t *testing.T) {
	bridge := &fakeLocalhostBridge{}
	m := New(Config{
		Tailscale:       fakeTailscale{ready: true, ip: "100.79.83.104"},
		PeerStore:       &fakePeerStore{peers: []store.TrustedPeer{{ID: "device-b", TailscaleIP: "100.109.251.97"}}},
		LocalhostBridge: bridge,
	})

	if err := m.SetLocalhostBridgeEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !bridge.closed {
		t.Fatal("expected bridge to be closed when disabled")
	}
	if m.LocalhostBridgeEnabled() {
		t.Fatal("expected bridge setting to be disabled")
	}

	bridge.closed = false
	if err := m.SetLocalhostBridgeEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if !m.LocalhostBridgeEnabled() {
		t.Fatal("expected bridge setting to be enabled")
	}
	if len(bridge.allowedPeerIPs) != 1 || bridge.allowedPeerIPs[0] != "100.109.251.97" {
		t.Fatalf("expected bridge refresh with trusted peer, got %+v", bridge.allowedPeerIPs)
	}
}
```

- [ ] **Step 2: Run manager bridge test to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager -run TestSetLocalhostBridgeEnabledClosesAndRestartsBridge
```

Expected:

```text
FAIL because SetLocalhostBridgeEnabled and LocalhostBridgeEnabled are undefined.
```

- [ ] **Step 3: Implement bridge enabled state in manager**

Update `Manager` struct and `New`:

```go
type Manager struct {
	// existing fields
	bridgeEnabled bool
}

func New(config Config) *Manager {
	return &Manager{
		tailscale:          config.Tailscale,
		pairClient:         config.PairClient,
		peerStore:          config.PeerStore,
		accessAuthorizer:   config.AccessAuthorizer,
		localhostBridge:    config.LocalhostBridge,
		networkDiagnostics: config.NetworkDiagnostics,
		clashEgress:        config.ClashEgress,
		secretLabel:        config.SecretLabel,
		deviceID:           config.DeviceID,
		deviceName:         config.DeviceName,
		bridgeEnabled:      true,
	}
}
```

Add methods:

```go
func (m *Manager) LocalhostBridgeEnabled() bool {
	m.controlMu.Lock()
	defer m.controlMu.Unlock()
	return m.bridgeEnabled
}

func (m *Manager) SetLocalhostBridgeEnabled(ctx context.Context, enabled bool) error {
	m.controlMu.Lock()
	if m.bridgeEnabled == enabled {
		m.controlMu.Unlock()
		if enabled {
			return m.refreshLocalhostBridge(ctx)
		}
		return nil
	}
	m.bridgeEnabled = enabled
	cancel := m.bridgeCancel
	m.bridgeCancel = nil
	m.controlMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if !enabled {
		if m.localhostBridge != nil {
			return m.localhostBridge.Close()
		}
		return nil
	}

	if m.ControlAddress() != "" {
		m.startLocalhostBridgePolling(ctx, m.localTailscaleIP(ctx))
		return nil
	}
	return m.refreshLocalhostBridge(ctx)
}
```

Update `StartControlServer` before calling `startLocalhostBridgePolling`:

```go
if m.LocalhostBridgeEnabled() {
	m.startLocalhostBridgePolling(ctx, hostFromAddress(listener.Addr().String()))
}
```

- [ ] **Step 4: Run manager bridge test to verify pass**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager -run TestSetLocalhostBridgeEnabledClosesAndRestartsBridge
```

Expected:

```text
ok github.com/absuq/portshare-desktop/internal/direct/manager
```

- [ ] **Step 5: Write failing controller bridge toggle test**

Update `internal/ui/direct_controller_test.go` fake manager:

```go
bridgeEnabled bool

func (f *fakeDirectManager) LocalhostBridgeEnabled() bool {
	return f.bridgeEnabled
}

func (f *fakeDirectManager) SetLocalhostBridgeEnabled(_ context.Context, enabled bool) error {
	f.bridgeEnabled = enabled
	return nil
}
```

Add to `DirectState` expectation test:

```go
func TestDirectControllerSetLocalhostBridgeEnabledUpdatesState(t *testing.T) {
	mgr := &fakeDirectManager{bridgeEnabled: true}
	ctrl := NewDirectController(mgr)

	if err := ctrl.SetLocalhostBridgeEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if ctrl.State().LocalhostBridgeEnabled {
		t.Fatal("expected bridge disabled in state")
	}
	if !strings.Contains(ctrl.State().Message, "已暂停 localhost 桥接") {
		t.Fatalf("unexpected message: %q", ctrl.State().Message)
	}

	if err := ctrl.SetLocalhostBridgeEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if !ctrl.State().LocalhostBridgeEnabled {
		t.Fatal("expected bridge enabled in state")
	}
}
```

- [ ] **Step 6: Run controller bridge test to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/ui -run TestDirectControllerSetLocalhostBridgeEnabledUpdatesState
```

Expected:

```text
FAIL because DirectState.LocalhostBridgeEnabled or controller method is undefined.
```

- [ ] **Step 7: Implement controller bridge toggle**

Update `internal/ui/direct_controller.go`:

```go
type DirectManager interface {
	// existing methods
	LocalhostBridgeEnabled() bool
	SetLocalhostBridgeEnabled(context.Context, bool) error
}

type DirectState struct {
	// existing fields
	LocalhostBridgeEnabled bool
}

func (c *DirectController) Refresh(ctx context.Context) error {
	// existing code
	c.state.LocalhostBridgeEnabled = c.manager.LocalhostBridgeEnabled()
	// existing code
}

func (c *DirectController) SetLocalhostBridgeEnabled(ctx context.Context, enabled bool) error {
	if err := c.requireManager(); err != nil {
		return err
	}
	if err := c.manager.SetLocalhostBridgeEnabled(ctx, enabled); err != nil {
		c.state.Message = "切换 localhost 桥接失败：" + err.Error()
		return err
	}
	c.state.LocalhostBridgeEnabled = enabled
	if enabled {
		c.state.Message = "已启用 localhost 桥接"
	} else {
		c.state.Message = "已暂停 localhost 桥接"
		c.state.LocalhostBridgePorts = nil
		c.state.LocalhostBridgeConflictPorts = nil
	}
	return nil
}
```

- [ ] **Step 8: Add UI bridge switch**

In `internal/ui/main_window.go`, after label declarations:

```go
bridgeEnabledCheck := widget.NewCheck("自动 localhost 桥接", func(enabled bool) {
	withTimeout(func(ctx context.Context) error {
		return a.directCtrl.SetLocalhostBridgeEnabled(ctx, enabled)
	})
})
```

In `render`, after state is loaded:

```go
bridgeEnabledCheck.SetChecked(state.LocalhostBridgeEnabled)
if state.ControlListening {
	bridgeEnabledCheck.Enable()
} else {
	bridgeEnabledCheck.Disable()
}
```

Add it to `statusPage`:

```go
statusPage := scrollPage(container.NewVBox(
	widget.NewLabel("运行状态"),
	statusLabel,
	ipLabel,
	controlLabel,
	bridgeEnabledCheck,
	bridgeLabel,
	bridgeConflictLabel,
))
```

- [ ] **Step 9: Run task tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/direct/manager ./internal/ui
```

Expected:

```text
Both packages pass.
```

- [ ] **Step 10: Commit localhost bridge controls**

Run:

```powershell
git add internal/direct/manager/manager.go internal/direct/manager/manager_test.go internal/ui/direct_controller.go internal/ui/direct_controller_test.go internal/ui/main_window.go
git commit -m "feat: control localhost bridge"
```

Expected:

```text
[codex/portshare-hardening <sha>] feat: control localhost bridge
```

---

### Task 5: Pairing Diagnostics and User-Facing Error Hints

**Files:**
- Modify: `internal/ui/direct_controller.go`
- Modify: `internal/ui/direct_controller_test.go`
- Modify: `docs/manual-verification.md`

- [ ] **Step 1: Write failing diagnostics tests**

Add these tests to `internal/ui/direct_controller_test.go`:

```go
func TestDescribePairErrorExplainsDNSFailure(t *testing.T) {
	err := describePairError("abs-u-q.tail51fe78.ts.net:17890", errors.New("lookup abs-u-q.tail51fe78.ts.net: no such host"))
	if err == nil || !strings.Contains(err.Error(), "MagicDNS") || !strings.Contains(err.Error(), "tailscale set --accept-dns=true") {
		t.Fatalf("expected MagicDNS hint, got %v", err)
	}
}

func TestDescribePairErrorExplainsTimeout(t *testing.T) {
	err := describePairError("100.109.251.97:17890", errors.New("dial tcp 100.109.251.97:17890: i/o timeout"))
	if err == nil || !strings.Contains(err.Error(), "Shields Up") || !strings.Contains(err.Error(), "Windows 防火墙") {
		t.Fatalf("expected timeout firewall hint, got %v", err)
	}
}

func TestDescribePairErrorExplainsSharedSecretMismatch(t *testing.T) {
	err := describePairError("100.109.251.97:17890", errors.New("authentication failed"))
	if err == nil || !strings.Contains(err.Error(), "共享密钥不一致") {
		t.Fatalf("expected shared secret hint, got %v", err)
	}
}
```

- [ ] **Step 2: Run diagnostics tests to verify failure**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/ui -run 'TestDescribePairErrorExplains'
```

Expected:

```text
FAIL because current describePairError only explains connection refused.
```

- [ ] **Step 3: Implement pair error classifier**

Replace `describePairError` in `internal/ui/direct_controller.go` with:

```go
func describePairError(address string, err error) error {
	if err == nil {
		return nil
	}
	raw := err.Error()
	message := strings.ToLower(raw)
	switch {
	case strings.Contains(message, "no such host") || strings.Contains(message, "dns"):
		return fmt.Errorf("无法解析对方地址 %s。请确认 MagicDNS 已启用，或直接输入对方 Tailscale IP。可在 PowerShell 检查：Resolve-DnsName <peer>.ts.net -Server 100.100.100.100；如默认 DNS 不生效，执行 tailscale set --accept-dns=true。原始错误：%w", address, err)
	case strings.Contains(message, "actively refused") || strings.Contains(message, "connection refused"):
		return fmt.Errorf("对方 %s 没有接受 portshare 直连连接。请确认对方电脑也运行新版 portshare，输入同一个直连密钥，并点击“启用直连密钥”；如果已经启用，请检查 Tailscale Shields Up 或 Windows 防火墙是否拦截 17890。原始错误：%w", address, err)
	case strings.Contains(message, "i/o timeout") || strings.Contains(message, "timed out") || strings.Contains(message, "timeout"):
		return fmt.Errorf("连接对方 %s 超时。请确认两端 Tailscale 可互通，Tailscale Shields Up 未阻止入站，Windows 防火墙允许 portshare 控制端口 17890，并用 Test-NetConnection <peer-ip> -Port 17890 验证。原始错误：%w", address, err)
	case strings.Contains(message, "authentication failed") || strings.Contains(message, "auth failed") || strings.Contains(message, "hmac"):
		return fmt.Errorf("共享密钥不一致，配对认证失败。请确认两台电脑输入完全相同的直连密钥，并重新点击“启用直连密钥”后再配对。原始错误：%w", err)
	default:
		return err
	}
}
```

- [ ] **Step 4: Run diagnostics tests to verify pass**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/ui -run 'TestDescribePairErrorExplains'
```

Expected:

```text
ok github.com/absuq/portshare-desktop/internal/ui
```

- [ ] **Step 5: Update manual verification troubleshooting**

Add this section to `docs/manual-verification.md` after “Tailscale 诊断”:

```markdown
## 配对失败排查

- 如果提示 MagicDNS 解析失败，优先直接输入对方 Tailscale IP。需要排查 DNS 时执行：

  ```powershell
  Resolve-DnsName <peer>.ts.net -Server 100.100.100.100
  tailscale set --accept-dns=true
  ```

- 如果提示连接被拒绝，检查对方是否已经启动新版 `portshare`，输入同一个共享密钥，并点击“启用直连密钥”。
- 如果提示连接超时，检查 Tailscale Shields Up、Windows 防火墙、Tailscale ACL，以及：

  ```powershell
  Test-NetConnection <peer-tailscale-ip> -Port 17890
  ```

- 如果提示共享密钥不一致，两端重新输入同一个密钥，并都重新点击“启用直连密钥”。
```

- [ ] **Step 6: Run task tests**

Run:

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./internal/ui
```

Expected:

```text
ok github.com/absuq/portshare-desktop/internal/ui
```

- [ ] **Step 7: Commit diagnostics**

Run:

```powershell
git add internal/ui/direct_controller.go internal/ui/direct_controller_test.go docs/manual-verification.md
git commit -m "fix: explain pairing failures"
```

Expected:

```text
[codex/portshare-hardening <sha>] fix: explain pairing failures
```

---

### Task 6: CI and Release Checklist

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `docs/release-checklist.md`

- [ ] **Step 1: Create Windows CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  pull_request:
  push:
    branches:
      - main

jobs:
  windows:
    runs-on: windows-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23.x'
          cache: true

      - name: Setup MinGW
        uses: egor-tensin/setup-mingw@v2
        with:
          platform: x64

      - name: Test
        shell: powershell
        run: |
          $env:CGO_ENABLED = '1'
          go test ./...

      - name: Vet
        shell: powershell
        run: |
          $env:CGO_ENABLED = '1'
          go vet ./...

      - name: Build Windows artifact
        shell: powershell
        run: |
          $env:CGO_ENABLED = '1'
          powershell.exe -NoProfile -ExecutionPolicy Bypass -File '.\scripts\build-windows.ps1'

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: portshare-direct
          path: .superpowers/tmp/portshare-direct.exe
```

- [ ] **Step 2: Create release checklist**

Create `docs/release-checklist.md`:

```markdown
# portshare Release Checklist

## Local Verification

Run from the release branch:

```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File '.\scripts\build-windows.ps1'
Get-FileHash '.\.superpowers\tmp\portshare-direct.exe' -Algorithm SHA256
```

## GitHub Verification

- PR is open against `main`.
- CI check is green.
- PR description includes validation commands.
- Release notes include the exe SHA256.

## Manual Two-Machine Verification

- Both computers run the same release exe.
- Both computers are logged into the same Tailscale tailnet.
- Both computers grant UAC administrator permission at startup.
- Pairing succeeds with the same shared secret.
- Trusted peer appears in both applications.
- Deleting a trusted peer removes it from the UI and revokes the Windows firewall rules.
- A service bound to `0.0.0.0` is reachable through the remote Tailscale IP after authorization.
- A TCP service bound only to `127.0.0.1` is reachable after localhost bridge appears.
- Pausing localhost bridge stops loopback-only service access through the Tailscale IP.
- Closing `portshare` stops the `17890` control listener.
- `tailscale ping <peer-ip>` shows whether the path is `direct`, `DERP`, or `peer-relay`.
- Link guardian optimization does not apply a route when current direct latency is already low.

## Release

```powershell
gh release create <tag> '.superpowers\tmp\portshare-direct.exe#portshare-direct.exe' --target main --title 'portshare <tag>' --notes-file <release-notes-file>
```
```

- [ ] **Step 3: Validate workflow YAML parses as text**

Run:

```powershell
rg -n "go test ./...|go vet ./...|build-windows.ps1|upload-artifact" .github/workflows/ci.yml docs/release-checklist.md
```

Expected:

```text
Both files contain the expected verification and artifact commands.
```

- [ ] **Step 4: Commit CI and checklist**

Run:

```powershell
git add .github/workflows/ci.yml docs/release-checklist.md
git commit -m "ci: add windows validation"
```

Expected:

```text
[codex/portshare-hardening <sha>] ci: add windows validation
```

---

### Task 7: Final Verification and PR

**Files:**
- No additional source files

- [ ] **Step 1: Run full verification**

Run:

```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File '.\scripts\build-windows.ps1'
Get-FileHash '.\.superpowers\tmp\portshare-direct.exe' -Algorithm SHA256
```

Expected:

```text
test exits 0
vet exits 0
build-windows.ps1 exits 0
SHA256 is printed for portshare-direct.exe
```

- [ ] **Step 2: Check final diff**

Run:

```powershell
git status --short --branch
git log --oneline --decorate -8
```

Expected:

```text
Working tree clean.
Branch contains the hardening commits.
```

- [ ] **Step 3: Push branch**

Run:

```powershell
git push -u origin codex/portshare-hardening
```

Expected:

```text
Branch is pushed to origin.
```

- [ ] **Step 4: Open PR**

Run:

```powershell
gh pr create --base main --head codex/portshare-hardening --title "[codex] harden portshare mvp" --body @"
## Summary

- Align README/AGENTS/NEXT_SESSION with the current direct-mode MVP.
- Add trusted peer deletion with Windows firewall rule revocation.
- Add localhost bridge pause/resume controls.
- Improve pairing failure diagnostics for DNS, timeout, connection refused, and shared-secret mismatch.
- Add Windows CI and release checklist.

## Validation

- go test ./...
- go vet ./...
- scripts/build-windows.ps1

## Manual Follow-up

- Run docs/release-checklist.md on two Windows machines before the next release.
"@
```

Expected:

```text
GitHub prints a PR URL.
```

- [ ] **Step 5: Watch CI**

Run:

```powershell
gh pr checks --watch
```

Expected:

```text
CI check passes.
```

---

## Self-Review

- Spec coverage:
  - 文档纠偏：Task 1。
  - 可信设备删除和防火墙撤权：Task 2 和 Task 3。
  - localhost bridge 删除/暂停策略：Task 4。
  - DNS、Shields Up、Windows 防火墙、对端未运行 portshare、密钥不匹配提示：Task 5。
  - CI、发布验收、release 版双机验证入口：Task 6 和 Task 7。
  - 公网转发：明确不纳入本计划，需要单独 spec。

- Placeholder scan:
  - 本计划没有保留空泛占位、笼统错误处理要求、或未落到文件和命令的实现步骤。

- Type consistency:
  - `RevokeTrustedPeer` 从 `internal/firewall.Authorizer` 映射到 `cmd/portshare.managerFirewallAuthorizer`，再进入 `internal/direct/manager.AccessAuthorizer`。
  - `RemoveTrustedPeer` 从 `DirectController` 进入 `DirectManager`，再进入 `internal/direct/manager.Manager`。
  - `SetLocalhostBridgeEnabled` 和 `LocalhostBridgeEnabled` 同时加入 manager 与 UI interface，`DirectState.LocalhostBridgeEnabled` 供 UI 渲染使用。
