# portshare 直连配对模式设计

## 背景

当前 MVP 已经验证两台 Windows 电脑可以通过 Tailscale tailnet 互相访问。`portshare` 的当前目标不是替代 Tailscale，也不是在本阶段代理业务端口，而是提供一个清晰的双机配对体验：两台电脑都运行 `portshare`，双方输入同一个共享密钥，通过 Tailscale IP 建立应用层信任。

应用名称统一为 `portshare`。窗口标题、托盘菜单、文档标题和用户可见的产品引用都不翻译。界面文案默认中文。

## 当前目标

- 将可见产品名统一为 `portshare`。
- 使用 Tailscale 作为必需的私有网络底座。
- 两台电脑都运行 `portshare`，并监听各自的 Tailscale IP `:17890`。
- 双方输入同一个共享密钥后，可以通过对方 Tailscale IP 完成配对。
- 可信设备记录保存到本机。
- 诊断 Tailscale 准备状态、控制端口可达性和配对失败原因。
- 明确说明关闭 `portshare` 不会关闭 Tailscale 自身的 tailnet 连通性。

## 当前非目标

- 本地业务端口转发。
- 任意 TCP 业务流量代理。
- UDP 转发。
- 公网分享或公网转发。
- Tailscale Serve/Funnel。
- 系统级透明路由。
- `portshare` 自己实现 NAT 穿透。
- 替代 Tailscale ACL、Shields Up 或 Windows 防火墙。

公网转发是下一阶段能力，需要单独设计。

## 直连配对定义

直连配对是指两个 `portshare` 客户端通过 Tailscale IP 互相连接，并使用共享密钥在应用层认证：

```text
电脑 A portshare
  -> 电脑 B Tailscale IP:17890
  -> 电脑 B portshare
  -> HMAC 共享密钥挑战响应
  -> 双方保存可信设备记录
```

当前 MVP 不通过 `portshare` 代理业务流量。配对成功只表示两端 `portshare` 建立了设备级信任，不表示 `portshare` 打通了所有业务端口。

底层 Tailscale 链路可能是 UDP peer-to-peer，也可能根据网络环境退回 DERP 中继。`portshare` 可以通过 `tailscale ping` 展示路径类型和延迟，但不强制绕过 DERP。

## Tailscale 依赖与诊断

`portshare` 在配对前必须主动检测 Tailscale 能力。

启动检查：

- `tailscale` CLI 可用。
- Tailscale 后端正在运行。
- 当前设备已登录 Tailscale。
- 当前设备至少有一个 Tailscale IPv4 地址。
- 应用可以将控制监听器绑定到本机 Tailscale IP。

对端检查：

- `tailscale ping <peer-ip>` 成功。
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
  - 对方没有启用直连密钥。
  - 共享密钥不匹配。
  - 本机控制端口绑定失败。
  - Tailscale Shields Up 或 Windows 防火墙拦截 `17890`。

可以暴露安全的一键修复动作：

- `tailscale set --accept-dns=true`
- `tailscale set --shields-up=false`

登录、重新认证或更大范围的 Tailscale 设置变更可能影响当前网络状态，不应静默执行。它们应以明确的手动步骤呈现给用户。

## 控制监听器

每个运行中的 `portshare` 实例都会启动一个固定默认控制端口：

```text
<本机 Tailscale IP>:17890
```

监听器不能绑定到 `0.0.0.0`。它应绑定到从本机 Tailscale 状态中选出的 Tailscale IP。绑定失败时，UI 应显示诊断信息，并且 direct mode 不能标记为 ready。

控制端口是 `portshare` 内部端点，不是业务服务端口。它只接受通过 `portshare` 协议认证的配对请求。

## 配对模型

配对是设备级信任。

用户流程：

1. 两台电脑都打开 `portshare`。
2. 两台电脑都输入同一个共享密钥。
3. 两台电脑都点击“启用直连密钥”。
4. 其中一方输入另一方的 Tailscale IP。
5. 发起方连接 `<peer-tailscale-ip>:17890`。
6. 双方使用共享密钥执行挑战响应握手。
7. 握手成功后，双方将对方视为可信设备。
8. 可信设备记录保存到本机。

共享密钥不能明文发送。MVP 认证使用基于随机 nonce 的 HMAC-SHA256 挑战响应：

