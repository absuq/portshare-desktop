# 2026-05-11 双机网络调优总结

## 现场结论

本次延迟升高不是 portshare 业务协议造成的，也不是 Tailscale 完全失去直连。现场状态是：

- Tailscale 内层显示 `direct`。
- Tailscale 外层公网 endpoint 落在香港代理/TUN 出口。
- Windows 默认 IPv4 和 IPv6 路由都优先走 `Meta` TUN。
- 当前远端 endpoint 是 IPv6，例如 `[2401:b60:1b::1033]:13674`。
- 旧版 portshare 只支持 IPv4 `/32` endpoint 绕过，无法处理这类 IPv6 direct endpoint。

因此实际问题是“直连但绕远路”：100.x 内层通了，但外层 UDP 打洞路径被代理/TUN 带到了高延迟地区。

## 排障顺序

1. 用 `tailscale status` 判断内层是 `direct`、`DERP` 还是 `peer-relay`。
2. 用 `tailscale ping --c 10 <peer>` 读取当前 endpoint 和延迟。
3. 用 `tailscale netcheck` 查看 Tailscale 看到的公网 IPv4/IPv6 和最近 DERP 地区。
4. 用 `Find-NetRoute -RemoteIPAddress <endpoint-ip>` 查看这个 endpoint 实际走哪个接口。
5. 用 `Get-NetRoute -DestinationPrefix '0.0.0.0/0','::/0'` 查看默认公网出口是否被 TUN 抢占。
6. 用 Clash/Mihomo 控制接口读取节点地区和延迟，优先选与物理地区接近、且切换后 Tailscale 能保持 direct 的节点。

## 沉淀到软件的能力

portshare 应把网络状态拆成三层展示：

- 内层 Tailscale：`direct`、`DERP`、`peer-relay`。
- 外层 endpoint：IPv4/IPv6、公网地址、当前路由接口、延迟。
- 代理/TUN 影响：是否走 `Meta`、`Clash`、`Mihomo`、`TUN` 等疑似代理接口。

优化动作也应保持最小影响：

- 不改系统默认路由。
- 不关闭代理。
- 不要求其他程序绕过代理。
- 只对当前 Tailscale direct endpoint 添加临时主机路由。
- IPv4 endpoint 使用 `/32`，IPv6 endpoint 使用 `/128`。
- 添加后必须重新 `restun`/`ping`/`Find-NetRoute` 验证，失败则撤销。

## 对用户可见的判断

- 如果显示 `direct` 但延迟高，同时当前出口是 `Meta`，应提示“直连但疑似代理绕路”。
- 如果 endpoint 是 IPv6，候选出口必须优先展示 IPv6 物理网卡，例如 `以太网 IPv6 -> fe80::1`。
- 如果 MagicDNS 默认解析成 `198.18.x.x`，应提示检查 fake-ip，并用 `100.100.100.100` 验证。
- 如果切换代理节点后 Tailscale 变 DERP 或更慢，应提示恢复原节点。
