# 宝塔面板傻瓜部署（已安装宝塔 + Linux）

面向：**服务器上已经装好宝塔（BT Panel）**，要把 **PostgreSQL + relay + 管理台** 跑在**同一台机**或数据库在另一台也可（把连接地址改成实际 IP）。

与「宝塔里只配一条 Webhook 调 notify」不同，本文是 **把整个 relay 服务部署在宝塔机器上**。若你只想让宝塔告警调用已有中转地址，请看：[宝塔自定义通知接入](./baota-integration.md)。

---

## 0. 你会用到宝塔里的哪些功能

| 宝塔功能 | 用途 |
|----------|------|
| **软件商店** | 安装 **PostgreSQL**（或 **Docker** 跑 Postgres） |
| **数据库** | 创建库 `telegram`、账号、密码 |
| **文件** | 上传 `relay` 二进制、`migrations`、管理台 **`admin-dist`**（`npm run build` 产物目录） |
| **网站 → Go项目**（新版宝塔） | **推荐**：添加 Go 项目、端口、域名、环境变量，面板自动守护进程 + 反代 |
| **Supervisor 管理器** | **备选**：不用「Go项目」时，用手动守护进程（见下文「方式二」） |
| **网站 → SSL** | 给已绑定域名申请证书、强制 HTTPS |
| **网站 → 配置文件** | 与 **telegram-query-bot** 同域共存时，编辑 Nginx `server{}` 增加 `/api/v2` 与 `/relay/`（见第 6 节） |

---

## 1. 安装 PostgreSQL

1. 打开宝塔 → **软件商店** → 搜索 **PostgreSQL** → 安装（版本 14+ 即可）。  
2. 安装完成后，在 **数据库** → **添加数据库**：  
   - 数据库名：`telegram`（可自定，与下文 `.env` / 环境变量一致即可）  
   - 用户名/密码：自己设强密码，**记下来**。  
3. 确认 **PostgreSQL 监听本机**（默认 `127.0.0.1:5432` 即可）。若 relay 跑在同一台机，无需对公网开放 5432。

---

## 2. 放 relay 程序与迁移文件

1. 在宝塔 **文件** 中创建目录，例如：`/www/wwwroot/telegram-relay`（路径可自定，下文以此为准）。  
2. 在你电脑上用交叉编译生成 Linux 二进制（在 **telegram-notification** 仓库根目录）：

   ```bash
   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o relay ./cmd/relay
   ```

3. 上传到该目录：  
   - 文件 **`relay`**  
   - 整个文件夹 **`migrations`**（与仓库里一致，**必须**与 `relay` 在同一目录下）  
4. 给执行权限（在宝塔终端或 SSH 执行）：

   ```bash
   chmod +x /www/wwwroot/telegram-relay/relay
   ```

---

## 3. 配置密钥：`.env` 或面板「环境变量」（二选一）

relay 启动时会：

- 从**当前工作目录**下的 **`.env`** 读环境变量（若存在）；  
- 并读取**当前工作目录**下的 **`migrations`** 做数据库迁移。

因此 **工作目录必须是** `/www/wwwroot/telegram-relay`（即 `relay` 与 `migrations` 所在目录）。宝塔「Go项目」一般会把项目目录设成执行文件所在目录；若你遇到「迁移失败 / 找不到 migrations」，请改用 **方式二 Supervisor** 并显式填写运行目录。

### 3.1 用 `.env`（与 [Linux 傻瓜部署](./deploy-linux-simple.md) 第 3 节相同）

在 **`/www/wwwroot/telegram-relay/.env`** 写好变量（至少包含 `LISTEN_ADDR`、`DATABASE_DSN` 或 `PG_*`、`AUTH_TOKEN`、`JWT_SECRET`、`BOOTSTRAP_*`、`TELEGRAM_*` 等）。

权限建议（SSH）：

```bash
chmod 600 /www/wwwroot/telegram-relay/.env
chown -R www:www /www/wwwroot/telegram-relay
```

### 3.2 或只在宝塔「环境变量」里写

若面板支持多行 `KEY=value`，可把 `.env` 里内容**原样贴进「添加 Go 项目」→ 环境变量**（此时可不建 `.env`，二选一即可，避免重复）。

---

## 4. 方式一：宝塔「网站 → Go项目 → 添加 Go 项目」（与面板一致）

1. 宝塔左侧 **网站** → 子项 **Go项目**（若没有，在软件商店安装 **Go 项目管理器** 或类似官方插件，以你面板实际名称为准）。  
2. 点击 **添加项目**，弹出表单后按下面填写。

