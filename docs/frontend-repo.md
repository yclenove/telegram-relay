# 管理台前端（独立仓库）

管理台已从本仓库拆出，单独维护与发布：

- 后端本仓库 / Go 模块：**telegram-relay**（`github.com/yclenove/telegram-relay`）
- 前端仓库名：**telegram-relay-admin**
- 建议路径：与本仓库同级克隆，例如 `../telegram-relay-admin`

构建后将 **`admin-dist`** 目录通过环境变量提供给网关：

```bash
export ADMIN_STATIC_DIR=/path/to/telegram-relay-admin/admin-dist
```

若未配置 `ADMIN_STATIC_DIR` 或目录不存在，网关仅提供 API，不托管管理台静态页。

管理台功能阶段与接口完成情况见本仓库 [`admin-console-plan.md`](./admin-console-plan.md)。
