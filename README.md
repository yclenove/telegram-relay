# Telegram Multi-Bot Relay Platform

Go 模块：`github.com/yclenove/telegram-relay`（与 GitHub 仓库名 **telegram-relay** 对齐）。

将原有单机器人中转服务升级为可运营平台，支持 PostgreSQL、多机器人路由、RBAC、异步队列发送和可视化管理台。

## 核心能力

- 告警接入：`/api/v1/notify`（兼容）与 `/api/v2/notify`（平台化入口）
- 多机器人管理：机器人、目标 destination、路由规则
- 异步发送：事件入库后由 worker 轮询 `dispatch_jobs` 分发
- 权限系统：登录鉴权 + RBAC 权限检查 + 审计日志
- 可视化后台：独立仓库 **telegram-relay-admin**（构建 `dist` 后由 `ADMIN_STATIC_DIR` 挂载），调用 `/api/v2/*` 管理 API
- 管理台能力清单与阶段完成情况见 [`docs/admin-console-plan.md`](docs/admin-console-plan.md)

## 文档

- [快速功能指引](docs/user-quick-guide.md)：首次部署、管理台配置顺序、常见问题
- [使用手册](docs/user-manual.md)：概念说明、RBAC、外部 HTTP 接入（v1/v2、鉴权、反代注意）、运维链接

## 主要接口

- 公开接口
  - `POST /api/v1/notify`
  - `POST /api/v2/notify`：与 v1 **相同**的入站安全模型（`Authorization: Bearer` + 按 `security.level` 的 `X-Timestamp` / `X-Signature` / IP 白名单）以及**同一套全局限流**；请勿将实例端口暴露给不可信网络而不经网关或防火墙保护。
- 管理端接口
  - `POST /api/v2/auth/login`：响应含 `access_token`、`refresh_token`（长期）、`permissions`
  - `POST /api/v2/auth/refresh`：请求体 `{ "refresh_token": "<jwt>" }`，成功时返回新的 `access_token` 与 `refresh_token`（旋转刷新令牌）
  - `GET/POST /api/v2/bots`；`PATCH/DELETE /api/v2/bots/{id}`（需 `bot.manage`）
  - `GET/POST /api/v2/destinations`；`PATCH/DELETE /api/v2/destinations/{id}`（需 `bot.manage`）
  - `GET/POST /api/v2/rules`；`PATCH/DELETE /api/v2/rules/{id}`（需 `rule.manage`）
  - `GET /api/v2/events`：分页与筛选，响应 `{ items, total }`；`GET /api/v2/events/{id}` 单条详情（需 `event.read`）
  - `GET /api/v2/dispatch-jobs`：发送任务分页列表，响应 `{ items, total }`；查询参数 `limit`、`offset`、`status`（需 `event.read`）
  - `GET /api/v2/audits`：分页与筛选，响应 `{ items, total }`；查询参数含 `object_id`、`actor_user_id`、`created_after`/`created_before`（RFC3339）等（需 `audit.read`）
  - `GET /api/v2/dashboard`
  - `POST /api/v2/notify-test`：与 v2 入队相同 JSON 校验，经 JWT 入队（需 `bot.manage` 或 `system.manage`），并写审计 `notify.test`；供管理台「测试推送」，无需在浏览器配置 `AUTH_TOKEN`
  - `GET /api/v2/roles`；`GET /api/v2/roles/{id}/permissions`（只读权限码列表）；`GET/POST /api/v2/users`、`PATCH/DELETE /api/v2/users/{id}`（需 `user.manage` 或 `system.manage`）

### 入队语义说明

- **event_id**：v1 要求请求体必填 `event_id`；v2 若省略则由服务生成 `evt-<纳秒时间戳>` 作为幂等键。对接文档中请写明上游去重期望，避免混用导致语义不一致。
- **机器人 Token 存储**：库字段 `bot_token_enc` 当前为 **Base64 编码**（非加密）。若需防备份/磁盘泄露，应规划 KMS 或应用层对称加密；详见 `internal/repository/postgres/store.go` 中 `EncryptSecret` 注释。

## 配置文件（公私分离）

- 公开配置：`configs/config.public.yaml`
- 私密配置：`configs/config.private.yaml`（禁止入库）
- 模板：
  - `configs/config.public.example.yaml`
  - `configs/config.private.example.yaml`

启动时加载：

```bash
export CONFIG_PUBLIC_FILE="configs/config.public.yaml"
export CONFIG_PRIVATE_FILE="configs/config.private.yaml"
go run ./cmd/relay
```

## 本地启动（PostgreSQL）

1. 准备 PostgreSQL，并创建数据库 `telegram`
2. 在仓库根复制环境变量模板并填写：`copy .env.example .env`（PowerShell）或 `cp .env.example .env`（Unix）。  
   启动时若存在 `.env` 会自动加载；未设置 `DATABASE_DSN` 时可用 `PG_HOST` / `PG_USER` / `PG_PASSWORD` / `PG_DATABASE` 自动拼接（与 MCP 常用变量一致）。
3. 或使用私密 YAML：设置 `database.dsn`、`auth.jwt_secret`、`auth.bootstrap_password` 等（见 `configs/config.private.example.yaml`）
4. 启动服务（自动执行 `migrations/*.sql`）

```bash
go mod tidy
go run ./cmd/relay
```

快速探活（读取根目录 `.env` 中的 `PG_*` 拼 DSN，临时监听 `:18080`，请求 `/healthz` 后退出）：

```bash
bash scripts/exec-smoke.sh
```

Windows 可执行：`pwsh -File scripts/run-local-smoke.ps1`（依赖已安装的 Git Bash）。

## 管理台前端（独立 Git 仓库）

前端工程已拆为 **telegram-relay-admin**，与本文档仓库同级目录即可，详见 [docs/frontend-repo.md](docs/frontend-repo.md)。

```bash
cd ../telegram-relay-admin
npm install
npm run dev
```

构建产物 `dist/` 通过环境变量 `ADMIN_STATIC_DIR` 指向该目录，由后端托管静态文件（若未配置则仅 API）。

## Docker 启动

```bash
cd deploy
docker compose up -d --build
```

默认包含 `postgres` + `telegram-relay` 两个服务。

## 默认管理员

- 用户名：`admin`（可通过 `BOOTSTRAP_USERNAME` 覆盖）
- 密码：读取 `BOOTSTRAP_PASSWORD`

首次启动会自动初始化超级管理员与基础权限。

若你**曾用旧口令启动成功过**，之后只改了 `.env` 里的 `BOOTSTRAP_PASSWORD`，数据库里的哈希**不会自动变**，管理台会一直 **401**。处理方式：

1. 在 `.env` 中临时设置 **`BOOTSTRAP_PASSWORD_SYNC=true`**，保存后**重启 relay 一次**（启动日志会出现 `bootstrap password sync enabled`），再用新口令登录；
2. 登录成功后把 **`BOOTSTRAP_PASSWORD_SYNC` 改回 `false`**（或删除该行），避免长期每次启动都覆盖管理员密码。

## 监控与演练

- 基础探针：`/healthz`、`/metrics`
- 平台统计：`/api/v2/dashboard`
- 参考文档：`docs/operations-observability.md`
- 压测脚本：`scripts/loadtest_notify.py`
