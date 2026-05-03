# Linux 线上傻瓜部署（PostgreSQL + relay 二进制 + 可选 Nginx）

面向：**已有一台 Linux 云服务器（Ubuntu / Debian / CentOS 等）**，希望把本仓库的 **relay** 跑起来，并用浏览器打开**管理台**。按顺序做即可，不要求你先会 Docker。

---

## 0. 你需要准备什么

| 项目 | 说明 |
|------|------|
| 服务器 | 1 台 Linux，建议 2C4G 起；能访问公网（worker 要调 Telegram API） |
| 域名（可选） | 若要用 HTTPS，准备一个域名并解析到服务器 IP |
| 本机或 CI | 用来编译 Linux 二进制（或在服务器上装 Go 直接编） |

---

## 1. 安装 PostgreSQL 并建库

任选一种你熟悉的方式：

- 用系统包管理器安装 PostgreSQL 14+；或  
- 用云厂商托管 PostgreSQL（把连接信息记下来）。

在库里执行（库名可按需改，下文以 **`telegram`** 为例）：

```sql
CREATE DATABASE telegram;
```

创建一个专用账号（示例）：

```sql
CREATE USER relay WITH PASSWORD '这里换成强密码';
GRANT ALL PRIVILEGES ON DATABASE telegram TO relay;
-- PostgreSQL 15+ 若迁移报权限错误，再连接 telegram 库执行：
-- \c telegram
-- GRANT ALL ON SCHEMA public TO relay;
-- GRANT ALL ON ALL TABLES IN SCHEMA public TO relay;
-- （或开发阶段简单做法：ALTER DATABASE telegram OWNER TO relay;）
```

记下：**主机、端口、库名、用户名、密码**。

---

## 2. 在服务器上建目录、放文件

建议目录：`/opt/telegram-relay`（你也可以换别的，但下文路径要一起改）。

```bash
sudo mkdir -p /opt/telegram-relay
sudo chown "$USER":"$USER" /opt/telegram-relay
```

### 2.1 准备 `relay` 可执行文件

**方式 A：在你自己的电脑上交叉编译（常见）**

在 **telegram-notification** 仓库根目录执行：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o relay ./cmd/relay
```

把生成的 **`relay`** 上传到服务器的 `/opt/telegram-relay/`。

**方式 B：在服务器上安装 Go 后直接编译**

把整份源码上传到服务器，进入仓库根目录执行同上 `go build`。

### 2.2 必须同目录放置 `migrations`

启动时会自动执行迁移：进程当前工作目录下要有 **`migrations`** 文件夹。

从仓库复制整个 **`migrations`** 目录到 `/opt/telegram-relay/migrations`（与 `relay` 同级）。

可选：复制 **`configs`** 到同目录，便于以后改用 YAML；仅用环境变量时可不拷。

---

## 3. 写环境变量（`.env` 或 systemd）

在 **`/opt/telegram-relay`** 下创建 `.env`（权限建议 `chmod 600 .env`），内容按下面**整块替换**为你的真实值：

```bash
# —— 监听 —— #
LISTEN_ADDR=:8780

# —— 数据库（二选一：整串 DSN 或 PG_*）—— #
DATABASE_DSN=postgres://relay:你的密码@127.0.0.1:5432/telegram?sslmode=disable
# 若用分项，可注释掉上一行，改用：
# PG_HOST=127.0.0.1
# PG_PORT=5432
# PG_USER=relay
# PG_PASSWORD=你的密码
# PG_DATABASE=telegram

# —— 入站鉴权 —— #
SECURITY_LEVEL=basic
AUTH_TOKEN=请换成长随机串

# 若 SECURITY_LEVEL=medium 或 strict，必须再配：
# HMAC_SECRET=请换成长随机串

# —— 管理端 JWT 与首次管理员 —— #
JWT_SECRET=至少32字符的随机串
BOOTSTRAP_USERNAME=admin
BOOTSTRAP_PASSWORD=你的后台登录强密码
BOOTSTRAP_PASSWORD_SYNC=false

# —— Telegram（配置校验要求非空；实际投递以管理台里配置的机器人为准）—— #
TELEGRAM_BOT_TOKEN=从BotFather拿到的token
TELEGRAM_CHAT_ID=-100xxxxxxxxxx

