# 下一会话交接说明

## 当前状态

- 公开仓库：`https://github.com/absuq/portshare-desktop`
- 默认分支：`main`
- 当前实现分支：`codex/portshare-mvp`
- 远端分支：`origin/codex/portshare-mvp`
- PR 创建入口：`https://github.com/absuq/portshare-desktop/pull/new/codex/portshare-mvp`
- 当前阶段：MVP 基础模块和 Fyne 桌面外壳已实现，应用可以编译并打开窗口，但 UI 业务工作流尚未完整接通。
- 当前设计规格：`docs/superpowers/specs/2026-04-29-portshare-design.md`
- 当前实现计划：`docs/superpowers/plans/2026-04-29-portshare-mvp.md`
- 当前进度和约束：`AGENTS.md`
- 手动验收步骤：`docs/manual-verification.md`

## 已完成内容

- Go module 和入口：`go.mod`、`cmd/portshare/main.go`。
- 领域模型：`internal/domain`。
- 中文默认、英文可切换字符串目录：`internal/i18n`。
- 配置保存和默认配置：`internal/config`。
- JSONL 审计日志和保留期清理：`internal/audit`。
- provider 抽象接口：`internal/provider`。
- deterministic fake provider：`internal/provider/fake`。
- Tailscale CLI 适配器雏形：`internal/provider/tailscale`。
- 本地 HTTP/HTTPS 探测和常用端口扫描：`internal/discovery`。
- 分享管理器：`internal/manager`，包含 tailnet/public 发布编排、公网 TTL 到期关闭、停止和状态查询转发。
- Fyne 应用外壳：`internal/ui`。
- 中文主窗口、系统托盘菜单、关闭窗口隐藏到托盘。
- 公网强确认对话框：10 分钟、30 分钟、1 小时、长期开放。
- 入口已接入配置、审计、manager 和 Tailscale provider。
- 手动验收文档：`docs/manual-verification.md`。

## 当前限制

- 当前应用不是完整可用的端口发布工具，仍是可启动的 MVP 外壳。
- 服务列表尚未显示 `discovery.ScanCommon` 的结果。
- 手动添加服务、刷新发现、服务选择、tailnet 发布、停止发布和状态刷新尚未接入 UI。
- “开启公网”会显示确认框，但确认后尚未调用 `manager.PublishPublic`。
- 托盘菜单项已存在，但“暂停所有公网”和“停止全部发布”尚未接入 manager。
- Tailscale provider 的 `Status` 还没有解析真实 `tailscale serve status --json` 输出。
- 真实 Tailscale 两机验收尚未执行。

## 本机开发环境

本机 PowerShell 默认找不到系统 Go/GCC。当前 worktree 已在 `.superpowers/tools/` 下准备了便携工具：

- Go：`.superpowers/tools/go1.26.2/`
- GCC/MinGW：`.superpowers/tools/w64devkit-1.23.0/`

普通 Fyne 桌面构建需要启用 CGO，并把 GCC 加入当前 PowerShell 会话 PATH：

```powershell
cd D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' run .\cmd\portshare
```

已验证：

- 普通 `go test ./...` 通过。
- 普通 `go vet ./...` 通过。
- `go build -o .superpowers/tmp/portshare.exe ./cmd/portshare` 通过。
- 桌面程序启动成功，主窗口标题为 `端口发布器`。

不要使用 `.superpowers/tools/w64devkit-2.7.0/` 作为当前 Go 1.26.2 的 CGO 编译器；它会生成 `pe-bigobj-x86-64` 对象，当前 cgo 解析失败。

## 新会话推荐提示词

```text
请阅读 AGENTS.md、docs/NEXT_SESSION.md、docs/superpowers/specs/2026-04-29-portshare-design.md、docs/superpowers/plans/2026-04-29-portshare-mvp.md 和 docs/manual-verification.md。

当前分支 codex/portshare-mvp 已完成 MVP 基础模块和 Fyne 外壳，但 UI 业务工作流尚未接通。请继续下一阶段：把 discovery、manager 和 provider 能力接入 UI，让刷新发现、服务列表、tailnet 发布、公网发布、停止发布、托盘操作和状态刷新真实可用。继续保持 provider 抽象，不要让 UI 直接调用 Tailscale CLI。
```

## 下一步建议

1. 从 `codex/portshare-mvp` 分支继续，不要直接在 `main` 上开发。
2. 先接入服务发现：刷新按钮调用 `discovery.ScanCommon`，服务列表展示结果。
3. 实现手动添加 HTTP/HTTPS 服务。
4. 实现服务选择和操作状态。
5. 将 tailnet 发布接入 `manager.PublishTailnet`。
6. 将公网确认结果接入 `manager.PublishPublic`。
7. 实现停止单个发布、暂停所有公网、停止全部发布。
8. 完善 Tailscale provider 状态解析和真实 URL 展示。
9. 跑普通 `go test ./...`、`go vet ./...`。
10. 按 `docs/manual-verification.md` 做真实 Tailscale 手动验收。

## Git 说明

如果直连 GitHub 失败，使用本机代理端口 `7897`：

```powershell
git config --local http.proxy http://127.0.0.1:7897
git config --local https.proxy http://127.0.0.1:7897
```

如果还没有远端认证：

```powershell
gh auth status
gh auth setup-git
```
