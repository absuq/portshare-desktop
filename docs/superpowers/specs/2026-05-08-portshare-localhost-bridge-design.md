# portshare 自动 localhost 桥接设计

## 背景

配对和 Windows 防火墙授权已经能让可信设备访问本机通过 Tailscale IP 暴露的 TCP/UDP 端口。但很多开发工具、管理工具和本地服务默认只监听 `127.0.0.1`。这类服务即使本机可访问，远端也无法通过 `100.x.x.x:<port>` 访问，因为 `127.0.0.1` 是本机回环地址，不属于 Tailscale 网络接口。

用户目标是：两台电脑配对后，既能访问监听在 `0.0.0.0` 或 Tailscale IP 上的服务，也能访问只监听在 `127.0.0.1` 上的本地服务，体验接近局域网内设备。

## 目标

- 自动发现本机 TCP 监听端口。
- 对原生可达端口不做处理：
  - `0.0.0.0:<port>`
  - `<本机 Tailscale IP>:<port>`
- 对仅监听 loopback 的 TCP 端口自动桥接：
  - `<本机 Tailscale IP>:<port> -> 127.0.0.1:<port>`
- 只允许可信设备的 Tailscale IP 连接桥接端口。
- 端口出现时自动创建桥接，端口消失时自动关闭桥接。
- UI 显示 localhost 桥接数量和端口摘要。
- 保留现有配对、防火墙授权和控制监听行为。

## 非目标

- 不做 UDP localhost 桥接。UDP 会话保持和返回路径更复杂，后续单独实现。
- 不劫持所有系统流量。
- 不修改业务程序自身配置。
- 不桥接没有监听的端口。
- 不桥接已有原生 Tailscale 可达监听的端口。
- 不允许未配对设备访问桥接端口。

## 行为定义

本机扫描到如下监听：

```text
127.0.0.1:18789 LISTENING
0.0.0.0:52726 LISTENING
100.79.83.104:17890 LISTENING
```

`portshare` 行为：

```text
100.79.83.104:18789 -> 127.0.0.1:18789  创建桥接
100.79.83.104:52726                      不桥接，原生可达
100.79.83.104:17890                      不桥接，portshare 控制监听已占用
```

远端可信设备访问：

```text
100.79.83.104:18789
  -> portshare bridge
  -> 127.0.0.1:18789
```

## 安全模型

桥接监听绑定在本机 Tailscale IP 上，不绑定 `0.0.0.0`。桥接接受连接前检查远端 IP：

- 远端 IP 是可信设备 Tailscale IP：允许。
- 远端 IP 不在可信设备列表：立即关闭。

Windows 防火墙规则仍然作为第一层防护，桥接远端 IP 检查作为第二层防护。

## 架构

新增模块 `internal/localhostbridge`：

- `Scanner`：扫描本机 TCP 监听。
- `Planner`：根据监听状态、本机 Tailscale IP 和可信设备 IP 生成桥接计划。
- `Bridge`：一个端口的 TCP 代理，监听 `<tailscale-ip>:port`，转发到 `127.0.0.1:port`。
- `Controller`：周期性扫描并启动/停止桥接。

`internal/direct/manager` 持有 `localhostbridge.Controller`：

- 启用直连监听后启动 bridge controller。
- 配对或可信设备变化后刷新可信 IP 列表。
- 停止直连监听时关闭 bridge controller。

## UI

状态区新增一行：

```text
localhost 桥接：18789, 3000
```

没有桥接时显示：

```text
localhost 桥接：无
```

可信设备列表继续显示全端口授权。localhost 桥接是本机能力，不属于单个可信设备。

## 测试策略

- Planner 单元测试：
  - loopback-only 端口需要桥接。
  - `0.0.0.0` 端口不桥接。
  - Tailscale IP 原生监听端口不桥接。
  - 没有可信设备时不桥接。
- Bridge 集成测试：
  - loopback echo server 经 bridge 可访问。
  - 未授权远端 IP 被拒绝。
- Controller 单元测试：
  - 扫描出现端口时启动桥接。
  - 扫描消失端口时停止桥接。
- UI controller 测试：
  - 状态显示 localhost 桥接端口。
- 手动验收：
  - 在 `127.0.0.1:18789` 启动 TCP 服务。
  - 确认 `100.x.x.x:18789` 可从已配对设备访问。
