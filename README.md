# 端口发布器

一个 Go + Fyne 桌面工具，用于把本机 HTTP/HTTPS 服务发布到 Tailscale tailnet，必要时手动开启公网访问。

## 当前状态

当前分支 `codex/portshare-mvp` 已完成 MVP 基础模块和桌面外壳：

- Go + Fyne 应用可以编译并打开中文窗口。
- 已实现 domain、i18n、config、audit、provider 抽象、fake provider、Tailscale provider 雏形、discovery 和 manager。
- 已实现系统托盘菜单、关闭窗口隐藏到托盘，以及公网发布确认对话框。
- 已添加真实 Tailscale 手动验收步骤：`docs/manual-verification.md`。

当前应用还不能作为完整端口发布工具正式使用：

- 服务列表尚未接入自动发现结果。
- 手动添加、刷新发现、tailnet 发布、停止发布、状态刷新等 UI 工作流尚未接通。
- 公网确认后尚未调用 `manager.PublishPublic`。
- 托盘“暂停所有公网”和“停止全部发布”尚未接入 manager。

首版目标：

- 默认中文界面，支持切换英文。
- 默认只开放到 tailnet。
- 公网发布必须强确认，支持倒计时关闭和长期开放。
- 支持多个端口同时管理。
- 支持自动发现本地网页服务和手动添加。
- 支持系统托盘常驻。
- 保存历史记录，默认保留 1 年。
- 首版基于 Tailscale，架构保留 provider 抽象，方便后续接入自建中转。

## 本机运行

当前 Windows 本机使用 worktree 内的便携 Go 和 w64devkit 工具链运行：

```powershell
cd D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' run .\cmd\portshare
```

验证命令：

```powershell
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' vet ./...
```

注意：不要使用 `.superpowers/tools/w64devkit-2.7.0/` 作为当前 CGO 编译器；它会生成当前 Go cgo 无法解析的 `pe-bigobj-x86-64` 对象。

设计规格见：

- `docs/superpowers/specs/2026-04-29-portshare-design.md`

继续开发前请先阅读：

- `docs/NEXT_SESSION.md`
- `docs/superpowers/plans/2026-04-29-portshare-mvp.md`

手动验收步骤见：

- `docs/manual-verification.md`
