# portshare 双端链路守护器设计

## 背景

portshare 当前已经具备 Tailscale 配对、全端口互信、localhost 桥接、实时延迟显示、Clash/Mihomo 出口识别、IPv4 `/32` 和 IPv6 `/128` Tailscale endpoint 临时绕过能力。现场调试发现，两台电脑即使都关闭 portshare，只要 Tailscale 服务仍在运行，双方仍然可以通过 tailnet 互通；这说明低延迟链路并不是 portshare 本身“转发”出来的，而是 Tailscale 后台通过 DERP/STUN 协商、UDP 打洞和 direct endpoint 选择形成的。

用户目标不是让 portshare 接管整机代理，也不是让 portshare 做业务端口转发，而是让两台电脑在 Tailscale 上尽可能稳定地获得类似同一局域网的低延迟体验。portshare 可以主动触发 Tailscale 重新探测、重新打洞、重新选路，并在确认收益后只对当前对端公网 endpoint 添加最小范围 host route，从而把优化限定在双方 Tailscale 直连链路上。

## 关键结论

- Tailscale 可以被主动“诱导”重新协商直连，但不能被 portshare 绝对命令固定使用某一个公网 IP。
- `tailscale debug restun` 和 `tailscale debug rebind` 可以触发 magicsock 重新 STUN 或重新绑定端口，但它们属于 Tailscale debug 接口，不是稳定 API，必须做版本检测和失败降级。
- 主页已有的延迟检测可以作为链路守护器的 `tailscale ping` 数据源和预热触发源，不需要另起一套重复 ping 循环。
- 低延迟 direct 的维持依赖 NAT 映射、keepalive 和真实流量。portshare 可以复用主页延迟检测和控制连接轻量心跳维持活跃度，但不能保证公网运营商、路由器或 VPN 变化时路径永不改变。
- Clash/Mihomo 节点切换会影响整机代理策略，不应该作为默认自动动作。默认自动优化只允许做 Tailscale 对端 endpoint 的精确 host route。

参考资料：

- Tailscale connection types: https://tailscale.com/docs/reference/connection-types
- Tailscale STUN protocol: https://tailscale.com/docs/reference/stun-protocol
- Tailscale NAT traversal: https://tailscale.com/blog/how-nat-traversal-works

## 目标

- 在 portshare 配对成功后，双端自动交换网络状态，并判断当前是否已经是低延迟 direct。
- 主动触发 Tailscale 重新探测直连，包括复用主页延迟检测做双端 ping 预热、可用时执行 restun/rebind、重新读取 endpoint。
- 检测双方访问对方 endpoint 的实际 Windows 路由，识别是否经过 `Meta`、`Clash`、`Mihomo`、`TUN` 等疑似代理虚拟网卡。
- 在双端都确认收益时，只给当前对端公网 endpoint 添加临时 host route：IPv4 使用 `/32`，IPv6 使用 `/128`。
- endpoint 变化、延迟升高、路径回到 DERP/peer-relay、验证失败时自动撤销或更新 portshare 添加的临时 route。
- UI 展示“正在优化、已低延迟直连、TUN 接管但低延迟、DERP 中继、优化失败、已回滚”等明确状态。
- 将本次人工调优经验沉淀为软件自动能力，而不是依赖用户手动切 Clash 节点或手动执行 PowerShell。

## 非目标

- 不实现自建隧道、VPN、WFP 驱动或新的内核网络层。
- 不修改系统默认路由 `0.0.0.0/0` 或 `::/0`。
- 不关闭 Clash/Mihomo/TUN，也不默认切换代理节点。
- 不让所有程序绕过代理，只优化双方 Tailscale direct endpoint 的访问路径。
- 不承诺固定使用某个公网 IP。公网 IP、NAT 映射、运营商路径由 Tailscale 和网络环境共同决定。
- 不把用户假设中的“portshare 自己实现代理/TUN 隧道方案”纳入本轮实现。

## 用户体验

直连页面新增“链路守护”区域，默认在双方配对成功后开启守护。守护器会先诊断和预热；只有在双端验证确认“当前 direct 高延迟且疑似 TUN/代理绕路，并且存在更合适的 endpoint 精确 route”后，才会自动应用临时 host route。区域显示：

