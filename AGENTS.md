# AGENTS.md

## 项目约定

- 默认使用中文回复。
- 设计文档、计划和需要审阅的内容默认使用中文。
- 当前主工作分支是 `codex/portshare-direct-mode`。
- 当前 worktree 是 `D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp`。

## 当前产品方向

`portshare` 的 MVP 已从“端口发布器”调整为“两台电脑之间的 Tailscale direct-mode 工具”：

- 两端都运行 `portshare`。
- 两端输入同一个共享密钥。
- 任意一方输入对方 Tailscale IP 或 MagicDNS 名称完成配对。
- 授权后可以创建本地 TCP 转发到对方任意 TCP 端口。
- 业务端口不通过 Tailscale Serve/Funnel 直接发布。
- `internal/provider/tailscale` 的 Serve/Funnel 代码保留为 legacy，不是当前主路径。

## 已完成

- 产品可见名统一为 `portshare`。
- 新增 `internal/tailscale`：Tailscale CLI runner、status 解析、ready 诊断、peer ping 解析。
- 新增 `internal/direct/protocol`：length-prefixed JSON frame 和 HMAC 共享密钥认证。
- 新增 `internal/direct/store`：可信设备 JSON 存储，不保存明文共享密钥。
- 新增 `internal/direct` server/client：配对和 `open_tcp` 控制协议。
- 新增 `internal/direct/forward`：本地 TCP listener 到对端目标端口的双向转发。
- 新增 `internal/direct/manager`：ready、控制监听、配对、可信设备、创建/停止转发。
- 主窗口已切换为 direct-mode UI。
- `cmd/portshare/main.go` 已注入真实 direct manager。
- 文档已更新为 direct-mode 规格、计划和手动验收。

## 当前验证

使用当前 worktree 内置工具链：

```powershell
cd D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' build -o .superpowers\tmp\portshare-direct.exe ./cmd/portshare
```

不要使用 `.superpowers/tools/w64devkit-2.7.0/` 作为当前 Go 1.26.2 的 CGO 编译器。

## 下一步

- 按 `docs/manual-verification.md` 做两台 Windows 电脑的真实 Tailscale direct-mode 验收。
- 在 UI 中展示 `tailscale ping` 的 direct/DERP/peer-relay 路由与延迟。
- 增加可信设备删除，并停止关联转发。
- 完善 DNS、Shields Up、对端未运行 portshare、密钥不匹配等诊断提示。
- 做最终 `test`、`vet`、build、push。
