# 宝塔自定义通知接入指南

> 若要把 **relay + PostgreSQL + 管理台** 部署在已安装宝塔的服务器上，请看更完整的 **[宝塔傻瓜部署](./deploy-baota-simple.md)**。本文仅说明「宝塔告警 Webhook 如何调用已运行的 relay」。

## 1. 基础信息

- 中转地址：`https://relay.yourdomain.com/api/v1/notify`
- 方法：`POST`
- 内容类型：`application/json`

## 2. 请求体模板建议

```json
{
  "title": "${TITLE}",
  "message": "${MSG}",
  "level": "${LEVEL}",
  "source": "baota",
  "event_time": "${TIME}",
  "event_id": "${EVENT_ID}",
  "labels": {
    "site": "${SITE}",
    "ip": "${SERVER_IP}"
  }
}
```

说明：具体变量名按你的宝塔版本替换。

## 3. Header 配置

### basic

- `Authorization: Bearer <AUTH_TOKEN>`

### medium / strict

在 `basic` 基础上增加：

- `X-Timestamp: <unix_timestamp>`
- `X-Signature: <hmac_sha256_hex>`

签名规则：`hex(HMAC_SHA256(HMAC_SECRET, "<timestamp>.<rawBody>"))`

## 4. 联调验证

使用测试脚本快速验证：

```bash
export RELAY_URL="https://relay.yourdomain.com/api/v1/notify"
export AUTH_TOKEN="replace-with-strong-token"
export SECURITY_LEVEL="medium"
export HMAC_SECRET="replace-with-hmac-secret"
python3 scripts/send_test.py
```

预期返回：

- HTTP 200
- JSON 中 `status` 为 `ok`

## 5. 常见问题

- 401：检查 Token 或签名/时间戳是否正确。
- 429：触发限流，调高 `RATE_LIMIT_PER_SECOND` 或降低通知频率。
- 502：中转服务访问 Telegram 失败，检查外网机到 `api.telegram.org` 网络连通性。
