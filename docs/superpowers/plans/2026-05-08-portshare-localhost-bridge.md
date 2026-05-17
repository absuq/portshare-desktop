# portshare Localhost Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 配对后自动把本机仅监听 `127.0.0.1` 的 TCP 服务桥接到本机 Tailscale IP 同端口，并只允许可信设备访问。

**Architecture:** 新增 `internal/localhostbridge`，由 scanner/planner/bridge/controller 四个小单元组成。`internal/direct/manager` 在直连模式启用后启动 controller，并在可信设备变化后刷新允许访问的远端 IP。

**Tech Stack:** Go `net` TCP listener、Windows `Get-NetTCPConnection` 扫描、现有 direct manager/store/UI。

---

### Task 1: Planner

**Files:**
- Create: `internal/localhostbridge/planner.go`
- Create: `internal/localhostbridge/planner_test.go`

- [ ] 写失败测试：`127.0.0.1:18789` 且存在可信 IP 时计划桥接到 `<tailscale-ip>:18789`。
- [ ] 写失败测试：`0.0.0.0:52726` 不桥接。
- [ ] 写失败测试：`<tailscale-ip>:17890` 不桥接。
- [ ] 实现 `ListeningPort`、`Plan`、`BuildPlan`。
- [ ] 运行 `go test ./internal/localhostbridge`。

### Task 2: TCP Bridge

**Files:**
- Create: `internal/localhostbridge/bridge.go`
- Create: `internal/localhostbridge/bridge_test.go`

- [ ] 写失败测试：loopback echo server 可以通过 bridge 访问。
- [ ] 写失败测试：不在可信 IP 列表中的远端连接被关闭。
- [ ] 实现 `Bridge.Start`、`Bridge.Close`、双向 `io.Copy`。
- [ ] 运行 `go test ./internal/localhostbridge`。

### Task 3: Scanner 和 Controller

**Files:**
- Create: `internal/localhostbridge/scanner.go`
- Create: `internal/localhostbridge/scanner_windows.go`
- Create: `internal/localhostbridge/scanner_default.go`
- Create: `internal/localhostbridge/controller.go`
- Create: `internal/localhostbridge/controller_test.go`

- [ ] 写失败测试：controller 根据扫描结果启动桥接。
- [ ] 写失败测试：端口消失时关闭桥接。
- [ ] 实现 Windows scanner 使用 `powershell Get-NetTCPConnection -State Listen | ConvertTo-Json`。
- [ ] 实现 controller 的 `Refresh`、`SetAllowedPeers`、`ActivePorts`、`Close`。
- [ ] 运行 `go test ./internal/localhostbridge`。

### Task 4: Manager 和 UI 接入

**Files:**
- Modify: `internal/direct/manager/manager.go`
- Modify: `internal/direct/manager/manager_test.go`
- Modify: `cmd/portshare/main.go`
- Modify: `internal/ui/direct_controller.go`
- Modify: `internal/ui/direct_controller_test.go`
- Modify: `internal/ui/main_window.go`

- [ ] 写失败测试：直连监听启动后 bridge controller 收到本机 Tailscale IP 和可信设备 IP。
- [ ] 写失败测试：配对成功后刷新 bridge 可信 IP。
- [ ] UI 状态新增 `localhost 桥接：...`。
- [ ] 主程序注入真实 `localhostbridge.Controller`。

### Task 5: 文档、验证、提交

**Files:**
- Modify: `docs/manual-verification.md`
- Modify: `docs/NEXT_SESSION.md`

- [ ] 更新手动验收：验证 `127.0.0.1:<port>` 经 Tailscale IP 同端口访问。
- [ ] 运行 `go test ./...`。
- [ ] 运行 `go vet ./...`。
- [ ] 运行 `powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1`。
- [ ] 提交并推送当前分支。
