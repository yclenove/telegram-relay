# Telegram Notification Relay

将国内服务器（如宝塔告警）产生的告警消息转发到 Telegram Bot API。

## 功能

- `POST /api/v1/notify` 统一告警入站接口
- 三档安全级别：`basic` / `medium` / `strict`
- 失败重试（指数退避）
- 基础可观测性：`/healthz` 和 `/metrics`
- 同时支持 Docker 与 systemd 部署

## 请求协议

请求路径：`POST /api/v1/notify`

请求体（JSON）：

```json
{
  "title": "CPU usage high",
  "message": "Host web-01 cpu > 90%",
  "level": "critical",
  "source": "baota",
  "event_time": "2026-04-13T09:00:00Z",
  "event_id": "evt-20260413-001",
  "labels": {
    "host": "web-01",
    "env": "prod"
  }
}
```

必填字段：`title`、`message`、`source`、`event_id`。

## 安全级别

### 1) basic

- Header: `Authorization: Bearer <AUTH_TOKEN>`

### 2) medium

- 继承 `basic`
- 新增：
  - `X-Timestamp`: Unix 秒级时间戳
  - `X-Signature`: `hex(HMAC_SHA256(hmac_secret, "<timestamp>.<rawBody>"))`

### 3) strict

- 继承 `medium`
- 再启用：
  - `IP_WHITELIST` 来源地址白名单
  - 全局限流（`RATE_LIMIT_PER_SECOND` + `RATE_LIMIT_BURST`）

## 环境变量

完整字段参考 `configs/config.example.yaml`。常用变量：

- `LISTEN_ADDR` 默认 `:8080`
- `SECURITY_LEVEL` (`basic|medium|strict`)
- `AUTH_TOKEN`
- `HMAC_SECRET`（`medium/strict` 必填）
- `IP_WHITELIST`（逗号分隔，仅 strict）
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_CHAT_ID`

## 配置安全分层（推荐）

建议拆分为两份：

- 公开配置：`configs/config.public.yaml`（可入库）
- 私密配置：`configs/config.private.yaml`（已在 `.gitignore` 忽略，不入库）

示例模板：

- `configs/config.public.example.yaml`
- `configs/config.private.example.yaml`

启动时通过环境变量加载：

```bash
export CONFIG_PUBLIC_FILE="configs/config.public.yaml"
export CONFIG_PRIVATE_FILE="configs/config.private.yaml"
go run ./cmd/relay
```

## 本地运行

```bash
go mod tidy
go run ./cmd/relay
```

## Docker 运行

```bash
cd deploy
docker compose up -d --build
```

## systemd 运行

1. 构建二进制：`go build -o relay ./cmd/relay`
2. 上传到 `/opt/telegram-relay/relay`
3. 安装服务文件：`deploy/relay.service` 到 `/etc/systemd/system/relay.service`
4. 执行：
   - `sudo systemctl daemon-reload`
   - `sudo systemctl enable --now relay`
   - `sudo systemctl status relay`

## 宝塔对接示例

在宝塔自定义通知中指向你的外网地址：

- URL: `https://relay.yourdomain.com/api/v1/notify`
- Method: `POST`
- Header: 至少包含 `Authorization`
- Body: 使用 JSON 模板映射宝塔变量到协议字段

如果你启用 `medium/strict`，需要保证 Header 中能带上 `X-Timestamp` 和 `X-Signature`。如果宝塔模板不支持动态签名，建议在国内服务器先调用一个本地签名脚本，再转发到中转服务。
