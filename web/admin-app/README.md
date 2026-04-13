# Telegram 网关管理台（admin-app）

基于 **Vue 3 + TypeScript + Vite + Element Plus + Pinia + Vue Router**，与后端 `/api/v2` 对接。

## 本地开发

1. 复制环境变量示例：

   ```bash
   cp .env.example .env
   ```

2. 编辑 `.env`，将 `VITE_API_BASE_URL` 设为后端地址，例如：

   ```env
   VITE_API_BASE_URL=http://127.0.0.1:8080
   ```

3. 安装依赖并启动：

   ```bash
   npm install
   npm run dev
   ```

## 生产构建

```bash
npm run build
```

产物输出到 `dist/`，与仓库根目录 Go 服务的 `ADMIN_STATIC_DIR=web/admin-app/dist` 对齐。

## 说明

- Token 存于 `localStorage`（键名见 `src/api/http.ts`），与 Pinia `auth` 仓库同步。
- 非登录接口返回 401 时会清理 token 并跳转登录页。
