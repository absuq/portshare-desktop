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
