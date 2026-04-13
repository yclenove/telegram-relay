# 管理台前端（独立仓库）

管理台已从本仓库拆出，单独维护与发布：

- 仓库名：**telegram-relay-admin**
- 建议路径：与本仓库同级克隆，例如 `../telegram-relay-admin`

构建后将 `dist` 目录通过环境变量提供给网关：

```bash
export ADMIN_STATIC_DIR=/path/to/telegram-relay-admin/dist
```

若未配置 `ADMIN_STATIC_DIR` 或目录不存在，网关仅提供 API，不托管管理台静态页。
