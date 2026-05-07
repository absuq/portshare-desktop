# 下一会话交接说明

## 当前分支

- 工作分支：`codex/portshare-direct-mode`
- worktree：`D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp`
- 主要规格：`docs/superpowers/specs/2026-05-07-portshare-direct-mode-design.md`
- 主要计划：`docs/superpowers/plans/2026-05-07-portshare-direct-mode.md`

## 当前 MVP 方向

`portshare` 的主路径已经切换为 direct-mode：

- 两台电脑都运行 `portshare`。
- 两台电脑输入同一个共享密钥。
- 一方输入对方 Tailscale IP 或 MagicDNS 名称完成配对。
- 配对后创建本地 TCP 转发，例如 `127.0.0.1:18080 -> 对方 portshare -> 127.0.0.1:3000`。

Tailscale Serve/Funnel 代码仍保留为 legacy，但不是当前 MVP 主路径。

## 已完成

- 可见产品名统一为 `portshare`。
- 新增 `internal/tailscale` 诊断适配器。
- 新增 direct protocol、HMAC 共享密钥握手、可信设备存储。
- 新增 direct server/client、TCP forward 和 direct manager。
- direct manager 已支持控制监听启动/停止、配对、可信设备读取、创建/停止本地转发。
- 主窗口已切换为 direct-mode UI。
- `cmd/portshare/main.go` 已注入真实 direct manager。
- 手动验收文档已切换为双机 direct-mode 验收步骤。

## 本地验证命令

```powershell
cd D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' build -o .superpowers\tmp\portshare-direct.exe ./cmd/portshare
```

不要使用 `.superpowers/tools/w64devkit-2.7.0/` 作为当前 CGO 编译器。

## 下一步

1. 按 `docs/manual-verification.md` 做真实双机 direct-mode 验收。
2. 在 UI 中展示 `tailscale ping` 的 direct/DERP 路由与延迟。
3. 增加可信设备删除，并停止关联转发。
4. 增加更清晰的 Tailscale DNS/Shields Up 故障提示。
5. 做最终 `go test ./...`、`go vet ./...`、build 和 push。
