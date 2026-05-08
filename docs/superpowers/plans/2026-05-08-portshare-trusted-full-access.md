# portshare Trusted Full Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 配对成功后，双方电脑自动为对方 Tailscale IP 授予本机 TCP/UDP 全端口入站访问权限。

**Architecture:** 新增 `internal/firewall` 作为系统授权适配层，使用 Windows `netsh advfirewall` 写入只针对可信设备 Tailscale IP 的入站允许规则。`internal/direct/manager` 在发起方配对成功和响应方认证成功两个路径调用授权器，并把授权状态保存到可信设备记录。

**Tech Stack:** Go、Windows netsh、Fyne UI、现有 direct handshake/store/manager。

---

### Task 1: 防火墙授权核心

**Files:**
- Create: `internal/firewall/firewall.go`
- Create: `internal/firewall/firewall_test.go`
- Create: `internal/firewall/runner_windows.go`
- Create: `internal/firewall/runner_default.go`

- [ ] 写失败测试：生成 TCP/UDP 两条规则，规则限制本机 Tailscale IP 和对方 Tailscale IP。
- [ ] 写失败测试：授权器先删除同名旧规则，再添加 TCP/UDP 新规则。
- [ ] 实现 `BuildTrustedPeerRules`、`Authorizer.AllowTrustedPeer`、Windows runner 和非 Windows空 runner。
- [ ] 运行 `go test ./internal/firewall`。

### Task 2: direct manager 接入授权

**Files:**
- Modify: `internal/direct/manager/manager.go`
- Modify: `internal/direct/manager/manager_test.go`
- Modify: `internal/direct/server.go`
- Modify: `internal/direct/server_client_test.go`
- Modify: `internal/direct/store/store.go`
- Modify: `internal/direct/store/store_test.go`

- [ ] 写失败测试：发起方 `PairPeer` 成功后调用授权器，保存 `AccessAuthorizedAt`。
- [ ] 写失败测试：响应方 server 认证成功后通过回调保存并授权发起方。
- [ ] 实现 `AccessAuthorizer` 接口和 `TrustedPeerAccess` 数据。
- [ ] 在 `direct.ServerConfig` 增加 `OnAuthenticated` 回调。
- [ ] 运行 `go test ./internal/direct ./internal/direct/manager ./internal/direct/store`。

### Task 3: UI 文案和主程序注入

**Files:**
- Modify: `cmd/portshare/main.go`
- Modify: `internal/ui/direct_controller.go`
- Modify: `internal/ui/direct_controller_test.go`
- Modify: `internal/ui/main_window.go`

- [ ] 写失败测试：配对成功消息包含“已授权全端口访问”。
- [ ] 将 `internal/firewall.NewAuthorizer(nil)` 注入 direct manager。
- [ ] UI 可信设备信息显示“已授权全端口”。
- [ ] 授权失败时提示“请以管理员身份运行 portshare 后重试”。

### Task 4: 验证和交付

**Files:**
- Modify: `docs/manual-verification.md`
- Modify: `docs/NEXT_SESSION.md`

- [ ] 更新手动验收，加入管理员权限、防火墙规则和端口监听边界说明。
- [ ] 运行 `go test ./...`。
- [ ] 运行 `go vet ./...`。
- [ ] 运行 `powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1`。
- [ ] 提交并推送当前分支。