| 表单字段 | 建议填写 |
|----------|----------|
| **项目执行文件** | 点文件夹图标，选 **`/www/wwwroot/telegram-relay/relay`**（必须指向已 `chmod +x` 的二进制）。 |
| **项目名称** | 任意，如 **`telegram-relay`**。 |
| **项目端口** | **`8780`**（须与 `LISTEN_ADDR=:8780` 一致；若你改成 `:9090`，这里也要改成 `9090`）。 |
| **项目命令行** | 一般面板会根据执行文件自动生成，形如 **`/www/wwwroot/telegram-relay/relay`**；**不要**随便加参数，除非你知道含义。 |
| **运行用户** | 选 **`www`**（与上文 `chown www:www` 一致，避免读不到 `.env`）。 |
| **项目域名** | 填你的域名，如 **`relay.example.com`**（面板会为该域名配置到本机端口 **8780** 的反代；具体以宝塔生成配置为准）。 |
| **启动参数** | **留空**（relay 无子命令）。 |
| **环境变量** | 若未使用 `.env`，在此粘贴全部变量（一行一个 `KEY=value`）；若已用 `.env` 且工作目录正确，可留空。 |

3. 提交后，在列表里 **启动** 项目，点 **日志** 查看是否出现 **`relay server started`**。  
4. 自检（宝塔 **终端**）：

   ```bash
   curl -sS http://127.0.0.1:8780/healthz
   ```

5. **SSL**：到 **网站** 列表里找到该域名对应站点（或由 Go 项目自动创建的站点）→ **SSL** → 申请 Let’s Encrypt 并开启 **强制 HTTPS**。  
6. 若 `SECURITY_LEVEL=strict` 且依赖真实客户端 IP，请在 Nginx 里确认已传递 **`X-Forwarded-For`** / **`X-Real-IP`**（与 [使用手册](./user-manual.md) 中反代说明一致）。

---

## 5. 方式二：Supervisor 守护（不用「Go项目」时）

1. 宝塔 → **软件商店** → **Supervisor 管理器** → 添加守护进程。  
2. **运行目录**：`/www/wwwroot/telegram-relay`（**必须**含 `migrations`）。  
3. **启动命令**：`/www/wwwroot/telegram-relay/relay`  
4. 用户：**`www`**。  
5. 启动后同样用 `curl http://127.0.0.1:8780/healthz` 自检。  
6. 域名与反代：按第 **8** 节手动 **添加站点** 并反代到 `127.0.0.1:8780`（勿与 Go 项目重复占用同一端口反代两次）。

---

## 6. 与 telegram-query-bot 同域共存（根站 query + relay）

适用场景：**同一域名**（例如 `api.example.com`）下 **根路径 `/` 已是 query-bot 管理端**（静态 `root` + `/api/`、`/v3/` 等反代到 Java），希望 **不改动 query 的 Webhook 与 Spring 路径**，在同一站点上增加 **relay**。

### 6.1 访问路径约定

| 路径 | 指向 | 说明 |
|------|------|------|
| `/` | query-bot 前端 `dist` | 保持现有 `root` + `try_files` |
| `/api/webhook/`、`/api/admin/` 等 | Java（如 `127.0.0.1:18089`） | 仍走原有 `location ^~ /api/` |
| **`/api/v2/`** | **relay**（`127.0.0.1:8780`） | 管理端与公开 `notify` 共用此前缀 |
| **`/relay/`** | **relay**（`127.0.0.1:8780`） | relay 管理台 SPA；上游路径需**剥掉** `/relay` 前缀 |

第三方入队 URL 示例：**`https://你的域名/api/v2/notify`**（与 `docs/third-party-integration.md` 一致，仅域名换成你的）。

### 6.2 Nginx 为何不能只靠一条 `location ^~ /api/`

若已有：

```nginx
location ^~ /api/ {
    proxy_pass http://127.0.0.1:18089;
    # ...
}
```

则 **`/api/v2/notify` 也会被送进 Java**。Nginx 对前缀 `location` 会选 **最长匹配**；因此必须再增加 **更长的** `location ^~ /api/v2/`，单独反代到 relay。  
**书写顺序无关**：`^~ /api/v2/` 与 `^~ /api/` 谁写在上面都可以，只要两段都存在。

### 6.3 插入 Nginx 的配置片段（放进该域名的 `server{}` 内）

放在现有 **`location ^~ /api/`** 块**上方或下方均可**（推荐紧挨着 `/api/`，便于维护）：