# —— 可选：管理台静态资源（见第 6 节构建 admin 后填写）—— #
# ADMIN_STATIC_DIR=/opt/telegram-relay/admin-dist
```

说明：

- **`AUTH_TOKEN`**：外部系统调用 `POST /api/v1|v2/notify` 时 Header 里的 Bearer（见《第三方接入说明》）。  
- **`BOOTSTRAP_PASSWORD_SYNC`**：只有当你改过密码但登录仍 401 时，可临时改为 `true` 启动一次同步哈希，然后再改回 `false`。  
- **`TELEGRAM_*`**：当前版本启动校验要求必填；请至少填**真实可用的** Bot 与 Chat ID，避免 worker 发消息失败。

---

## 4. 第一次启动（建议先前台看日志）

```bash
cd /opt/telegram-relay
./relay
```

看到日志里 **`relay server started`**，且 **`listen_addr`** 为 `:8780`（或你设的端口）即正常。

另开终端测试：

```bash
curl -sS http://127.0.0.1:8780/healthz
```

应返回含 **`"status":"ok"`** 的 JSON。

按 **`Ctrl+C`** 停掉前台进程，继续下面用 systemd 常驻。

---

## 5. 用 systemd 守护（推荐）

创建 `/etc/systemd/system/telegram-relay.service`（**注意 `WorkingDirectory` 必须是放 `relay` 和 `migrations` 的目录**）：

```ini
[Unit]
Description=Telegram Relay
After=network.target postgresql.service

[Service]
Type=simple
User=relay
Group=relay
WorkingDirectory=/opt/telegram-relay
ExecStart=/opt/telegram-relay/relay
Restart=always
RestartSec=3

# 若不用 .env，可改成在这里写 Environment=...

[Install]
WantedBy=multi-user.target
```

创建系统用户（若还没有）：

```bash
sudo useradd --system --home /opt/telegram-relay --shell /usr/sbin/nologin relay
sudo chown -R relay:relay /opt/telegram-relay
```

若使用 `.env`，请确保 **`relay` 进程用户**对 `/opt/telegram-relay/.env` 有读权限。

启用并启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now telegram-relay
sudo systemctl status telegram-relay
```

---

## 6. 管理台（telegram-relay-admin）

管理台是**另一个仓库**，需要 **Node 18+** 构建。

在你电脑上：

```bash
git clone <你的 admin 仓库地址> telegram-relay-admin
cd telegram-relay-admin
npm ci
npm run build
```

把生成的 **`admin-dist`** 整个目录上传到服务器，例如 `/opt/telegram-relay/admin-dist`。

在 relay 的 `.env` 里增加：

```bash
ADMIN_STATIC_DIR=/opt/telegram-relay/admin-dist
```

重启 relay：

```bash
sudo systemctl restart telegram-relay
```

浏览器访问：`http://服务器IP:8780/`（或你反代后的域名），用 **`.env` 里的管理员账号**登录。

登录后务必完成：**机器人 → 发送目标 → 路由规则**；否则外部 `notify` 或「测试推送」可能提示没有发送目标。

---

## 7.（可选）Nginx 反向代理 + HTTPS

思路：Nginx 对外 443，反代到本机 `127.0.0.1:8780`；防火墙只放行 80/443。

示例站点配置片段（**按你的域名改 `server_name`，证书路径按 certbot/宝塔实际路径改**）：

```nginx
server {
    listen 443 ssl http2;
    server_name relay.example.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8780;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

若 `SECURITY_LEVEL=strict` 且校验客户端 IP，务必确认 **`X-Forwarded-For`** 与 relay 侧白名单配置一致（详见《使用手册》）。

---

## 8. 防火墙与安全组

- 云厂商安全组：若**不经 Nginx**，需放行 **`8780/tcp`**；若**经 Nginx**，只放行 **80/443**。  
- 系统防火墙（如 `ufw`）：与上同理。

---

## 9. 部署后检查清单

1. `curl https://你的域名/healthz` 或 `curl http://127.0.0.1:8780/healthz` 正常。  
2. 能打开管理台登录页并成功登录。  
3. 管理台里已配置 **默认机器人 + 发送目标 + 至少一条能命中的路由规则**（或明确用「来源/级别」与测试数据一致）。  
4. 宝塔/其它 Webhook 调 `notify` 时，**Bearer / HMAC** 与 `SECURITY_LEVEL` 一致（见 `docs/third-party-integration.md` 与 `docs/baota-integration.md`）。

---

## 10. 常见问题（只看这一段也能排）

| 现象 | 处理 |
|------|------|
| 启动报 `missing database dsn` | 配好 `DATABASE_DSN` 或 `PG_HOST`+`PG_USER` 等 |
| 启动报 `missing auth token` | 设置 `AUTH_TOKEN` |
| medium/strict 报缺 HMAC | 设置 `HMAC_SECRET` |
| 管理台 401 | 密码与库不一致；可短期 `BOOTSTRAP_PASSWORD_SYNC=true` 同步一次 |
| notify / 测试推送失败 | 多为**没有匹配路由且无默认目标**；在管理台补规则或默认机器人 |

更细的接口与运维说明见：[使用手册](./user-manual.md)、[快速功能指引](./user-quick-guide.md)。
