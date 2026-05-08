# 下一会话交接说明

## 当前分支

- 工作分支：`codex/portshare-direct-mode`
- worktree：`D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp`
- 当前规格：`docs/superpowers/specs/2026-05-07-portshare-direct-mode-design.md`
- 当前全端口访问规格：`docs/superpowers/specs/2026-05-08-portshare-trusted-full-access-design.md`
- 当前移除计划：`docs/superpowers/plans/2026-05-08-portshare-remove-forwarding.md`
- 当前全端口访问计划：`docs/superpowers/plans/2026-05-08-portshare-trusted-full-access.md`

## 当前 MVP 方向

`portshare` 当前阶段只做双机 Tailscale 直连配对：

- 两台电脑都运行 `portshare`。
- 两台电脑输入同一个共享密钥。
- 两台电脑启用直连密钥，监听各自的 Tailscale IP `:17890`。
- 任意一方输入对方 Tailscale IP 或 MagicDNS 名称完成配对。
- 配对结果写入可信设备列表。

当前 MVP 不做本地业务端口转发，也不代理任意 TCP 业务流量。关闭 `portshare` 只会停止 `17890` 控制监听，不会关闭 Tailscale 自身的 tailnet 连通性。

配对成功后，`portshare` 会为对方 Tailscale IP 写入 Windows 防火墙入站允许规则，授权 TCP/UDP 全端口访问本机 Tailscale IP。该能力需要以管理员身份运行 `portshare`。没有服务监听、服务只绑定 `127.0.0.1`、或 Tailscale ACL 阻止互访时，端口仍然不会连通。

## 已完成

- 可见产品名统一为 `portshare`。
- 新增 `internal/tailscale` 诊断适配器。
- 新增 direct protocol、HMAC 共享密钥握手、可信设备存储。
- 新增 direct server/client 和 direct manager。
- 主窗口已切换为 direct-mode UI。
- `cmd/portshare/main.go` 已注入真实 direct manager。
- 已移除本地业务端口转发 UI、manager 编排、协议消息和 forward 包。
- 新增 Windows 防火墙可信设备全端口授权。
- 发起方配对成功后授权对方 Tailscale IP 访问本机。
- 响应方认证成功后保存并授权发起方 Tailscale IP。
- 手动验收文档已切换为双机配对验收步骤。

## 本地验证命令

```powershell
cd D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
.\scripts\build-windows.ps1
```

不要使用 `.superpowers/tools/w64devkit-2.7.0/` 作为当前 CGO 编译器。
Windows 桌面版必须通过 `scripts\build-windows.ps1` 构建；脚本会加 `-ldflags='-H windowsgui'`，避免双击运行时弹出终端窗口。

## 下一步

1. 按 `docs/manual-verification.md` 做真实双机配对和全端口访问验收。
2. 在 UI 中展示 `tailscale ping` 的 direct/DERP 路由与延迟。
3. 增加可信设备删除，并同步删除对应 Windows 防火墙规则。
4. 增加更清晰的 Tailscale DNS/Shields Up/Windows 防火墙故障提示。
5. 单独设计下一阶段“公网转发”能力。
