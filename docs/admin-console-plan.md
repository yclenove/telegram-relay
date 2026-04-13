# 管理台（telegram-relay-admin）演进计划

本文档跟踪管理端与 `/api/v2` 配套能力的完成情况；已完成项使用 `[x]` 标记。

## 一、认证与路由

- [x] 登录后 **permissions 持久化**（与 token 一并写入 localStorage），刷新后菜单可用
- [x] Vue Router **返回值式** `beforeEach`，权限路由与 `user.manage` / `destinations` / `users` 子路由
- [x] 路由切换后 **document.title** 与模块名同步

## 二、界面与中文化

- [x] Element Plus **中文 locale**、布局侧栏、登录与仪表盘美化
- [x] 统一 **错误文案**（`getErrorMessage`、MessageBox 取消静默）

## 三、机器人与发送目标

- [x] **GET/POST** `/api/v2/bots`；**PATCH/DELETE** `/api/v2/bots/{id}`（管理端表格编辑/删除）
- [x] **GET/POST** `/api/v2/destinations`（列表含 `bot_name`、新建表单）
- [x] **PATCH/DELETE** `/api/v2/destinations/{id}`（管理端编辑/删除发送目标）

## 四、路由规则

- [x] 规则列表与创建；目标 **下拉** 选择
- [x] **PATCH/DELETE** `/api/v2/rules/{id}`（管理端编辑/删除、启用与目标）

## 五、用户与 RBAC

- [x] 迁移与引导数据含 **`user.manage`**
- [x] **GET/POST/PATCH/DELETE** `/api/v2/users*`，**GET** `/api/v2/roles`；UsersView 角色展示名称

## 六、事件与审计

- [x] **GET** `/api/v2/events` 支持 **分页与筛选**（`limit`/`offset`、`source`/`level`/`status`），响应 `{ items, total }`
- [x] **GET** `/api/v2/audits` 支持 **分页与筛选**（`limit`/`offset`、`action`/`object_type`），响应 `{ items, total }`
- [x] 管理端 **EventsView / AuditsView** 分页器与筛选表单

## 七、工程与文档

- [x] README 管理接口说明与 **`.gitignore` 忽略 `.cursor/`**
- [x] 阶段构建：`go test ./...`、`npm run build`；管理端 `npm run test`（Vitest，`buildListQuery` 等）

---

说明：私密配置仍不入库；部署新接口后需 **重启 relay** 并视情况 **重新登录** 以刷新 JWT。

## 八、增强（已完成）

- [x] **发送任务**：`GET /api/v2/dispatch-jobs` 分页与 `status` 筛选（`event.read`），管理端「发送任务」页
- [x] **事件详情**：`GET /api/v2/events/{id}` 返回完整事件；事件列表「详情」抽屉
- [x] **审计扩展筛选**：`object_id`、`actor_user_id`、`created_after`/`created_before`（RFC3339）
- [x] **规则 match_labels**：创建/更新时 JSON 对象校验与持久化
- [x] **仪表盘**：近 24 小时事件数、失败任务数等指标
- [x] **角色只读增强**：`GET /api/v2/roles/{id}/permissions` 列出角色权限码；管理端「角色与权限」只读页
- [x] **前端 Vitest**：`buildListQuery` 工具函数单测
