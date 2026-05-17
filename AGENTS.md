# AGENTS.md

## 项目约定

- 默认使用中文回复。
- 设计文档、计划和需要审阅的内容默认使用中文。
- 当前主工作分支是 `codex/portshare-hardening`。
- 当前 worktree 是 `D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp`。

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

- 可见产品名统一为 `portshare`。
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
- 已实现可信设备删除，并同步撤销对应 Windows 防火墙 TCP/UDP 规则。
- 已实现撤权离线容错：删除防火墙规则不再依赖当前本机 Tailscale IP。
- 已实现 localhost bridge 暂停和恢复开关。
- 已补充配对失败提示：MagicDNS、Shields Up、Windows 防火墙、对端未运行、共享密钥不匹配等场景。
- 已补充 GitHub CI 和 release checklist。

## 当前验证

使用当前 worktree 内置工具链：

```powershell
cd D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + (Join-Path (Get-Location) '.superpowers\tools\go1.26.2\go\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
go test ./...
go vet ./...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File '.\scripts\build-windows.ps1'
```

不要使用 `.superpowers/tools/w64devkit-2.7.0/` 作为当前 Go 1.26.2 的 CGO 编译器。Windows 桌面版必须通过 `scripts\build-windows.ps1` 构建；脚本会自动优先使用便携 Go 和 w64devkit 工具链。

## 下一步

1. 按 `docs/manual-verification.md` 做 release 版真实双机配对、全端口访问、localhost 桥接、可信设备删除撤权和链路守护验收。
2. 在 GitHub 上确认 CI 运行结果，并把验证命令和构建产物 SHA256 写入 PR/release 记录。
3. 单独设计下一阶段“公网转发”能力，明确 provider 抽象、安全确认、访问控制和退出策略。
