# 手动验收

## 准备

1. 两台 Windows 电脑登录同一个 Tailscale tailnet。
2. 两台电脑都运行同一个版本的 `portshare`。
3. 在两台电脑上执行 `tailscale ip -4`，记录各自的 Tailscale IP。

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
5. 确认可信设备列表出现电脑 B。
6. 在电脑 B 输入电脑 A 的 Tailscale IP 并配对，确认可信设备列表出现电脑 A。
7. 使用不同共享密钥再试一次，确认配对失败并显示可理解的错误。

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
- 当前 MVP 不会代理、开放或关闭对方的业务端口。
- 关闭 `portshare` 只会停止 `portshare` 自己的控制监听，不会关闭 Tailscale。
- 如果关闭 `portshare` 后两台电脑仍能访问彼此某些端口，那是 Tailscale tailnet、Tailscale ACL、Shields Up 或 Windows 防火墙共同决定的结果，不是 `portshare` 仍在转发。

## 记录结果

验收时记录：

- 两台电脑的 Tailscale IP。
- `17890` 控制端口是否可达。
- 配对成功/失败结果。
- 错误提示是否能说明下一步该检查什么。
- 关闭 `portshare` 后 `17890` 是否停止响应。
