# 快速功能指引

面向「第一次把 relay + 管理台跑起来」的操作顺序，按步骤做即可。

## 1. 启动前检查

1. **PostgreSQL** 已创建库（默认库名可为 `telegram`），且网络可达。
2. 已准备 **`.env` 或 YAML 私密配置**，至少包含：`AUTH_TOKEN`、`JWT_SECRET`、`BOOTSTRAP_USERNAME` / `BOOTSTRAP_PASSWORD`、`TELEGRAM_BOT_TOKEN`、`TELEGRAM_CHAT_ID`、数据库 DSN（或 `PG_*` 分项）。
3. **监听端口**：默认 **`LISTEN_ADDR` 未设置时为 `:8780`**（避免与本机 8080 冲突）。若需其它端口，设置环境变量 `LISTEN_ADDR=:你的端口`。
4. 若 Telegram 需走本地代理（如 v2rayN），设置 **`TELEGRAM_PROXY`**，例如 `http://127.0.0.1:10809`。

## 2. 启动 relay

```bash
go run ./cmd/relay
```

看到日志 `relay server started` 且 `listen_addr` 与预期一致后，访问 `http://127.0.0.1:8780/healthz` 应返回 `{"status":"ok"}`。

## 3. 启动管理台（独立仓库）

在 **telegram-relay-admin** 目录：

```bash
cp .env.example .env
# 确认 VITE_PROXY_TARGET 指向 relay（默认 http://127.0.0.1:8780）
npm install && npm run dev
```

浏览器打开 Vite 提示的本地地址，用引导账号登录。

## 4. 控制台配置顺序（推荐）

1. **机器人**：添加 Bot Token、名称；至少一个启用中的机器人。
2. **发送目标**：绑定 Chat ID / Topic、解析模式（HTML 等），关联到上一步的机器人。
3. **路由规则**：设置匹配条件（来源、级别、labels）与优先级，指向发送目标。
4. **事件中心 / 发送任务**：入队后在此观察事件与异步投递状态。

## 5. 验证「能收到 Telegram」

- 用外部脚本或管理台 **「测试推送」**（若已部署）发一条测试事件；或调用 `POST /api/v2/notify`（需鉴权，见《使用手册》）。
- 在 **发送任务** 中查看是否从 `pending` 变为成功；失败时看 `last_error`。

## 6. 常见问题

| 现象 | 可能原因 |
|------|----------|
| 管理台 401 | 密码与库中哈希不一致；可短期开启 `BOOTSTRAP_PASSWORD_SYNC=true` 同步一次后关闭 |
| 管理台接口全失败 | `VITE_PROXY_TARGET` 与 relay 实际端口不一致 |
| `/api/v2/notify` 401 | 缺少 `Authorization: Bearer`，或 `security.level` 要求 HMAC 但未带 `X-Timestamp`/`X-Signature` |
| 任务一直 pending / 失败 | Telegram Token、Chat ID、网络或 `TELEGRAM_PROXY` 配置错误 |
| strict 下 401 | 来源 IP 不在白名单；若经反代，需配置真实客户端 IP 传递（见《使用手册》HTTPS 章节） |

更完整的 API 与生产部署说明见 **[user-manual.md](./user-manual.md)**。