- 守护状态：`待配对`、`采集双端状态`、`预热直连`、`已低延迟直连`、`可优化`、`已应用精确绕过`、`已回滚`、`优化失败`。
- 当前路径：`direct`、`DERP`、`peer-relay`、`未知`。
- 当前 endpoint：例如 `115.233.222.82:52477` 或 `[2401:...]:41641`。
- 当前延迟：来自主页已有延迟检测，检测周期控制在 1 秒内，链路守护器直接复用最新样本。
- 当前出口：例如 `Meta -> 198.18.0.2`、`以太网 -> 192.168.1.1`。
- 对端视角：显示对方看到本机的 endpoint、延迟和出口接口。
- 建议动作：例如“当前已经是低延迟 direct，无需绕过”、“direct 但疑似 TUN 绕路，建议执行精确绕过”。

默认按钮：

- `重新优化`：重新执行双端采集、基于主页延迟检测的预热、restun/rebind、验证。
- `撤销优化`：删除 portshare 创建的 host route。
- `自动精确绕过` 开关：默认开启，只影响 Tailscale 对端 endpoint；关闭后只诊断和提示，不自动加 route。
- `高级设置`：展示 Clash 控制状态、手动代理测速、手动节点选择提示，但默认不自动切节点。

## 双端数据交换

portshare 已经有配对握手能力，链路守护器复用配对后的本地控制端口交换状态。每端定期发布一个 `LinkSnapshot`：

- 本机 Tailscale IP 和 hostname。
- 对端 Tailscale IP。
- Tailscale 版本、CLI 可用性、是否支持 `debug restun` / `debug rebind`。
- `tailscale status --json` 中对端的 `CurAddr`、连接类型、在线状态。
- 主页延迟检测结果：路径类型、endpoint、延迟样本、失败原因；实现上仍可由 `tailscale ping` 提供，但由统一检测器调度。
- `Find-NetRoute -RemoteIPAddress <endpoint-ip>` 结果：接口名、接口 index、下一跳、route metric。
- 默认公网出口候选：IPv4/IPv6 默认路由、物理网卡、疑似代理/TUN 标记。
- Clash/Mihomo 状态：是否发现控制端、当前节点信息、控制端类型。该信息仅用于诊断和高级手动项。
- portshare 已添加的 route 记录。

交换数据只在双方已授权设备之间进行，不发送到公网服务。UI 中显示公网 endpoint 时需要提醒用户：这是 Tailscale 为直连探测使用的公网候选地址。

## 主动触发流程

1. **采集基线**
   - 双端同时读取 Tailscale 状态、ping 对端、解析 endpoint。
   - 双端分别检查“访问对方 endpoint 当前走哪个 Windows 接口”。
   - 如果已经是 `direct + 低延迟`，直接进入守护状态，不改路由。

2. **预热直连**
   - 双端复用主页已有延迟检测，对对方 Tailscale IP 持续采样，采样周期控制在 1 秒内。
   - 链路守护器不单独启动第二套 `tailscale ping` 循环，避免重复探测和 UI 状态打架。
   - portshare 控制连接保持轻量心跳，心跳节奏跟随统一延迟检测周期，促使 NAT 映射持续存在。
   - 预热后再次读取 `CurAddr` 和最新延迟样本。

3. **重新探测**
   - 如果 CLI 支持，则尝试执行 `tailscale debug restun`。
   - 如果 endpoint 或延迟仍异常，再尝试 `tailscale debug rebind`。
   - 因为 debug 命令不稳定，执行失败只记录诊断，不中断软件。

4. **双端验证**
   - 双端分别确认自己访问对方 endpoint 的 route。
   - 只有当一端或两端发现“direct 但高延迟，且当前 route 走疑似 TUN/代理接口”时，才进入可优化状态。
   - 如果路径是 DERP 或 peer-relay，优先提示“尚未打洞成功”，不直接加 host route。

5. **精确 host route**
   - 对当前对端 endpoint 添加临时 route：
     - IPv4：`<endpoint-ip>/32`
     - IPv6：`<endpoint-ip>/128`
   - 仅写入 `ActiveStore`，不写入 `PersistentStore`。
   - route 创建后必须在 portshare 本地状态中记录 owner、peer、endpoint、interface index、gateway、创建时间。Windows route 本身不依赖自定义 owner 字段。
   - 添加后立即复测 `Find-NetRoute` 和 `tailscale ping`。