```nginx
# ---- telegram-relay（Go，本机 8780）：管理 API + 入队 ----
location ^~ /api/v2/ {
    proxy_pass http://127.0.0.1:8780;
    proxy_http_version 1.1;
    proxy_connect_timeout 60s;
    proxy_send_timeout 60s;
    proxy_read_timeout 60s;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

# ---- relay 管理台静态 SPA（由 relay 的 ADMIN_STATIC_DIR 提供）----
# 无尾斜杠的 /relay 不会命中 location ^~ /relay/，会落到下方 location / 的 try_files，误返回根站 query 的 index.html → 白屏。
location = /relay {
    return 301 /relay/;
}
location ^~ /relay/ {
    proxy_pass http://127.0.0.1:8780/;
    proxy_http_version 1.1;
    proxy_connect_timeout 60s;
    proxy_send_timeout 60s;
    proxy_read_timeout 60s;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

要点：

- **`/api/v2/`** 的 `proxy_pass` **不要**写 URI 尾斜杠（保持上游路径仍为 `/api/v2/...`），与 relay 内置路由一致。  
- **`/relay/`** 的 `proxy_pass` **必须**为 `http://127.0.0.1:8780/`（**带**尾斜杠），浏览器访问 `/relay/`、`/relay/assets/xxx` 会变成上游 `/`、`/assets/xxx`。  
- **`SECURITY_LEVEL=strict`** 时，请确认已传递 **`X-Forwarded-For` / `X-Real-IP`**，且 relay 白名单理解的是「客户端 IP」还是「反代 IP」（见 [使用手册](./user-manual.md)）。

改完后在 SSH 执行 **`nginx -t`**，再 **`nginx -s reload` 或宝塔重载配置**。

#### 6.3.1 白屏：Network 里是 `/assets/...` 而不是 `/relay/assets/...`

说明浏览器拿到的 **`index.html` 仍是旧版**（或未命中 relay 静态目录）。请逐项核对：

1. **`/relay/` 的 `proxy_pass` 必须是** `http://127.0.0.1:8780/` **带尾斜杠**，以便把 `/relay/xxx` 转成上游 `/xxx`（与构建目录内物理路径 **`assets/`** 一致）。若漏写尾斜杠，上游会收到错误路径，易 404 / 白屏。  
2. **管理台必须用新版 `npm run build` 产物**（`index.html` 内脚本地址为 **`/relay/assets/...`**），并覆盖到 **`ADMIN_STATIC_DIR`** 后重启 relay。  
3. **开发者工具 Network 勾选「Disable cache」后硬刷新**（Ctrl+Shift+R）；若仍见 **304**，可清空该站点缓存或换一个无痕窗口验证。  
4. 若脚本请求落在 **`https://域名/assets/...`（根路径）**，说明当前页仍是 **根站 query 的 HTML** 或旧 admin：说明 **`location ^~ /relay/` 未生效或被其它规则抢走**，请检查该 `server{}` 内 `location` 顺序与 `server_name` 是否对应你访问的域名。

### 6.4 relay 本机进程与宝塔 Go 项目

- relay 仍监听 **`127.0.0.1:8780`**（或仅内网），**不必**再单独占用一个对外「项目域名」；也可用 **Supervisor** 只守护进程，由**已有 query 站点**反代 `/api/v2` 与 `/relay/`。  
- 若你已为 relay 单独建过 Go 项目域名，注意 **不要** 让两套 Nginx 对同一 `server_name` 重复反代冲突。

### 6.5 relay-admin（telegram-relay-admin）生产构建

同域子路径部署时，前端资源必须以 **`/relay/`** 为公共前缀，否则浏览器会从根站加载 `/assets/...`，命中 query 的 `index.html` 而白屏。

1. **生产构建**（`npm run build`）默认 **`base: '/relay/'`**（见 `telegram-relay-admin/vite.config.ts`）；本地 `npm run dev` 仍为 `/`。若管理台独占域名根路径部署，构建前设置环境变量 **`VITE_BASE=/`**（或 `.env.production` 中 `VITE_BASE=/`）。  
2. **路由**：项目已使用 `createWebHistory(import.meta.env.BASE_URL)`（见 `src/router/index.ts`），`base` 正确时，路由会挂在 `/relay/` 下。  
3. **API 基址**：`src/api/http.ts` 使用 `VITE_API_BASE_URL`，**默认留空**即可，使请求仍为 **`/api/v2/...`**（与第 6.3 节 Nginx 一致）。**不要**把 `VITE_API_BASE_URL` 设成 `/relay`，除非你把 API 也改到子路径（本方案不需要）。  

构建后把 **`admin-dist`** 整目录放到 relay 的 `ADMIN_STATIC_DIR`（见第 7 节），重启 relay。浏览器打开：**`https://你的域名/relay/`**。

### 6.6 探活（可选）

根站 `/healthz` 若仍由 query 处理，外网无法用路径判断 relay 是否存活。可在 **`server{}`** 内为 relay 单独加：

```nginx
location = /relay-healthz {
    proxy_pass http://127.0.0.1:8780/healthz;
    proxy_set_header Host $host;
}
```

或仅在服务器本机执行：`curl -sS http://127.0.0.1:8780/healthz`。

---

## 7. 管理台静态资源（可选但强烈建议）

