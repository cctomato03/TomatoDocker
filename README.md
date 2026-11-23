# TomatoDocker · Docker 管理系统

一个开箱即用、可直接二次开发的 Docker 管理平台：**Go + Gin + GORM + Docker SDK** 后端，**Vue 3 + Web Admin Pro** 前端，
内嵌迁移、单可执行文件部署。对标 Portainer 的核心资源管理与 Stack 部署能力。

> 📘 二次开发看 [docs/开发手册.md](./docs/开发手册.md) ｜ 🖱 功能使用看 [docs/操作手册.md](./docs/操作手册.md) ｜ 🐳 Docker 模块看 [docs/Docker管理.md](./docs/Docker管理.md)

## 特性

- **Docker 资源**：本地/远程（TCP/TLS/SSH）环境，容器、镜像、网络、数据卷全生命周期管理
- **Docker Swarm**：初始化/退出、节点、服务（扩缩容/更新/日志）、任务、密钥、配置
- **Stack**：Web 编辑器自定义 compose + `.env`，支持单机 Compose 与 Swarm Stack 部署、停止、重新部署、历史回滚、导入导出
- **Stack 持久化**：所有 Stack/环境/授权保存在独立 SQLite（默认 `./data/tomato-docker.db`），拷贝即可恢复
- **权限体系**：用户 / 角色 / 菜单（目录·页面·按钮·接口）/ 部门 / 岗位 / 数据权限 / 多租户；Docker 额外支持环境级「只读/操作/管理」资源授权
- **工作台**：4 张统计卡片 + 「我的待办 / 我的消息 / 通知公告」页签，顶栏铃铛显示未读
- **系统设置**：站点个性化（名称/logo/登录页文案/注册开关/验证码/滑块）、字典、参数、租户、文件、定时任务、代码生成
- **通知与协作**：通知公告（发布扇出站内信）、站内信、待办；业务系统可通过开放接口推送（`X-API-KEY` + 幂等键）
- **安全**：双 token 静默刷新、图形验证码、滑块校验、登录失败锁定、强口令策略、字段级操作日志、TLS/SSH 凭据加密存储
- **运维**：健康检查、Prometheus 指标、结构化日志、内嵌数据库迁移、Docker Compose

## 快速开始

```bash
# 基础设施（MySQL 3307 / Redis 6380）
docker compose -f deploy/docker-compose.yml up -d mysql redis

# 后端（:8090，首次启动自动建表 + 初始化 + 建超管）
APP__APP__ADMIN_PASSWORD=Admin@123456 go run ./cmd/server -c configs/config.yaml

# 前端（:5173）
cd web && pnpm install && pnpm dev
```

默认账号 `admin`，密码取 `APP__APP__ADMIN_PASSWORD`。

首次启动会自动创建 `data/tomato-docker.db` 并登记「本地 Docker」环境。Docker 模块默认连接：

- Windows：`npipe:////./pipe/docker_engine`
- Linux/macOS：`unix:///var/run/docker.sock`

如需管理远程 Docker，在「Docker 管理 → 环境管理」新增 TCP / TLS / SSH 环境即可。

生产部署（单二进制或 Docker）：

```bash
cd web && pnpm build && cd ..
powershell -ExecutionPolicy Bypass -File scripts/build.ps1   # 或 make build
```

### Docker 部署（推荐，类似 Portainer）

应用镜像已包含前端，挂载宿主机 Docker Socket 后即可管理本机容器/Swarm/Stack：

```bash
cp deploy/.env.example deploy/.env      # 修改 JWT_SECRET、管理员密码、端口
docker compose -f deploy/docker-compose.yml up -d --build
```

打开 `http://localhost:8080`。数据持久化：

- MySQL / Redis：`mysql-data`、`redis-data`
- 日志 / 文件 / Docker SQLite：`app-logs`、`app-storage`、`app-data`

也可以直接使用 GitHub Actions 构建的预构建镜像（不构建源码）：

```bash
docker login ghcr.io          # 如果 GHCR 包为私有
TAG=latest docker compose -f deploy/docker-compose.release.yml up -d
```

镜像地址：`ghcr.io/cctomato03/tomato-docker:latest`。

> 挂载 `/var/run/docker.sock` 等价于把宿主机 Docker 控制权交给该容器，请限制后台账号权限并只暴露给可信网络。

## 发布（GitHub Actions）

推送到 `master`/`main` 会自动构建并上传 Actions artifact；推送 `v*` 标签会自动创建 GitHub Release，
并附带 Linux/Windows 两个平台的单文件包与 `checksums.txt`：

```bash
git tag v1.0.0
git push origin master --tags
```

工作流文件：[.github/workflows/release.yml](./.github/workflows/release.yml)。

## 测试

```bash
node scripts/e2e-api.mjs   http://127.0.0.1:8090 admin  # 接口 76 条断言
node scripts/e2e-smoke.mjs http://127.0.0.1:8090 admin  # 浏览器 61 条断言
node scripts/e2e-docker-smoke.mjs http://127.0.0.1:8090 admin  # Docker 15 个页面逐页渲染 + 控制台/HTTP 错误检查
```

环境变量：`E2E_ADMIN_PASSWORD`（管理员密码）、`E2E_REDIS_CONTAINER`（读验证码用，默认自动探测）。

## 目录速览

```
cmd/server      入口            internal/{api,service,repository,model,router,middleware,migrate}
internal/docker Docker Engine/Swarm 封装 + Compose 部署引擎
internal/dockerstore 独立 SQLite（环境 / Stack / 历史版本 / 资源授权）
migrations      数据库迁移       pkg/{crypto,jwt,errs,response,captcha,id,storage}
scripts         构建 / 图标子集 / E2E          web/  前端（Vue3 + Web Admin Pro）
configs         默认配置（环境变量 APP__XXX__YYY 覆盖）      deploy/  Dockerfile + compose
```
