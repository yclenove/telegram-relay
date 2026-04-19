# 第三方系统接入说明

面向需要将告警、工单、自定义业务事件推入本平台的**外部系统开发者 / 运维**。

更偏「产品使用」的说明见 [使用手册](./user-manual.md)；本文侧重**对接步骤、请求格式与排障**。

---

## 1. 能力与边界（必读）

### 1.1 当前已支持

| 能力 | 说明 |
|------|------|
| HTTP JSON 入队 | `POST /api/v1/notify` 或 `POST /api/v2/notify` |
| 鉴权 | `Authorization: Bearer <token>`；`security.level` 为 medium/strict 时需 `X-Timestamp` + `X-Signature` |
| 幂等 | v1 必填 `event_id`；v2 可省略，由服务生成 `evt-<纳秒>` |
| 限流 | 实例级全局限流（与路径无关，v1/v2 共用） |

### 1.2 当前未支持（与「一接入方一密钥」相关）

**本版本没有在平台内为「每个第三方接入方」单独签发、轮换、吊销独立 Bearer Token 或独立 API Secret 的能力。**

运行时仅读取配置中的 **一个** `security.token`（环境变量常为 `AUTH_TOKEN`），以及 medium/strict 下的 **一个** `HMAC_SECRET`。  
因此：

- 所有调用公开 `/api/v1|v2/notify` 的第三方在机密层面**共享同一套入站凭据**；
- 区分责任方更多依赖业务字段（如 **`source`**、`labels`）与上游网关日志，而不是凭据隔离。

若业务强需求「A 厂商泄露密钥不影响 B 厂商」，在现有产品上可采用：

1. **前置 API 网关 / BFF**：由你们自研或 WAF 为不同厂商配置不同 Header/路径，网关校验后再以**单一** relay 凭据调用本服务；
2. **多 relay 实例**：按接入域或安全域拆分部署，每实例一套 `AUTH_TOKEN`（运维与成本更高）；
3. **后续版本**：在管理台维护「入站凭证」表（名称、哈希、启用状态、可选绑定默认 `source` 前缀等），校验逻辑改为查库——**尚未在主线实现**，有需求可走需求评审与排期。

---

## 2. 环境与 URL

- **默认监听**：`:8780`（可用 `LISTEN_ADDR` 或 YAML `server.listen_addr` 覆盖）。
- **生产**：建议仅对公网暴露 **HTTPS**（TLS 在 Nginx / LB 终结，内网反代到 relay）。
- **路径**（与部署根路径无关时）：
  - `POST https://<你的域名>/api/v1/notify`
  - `POST https://<你的域名>/api/v2/notify`

---

## 3. 请求体（JSON）

与 `NotifyRequest` 一致（与仓库 `internal/model/notify.go` 对齐）：

| 字段 | v1 | v2 | 说明 |
|------|----|----|------|
| `title` | 必填 | 必填 | 标题 |
| `message` | 必填 | 必填 | 正文 |
| `source` | 必填 | 必填 | 来源标识，**强烈建议**与路由规则 `match_source` 一致，便于分发 |
| `level` | 可选 | 可选 | 如 `info` / `warning`，用于规则匹配 |
| `event_id` | **必填** | 可选 | 幂等键；v2 省略时由服务生成 |
| `event_time` | 可选 | 可选 | 展示用 |
| `labels` | 可选 | 可选 | JSON 对象 `{"k":"v"}`，用于规则 `match_labels` |

**成功响应示例**（v2，入队成功）：

```json
{"event_db_id": 123, "status": "queued"}
```

---

## 4. 鉴权：`security.level`

与部署配置 `security.level` 一致（见 `configs/config.public.example.yaml` 等）。

### 4.1 basic

请求头：

```http
Authorization: Bearer <与 security.token / AUTH_TOKEN 相同的值>
Content-Type: application/json
```

### 4.2 medium / strict

在 basic 基础上增加：

- `X-Timestamp`：Unix 秒级时间戳（与服务器时间差须在 `timestamp_skew_sec` 内）。
- `X-Signature`：小写十六进制字符串，算法为：

```text
payload = "<timestamp>." + <原始请求体字节转 UTF-8 字符串，与 body 完全一致>
X-Signature = hex( HMAC_SHA256( HMAC_SECRET, payload ) )
```

**strict** 另校验客户端 IP 是否在 `ip_whitelist`；经反向代理时注意 `RemoteAddr` 可能是反代 IP，详见 [使用手册](./user-manual.md) 中 HTTPS / 反代章节。

---

## 5. 调用示例

### 5.1 curl（basic + v2）

```bash
curl -sS -X POST "https://relay.example.com/api/v2/notify" \
  -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"订单异常","message":"库存不足","source":"order-service","level":"warning"}'
```

### 5.2 Python 3（basic）

```python
import json
import urllib.request

url = "https://relay.example.com/api/v2/notify"
body = {
    "title": "心跳",
    "message": "ok",
    "source": "cron-health",
    "level": "info",
}
req = urllib.request.Request(
    url,
    data=json.dumps(body).encode("utf-8"),
    headers={
        "Authorization": "Bearer YOUR_AUTH_TOKEN",
        "Content-Type": "application/json",
    },
    method="POST",
)
with urllib.request.urlopen(req, timeout=10) as resp:
    print(resp.read().decode())
```

### 5.3 Python 3（medium：带签名）

```python
import hashlib
import hmac
import json
import time
import urllib.request

url = "https://relay.example.com/api/v2/notify"
raw_body = json.dumps(
    {"title": "t", "message": "m", "source": "svc", "level": "info"},
    separators=(",", ":"),
)
ts = str(int(time.time()))
signing = f"{ts}.{raw_body}".encode("utf-8")
sig = hmac.new(b"YOUR_HMAC_SECRET", signing, hashlib.sha256).hexdigest()

req = urllib.request.Request(
    url,
    data=raw_body.encode("utf-8"),
    headers={
        "Authorization": "Bearer YOUR_AUTH_TOKEN",
        "Content-Type": "application/json",
        "X-Timestamp": ts,
        "X-Signature": sig,
    },
    method="POST",
)
with urllib.request.urlopen(req, timeout=10) as resp:
    print(resp.read().decode())
```

**注意**：参与签名的 `raw_body` 必须与 HTTP 请求体**逐字节一致**（含空格、字段顺序），否则签名校验失败。

---

## 6. 与路由、投递的关系

1. 事件入队后写入数据库，由 **路由规则** 决定发往哪个 **发送目标（Destination）**。
2. 请与平台管理员约定：`source` / `level` / `labels` 与规则的匹配关系，避免「入队成功但无规则命中」导致不投递或落入默认策略（若有）。

---

## 7. 常见错误码

| HTTP | 常见原因 |
|------|----------|
| 400 | JSON 非法、缺必填字段（如 v1 缺 `event_id`） |
| 401 | Bearer 错误、签名/时间窗错误、strict 下 IP 不在白名单 |
| 429 | 触发全局限流 |
| 502 | 入队写库失败等（查 relay 日志与数据库） |

---

## 8. 相关链接

- [使用手册](./user-manual.md)（RBAC、管理 API、运维）
- [快速指引](./user-quick-guide.md)
- [宝塔 / 环境示例](./baota-integration.md)
