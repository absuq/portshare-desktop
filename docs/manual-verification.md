# 手动验收

## 准备

1. 两台 Windows 电脑登录同一个 Tailscale tailnet。
2. 两台电脑都运行同一个版本的 `portshare`。
3. 双击启动 `portshare` 时应出现 Windows UAC 管理员授权提示；允许后继续。
4. 在两台电脑上执行 `tailscale ip -4`，记录各自的 Tailscale IP。

## Tailscale 诊断

1. 在两台电脑的 `portshare` 中点击“检测 Tailscale”。
2. 确认 UI 显示 `Tailscale：ready` 和本机 Tailscale IP。
3. 如果 MagicDNS 不能解析，先验证：

   ```powershell
   Resolve-DnsName <peer>.tailxxxx.ts.net -Server 100.100.100.100
   tailscale set --accept-dns=true
   ```

4. 如果对方控制端口不可达，检查：

   ```powershell
   Test-NetConnection <peer-tailscale-ip> -Port 17890
   tailscale status
   ```

## 直连配对

1. 两台电脑都输入同一个共享密钥。
2. 两台电脑都点击“启用直连密钥”，启动 `<本机 Tailscale IP>:17890` 控制监听。
3. 在电脑 A 输入电脑 B 的 Tailscale IP。
4. 点击“配对设备”。
5. 确认 UI 显示“已配对并授权全端口访问”。
6. 确认可信设备列表出现电脑 B，并显示“已授权全端口”。
7. 在电脑 B 输入电脑 A 的 Tailscale IP 并配对，确认可信设备列表出现电脑 A。
8. 使用不同共享密钥再试一次，确认配对失败并显示可理解的错误。

## 全端口访问验证

1. 在电脑 B 上选择一个实际正在监听的端口，例如某个本地服务端口。
2. 确认该服务不是只绑定到 `127.0.0.1`。如果服务只监听 `127.0.0.1`，电脑 A 通过电脑 B 的 Tailscale IP 仍然无法访问它。
3. 在电脑 A 上执行：

   ```powershell
   Test-NetConnection <computer-b-tailscale-ip> -Port <service-port>
   ```

4. 确认 `TcpTestSucceeded` 为 `True`。
5. 如果失败，在电脑 B 上检查是否有进程监听该端口：

   ```powershell
   netstat -ano | findstr :<service-port>
   ```

6. 检查 portshare 创建的 Windows 防火墙规则：

   ```powershell
   netsh advfirewall firewall show rule name=all | findstr portshare
   ```

## localhost 桥接验证

1. 在电脑 B 上启动一个只监听 localhost 的 TCP 服务。例如：

   ```powershell
   python -m http.server 18789 --bind 127.0.0.1
   ```

2. 在电脑 B 上确认服务只监听 `127.0.0.1`：

   ```powershell
   netstat -ano -p tcp | findstr :18789
   ```

3. 在电脑 B 的 `portshare` 中确认状态区出现：

   ```text
   localhost 桥接：18789
   ```

   如果没有出现，等待 5 秒后点击“检测 Tailscale”刷新状态。

4. 在电脑 A 上访问电脑 B 的 Tailscale IP 同端口：

   ```powershell
   Test-NetConnection <computer-b-tailscale-ip> -Port 18789
   curl.exe http://<computer-b-tailscale-ip>:18789/
   ```

5. 确认 `TcpTestSucceeded` 为 `True`，并且 HTTP 请求能返回电脑 B 上的本地服务内容。

## localhost 冲突提示验证

如果同一个端口同时存在 loopback 监听和原生可达监听，例如：

```text
127.0.0.1:3000 LISTENING
0.0.0.0:3000 LISTENING
```

`portshare` 不会创建 `100.x.x.x:3000 -> 127.0.0.1:3000` 桥接，因为 `100.x.x.x:3000` 已经会命中原生监听。UI 应显示：

```text
localhost 冲突：3000 原生监听，未桥接
```

此时远端访问：

```text
<computer-b-tailscale-ip>:3000
```

访问的是原生 `0.0.0.0:3000` 服务，不是 `127.0.0.1:3000` 服务。

## 网络路径与延迟优化验证

目标是区分两层状态：

- 内层：两台电脑的 Tailscale 100.x 地址是否 `direct`。
- 外层：Tailscale direct 使用的公网 endpoint 实际从哪个网卡和地区出去。

### 链路守护器验证

1. 配对完成后，确认可信设备列表中的延迟会自动刷新；刷新周期应明显小于 1 秒，通常约为 200ms。
2. 在“网络”页确认可以看到“链路守护”状态和“自动精确绕过”开关。
3. 保持“自动精确绕过”开启，选择对端可信设备后点击“重新优化”。
4. 如果当前已经是低延迟 direct，UI 应显示“当前已经是低延迟直连”或“TUN 接管但当前是低延迟直连”，并且不会新增临时路由。
5. 如果当前是 `direct + 高延迟 + Meta/Clash/Mihomo/TUN 出口`，UI 应自动选择推荐物理出口并应用 endpoint 精确绕过。
6. 点击“重新优化”后，状态文本应包含主页延迟样本，例如“主页延迟 23ms”，说明链路守护器复用了主页延迟检测，而不是另起第二套检测循环。
7. 关闭“自动精确绕过”后再次点击“重新优化”，如果仍是高延迟 TUN 绕路，UI 只提示可优化，不应自动新增 host route。