- 发起方发送协议版本、本机设备身份和随机 nonce。
- 响应方返回自己的 nonce 和 HMAC 证明。
- 发起方验证响应方证明，然后发送自己的 HMAC 证明。
- 响应方验证发起方证明。

本地保存的配对记录不能包含明文共享密钥。

配对记录包含：

- 对方 Tailscale IP。
- 对方显示名，如果可用。
- 首次配对时间。
- 最近连接时间。
- 最近观察到的路径类型，例如 direct 或 DERP。
- 从密钥派生出的本地配对标识，而不是密钥本身。

## UI 要求

主界面以直连配对为中心：

- 产品标题：`portshare`。
- 本机 Tailscale 状态面板：
  - Running 或未运行。
  - 本机 Tailscale IP。
  - 控制监听器状态。
  - 需要时显示 DNS 准备状态。
- 共享密钥输入：
  - 输入或更新当前密钥。
  - 显示 direct mode 是否 ready。
  - 如果用户直接配对但密钥为空，可以生成一个可分享的配对密钥并提示对方输入。
- 配对对端面板：
  - 输入对方 Tailscale IP 或名称。
  - 连接/配对操作。
  - 配对进度和诊断信息。
- 可信设备列表：
  - 设备名/IP。
  - 最近连接时间。
  - 最近一次 `tailscale ping` 得到的路径类型和延迟。
  - 后续提供删除配对操作。

不显示本地转发、远端端口、本地端口或业务流量代理入口。

## 安全提示

UI 必须明确提示：

- 共享密钥应像密码一样保管。
- 只和自己控制或明确可信的电脑配对。
- 当前 `portshare` 不通过 Tailscale Serve 或 Funnel 暴露业务端口。
- 当前 `portshare` 不关闭 Tailscale 本身的连通性。
- 如果两台电脑在关闭 `portshare` 后仍能访问彼此业务端口，那是 Tailscale ACL、Shields Up 和 Windows 防火墙的结果，不是 `portshare` 在转发。

认证失败应写入审计日志，记录对端 IP 和失败原因，但不能记录共享密钥。

## 架构

当前 direct-mode 内部区域：

- `internal/tailscale`：本机 Tailscale CLI、状态和诊断适配器。
- `internal/direct/protocol`：握手协议。
- `internal/direct/server`：控制监听器和配对请求处理。
- `internal/direct/client`：连接对端并执行认证握手。
- `internal/direct/store`：可信设备持久化。
- `internal/direct/manager`：ready 状态、控制监听、配对、诊断和审计事件编排。

旧的 `internal/provider/tailscale` 可以继续保留用于 legacy Serve/Funnel 行为，但 direct-mode 主流程不能调用 `tailscale serve` 或 `tailscale funnel`。

## 协议草案

协议运行在对端控制端口上的 TCP 连接中。握手使用带长度前缀的 JSON 控制消息。

最小消息类型：

- `hello`
- `hello_response`
- `auth_proof`
- `auth_ok`
- `auth_error`

所有控制消息都包含协议版本。版本不兼容时返回清晰错误。

## 测试策略

单元测试：

- HMAC 挑战响应在密钥一致时成功。
- HMAC 挑战响应在密钥不一致时失败。
- 保存的可信设备记录不包含明文共享密钥。
- Tailscale 诊断能分类 CLI 缺失、后端停止、无 IP、DNS 未接受、对端不可达等状态。
- UI controller 能归一化 Tailscale IP、host 和显式端口。
- UI controller 能把连接拒绝解释为对方未启用直连监听或被防火墙拦截。

loopback 集成测试：

- 在 loopback 测试地址和不同控制端口上启动 direct server。
- 使用共享密钥配对。
- 使用错误共享密钥确认配对失败。
- 关闭 server 后确认配对连接失败。

手动验收：

- 在同一 tailnet 的两台 Windows 电脑上运行 `portshare`。
- 确认两端都显示 Tailscale ready 和控制监听器 ready。
- 两端输入同一个共享密钥。
- 通过输入对端 Tailscale IP 完成配对。
- 确认可信设备列表出现对方设备。
- 关闭一端 `portshare` 后确认对方无法连接该端 `17890`。
- 确认关闭 `portshare` 不会关闭 Tailscale 自身连通性。

## 未来

- 单独设计公网转发。
- 增加可信设备删除。
- 在共享密钥 MVP 后增加更强的长期设备密钥。
- 如果设备级信任对真实用户过宽，再增加按对端或按能力的访问策略。