6. **回滚与更新**
   - 如果添加 route 后延迟没有改善、路径变为 DERP/peer-relay、或 endpoint 变化，撤销旧 route。
   - 如果 endpoint 变化但仍满足优化条件，删除旧 route 后为新 endpoint 重新验证并添加。
   - portshare 退出时默认撤销本次运行期添加的临时 route；异常退出后下次启动根据本地持久化的 owner 记录匹配并清理残留 route。

## 决策规则

- `direct + 延迟 <= 50ms`：认为已经是低延迟直连，不自动改 route。
- `direct + 延迟 <= 50ms + TUN 接口`：显示“低延迟 direct，TUN 接管但当前路径可用”，不自动绕过。
- `direct + 延迟 > 120ms + TUN 接口`：进入“可优化”；如果 `自动精确绕过` 开启且双端验证通过，则自动应用临时 host route。
- `direct + 延迟 > 120ms + 物理接口`：提示公网路径本身较慢，不优先改 route。
- `DERP`：提示中继状态，优先 restun/rebind 和预热，不加 endpoint route。
- `peer-relay`：提示经过 peer relay，优先诊断 UDP、NAT、TUN 和防火墙。
- 双端状态冲突时，以更保守策略为准：先预热和复测，不立即改 route。

阈值首版固定为 50ms 和 120ms，后续可做成高级设置。

## 与 Clash/Mihomo 的关系

portshare 可以读取 Clash/Mihomo 控制端和节点延迟，但默认不自动切换节点，因为切节点通常影响整机代理体验。链路守护器只使用这些信息做三件事：

- 判断当前 endpoint route 是否疑似经过 TUN/代理。
- 在高级设置中提示用户：当前节点、节点延迟、可能的 Tailscale 影响。
- 在用户明确点击高级手动操作时，才允许调用 Clash/Mihomo 控制端。

如果用户提供 HTTP/SOCKS5 代理地址，portshare 可以用它做诊断请求、测速和控制连接实验，但不能直接让 `tailscaled.exe` 的 P2P 数据面使用该代理。Tailscale direct 数据面仍由 Tailscale 后台和系统路由决定。

## 权限与进程

- restun/rebind/status 通过隐藏子进程执行，避免弹出 PowerShell；ping 类延迟检测复用主页统一检测器。
- route 添加和删除需要管理员权限，复用应用启动时的管理员权限检测。
- 如果当前不是管理员，链路守护器仍可做诊断、预热、restun/rebind；涉及 route 修改时提示重启为管理员。
- 所有外部命令要有超时，避免 UI 卡住。
- 命令输出不得直接暴露敏感凭证；日志只记录 endpoint、接口名、路径状态和错误摘要。

## 状态持久化

本功能只持久化最少信息：

- 已授权 peer 的守护器是否启用。
- 上一次优化结果摘要：时间、路径、延迟、是否应用 route。
- portshare 创建的 route owner 记录，用于下次启动清理。

不持久化 Clash 节点密码、代理账号密码、远程调试凭证或用户输入的临时敏感信息。

## 测试计划

- 单元测试：解析 `tailscale ping` 的 direct、DERP、peer-relay、IPv4 endpoint、IPv6 endpoint。
- 单元测试：解析 `tailscale status --json` 中 peer 的 `CurAddr`。
- 单元测试：判断疑似 TUN/代理接口和物理接口。
- 单元测试：链路决策规则，包括低延迟 direct、高延迟 TUN、DERP、peer-relay、双端冲突。
- 单元测试：route 命令构造，确认 IPv4 `/32`、IPv6 `/128`、`ActiveStore`、interface index 和 gateway 正确。
- 集成测试：fake runner 模拟 restun/rebind 成功、失败、超时。
- 集成测试：链路守护器复用主页延迟检测样本，检测周期不超过 1 秒，且不会启动重复 ping 循环。
- UI 测试：守护状态、对端视角、重新优化、撤销优化、高级设置状态流转。
- 手动验收：
  - 两端配对成功后自动显示双端视角。
  - 已低延迟 direct 时不自动改 route。
  - 高延迟且疑似 TUN 绕路时进入可优化。
  - 应用精确绕过后 `Find-NetRoute <endpoint>` 指向目标物理出口。
  - endpoint 改变后旧 route 被撤销。
  - portshare 退出后临时 route 被清理。

## 后续计划入口

本 spec 通过后，下一步编写实现计划。计划应优先复用现有 `internal/netdiag`、Tailscale CLI runner、route manager、Clash/Mihomo 发现逻辑和直连页面状态模型，避免另起一套网络诊断体系。
