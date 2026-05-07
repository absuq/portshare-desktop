# 手动验收

## 准备

1. 两台 Windows 电脑登录同一个 Tailscale tailnet。
2. 两台电脑都运行 `portshare`。
3. 在电脑 B 启动测试服务：

   ```powershell
   python -m http.server 3000 --bind 127.0.0.1
   ```

4. 在两台电脑上执行 `tailscale ip -4`，记录各自的 Tailscale IP。

## Tailscale 诊断

1. 在 `portshare` 点击“检测 Tailscale”。
2. 确认 UI 显示 `Tailscale：ready` 和本机 Tailscale IP。
3. 如果 MagicDNS 不能解析，先验证：

   ```powershell
   Resolve-DnsName <peer>.tailxxxx.ts.net -Server 100.100.100.100
   tailscale set --accept-dns=true
   ```

## 直连配对

1. 两台电脑都输入同一个共享密钥。
2. 两台电脑都点击“启用直连密钥”，启动 `<本机 Tailscale IP>:17890` 控制监听。
3. 在电脑 A 输入电脑 B 的 Tailscale IP。
4. 点击“配对设备”。
5. 确认可信设备列表出现电脑 B。
6. 使用不同共享密钥再试一次，确认配对失败。

## TCP 转发

1. 在电脑 A 选中电脑 B。
2. 远端 host 填 `127.0.0.1`。
3. 远端端口填 `3000`。
4. 本地端口填 `18080`，或留空自动分配。
5. 点击“创建本地转发”。
6. 在电脑 A 访问：

   ```powershell
   curl.exe -i http://127.0.0.1:18080/
   ```

7. 确认返回电脑 B 的测试服务内容。
8. 点击“停止选中转发”。
9. 再次访问 `http://127.0.0.1:18080/`，确认无法连接。

## 关闭验证

1. 在电脑 B 退出 `portshare` 或停止直连监听。
2. 在电脑 A 再次访问本地转发地址，确认无法返回业务内容。
3. 重新启动电脑 B 的 `portshare` 并启用相同共享密钥，确认可以再次创建转发。

## 记录结果

验收时记录：

- 两台电脑的 Tailscale IP。
- 是否 direct 或 DERP 路由。
- 配对成功/失败结果。
- 本地转发端口。
- 停止转发后是否不可访问。
