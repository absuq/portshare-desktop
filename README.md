# 端口发布器

一个计划中的 Go + Fyne 桌面工具，用于把本机 HTTP/HTTPS 服务发布到 Tailscale tailnet，必要时手动开启公网访问。

首版目标：

- 默认中文界面，支持切换英文。
- 默认只开放到 tailnet。
- 公网发布必须强确认，支持倒计时关闭和长期开放。
- 支持多个端口同时管理。
- 支持自动发现本地网页服务和手动添加。
- 支持系统托盘常驻。
- 保存历史记录，默认保留 1 年。
- 首版基于 Tailscale，架构保留 provider 抽象，方便后续接入自建中转。

设计规格见：

- `docs/superpowers/specs/2026-04-29-portshare-design.md`

继续开发前请先阅读：

- `docs/NEXT_SESSION.md`
- `docs/superpowers/plans/2026-04-29-portshare-mvp.md`

手动验收步骤见：

- `docs/manual-verification.md`
