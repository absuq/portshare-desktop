# 下一会话交接说明

## 当前状态

- 公开仓库已创建：`https://github.com/absuq/portshare-desktop`
- 默认分支：`main`
- 当前阶段：需求和 MVP 实现计划已完成，尚未开始写应用代码。
- 已选择执行方式：Subagent-Driven，也就是按实现计划逐任务派发子代理执行，每个任务完成后由主会话 review。
- 当前设计规格：`docs/superpowers/specs/2026-04-29-portshare-design.md`
- 当前实现计划：`docs/superpowers/plans/2026-04-29-portshare-mvp.md`
- 项目进度清单：`AGENTS.md`

## 新会话开始前

在你新建的 Codex 项目文件夹里执行：

```powershell
git clone https://github.com/absuq/portshare-desktop.git
cd portshare-desktop
```

如果 `git clone` 或 `git push` 直连 GitHub 失败，使用本机代理端口 `7897`：

```powershell
git config --local http.proxy http://127.0.0.1:7897
git config --local https.proxy http://127.0.0.1:7897
```

如果还没有远端认证，确认 GitHub CLI 登录状态：

```powershell
gh auth status
gh auth setup-git
```

## 新会话推荐提示词

可以直接把下面这段发给 Codex：

```text
请阅读 AGENTS.md、docs/NEXT_SESSION.md、docs/superpowers/specs/2026-04-29-portshare-design.md 和 docs/superpowers/plans/2026-04-29-portshare-mvp.md。

我们已经确认使用 Subagent-Driven 方式实现 MVP。请使用 superpowers:subagent-driven-development，从计划的 Task 1 开始执行。每个任务完成后先 review，再继续下一个任务。不要跳过测试；每个任务按计划里的测试和 commit 步骤推进。
```

## 下一步执行要求

新会话开始实现时：

1. 先读取 `AGENTS.md` 和本文件。
2. 再读取设计规格和实现计划。
3. 使用 `superpowers:subagent-driven-development`。
4. 从 `docs/superpowers/plans/2026-04-29-portshare-mvp.md` 的 Task 1 开始。
5. 每个任务完成后运行对应测试。
6. 每个任务完成后提交 commit。
7. 推送前如果直连 GitHub 失败，使用本文件中的 `7897` 代理配置。

## 重要设计边界

- 首版是 Go + Fyne 桌面应用。
- 默认中文界面，可切换英文。
- 首版只支持 HTTP/HTTPS 网页服务。
- 默认开放到 Tailscale tailnet。
- 公网开放必须强确认，并支持倒计时关闭和长期开放。
- 支持多个端口同时开放。
- 支持系统托盘常驻。
- 保存历史日志，默认保留 1 年。
- Tailscale 是首版 provider，但核心架构必须保留 provider 抽象，方便未来自建中转。

## 当前 Git 说明

本仓库本地曾遇到直连 `github.com:443` 超时/连接重置。根因是直连 GitHub 链路不稳定，而 `127.0.0.1:7897` 代理可用。

已验证通过代理可正常执行：

```powershell
git ls-remote origin refs/heads/main
git push
```

如果换到新文件夹后代理配置没有继承，需要重新执行本文件开头的 `git config --local` 命令。
