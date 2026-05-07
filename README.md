# portshare

`portshare` 是一个 Go + Fyne 桌面工具，用于让同一 Tailscale tailnet 内的两台电脑通过共享密钥配对，并按需创建本地 TCP 转发入口。

当前 MVP 的主路径不是 Tailscale Serve/Funnel 端口发布，而是：

```text
本机浏览器或应用
  -> 127.0.0.1:<本地端口>
  -> 本机 portshare
  -> 对方 Tailscale IP:17890
  -> 对方 portshare
  -> 对方本机 127.0.0.1:<目标端口>
```

Tailscale 仍是必需的私有网络底座。`portshare` 会使用 Tailscale CLI 做本机状态检测、Tailscale IP 识别和后续诊断，但不会通过 `tailscale serve` 或 `tailscale funnel` 暴露业务端口。

## 当前状态

- 可见产品名已改为 `portshare`。
- 已实现 Tailscale ready 诊断、MagicDNS/路由提示基础能力。
- 已实现共享密钥 HMAC 握手协议，不保存明文密钥。
- 已实现可信设备 JSON 持久化。
- 已实现直连控制监听和本地 TCP 转发。
- 主窗口已切换为 direct-mode：检测 Tailscale、启用共享密钥、配对设备、创建/停止本地转发。
- 旧 Serve/Funnel provider 代码仍保留为 legacy，不是当前 MVP 主路径。

## 本机运行

Windows/Fyne 构建需要 CGO 和 GCC。当前仓库 worktree 中已准备便携 Go 和 w64devkit：

```powershell
cd D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' run .\cmd\portshare
```

验证命令：

```powershell
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' build -o .superpowers\tmp\portshare-direct.exe ./cmd/portshare
```

不要使用 `.superpowers/tools/w64devkit-2.7.0/` 作为当前 Go 1.26.2 的 CGO 编译器；它会生成当前 cgo 无法解析的对象格式。

## 文档

- 直连模式规格：`docs/superpowers/specs/2026-05-07-portshare-direct-mode-design.md`
- 直连模式计划：`docs/superpowers/plans/2026-05-07-portshare-direct-mode.md`
- 手动验收：`docs/manual-verification.md`
- 下一会话交接：`docs/NEXT_SESSION.md`
