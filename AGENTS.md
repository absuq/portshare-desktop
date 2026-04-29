# AGENTS.md

## 已完成

- 确认工具定位：桌面端端口发布器，用于把本机 HTTP/HTTPS 服务开放给另一台电脑或公网。
- 确认首版语言和界面：Go + Fyne，默认中文，可切换英文。
- 确认首版发布后端：基于 Tailscale，但应用架构必须保留 provider 抽象。
- 确认首版访问范围：默认 tailnet 内访问，公网发布需要手动开启。
- 确认公网安全策略：强确认，支持倒计时关闭，也允许明确选择长期开放。
- 确认首版服务范围：只支持 HTTP/HTTPS 网页服务。
- 确认多端口能力：首版支持多个端口同时开放和管理。
- 确认服务添加方式：自动发现本地服务，同时允许手动添加。
- 确认系统托盘：窗口关闭后常驻托盘，托盘提供关键操作。
- 确认历史记录：保存开放、关闭、失败和到期关闭事件，默认保留 1 年。
- 确认需求文档：设计规格写入 `docs/superpowers/specs/2026-04-29-portshare-design.md`。
- 确认实现计划：MVP 实现计划写入 `docs/superpowers/plans/2026-04-29-portshare-mvp.md`。
- 确认会话交接：下一会话启动说明写入 `docs/NEXT_SESSION.md`。
- 已创建隔离实现分支：`codex/portshare-mvp`，并推送到 `origin/codex/portshare-mvp`。
- 已搭建 Go 项目结构：`go.mod`、`cmd/portshare/main.go`、`internal/domain`。
- 已实现中文默认、英文可切换的 `internal/i18n` 字符串目录。
- 已实现 JSON 配置存储：`internal/config`，默认保存到用户配置目录 `PortShare/config.json`。
- 已实现 JSONL 审计日志：`internal/audit`，支持追加、读取和保留期清理。
- 已实现 provider 抽象接口和 deterministic fake provider。
- 已实现分享管理器：tailnet/public 发布编排、公网 TTL 到期关闭、停止和状态查询转发。
- 已实现本地 HTTP/HTTPS 探测：`Probe` 和常用开发端口 `ScanCommon`。
- 已实现 Tailscale provider 适配器和命令 runner 抽象。
- 已实现 Fyne 桌面应用外壳、中文主窗口、系统托盘菜单和关闭窗口隐藏到托盘。
- 已实现公网发布确认对话框，支持 10 分钟、30 分钟、1 小时和长期开放选择。
- 已把配置、审计、manager 和 Tailscale provider 接入应用入口。
- 已添加手动验收文档：`docs/manual-verification.md`。
- 已完成本机构建环境修复：在 `.superpowers/tools/` 下安装便携 Go 和 w64devkit，普通 `go test ./...`、`go vet ./...`、`go build` 均已通过。
- 已实际启动桌面程序并检测到主窗口标题：`端口发布器`。

## 当前限制

- 当前应用可以编译并打开窗口，但还不能作为端口发布工具正式使用。
- UI 仍是外壳：服务列表没有接入自动发现结果，手动添加、刷新、tailnet 发布、停止发布、状态刷新等按钮还没有实际工作流。
- “开启公网”按钮会弹确认框，但确认后尚未调用 manager/provider。
- 托盘菜单项已存在，但“暂停所有公网”和“停止全部发布”尚未接入 manager。
- Tailscale provider 当前是 CLI 命令适配雏形，`Status` 仍未解析真实 `tailscale serve status --json` 输出。
- 真实 Tailscale 两机验收尚未执行。

## 计划完成

- 继续在 `codex/portshare-mvp` 分支上开发，不要直接在 `main` 上实现功能。
- 将 `internal/discovery.ScanCommon` 接入 UI 刷新按钮和服务列表。
- 实现手动添加 HTTP/HTTPS 服务，并校验 scheme、host、port。
- 实现服务选择状态和操作面板状态：未选择、已发布、错误、到期倒计时。
- 将“开放到 tailnet”接入 `manager.PublishTailnet`。
- 将“开启公网”确认结果接入 `manager.PublishPublic`，并显示 TTL/长期开放状态。
- 实现停止单个发布、暂停所有公网、停止全部发布。
- 将托盘“暂停所有公网”和“停止全部发布”接入 manager。
- 实现状态刷新和 provider 状态不一致提示。
- 完善 Tailscale provider 状态解析和真实 URL 展示。
- 执行 `docs/manual-verification.md` 中的真实两机/Tailscale 手动验收。

## 本地开发约束

- 默认使用中文回复。
- 继续使用 Go + Fyne；首版只支持 HTTP/HTTPS 网页服务。
- 保持 provider 抽象，不要把 UI 直接绑定到 Tailscale CLI。
- 不要提交 `.superpowers/`，其中只保存本机便携工具链和临时产物。
- Windows 本机普通 Fyne 构建需要 CGO 和 GCC。当前验证过的命令：

```powershell
cd D:\developsoftweare\portshare-desktop\.worktrees\portshare-mvp
$env:PATH = (Join-Path (Get-Location) '.superpowers\tools\w64devkit-1.23.0\w64devkit\bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' test ./...
& '.\.superpowers\tools\go1.26.2\go\bin\go.exe' run .\cmd\portshare
```

- 不要使用 w64devkit `2.7.0` 作为当前 Go 1.26.2 的 CGO 编译器；它会生成 `pe-bigobj-x86-64`，当前 cgo 解析失败。

## 未来需完成

- 支持原始 TCP 发布。
- 支持公网访问令牌、简单密码页或认证代理。
- 支持自建中转 provider。
- 支持 FRP/ngrok 风格 provider。
- 支持 Cloudflare Tunnel 或其他隧道 provider。
- 支持每个服务独立访问策略。
- 支持自定义域名。
- 支持更完整的 provider 配置界面。
- 支持发布策略模板。
- 支持更细的风险分级和安全审计。
