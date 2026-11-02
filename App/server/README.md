# Go 模块

`internal/domain/henu` 与 `internal/domain/classroom` 为纯解析层；`mobile/henuparse` 经 gomobile 供 Android 使用。运行测试见 [开发说明](../../docs/BUILDING.md)。

## 可选网页后端

`cmd/api` 是可选的原有网页后端，Android 本机模式不依赖它。运行网页后端需要 PostgreSQL 和自行创建的凭据；数据库集成测试会跳过缺失的测试库，不应连接个人正式数据库。

在 `App` 目录准备配置：

~~~powershell
Copy-Item .env.example .env
# 编辑 .env：至少替换 DATABASE_URL、JWT_SECRET、APP_USER、APP_PASSWORD_HASH
~~~

生成登录密码的 bcrypt 哈希（在 `App/server` 目录执行）：

~~~powershell
go run ./cmd/genpassword "your-password"
~~~

启动后端：

~~~powershell
cd App/server
go run ./cmd/api
~~~

也可以回到 `App` 目录用 Docker Compose 启动：

~~~powershell
docker compose up --build
~~~

Compose 会读取 `App/.env`，数据库主机使用服务名 `db`；本机直接 `go run` 时请把 `DATABASE_URL` 的主机改为 `localhost`。不要把 `.env`、数据库文件或证书提交到 Git。若将后端暴露到公网，请自行配置 HTTPS、访问控制和登录限流，不要直接暴露 `8080` 端口。
