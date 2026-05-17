# 下一会话交接说明

## 当前分支

- 工作分支：`codex/portshare-hardening`
- worktree：`D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp`
- 当前规格：`docs/superpowers/specs/2026-05-07-portshare-direct-mode-design.md`
- 当前全端口访问规格：`docs/superpowers/specs/2026-05-08-portshare-trusted-full-access-design.md`
- 当前移除计划：`docs/superpowers/plans/2026-05-08-portshare-remove-forwarding.md`
- 当前全端口访问计划：`docs/superpowers/plans/2026-05-08-portshare-trusted-full-access.md`
- 已发布版本：`v0.1.0`
- Release 地址：`https://github.com/absuq/portshare-desktop/releases/tag/v0.1.0`
- 当前主线已合并 PR：`#2 [codex] add tailscale link guardian`

## 当前 MVP 方向

`portshare` 当前阶段只做双机 Tailscale 直连配对：

- 两台电脑都运行 `portshare`。
- 两台电脑输入同一个共享密钥。
- 两台电脑启用直连密钥，监听各自的 Tailscale IP `:17890`。
- 任意一方输入对方 Tailscale IP 或 MagicDNS 名称完成配对。
- 配对结果写入可信设备列表。
- 配对成功后，`portshare` 为对方 Tailscale IP 写入 Windows 防火墙入站允许规则，授权 TCP/UDP 全端口访问本机 Tailscale IP。
- 如果服务只监听 `127.0.0.1:<port>`，`portshare` 会自动创建 `<本机 Tailscale IP>:<port> -> 127.0.0.1:<port>` 的 localhost bridge，并只允许可信设备 IP 连接。
- 如果同一端口已有 `0.0.0.0:<port>` 或 `<本机 Tailscale IP>:<port>` 原生监听，`portshare` 不会桥接同端口的 `127.0.0.1:<port>`，UI 会显示冲突提示。

当前 MVP 不做手动业务端口转发，也不代理任意 TCP 业务流量。关闭 `portshare` 只会停止 `17890` 控制监听和 localhost bridge，不会关闭 Tailscale 自身的 tailnet 连通性。

## 已完成

- 可见产品名统一为 `portshare`。
- 新增 `internal/tailscale` 诊断适配器。
- 新增 direct protocol、HMAC 共享密钥握手、可信设备存储。
- 新增 direct server/client 和 direct manager。
- 主窗口已切换为 direct-mode UI。
- `cmd/portshare/main.go` 已注入真实 direct manager。
- 已移除本地业务端口转发 UI、manager 编排、协议消息和 forward 包。
- 新增 Windows 防火墙可信设备全端口授权。
- Windows exe 启动时请求管理员权限。
- 发起方配对成功后授权对方 Tailscale IP 访问本机。
- 响应方认证成功后保存并授权发起方 Tailscale IP。
- 新增自动 localhost TCP 桥接：loopback-only 服务可通过 Tailscale IP 同端口访问。
- 新增 localhost 冲突提示：同端口已有原生监听时提示未桥接。
- 新增可信设备删除，并同步撤销对应 Windows 防火墙 TCP/UDP 规则。
- 防火墙撤权已支持本机 Tailscale IP 不可用的离线场景。
- 新增 localhost bridge 暂停和恢复开关。
- 配对失败提示已覆盖 MagicDNS、Shields Up、Windows 防火墙、对端未运行 `portshare`、共享密钥不匹配等常见原因。
- 新增 GitHub CI 和 release checklist。
- 手动验收文档已切换为双机配对、全端口访问、localhost 桥接、删除撤权和链路守护验收步骤。

## 本地验证命令

```powershell
cd D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + (Join-Path (Get-Location) '.superpowers\tools\go1.26.2\go\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
go test ./...
go vet ./...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File '.\scripts\build-windows.ps1'
```

不要使用 `.superpowers/tools/w64devkit-2.7.0/` 作为当前 CGO 编译器。Windows 桌面版必须通过 `scripts\build-windows.ps1` 构建；脚本会加 `-ldflags='-H windowsgui'`，避免双击运行时弹出终端窗口。

## 下一步

1. 按 `docs/manual-verification.md` 做 release 版真实双机验收，覆盖配对、全端口访问、localhost bridge、可信设备删除撤权、bridge 暂停恢复和链路守护。
2. 在 GitHub 上确认 CI 状态，并把本地验证命令、CI 结果和 exe SHA256 同步到 PR/release 记录。
3. 单独设计下一阶段“公网转发”能力，明确 provider 抽象、安全确认、访问控制、倒计时关闭和长期开放策略。
