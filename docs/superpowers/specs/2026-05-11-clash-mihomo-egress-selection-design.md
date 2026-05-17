# Portshare Clash/Mihomo 出口选择设计

日期：2026-05-11

## 背景

当前 Portshare 已经可以配对两台电脑，并基于 Tailscale 建立全端口访问能力。用户的实际网络链路不是单纯的“本机 -> Tailscale -> 对端”，而是：

```text
电脑 A -> portshare -> Clash/Mihomo 代理/TUN -> Tailscale 公网直连
       -> Tailscale 公网直连 -> Clash/Mihomo 代理/TUN -> portshare -> 电脑 B
```

在这个链路中，Tailscale 自身不能直接被指定“使用上海某个公网 IP”。它会根据当前本机出口、NAT 映射、DERP 探测和对端 endpoint 自动选择路径。Portshare 能做的是识别 Clash/Mihomo 的可用出口节点，让用户选择节点，再触发 Tailscale 重新探测并验证是否获得低延迟 direct 路径。

本机实测环境：

- 代理进程：`clash-verge.exe`、`clash-verge-service`、`verge-mihomo`
- TUN 网卡：`Meta`
- `mixed-port: 7897`
- `socks-port: 7898`
- `port: 7899`
- 运行配置中 TCP `external-controller` 为空或未监听
- 运行配置中存在 `external-controller-pipe: \\.\pipe\verge-mihomo`

因此 `7897` 不能被视为控制端口，它是代理入口。控制接口需要动态发现，优先使用 Clash/Mihomo 配置中的 named pipe。

## 目标

新增“出口选择”能力，用于动态发现 Clash/Mihomo TUN 代理和控制接口，读取节点列表，显示每个节点的地区、公网映射和延迟，并允许用户选择节点后验证 Tailscale 是否进入更低延迟的公网 direct 路径。

## 非目标

- 不实现自建公网中转服务。
- 不强行修改 Tailscale 内部 endpoint 选择算法。
- 不把 `mixed-port`、`socks-port`、`port` 当成控制端口。
- 不保存或明文展示 Clash/Mihomo 的 `secret`。
- 不影响系统里其他程序是否走代理。

## 用户体验

主界面新增或替换现有“网络路径/临时绕过代理”区域为“出口优化”区域。

展示内容：

- TUN 状态：例如 `Meta 已启用`
- 代理入口：例如 `mixed 7897 / socks 7898 / http 7899`
- 控制接口：例如 `named pipe \\.\pipe\verge-mihomo`
- 当前 Tailscale 路径：`direct / DERP / peer-relay`
- 当前 Tailscale endpoint、公网映射和延迟
- 节点候选列表：
  - 节点名称
  - 节点类型
  - 组名
  - 地区标签（从节点名推断，如上海、杭州、香港、日本）
  - Clash/Mihomo 节点延迟
  - 切换后 Tailscale direct 延迟
  - 是否推荐

主要操作：

- `检测代理/TUN`
- `刷新节点延迟`
- `应用出口节点`
- `恢复原节点`
- `重新检测 Tailscale`

用户选择节点后，Portshare 应该先记录当前节点，再切换到目标节点，触发 Tailscale `restun/rebind`，执行 `tailscale ping --c 10 <peer>` 验证结果。如果验证失败或更慢，提示用户并允许恢复原节点。

## 动态发现策略

### 1. 进程和端口发现

通过 Windows API 或 PowerShell 等价能力读取：

- 代理相关进程：`clash`、`mihomo`、`meta`、`verge`、`sing-box`
- 监听端口：`Get-NetTCPConnection -State Listen`
- 进程路径和 PID

这些信息仅用于展示和辅助定位，不用于认定控制端口。

### 2. TUN 网卡发现

读取网络适配器和路由，识别名称或描述包含以下关键字的接口：

- `Meta`
- `Mihomo`
- `Clash`
- `TUN`
- `sing-box`
- `proxy`

当前 Tailscale endpoint 如果走这些接口，应标记为“direct 但疑似代理/TUN 绕路”。

### 3. 配置文件发现

按常见路径扫描 Clash Verge/Mihomo 配置：

- `%APPDATA%\io.github.clash-verge-rev.clash-verge-rev`
- `%LOCALAPPDATA%\io.github.clash-verge-rev.clash-verge-rev`
- `%APPDATA%\clash_win`

解析以下字段：

- `external-controller`
- `external-controller-pipe`
- `secret`
- `mixed-port`
- `socks-port`
- `port`
- `allow-lan`
- `tun.enable`

如果存在多个配置文件，优先级为运行态配置高于基础配置。对于 Clash Verge，优先读取 `clash-verge.yaml` 或实际运行态生成文件；当 `external-controller-pipe` 存在且命名管道可访问时，优先使用 named pipe。

### 4. 控制接口发现

控制接口优先级：

1. 可访问的 `external-controller-pipe`
2. 正在监听且能响应 Clash API 的 TCP `external-controller`
3. 用户手动指定控制接口

