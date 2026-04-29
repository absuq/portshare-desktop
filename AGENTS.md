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

## 计划完成

- 按 `docs/superpowers/plans/2026-04-29-portshare-mvp.md` 分任务执行 MVP。
- 新会话开始时先阅读 `docs/NEXT_SESSION.md`，再使用 Subagent-Driven 方式从 Task 1 开始。
- 搭建 Go 项目结构和 Fyne 桌面应用入口。
- 实现中文默认界面和英文语言切换。
- 实现服务列表、手动添加、自动发现和刷新。
- 实现 provider 抽象接口和 fake provider。
- 实现 TailscaleProvider。
- 实现 tailnet 发布、关闭和状态刷新。
- 实现公网发布强确认、计时关闭和长期开放二次确认。
- 实现多端口同时管理。
- 实现系统托盘菜单：打开主界面、暂停所有公网、停止全部发布、退出。
- 实现配置保存和重启恢复。
- 实现历史日志和日志保留清理。
- 实现状态不一致检测和同步提示。
- 编写单元测试、fake provider 集成测试和真实 Tailscale 手动验收步骤。

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
