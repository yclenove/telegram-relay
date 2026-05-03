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
| 多接入方入站凭证（可选） | 除全局 `AUTH_TOKEN` 外，可在管理台创建「入站凭证」：Bearer 明文为 **`key_id` + `.` + `secret`**（整串参与 bcrypt 存库）；每条凭证有**独立 HMAC 密钥**；事件会记录 `ingest_credential_id`（便于审计与排障） |
| 幂等 | v1 必填 `event_id`；v2 可省略，由服务生成 `evt-<纳秒>` |
| 限流 | 实例级全局限流（与路径无关，v1/v2 共用） |

### 1.2 全局凭据与「入站凭证」如何并存

- **全局**：仍使用配置中的 `security.token`（常为环境变量 `AUTH_TOKEN`）与 medium/strict 下的 `HMAC_SECRET`。适合单团队或网关统一代发。
- **按接入方（数据库）**：具备 `ingest_credential.manage` 的管理员可在管理台创建/禁用/轮换「入站凭证」。第三方请求头仍为 `Authorization: Bearer <整串>`，其中 `<整串>` 为创建成功时一次性展示的 **`key_id.secret`**（`secret` 为服务端随机段，**不是** key_id 本身）。
- **校验顺序**：若 Bearer 与全局 token **长度相同**，会先做常量时间比较以匹配全局 token；否则解析首段为 `key_id`、余下为 `secret`，查库校验 **整条** Bearer 的 bcrypt 哈希；medium/strict 下使用该行的 **独立 HMAC 密钥** 验签（与全局 `HMAC_SECRET` 二选一，取决于本次请求走的是哪套凭据）。

轮换后旧 `secret` 与旧 HMAC 立即失效；禁用后该行不再参与鉴权。

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
| `source` | 必填 | 建议填写；使用**数据库入站凭证**鉴权时可省略，relay 会用该凭证在管理台的**名称**，若名称为空则用 `ingest-<key_id>`（与路由 `match_source` 对齐可减少误投） |
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
Authorization: Bearer <全局 AUTH_TOKEN，或管理台签发的 key_id.secret 整串>
Content-Type: application/json
```

`<...>` 为以下**之一**即可通过 basic 校验：

- 与 `security.token` / `AUTH_TOKEN` **完全相同**的字符串；或  
- 管理台「入站凭证」创建/轮换时展示的 **`plain_token`**（格式 `key_id` + `.` + `secret`）。

### 4.2 medium / strict

在 basic 基础上增加：

- `X-Timestamp`：Unix 秒级时间戳（与服务器时间差须在 `timestamp_skew_sec` 内）。
- `X-Signature`：小写十六进制字符串，算法为：

```text
payload = "<timestamp>." + <原始请求体字节转 UTF-8 字符串，与 body 完全一致>
X-Signature = hex( HMAC_SHA256( <密钥>, payload ) )
```

其中 **`<密钥>`**：使用全局 `AUTH_TOKEN` 入站时为配置项 `HMAC_SECRET`；使用某条「入站凭证」入站时为该凭证一次性下发的 **`plain_hmac_secret`**（与 `HMAC_SECRET` 彼此独立）。

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

### 5.3 PHP（basic，需 `allow_url_fopen`）

```php
<?php
$url = 'https://relay.example.com/api/v2/notify';
$token = 'YOUR_AUTH_TOKEN';
$body = json_encode([
    'title' => '心跳',
    'message' => 'ok',
    'source' => 'cron-health',
    'level' => 'info',
], JSON_UNESCAPED_UNICODE);
$opts = [
    'http' => [
        'method' => 'POST',
        'header' => "Authorization: Bearer {$token}\r\nContent-Type: application/json\r\n",
        'content' => $body,
        'timeout' => 10,
    ],
];
$ctx = stream_context_create($opts);
$res = file_get_contents($url, false, $ctx);
if ($res === false) {
    fwrite(STDERR, "request failed\n");
    exit(1);
}
echo $res;
```

使用「入站凭证」且希望由服务端填默认 `source` 时，可从上述 JSON 中去掉 `source` 键（名称非空则用名称，否则 `ingest-<key_id>`）。

### 5.4 Python 3（medium：带签名）

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

**注意**：参与签名的 `raw_body` 必须与 HTTP 请求体**逐字节一致**（含空格、字段顺序），否则签名校验失败。若 JSON 省略 `source` 且使用入站凭证，验签仍针对**客户端发出的原始字节**；relay 在验签通过之后才会为路由解析补齐 `source` 并入库（可能与发送体字段集合不一致，属预期行为）。

---

## 6. 与路由、投递的关系

1. 事件入队后写入数据库，由 **路由规则** 决定发往哪个 **发送目标（Destination）**。
2. 请与平台管理员约定：`source` / `level` / `labels` 与规则的匹配关系，避免「入队成功但无规则命中」导致不投递或落入默认策略（若有）。
3. 若使用数据库「入站凭证」，事件行会带上 **`ingest_credential_id`**（全局 `AUTH_TOKEN` 入站时该字段为空），便于在事件列表或审计中区分接入方。

---

## 7. 常见错误码

| HTTP | 常见原因 |
|------|----------|
| 400 | JSON 非法、缺必填字段（如 v1 缺 `event_id`；全局 Token 入站时缺 `source`） |
| 401 | Bearer 错误、签名/时间窗错误、strict 下 IP 不在白名单 |
| 429 | 触发全局限流 |
| 502 | 入队写库失败等（查 relay 日志与数据库） |

---

## 8. 相关链接

- [使用手册](./user-manual.md)（RBAC、管理 API、运维）
- [快速指引](./user-quick-guide.md)
- [宝塔 / 环境示例](./baota-integration.md)