判断 TCP 控制接口时，必须用 Clash API 指纹确认，例如 `/version`、`/configs`、`/proxies` 返回合法 JSON 或认证错误。不能因为某端口属于 `verge-mihomo` 就认定它是控制端口。

## Clash/Mihomo 控制能力

需要支持 HTTP API 和 Windows named pipe 两种 transport，并在上层提供统一接口：

```go
type ClashController interface {
    Version(ctx context.Context) (ClashVersion, error)
    Proxies(ctx context.Context) (ProxySnapshot, error)
    Delay(ctx context.Context, proxyName string, testURL string, timeoutMS int) (time.Duration, error)
    Select(ctx context.Context, groupName string, proxyName string) error
}
```

named pipe transport 的目标是访问 `\\.\pipe\verge-mihomo`，请求语义与 Clash external-controller HTTP API 保持一致。

认证要求：

- 如果配置中有 `secret`，请求必须使用 `Authorization: Bearer <secret>`。
- UI 不显示 secret 原文。
- 日志中不写入 secret。

## 节点和地区识别

节点来源于 `/proxies` 或 named pipe 等价 API。Portshare 需要识别可选择的 selector/url-test/fallback/load-balance 组，找到当前活跃组和可切换节点。

地区标签第一版采用节点名规则推断：

- `上海`、`沪` -> 上海
- `杭州`、`杭` -> 杭州
- `香港`、`HK` -> 香港
- `日本`、`JP`、`东京` -> 日本/东京
- `台湾`、`TW` -> 台湾
- `新加坡`、`SG` -> 新加坡
- 其他无法识别时显示 `未知地区`

节点延迟来自 Clash/Mihomo 的 delay API。Tailscale direct 延迟来自切换后执行的 `tailscale ping --c 10 <peer>`。

## 出口应用流程

1. 用户点击 `检测代理/TUN`。
2. Portshare 发现 TUN、代理入口、控制接口和当前 Tailscale 路径。
3. 用户点击 `刷新节点延迟`。
4. Portshare 从 Clash/Mihomo 拉取节点列表，并对候选节点执行 delay 测试。
5. 用户选择一个节点并点击 `应用出口节点`。
6. Portshare 记录当前节点。
7. Portshare 调用 Clash/Mihomo 控制接口切换节点。
8. Portshare 执行 `tailscale debug restun`，必要时再执行 `tailscale debug rebind`。
9. Portshare 执行 `tailscale ping --c 10 <peer>`。
10. 如果结果为 direct，记录 direct endpoint 和延迟；如果延迟优于原路径，标记为推荐/已优化。
11. 如果结果为 DERP、peer-relay、连接失败或明显更慢，提示用户并提供恢复原节点操作。

## 错误处理

- 找不到 TUN：显示“未检测到 Clash/Mihomo TUN”，但仍允许检测控制接口。
- 找不到控制接口：显示代理入口端口，并提示无法读取节点列表。
- named pipe 不可访问：回退到 TCP `external-controller` 探测。
- 控制接口需要 secret 但配置缺失：提示用户在设置里手动填写控制密钥。
- 节点切换失败：不触发 Tailscale 重新探测。
- Tailscale 切换后变 DERP：提示失败，并建议恢复原节点。
- Tailscale endpoint 变更：刷新路径报告，不把旧 endpoint 继续用于路由策略。

## 安全和隐私

- 不向公网开放 Clash/Mihomo 控制接口。
- 不修改 Clash/Mihomo 的订阅配置。
- 不保存 secret 到 Portshare 配置，第一版只从本机配置中读取运行时使用；如果未来支持手动输入，需要使用 Windows Credential Manager 或等价机制。
- 日志和 UI 均不展示 secret 原文。
- 节点名称、地区和延迟仅在本机显示。

## 测试策略

单元测试：

- 配置解析：`mixed-port`、`socks-port`、`port`、`external-controller`、`external-controller-pipe`、`secret`。
- 控制接口发现：named pipe 优先、TCP API 指纹确认、代理入口不被误判为控制端口。
- 节点地区推断。
- 节点列表解析和 selector 组选择。
- 出口应用流程：成功 direct、更慢、DERP、切换失败、恢复原节点。

集成/手动测试：

- Clash Verge Rev + Mihomo named pipe。
- `7897` 为 mixed-port 时，不被识别为控制端口。
- `\\.\pipe\verge-mihomo` 可用时能读取 `/version` 和 `/proxies`。
- 切换上海/杭州/香港等节点后，UI 显示节点延迟和 Tailscale direct 延迟。
- 失败时可以恢复原节点。

## 验收标准

- Portshare 能自动显示当前 TUN 网卡和代理入口端口。
- Portshare 能正确识别 `7897` 是代理入口，而不是控制端口。
- Portshare 能通过 `\\.\pipe\verge-mihomo` 或有效 TCP external-controller 读取节点。
- UI 能展示地区、节点延迟和 Tailscale direct 延迟。
- 用户能选择低延迟地区节点并验证 Tailscale direct 路径。
- 切换失败或变 DERP 时，不留下错误状态，并能恢复原节点。
