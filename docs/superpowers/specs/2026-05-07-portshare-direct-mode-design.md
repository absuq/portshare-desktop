# portshare 直连模式设计

## 背景

当前 MVP 已经验证：可以使用 Tailscale Serve 把本机 HTTP 服务发布到 tailnet 内访问。这证明了基础网络环境可行，但这不是新的产品目标。新的目标是设备到设备直连使用：两台电脑都运行 `portshare`，双方输入同一个共享密钥建立信任，然后按需创建本地 TCP 转发入口。

应用名称统一为 `portshare`。窗口标题、托盘菜单、文档标题和用户可见的产品引用都不翻译。界面文案可以继续默认中文。

## 目标

- 将可见产品名从“端口发布器”改为 `portshare`。
- 将 MVP 主流程替换为“设备配对 + 本地 TCP 转发”。
- 使用 Tailscale 作为必需的私有网络底座。
- 新直连模式不再依赖 Tailscale Serve 或 Funnel。
- 不通过 Tailscale Serve、Funnel、局域网或公网直接暴露业务服务端口。
- 配对后的设备可以通过 `portshare` 管理的本地转发入口访问对方任意 TCP 端口。
- 清晰诊断 Tailscale 准备状态和连接失败原因，让用户知道下一步该修什么。

## 非目标

- UDP 转发。
- 公网分享。
- 首版 direct-mode MVP 不做每端口授权。
- `portshare` 不自己实现 NAT 穿透。
- 不替代 Tailscale，也不支持脱离 Tailscale 运行。
- 不做系统级透明路由，也就是不会让所有远端端口自动出现在本机；用户仍通过本地转发入口访问。

## 直连模式定义

直连模式是指两个 `portshare` 客户端通过 Tailscale IP 互相连接，并使用共享密钥在应用层认证。业务流量经过两端 `portshare`：

```text
本机应用或浏览器
  -> 127.0.0.1:<本地转发端口>
  -> 本机 portshare
  -> 对方 Tailscale IP:17890
  -> 对方 portshare
  -> 对方本机 TCP 目标，例如 127.0.0.1:3000
```

底层 Tailscale 链路可能是 UDP peer-to-peer，也可能根据网络环境退回 DERP 中继。`portshare` 应通过 `tailscale ping` 将路径类型和延迟展示给用户，但不自己实现 NAT 穿透，也不强制绕过 DERP。

## Tailscale 依赖与诊断

`portshare` 在配对或转发前必须主动检测并使用 Tailscale 能力。

启动检查：

- `tailscale` CLI 可用。
- Tailscale 后端正在运行。
- 当前设备已登录 Tailscale。
- 当前设备至少有一个 Tailscale IPv4 地址。
- 应用可以将控制监听器绑定到本机 Tailscale IP。

对端检查：

- `tailscale ping <peer-ip>` 成功。
- UI 显示当前路径是 direct 还是 DERP，并显示延迟。
- 对端控制端点 `<peer-ip>:17890` 可连接。
- 如果用户输入的是 DNS 名称，检查 MagicDNS 解析；如果本机没有接受 Tailscale DNS，提示执行 `tailscale set --accept-dns=true` 或改用 Tailscale IP。

故障排查检查：

- 名称解析失败时检测 `accept-dns=false`。
- 入站控制连接疑似被阻断时检测或提示 Shields Up。
- 区分常见失败层级：
  - Tailscale 未安装。
  - Tailscale 未运行。
  - 未登录 Tailscale。
  - 没有 Tailscale IP。
  - 对方离线或不可达。
  - 对方没有运行 `portshare`。
  - 共享密钥不匹配。
  - 本机控制端口绑定失败。
  - 本地转发端口已被占用。
  - 对方目标端口拒绝连接或超时。

可以暴露安全的一键修复动作：

- `tailscale set --accept-dns=true`
- `tailscale set --shields-up=false`

登录、重新认证或更大范围的 Tailscale 设置变更可能影响当前网络状态，不应静默执行。它们应以明确的手动步骤呈现给用户。

## 控制监听器

每个运行中的 `portshare` 实例都会启动一个固定默认控制端口：

```text
<本机 Tailscale IP>:17890
```

直连模式下监听器不能绑定到 `0.0.0.0`。它应绑定到从本机 Tailscale 状态中选出的 Tailscale IP。绑定失败时，UI 应显示诊断信息，并且 direct mode 不能标记为 ready。

控制端口是 `portshare` 内部端点，不是业务服务端口。它只接受通过 `portshare` 协议认证的请求。

## 配对模型

配对是设备级信任，不是端口级授权。

用户流程：

1. 两台电脑都打开 `portshare`。
2. 两台电脑都输入同一个共享密钥。
3. 其中一方输入另一方的 Tailscale IP。
4. 发起方连接 `<peer-tailscale-ip>:17890`。
5. 双方使用共享密钥执行挑战响应握手。
6. 握手成功后，双方将对方视为可信设备。
7. 可信设备记录保存到本机。

共享密钥不能明文发送。MVP 认证使用基于随机 nonce 的 HMAC-SHA256 挑战响应：

- 发起方发送协议版本、本机设备身份和随机 nonce。
- 响应方返回自己的 nonce 和 HMAC 证明。
- 发起方验证响应方证明，然后发送自己的 HMAC 证明。
- 响应方验证发起方证明。
- 双方从共享密钥和两端 nonce 派生本次连接的会话密钥，用于后续请求认证。

MVP 可以在每次新会话中继续使用共享密钥完成认证，而不实现长期公钥体系。但本地保存的配对记录不能包含明文共享密钥。

配对记录包含：

- 对方 Tailscale IP。
- 对方显示名，如果可用。
- 首次配对时间。
- 最近连接时间。
- 最近观察到的路径类型，例如 direct 或 DERP。
- 从密钥派生出的本地配对标识，而不是密钥本身。