验收步骤：

1. 在 portshare 中选择可信设备，点击“检测网络路径”。
2. 如果显示 `直连但疑似代理绕路`，查看“当前出口”是否为 `Meta`、`Clash`、`Mihomo`、`TUN` 等虚拟网卡。
3. 如果显示 `TUN 接管但低延迟直连`，说明当前 `tailscale ping` 已经处于低延迟 direct，可先保持现状，不必强制绕过。
4. 查看候选公网出口，确认同时能看到 IPv4 和 IPv6 默认出口。IPv6 endpoint 应显示为 `/128` 主机路由，IPv4 endpoint 应显示为 `/32` 主机路由。
5. 选择与当前 endpoint 地址族一致的物理网卡出口，例如：

   ```text
   以太网 IPv6 -> fe80::1
   以太网 IPv4 -> 192.168.1.1
   ```

6. 点击“临时绕过代理”，确认 UI 显示类似：

   ```text
   临时路由：IPv6 2401:b60:1b::1033/128 -> fe80::1
   ```

   或：

   ```text
   临时路由：IPv4 115.233.222.82/32 -> 192.168.1.1
   ```

7. 再次点击“检测网络路径”，确认当前出口变为物理网卡，或延迟下降。
8. 如果延迟变高、变 DERP，或 endpoint 变化，点击“撤销绕过”，重新检测后再选择新的 endpoint 对应出口。

辅助命令：

```powershell
tailscale status
tailscale ping --c 10 <peer-tailscale-ip>
tailscale netcheck
Find-NetRoute -RemoteIPAddress <tailscale-direct-endpoint-ip>
Get-NetRoute -DestinationPrefix '0.0.0.0/0','::/0'
```

经验记录：

- `tailscale status` 显示 `direct` 只说明内层没有走 DERP；它不保证外层公网路径足够近。
- 如果 `tailscale netcheck` 看到的公网 IP 在香港，而物理宽带应在浙江/上海附近，通常说明 Tailscale 外层打洞流量被代理/TUN 接管。
- Clash/Mihomo 的 `PROCESS-NAME,tailscaled.exe,DIRECT` 在 TUN 模式下可能不可靠，因为连接元数据里的进程名可能为空。
- Clash Verge 的 `mixed-port` 例如 `7897` 是代理入口，不是 external-controller；真实控制口应从配置中的 `external-controller` 或 `external-controller-pipe` 读取。
- `Find-NetRoute` 显示 endpoint 走 `Meta` 不一定等于高延迟绕路；如果 peer endpoint 已变成双方物理公网 IP 且 `tailscale ping` 低于约 50ms，应按低延迟 direct 处理。
- MagicDNS 被 fake-ip 影响时，`Resolve-DnsName <peer>.ts.net -Server 100.100.100.100` 能返回正确 100.x，而默认 DNS 可能返回 198.18.x。
- 最小影响的优化手段不是改默认路由，也不是关闭代理，而是只给当前 Tailscale direct endpoint 添加临时主机路由。

## 关闭验证

1. 在电脑 B 退出 `portshare`。
2. 在电脑 A 执行：

   ```powershell
   Test-NetConnection <computer-b-tailscale-ip> -Port 17890
   ```

3. 确认 `TcpTestSucceeded` 为 `False`，或连接被拒绝。
4. 重新启动电脑 B 的 `portshare`，输入相同共享密钥并点击“启用直连密钥”。
5. 再次测试 `17890`，确认可以恢复连接。

## 边界说明

- 当前 MVP 不提供本地业务端口转发。
- 当前 MVP 不提供手动业务端口转发。
- 当前 MVP 会自动桥接只监听 `127.0.0.1` 的 TCP 服务，让可信设备通过本机 Tailscale IP 同端口访问。
- 当前 MVP 不桥接 UDP localhost 服务。
- 如果同端口已有 `0.0.0.0` 或 Tailscale IP 原生监听，localhost 同端口不会桥接，UI 会显示冲突提示。
- 配对成功后会为对方 Tailscale IP 写入本机 TCP/UDP 全端口入站允许规则。
- 关闭 `portshare` 只会停止 `portshare` 自己的控制监听，不会关闭 Tailscale。
- 没有服务监听的端口仍然不会连通。
- 只绑定 `127.0.0.1` 的 TCP 服务需要等待 `portshare` 自动桥接后才能通过 Tailscale IP 访问。
- 如果 Tailscale ACL 禁止两台设备互访，`portshare` 不能绕过 ACL。
- 如果关闭 `portshare` 后两台电脑仍能访问彼此某些端口，那是 Tailscale tailnet、Tailscale ACL、Windows 防火墙规则和服务监听状态共同决定的结果，不是 `portshare` 仍在转发。

## 记录结果

验收时记录：

- 两台电脑的 Tailscale IP。
- `17890` 控制端口是否可达。
- 配对成功/失败结果。
- Windows 防火墙规则是否创建成功。
- 实际服务端口是否能通过对方 Tailscale IP 访问。
- localhost-only TCP 服务是否自动桥接成功。
- 错误提示是否能说明下一步该检查什么。
- 关闭 `portshare` 后 `17890` 是否停止响应。
