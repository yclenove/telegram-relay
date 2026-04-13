# Telegram Multi-Bot Relay Platform

Go 模块：`github.com/yclenove/telegram-relay`（与 GitHub 仓库名 **telegram-relay** 对齐）。

将原有单机器人中转服务升级为可运营平台，支持 PostgreSQL、多机器人路由、RBAC、异步队列发送和可视化管理台。

## 核心能力

- 告警接入：`/api/v1/notify`（兼容）与 `/api/v2/notify`（平台化入口）
- 多机器人管理：机器人、目标 destination、路由规则
- 异步发送：事件入库后由 worker 轮询 `dispatch_jobs` 分发
- 权限系统：登录鉴权 + RBAC 权限检查 + 审计日志
- 可视化后台：独立仓库 **telegram-relay-admin**（构建 `dist` 后由 `ADMIN_STATIC_DIR` 挂载），调用 `/api/v2/*` 管理 API

## 主要接口

- 公开接口
  - `POST /api/v1/notify`
  - `POST /api/v2/notify`
- 管理端接口
  - `POST /api/v2/auth/login`
  - `GET/POST /api/v2/bots`
  - `POST /api/v2/destinations`
  - `GET/POST /api/v2/rules`
  - `GET /api/v2/events`
  - `GET /api/v2/audits`
  - `GET /api/v2/dashboard`

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
2. 设置私密配置中的 `database.dsn`、`auth.jwt_secret`、`auth.bootstrap_password`
3. 启动服务（自动执行 `migrations/*.sql`）

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

## 监控与演练

- 基础探针：`/healthz`、`/metrics`
- 平台统计：`/api/v2/dashboard`
- 参考文档：`docs/operations-observability.md`
- 压测脚本：`scripts/loadtest_notify.py`