## 转发模型

配对后，任意一方都可以创建指向对方的本地 TCP 转发入口。

创建转发时输入：

- 可信设备。
- 对方目标主机和端口，默认是 `127.0.0.1:<remote-port>`。
- 本地监听主机，默认是 `127.0.0.1`。
- 本地监听端口，可以由用户指定，也可以自动分配。

示例：

```text
127.0.0.1:18080 -> 100.109.251.97:17890 -> 127.0.0.1:3000
```

创建转发不需要再次执行配对授权。但它要求对方仍可连接，并且能够用现有共享密钥关系通过认证。

转发支持任意 TCP 字节流。它不能假设协议是 HTTP，必须支持 SSH、数据库、开发服务器和其他 TCP 协议。

停止某个转发会关闭对应本地监听器和活跃流连接，但不会删除可信设备配对。

删除配对会停止所有关联该对端的转发。

## UI 要求

主界面以直连模式为中心：

- 产品标题：`portshare`。
- 本机 Tailscale 状态面板：
  - Running 或未运行。
  - 本机 Tailscale IP。
  - 控制监听器状态。
  - 需要时显示 DNS 准备状态。
- 共享密钥输入：
  - 输入或更新当前密钥。
  - 显示 direct mode 是否 ready。
- 配对对端面板：
  - 输入对方 Tailscale IP 或名称。
  - 连接/配对操作。
  - 配对进度和诊断信息。
- 可信设备列表：
  - 设备名/IP。
  - 最近连接时间。
  - 最近一次 `tailscale ping` 得到的路径类型和延迟。
  - 删除配对操作。
- 转发面板：
  - 选择可信设备。
  - 输入远端 host/port。
  - 输入或自动分配本地端口。
  - 创建转发。
  - 显示正在运行的转发及本地地址，并提供停止操作。

现有服务发现和 Tailscale Serve 按钮可以在过渡期保留到高级或旧功能区域，但它们不是新的 MVP 主流程。

## 安全提示

UI 必须明确提示：

- 已配对设备可以请求访问本机任意 TCP 端口。
- 只和自己控制或明确可信的电脑配对。
- 共享密钥应像密码一样保管。
- direct mode 不通过 Tailscale Serve 或 Funnel 直接暴露业务端口。

认证失败应写入审计日志，记录对端 IP 和失败原因，但不能记录共享密钥。

## 架构调整

现有 provider 抽象面向“发布服务”。直连模式需要独立子系统，不应该硬塞进发布 provider 形状里。

新增内部区域：

- `internal/tailscale`：本机 Tailscale CLI、状态和诊断适配器。
- `internal/direct/protocol`：握手和流协议。
- `internal/direct/server`：控制监听器和对端请求处理。
- `internal/direct/client`：连接对端并创建认证会话。
- `internal/direct/forward`：本地 TCP 监听器和双向字节流转发。
- `internal/direct/store`：可信设备持久化。
- `internal/direct/manager`：ready 状态、配对、转发、诊断和审计事件编排。

旧的 `internal/provider/tailscale` 可以继续保留用于 legacy Serve/Funnel 行为，但 direct mode 不能调用 `tailscale serve`。

## 协议草案

协议运行在对端控制端口上的 TCP 连接中。

握手和转发建立阶段使用带长度前缀的 JSON 控制消息。转发请求被接受后，TCP 载荷使用原始字节双向复制。

最小消息类型：

- `hello`
- `hello_response`
- `auth_proof`
- `auth_ok`
- `open_tcp`
- `open_tcp_ok`
- `open_tcp_error`

`open_tcp` 请求包含对端本机目标 host 和 port。收到 `open_tcp_ok` 后，双方开始转发原始字节，直到任一端关闭连接。

所有控制消息都包含协议版本。版本不兼容时返回清晰错误。

## 测试策略

单元测试：

- HMAC 挑战响应在密钥一致时成功。
- HMAC 挑战响应在密钥不一致时失败。
- 保存的可信设备记录不包含明文共享密钥。
- Tailscale 诊断能分类 CLI 缺失、后端停止、无 IP、DNS 未接受、对端不可达等状态。
- 转发管理器拒绝本地端口冲突。
- 转发管理器可以通过 fake peer session 转发字节。

loopback 集成测试：

- 在 loopback 测试地址和不同控制端口上启动两个 direct server。
- 使用共享密钥配对。
- 从一个测试实例创建指向另一个实例的本地转发。
- 验证对端后面的 HTTP 测试服务可以通过本地转发访问。
- 停止转发后，验证本地入口不可访问。

手动验收：

- 在同一 tailnet 的两台 Windows 电脑上运行 `portshare`。
- 确认两端都显示 Tailscale ready 和控制监听器 ready。
- 两端输入同一个共享密钥。
- 通过输入对端 Tailscale IP 完成配对。
- 确认路径诊断显示 direct 或 DERP 以及延迟。
- 创建指向对端 `127.0.0.1:3000` 的本地转发。
- 在发起端访问 `http://127.0.0.1:<local-port>/`。
- 停止转发并确认访问失败。
- 在一台机器上关闭 DNS 接受，并确认当用户输入 DNS 名称时诊断能解释 `accept-dns` 问题。

## 从当前 MVP 迁移

短期：

- 添加 direct-mode 模块时保持现有测试通过。
- 将可见产品名改为 `portshare`。
- 将 Tailscale Serve/Funnel 操作移出主流程。

未来：

- 决定是移除 legacy Serve/Funnel，还是作为高级分享模式保留。
- 在共享密钥 MVP 后增加更强的长期设备密钥。
- 如果设备级信任对真实用户过宽，再增加按对端或按端口的访问策略。
