# 新加坡服务器代理搭建指南

## 方案：Shadowsocks-rust + Clash

### 一、服务器端配置（只需执行一次）

#### 1. 安装 Shadowsocks-rust

```bash
# 下载最新版本
wget https://github.com/shadowsocks/shadowsocks-rust/releases/download/v1.18.2/shadowsocks-v1.18.2.x86_64-unknown-linux-gnu.tar.xz

# 解压
tar -xf shadowsocks-v1.18.2.x86_64-unknown-linux-gnu.tar.xz

# 移动到系统目录
sudo mv ssserver /usr/local/bin/
sudo chmod +x /usr/local/bin/ssserver
```

#### 2. 创建配置文件

```bash
sudo mkdir -p /etc/shadowsocks
sudo nano /etc/shadowsocks/config.json
```

配置内容（记得修改密码）：

```json
{
  "server": "0.0.0.0",
  "server_port": 8388,
  "password": "your-strong-password-here",
  "timeout": 300,
  "method": "chacha20-ietf-poly1305"
}
```

#### 3. 创建 systemd 服务

```bash
sudo nano /etc/systemd/system/shadowsocks.service
```

写入以下内容：

```ini
[Unit]
Description=Shadowsocks Server
After=network.target

[Service]
Type=simple
User=nobody
ExecStart=/usr/local/bin/ssserver -c /etc/shadowsocks/config.json
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

#### 4. 启动服务

```bash
# 重载 systemd
sudo systemctl daemon-reload

# 启动服务
sudo systemctl start shadowsocks

# 设置开机自启
sudo systemctl enable shadowsocks

# 查看状态
sudo systemctl status shadowsocks
```

#### 5. 配置防火墙（如果有）

```bash
# Ubuntu/Debian
sudo ufw allow 8388/tcp
sudo ufw allow 8388/udp

# CentOS/RHEL
sudo firewall-cmd --permanent --add-port=8388/tcp
sudo firewall-cmd --permanent --add-port=8388/udp
sudo firewall-cmd --reload
```

**注意：记得在云服务器控制台的安全组中开放 8388 端口！**

---

## 二、客户端配置（Clash）

### 方式1：直接使用配置文件

创建 `clash-config.yaml` 文件：

```yaml
mixed-port: 7890
allow-lan: false
mode: rule
log-level: info

proxies:
  - name: "新加坡"
    type: ss
    server: YOUR_SERVER_IP
    port: 8388
    cipher: chacha20-ietf-poly1305
    password: your-strong-password-here
    udp: true

proxy-groups:
  - name: "代理选择"
    type: select
    proxies:
      - "新加坡"
      - DIRECT

rules:
  # 直连中国大陆
  - GEOIP,CN,DIRECT
  # 局域网直连
  - IP-CIDR,192.168.0.0/16,DIRECT,no-resolve
  - IP-CIDR,10.0.0.0/8,DIRECT,no-resolve
  - IP-CIDR,172.16.0.0/12,DIRECT,no-resolve
  - IP-CIDR,127.0.0.0/8,DIRECT,no-resolve
  # 其他走代理
  - MATCH,代理选择
```

### 方式2：订阅链接（推荐）

如果你想通过订阅链接导入，可以搭建一个简单的 HTTP 服务器提供配置：

```bash
# 在本地或服务器上创建一个配置文件
# 然后通过任何 HTTP 服务器托管
# 比如使用 GitHub Gist 或者 Cloudflare Pages
```

---

## 三、快速测试

### 测试服务器是否正常

```bash
# 在服务器上查看日志
sudo journalctl -u shadowsocks -f

# 测试端口是否开放
ss -tulpn | grep 8388
```

### 测试客户端连接

1. 打开 Clash
2. 导入配置文件
3. 启用系统代理
4. 访问 https://www.google.com 测试

---

## 四、进阶配置

### 启用 BBR 加速（提升速度）

```bash
# 检查内核版本（需要 4.9+）
uname -r

# 启用 BBR
echo "net.core.default_qdisc=fq" | sudo tee -a /etc/sysctl.conf
echo "net.ipv4.tcp_congestion_control=bbr" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p

# 验证
sysctl net.ipv4.tcp_congestion_control
```

### 修改密码

```bash
# 修改配置文件
sudo nano /etc/shadowsocks/config.json

# 重启服务
sudo systemctl restart shadowsocks
```

---

## 故障排查

### 服务无法启动

```bash
# 查看详细日志
sudo journalctl -u shadowsocks -n 50 --no-pager

# 检查配置文件格式
cat /etc/shadowsocks/config.json | python3 -m json.tool
```

### 无法连接

1. 检查服务器防火墙和安全组
2. 确认服务器 IP 地址正确
3. 确认密码一致
4. 检查端口是否被占用

### 速度慢

1. 启用 BBR 加速
2. 尝试更换加密方式（如 aes-256-gcm）
3. 检查服务器带宽

---

## 安全建议

1. **定期更换密码**：至少每3个月更换一次
2. **使用强密码**：至少16位，包含大小写字母、数字、特殊字符
3. **更换默认端口**：避免使用 8388，改为其他端口
4. **启用 fail2ban**：防止暴力破解
5. **定期更新**：保持 shadowsocks-rust 为最新版本

---

## 总结

- **服务器端**：安装 shadowsocks-rust，配置服务，开放端口
- **客户端**：导入配置到 Clash，选择代理模式
- **优化**：启用 BBR，使用强密码

完成以上步骤后，你就有了一个属于自己的、稳定的代理服务器！
