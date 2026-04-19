# Telegram 多机器人网关 — 使用手册

## 1. 系统是什么

本系统将 **HTTP 入站告警/通知** 写入 PostgreSQL，按 **路由规则** 匹配到 **发送目标（Destination）**，再由 **Worker** 调用 **Telegram Bot API** 投递到指定 Chat/Topic。

- **公开入队接口**：`/api/v1/notify`、`/api/v2/notify`（供监控系统、脚本等调用）。
- **管理 API**：`/api/v2/*`（需登录 JWT + RBAC 权限）。
- **管理台前端**：独立仓库 `telegram-relay-admin`，构建产物可由 relay 进程托管静态文件。

## 2. 默认端口与覆盖方式

- **默认监听**：`:8780`（在 `internal/config/config.go` 的默认值中定义）。
- **覆盖**：环境变量 **`LISTEN_ADDR`**（如 `LISTEN_ADDR=:9090`）或 YAML `server.listen_addr`。

Docker / systemd 示例见仓库 `deploy/` 目录。

## 3. 外部程序如何推送消息

**可以。** 任意能发起 HTTPS/HTTP 的客户端，向本服务提交 **POST** 即可。

更完整的对接清单、示例代码与「多接入方密钥」能力边界见 **[第三方接入说明](./third-party-integration.md)**。

### 3.1 路径与请求体

| 路径 | 说明 |
|------|------|
| `POST /api/v1/notify` | 兼容旧版；请求体中 **`event_id` 必填** |
| `POST /api/v2/notify` | 平台化入队；**`event_id` 可省略**（服务会生成 `evt-<纳秒>`） |

JSON 字段与 `NotifyRequest` 一致：

| 字段 | 必填 | 说明 |
|------|------|------|
| `title` | 是 | 标题 |
| `message` | 是 | 正文 |
| `source` | 是 | 来源标识，用于规则匹配 |
| `level` | 否 | 级别，用于规则匹配 |
| `event_id` | v1 必填 | 幂等键；v2 可省略 |
| `event_time` | 否 | 展示用时间 |
| `labels` | 否 | JSON 对象，用于规则 `match_labels` |

### 3.2 鉴权（与 `security.level` 一致）

- **basic**：请求头 `Authorization: Bearer <AUTH_TOKEN>`（与配置中 `AUTH_TOKEN` / `security.token` 一致）。
- **medium**：在 basic 基础上增加 `X-Timestamp`（Unix 秒）与 `X-Signature`（见下）。
- **strict**：在 medium 基础上校验 **来源 IP** 是否在白名单（见 3.4）。

**HMAC 签名（medium/strict）**：

- 载荷字符串：`<timestamp>.<原始 JSON 请求体字符串>`
- `X-Signature` = `hex(HMAC_SHA256(HMAC_SECRET, 载荷))`（小写 hex）

时间戳与服务器时间差不得超过配置的 `timestamp_skew_sec`。

### 3.3 curl 示例（basic + v2）

```bash
curl -sS -X POST "http://127.0.0.1:8780/api/v2/notify" \
  -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"测试","message":"正文","source":"curl-demo","level":"info"}'
```

成功时响应类似：`{"event_db_id":123,"status":"queued"}`。

生产环境将 `http` 换成 **`https://你的域名`**，路径不变。

### 3.4 生产 HTTPS 与反向代理

- 常见做法：TLS 在 **Nginx / Caddy / 云 LB** 终结，内网再反代到 relay（如 `http://127.0.0.1:8780`）。
- **浏览器管理台**：`VITE_API_BASE_URL` 应指向 **HTTPS API 根地址**，避免页面 https、接口 http 的混合内容被拦截。
- **`security.level=strict`** 时，relay 看到的可能是 **反代内网 IP**。若需按真实客户端 IP 限制，应在反代上配置 **`X-Forwarded-For`**（或同类头），并在 relay 前使用支持传递客户端 IP 的方案；当前代码以 **`r.RemoteAddr`** 做白名单校验，部署在反代后时需自行评估是否改为信任转发头（若后续版本支持会在 README 单独说明）。

## 4. 管理端与权限（RBAC）

登录后 JWT 内携带 `permissions` 字符串数组。常见权限码：

| 权限码 | 能力 |
|--------|------|
| `bot.manage` | 机器人、发送目标 |
| `rule.manage` | 路由规则 |
| `event.read` | 事件、发送任务、仪表盘统计 |
| `audit.read` | 审计日志 |
| `user.manage` | 用户与角色 |
| `system.manage` | 超级权限（含上述能力） |

## 5. 测试推送（管理端）

具备 **`bot.manage`** 的账号可调用 **`POST /api/v2/notify-test`**（请求体与 v2 入队相同），由服务端使用当前登录用户写 **审计**，无需在浏览器中配置 `AUTH_TOKEN`。详见管理台「测试推送」页面（若已部署对应前端版本）。

## 6. 运维与可观测性

- 健康检查：`GET /healthz`
- 简单指标：`GET /metrics`
- 业务统计：`GET /api/v2/dashboard`（需 `event.read`）

更多见 **[operations-observability.md](./operations-observability.md)**。

## 7. 相关文档

- [宝塔接入示例](./baota-integration.md)
- [管理台能力清单](./admin-console-plan.md)
- [开发与提交规范](./development-rules.md)
