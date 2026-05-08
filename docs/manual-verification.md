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
