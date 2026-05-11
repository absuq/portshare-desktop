# portshare Tailscale 直连诊断与临时绕过代理设计

## 背景

当前 portshare 已经能通过 Tailscale 让两台电脑完成配对、全端口授权、localhost 桥接和远程桌面访问。实际测试中出现过一种问题：Tailscale 仍显示 `direct`，但延迟从十几毫秒升到两百毫秒以上。排查发现本机到对端公网 endpoint 的路由被 `Meta` 这类代理/TUN 虚拟网卡接管，导致“直连”外层 UDP 包绕路。

本功能用于在 portshare 内检测这种状态，并允许用户选择物理公网出口，为 Tailscale 对端公网 endpoint 添加临时主机路由，从而让 Tailscale 直连包绕过代理。功能目标是只影响 Tailscale 对端公网 endpoint，不修改系统默认路由，不关闭代理，不改变浏览器或其他应用的代理策略。

## 目标

- 显示 Tailscale 是否就绪、是否已直连、直连 endpoint、当前延迟和是否疑似走代理/TUN。
- 检测本机可用公网出口，至少列出物理网卡、网关、接口名称、接口 IP、接口 metric。
- 检测当前到 Tailscale 对端公网 endpoint 的实际路由，显示正在使用的接口。
- 当发现当前路由走 `Meta`、`Clash`、`Mihomo`、`Vortex`、`TUN` 等疑似代理接口时，提示用户可选择物理网卡临时绕过。
- 用户选择出口后，只对 Tailscale 对端公网 IP 添加 `/32` 临时主机路由，例如 `115.233.222.82/32 -> 192.168.1.1`。
- 支持撤销 portshare 添加的临时绕过路由。
- 保持全程需要用户明确点击确认，不自动改路由。

## 非目标

- 不修改系统默认路由 `0.0.0.0/0`。
- 不关闭代理软件，也不修改代理软件配置。
- 不把所有程序都绕过代理。
- 不实现长期持久路由，首版只做当前系统运行期间有效的临时路由。
- 不实现 Windows Filtering Platform 级别的严格按进程路由。普通 Windows route 表无法简单做到“只对 tailscaled.exe 生效”。本设计通过对 Tailscale 对端 endpoint IP 添加 `/32` 主机路由，把影响范围收窄到该 endpoint。若其他程序也访问同一个 endpoint IP，也会命中这条主机路由，但不会影响其他目标地址。

## 用户体验

直连页面新增“网络路径”区域，显示：

- 路径状态：`直连正常`、`直连但疑似代理绕路`、`DERP 中继`、`未连接`、`检测失败`。
- 对端 endpoint：例如 `115.233.222.82:41641`。
- 延迟：来自 `tailscale ping`，例如 `11ms` 或 `249ms`。
- 当前出口：例如 `Meta -> 198.18.0.2` 或 `以太网 -> 192.168.1.1`。
- 建议：例如“当前 Tailscale 直连包疑似走代理虚拟网卡，建议选择物理网卡临时绕过。”

新增操作：

- `检测网络路径`：运行诊断并刷新显示。
- `选择公网出口`：列出候选出口。
- `临时绕过代理`：对当前对端 endpoint IP 添加临时 `/32` 路由。
- `撤销绕过`：删除 portshare 添加的临时路由。

候选出口列表只展示默认路由或可达公网的非 Tailscale 接口，并标注疑似代理接口。物理网卡优先展示，代理/TUN 接口作为当前诊断信息展示，不推荐作为绕过目标。

## 诊断逻辑

新增 `internal/netdiag` 包，负责 Windows 网络路径诊断和临时路由控制。

核心输入：

- 本机 Tailscale 状态：`tailscale status --json`。
- 对端 Tailscale IP：来自可信设备列表。
- Tailscale 路径：`tailscale ping --until-direct=false --c 3 <peer>`。
- 当前路由：PowerShell `Find-NetRoute -RemoteIPAddress <endpoint-ip>`。
- 默认出口候选：`Get-NetRoute -DestinationPrefix 0.0.0.0/0`、`Get-NetIPInterface`、`Get-NetIPAddress`、`Get-NetAdapter`。

路径判断：

- `tailscale ping` 返回 `via DERP(...)`：状态为 DERP 中继。
- `tailscale ping` 返回公网 `ip:port`：状态为直连。
- 直连延迟高于阈值，且 `Find-NetRoute` 显示出口接口名匹配疑似代理/TUN：状态为直连但疑似代理绕路。
- 默认高延迟阈值为 `120ms`，仅用于提示，不阻止用户手动选择出口。

候选公网出口：

- 必须有 IPv4 默认路由和下一跳网关。
- 排除 Tailscale 接口。
- 排除 Loopback。
- 标记疑似代理接口，但不默认推荐。
- 按物理接口优先、metric 从低到高排序。

## 临时绕过逻辑

当用户选择出口并确认后，portshare 执行：

- 解析当前 peer 的 Tailscale 直连 endpoint IP，例如 `115.233.222.82`。
- 使用选中的网关和接口添加临时主机路由：
  - `New-NetRoute -DestinationPrefix 115.233.222.82/32 -InterfaceIndex <index> -NextHop <gateway> -PolicyStore ActiveStore`
- 记录这条路由到本地运行时状态，包含 peer Tailscale IP、endpoint IP、interface index、gateway、创建时间。
- 重新运行 `tailscale ping` 与 `Find-NetRoute`，刷新 UI。

撤销时执行：

- 删除 portshare 记录的匹配路由：
  - `Remove-NetRoute -DestinationPrefix <endpoint>/32 -InterfaceIndex <index> -NextHop <gateway> -Confirm:$false`
- 删除后刷新诊断状态。

路由只写入 `ActiveStore`，不加 `-PolicyStore PersistentStore`，所以不会跨重启长期保留。

## 安全与权限

- 应用已要求管理员权限，新增路由操作复用该权限。
- 添加路由前必须显示确认文案：只会影响 Tailscale 对端公网 endpoint，不会修改默认路由。
- 如果 endpoint IP 为空、不是公网 IPv4、或当前 peer 未直连，则禁用“临时绕过代理”。
- 如果已有相同目标的非 portshare 路由，先提示用户，不覆盖未知来源路由。
- 所有外部命令继续通过隐藏子进程 helper 执行，避免弹出 PowerShell 窗口。

## 错误处理

- Tailscale 未运行：显示现有 Tailscale 未就绪信息。
- Peer 未配对：提示先配对可信设备。
- `tailscale ping` 超时：显示检测失败，可重试。
- `Find-NetRoute` 失败：显示无法读取当前出口，不允许一键绕过。
- 添加路由失败：展示 PowerShell 错误摘要。
- 添加后延迟仍高：显示“绕过已应用，但延迟未明显下降”，不自动反复改路由。

## 测试计划

- 单元测试解析 `tailscale ping` 的 direct、DERP、peer-relay 和高延迟结果。
- 单元测试解析 `tailscale status --json` 中 peer 的 `CurAddr`。
- 单元测试候选出口排序与代理接口识别。
- 单元测试路由命令构造，确认使用 `/32`、`ActiveStore`、选中 interface index 和 gateway。
- UI 控制器测试：诊断状态、候选出口、应用绕过、撤销绕过的状态流转。
- 手动验收：
  - Meta/TUN 开启时检测为疑似代理绕路。
  - 应用临时绕过后，`Find-NetRoute <peer endpoint>` 指向物理网卡。
  - `tailscale ping <peer>` 延迟下降。
  - 浏览器等其他流量仍保持原代理行为。
  - 撤销后路由删除。
