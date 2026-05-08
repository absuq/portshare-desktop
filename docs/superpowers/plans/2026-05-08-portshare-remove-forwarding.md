# portshare 移除本地转发实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从当前 MVP 中完整移除“本地业务端口转发”，只保留 Tailscale 检测、共享密钥、直连监听和设备配对。

**Architecture:** `portshare` 当前阶段不再代理业务 TCP 流量，也不承诺让端口通过应用打通。应用只验证两台设备在 Tailscale 上可达，并通过共享密钥建立设备级信任；后续“公网转发”单独设计。

**Tech Stack:** Go、Fyne、Tailscale CLI、现有 direct 握手协议。

---

### Task 1: 移除 UI 转发入口

**Files:**
- Modify: `internal/ui/direct_controller.go`
- Modify: `internal/ui/main_window.go`
- Modify: `internal/ui/direct_controller_test.go`

- [x] 删除 `DirectState.Forwards` 和 controller 中的 `CreateForward` / `StopForward`。
- [x] 删除主窗口右侧“本地转发”面板、远端端口、本地端口和相关按钮。
- [x] 调整 UI 测试，保留刷新、启用密钥、配对、错误说明和密钥生成测试。

### Task 2: 移除 manager 转发编排

**Files:**
- Modify: `internal/direct/manager/manager.go`
- Modify: `internal/direct/manager/manager_test.go`

- [x] 删除 manager config 中的 `DirectClient`、`ForwardFactory`、forward 状态和序号。
- [x] 删除 `ForwardRequest`、`RunningForward`、`CreateForward`、`StopForward`。
- [x] 删除只验证转发的测试和辅助类型，保留配对、存储、控制监听相关测试。

### Task 3: 移除 direct TCP 转发协议

**Files:**
- Modify: `internal/direct/client.go`
- Modify: `internal/direct/server.go`
- Modify: `internal/direct/protocol/messages.go`
- Modify: `internal/direct/server_client_test.go`
- Delete: `internal/direct/forward/forward.go`
- Delete: `internal/direct/forward/forward_test.go`

- [x] 删除 `OpenTCP` 客户端 API 和 server 对 `open_tcp` 的处理。
- [x] 删除 `open_tcp` 相关协议消息。
- [x] 删除 loopback 转发集成测试，保留配对和认证测试。
- [x] 删除 `internal/direct/forward` 包。

### Task 4: 更新文档和验证

**Files:**
- Modify: `docs/manual-verification.md`
- Modify: `docs/NEXT_SESSION.md`

- [x] 文档只描述双机 Tailscale 检测、共享密钥和配对验收。
- [x] 明确说明关闭 portshare 不会关闭 Tailscale 自身连通性。
- [x] 运行 `go test ./...`、`go vet ./...`。
- [x] 构建 `.superpowers/tmp/portshare-direct.exe`。

后续 Windows 桌面版应使用 `scripts/build-windows.ps1` 构建。该脚本会设置 GUI 子系统，避免双击启动时弹出终端窗口。