1. 在电脑克隆 **telegram-relay-admin**，执行 `npm ci && npm run build`（**同域子路径部署时务必按第 6.5 节设置 `base`**；产物目录默认为 **`admin-dist/`**）。  
2. 把 **`admin-dist`** 目录整体上传到服务器，例如：`/www/wwwroot/telegram-relay/admin-dist`。  
3. 在 **relay（telegram-notification）进程能读到的环境**里增加（任选其一，不要写进前端仓库的 `.env`）：

   - **推荐**：与 `relay` 二进制同目录、且启动时工作目录在该目录下的 **`.env`**（仓库里的 `.env.example` 已补充同名项说明），例如增加一行：  
     `ADMIN_STATIC_DIR=/www/wwwroot/telegram-relay/admin-dist`
   - 或使用宝塔 **Go 项目 / Supervisor** 面板里的 **「环境变量」**，键名同为 `ADMIN_STATIC_DIR`，值为服务器上 **`admin-dist` 目录**（不是 zip 文件路径）。

   ```bash
   ADMIN_STATIC_DIR=/www/wwwroot/telegram-relay/admin-dist
   ```

4. 在 **Go 项目** 或 **Supervisor** 里 **重启** relay。

浏览器访问：

- **独占域名部署**：`https://你的域名/` 或 `http://服务器IP:8780/`。  
- **与 query 同域（第 6 节）**：管理台 **`https://你的域名/relay/`**；入队 **`https://你的域名/api/v2/notify`**。

---

## 8. 手动「网站」反代（仅在不使用 Go 项目绑域名时）

若你**没有**用 Go 项目的「项目域名」，而是自己建站点反代：

1. **网站** → **添加站点** → 域名填 **`relay.xxx.com`**。  
2. **设置** → **配置文件**，`location /` 反代到 `http://127.0.0.1:8780`（配置片段见下）。

```nginx
location / {
    proxy_pass http://127.0.0.1:8780;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

3. **SSL** 里申请证书并强制 HTTPS。  
4. 防火墙/安全组：放行 **80、443**；若前面已有 Go 项目反代，**不要**再重复监听冲突。

---

## 9. 宝塔「告警/通知」里填 Webhook（可选）

当你希望 **宝塔磁盘告警、进程告警** 等推到本 relay：

- URL：  
  - 独占子域或根站反代到 relay 时：`https://relay.xxx.com/api/v2/notify`（或 v1）；  
  - **与 query 同域且已按第 6.3 节配置**：`https://你的主域/api/v2/notify`。  
- Header：`Authorization: Bearer <与 AUTH_TOKEN 一致>`  
- Body JSON：见 [宝塔自定义通知接入](./baota-integration.md) 与 [第三方接入说明](./third-party-integration.md)。

若 `SECURITY_LEVEL` 为 **medium/strict**，还要加 **时间戳与 HMAC**。

---

## 10. 登录后台后要做的三件事（否则 notify 会失败）

1. **机器人**：添加 Bot Token。  
2. **发送目标**：绑定 Chat ID / Topic。  
3. **路由规则**：来源/级别与实际上报一致，或留空级别表示全匹配；必要时设 **默认机器人**。

详见 [快速功能指引](./user-quick-guide.md)。

---

## 11. 常见问题

| 现象 | 处理 |
|------|------|
| Go 项目启动闪退 | 看项目日志；多为 **工作目录不对**（找不到 `migrations`）、**DSN 错**、**缺环境变量**；可改用 Supervisor 并固定运行目录 |
| `.env` 不生效 | 确认 **运行用户** 对 `.env` 有读权限；或改把变量全写进面板 **环境变量** |
| 端口与监听不一致 | **`项目端口`** 必须与 **`LISTEN_ADDR`** 端口一致（如都为 `8780`） |
| 网站 502 | relay 是否在跑；反代是否指向 **127.0.0.1:8780** |
| 管理台空白 | 是否设置 **`ADMIN_STATIC_DIR`** 且路径正确 |
| 外网 notify 401 | `AUTH_TOKEN` 是否一致；medium 是否未带签名 |
| **`/api/v2` 进了 Java、relay 收不到** | 是否已增加 **`location ^~ /api/v2/`** 指向 **8780**；与 `^~ /api/` 并存时由 **最长前缀** 决定，勿删掉 Spring 的 `/api/` |
| **`/relay/` 白屏或 404** | relay-admin 生产构建是否设置 **`base: '/relay/'`**；`proxy_pass` 是否带 **尾斜杠** 以剥离前缀 |
| **query Webhook 异常** | 确认 **未改** Spring 的 `context-path`；`/api/webhook` 仍走原有 **`^~ /api/`** |

更通用的命令行部署见：[Linux 傻瓜部署](./deploy-linux-simple.md)。
