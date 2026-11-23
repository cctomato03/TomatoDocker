# Docker 管理模块

> 目标：对标 Portainer 的核心能力，在现有 Web Basic Admin 权限体系上增加「本地 Docker / Docker Swarm 资源管理」与「自定义 Stack 文件部署」。
>
> 所有 Stack 文件、环境连接信息、历史版本、资源授权都保存在独立 SQLite 中，拷贝一个 `.db` 文件即可恢复 Stack 定义。

---

## 1. 能力清单

| 分类 | 能力 |
| --- | --- |
| 环境 | 本地 Socket（Windows npipe / Linux unix）、远程 TCP、TLS 双向认证、SSH 隧道；连接测试、默认环境、多环境切换 |
| 容器 | 列表/筛选、启动/停止/重启/暂停/恢复、删除、创建（端口/环境变量/挂载/网络/重启策略）、日志、资源快照、非交互命令执行、Docker inspect |
| 镜像 | 列表/搜索、流式拉取进度、删除、悬空镜像清理、inspect |
| 网络 | 列表、创建（bridge/overlay/macvlan、子网/网关）、删除、容器接入/断开、清理未使用 |
| 数据卷 | 列表、创建（驱动/参数）、删除、清理未使用、inspect |
| Swarm | 初始化/退出、节点列表与角色/可用性/标签修改、节点移除、服务创建/更新/扩缩容/删除/日志、任务列表、密钥与配置创建/删除 |
| Stack | 自定义 `docker-compose.yml` + `.env`、Compose 单机部署、Swarm Stack 部署、重新部署、停止、删除（可选卷）、历史版本回滚、操作记录、JSON 导出/导入、SQLite 备份下载 |
| 权限 | 功能权限（菜单/按钮/接口）+ 环境资源授权（只读 / 操作 / 管理）双层校验 |

---

## 2. 架构

```
浏览器（Vue3）
  └─ /api/v1/docker/**（JWT + 功能权限 middleware.Perm）
       └─ internal/service/docker*.go（资源级权限 + 业务编排）
            ├─ internal/docker        Docker Engine / Swarm SDK 封装、Compose 部署引擎
            └─ internal/dockerstore   独立 SQLite（环境、Stack、版本、授权、活动）
```

- **SQLite 主库**：`./data/tomato.db`，保存菜单、角色、用户、日志、通知等后台数据。
- **SQLite Docker 域库**：`./data/tomato-docker.db`，保存环境、Stack、历史版本、资源授权；与主库解耦，便于单独恢复 Stack。
- **Docker SDK**：`github.com/docker/docker v28.5.2`；Compose 解析：`github.com/compose-spec/compose-go/v2 v2.8.0`。
- **纯 Go SQLite**：`github.com/glebarez/sqlite`（底层 `glebarez/go-sqlite`），发布二进制无需 CGO。
- **无 Redis**：会话、验证码、限流使用进程内缓存；单实例部署，重启后会话失效。

### 2.1 SQLite 表

| 表 | 用途 |
| --- | --- |
| `docker_environment` | 环境连接配置；TLS/SSH 敏感字段 AES-GCM 加密 |
| `docker_environment_grant` | 环境级资源授权：角色/用户 × 只读/操作/管理 |
| `docker_stack` | Stack 当前 compose/.env、状态、版本号、最近部署时间 |
| `docker_stack_revision` | 每次保存/回滚生成的历史版本，支持一键回滚 |
| `docker_stack_activity` | 部署/停止/回滚操作记录（成功/失败与原因） |

---

## 3. 权限模型

### 3.1 功能权限（静态）

菜单/按钮/接口权限码：

| 模块 | 权限码 |
| --- | --- |
| 环境 | `docker:environment:list/create/update/delete/test/grant` |
| 容器 | `docker:container:list/create/operate/delete/exec` |
| 镜像 | `docker:image:list/pull/delete` |
| 网络 | `docker:network:list/create/update/delete` |
| 数据卷 | `docker:volume:list/create/delete` |
| Stack | `docker:stack:list/create/update/delete/deploy/stop/export/import` |
| Swarm | `docker:swarm:view/init/leave` |
| 节点 | `docker:node:list/update/delete` |
| 服务 | `docker:service:list/create/update/delete/scale` |
| 任务 | `docker:task:list` |
| 密钥/配置 | `docker:secret:list/create/delete`、`docker:config:list/create/delete` |

在「角色管理 → 分配权限」中勾选；超级管理员 `*:*:*` 隐式拥有全部。

### 3.2 环境资源授权（动态）

同一功能权限下，普通用户只能看到被授权的环境：

| 级别 | 能力 |
| --- | --- |
| 1 只读 | 查看环境、容器、镜像、网络、卷、Stack、Swarm 资源 |
| 2 操作 | 只读 + 启停容器、拉取/删除镜像、创建网络/卷、部署/停止/删除 Stack、服务扩缩容、节点/密钥/配置变更 |
| 3 管理 | 操作 + 修改/删除环境、保存环境授权 |

- 超级管理员绕过资源授权，拥有全部环境。
- 非超级管理员创建环境后会自动获得该环境的「管理」授权。
- 角色授权使用 `principal.RoleIDs`；用户授权使用用户 ID。
- 资源授权在「Docker 管理 → 资源授权」页面维护；接口 `PUT /docker/environments/:id/grants` 覆盖式保存。

> 重要：功能权限与环境资源授权是「与」关系，缺一不可。只有功能权限没有环境授权时，环境列表为空；直接调用资源接口返回 403。

---

## 4. Stack 部署引擎

### 4.1 保存与版本

- 新建 Stack 时校验名称（小写字母/数字/`-`/`_`，最长 64）与 compose 语法。
- 每次保存生成一条 `docker_stack_revision`，版本号自增。
- `updated_at > last_deployed_at` 时前端显示「未部署变更」。
- 支持回滚到任意历史版本；可选择仅回滚文件或回滚并立即部署。

### 4.2 单机 Compose

- 按 `depends_on` 拓扑排序创建容器；同名项目旧容器先移除（数据卷保留）。
- 自动创建顶层网络/数据卷，打 `com.docker.compose.project` 标签，删除 Stack 时按标签清理。
- 端口、环境变量、bind/volume/tmpfs 挂载、healthcheck、restart、资源限制、日志驱动、cap/ulimit/sysctl 等常用字段已映射。
- 镜像缺失时自动拉取；`pull_policy: always` 强制拉取。
- **限制**：不支持 `build`、`env_file`、`secrets/configs`、端口范围、`image` 类型挂载；绑定挂载必须是宿主机绝对路径。

### 4.3 Swarm Stack

- 自动创建 overlay 网络（attachable）与本地数据卷，打 `com.docker.stack.namespace` 标签。
- Compose service 转换为 `swarm.ServiceSpec`：镜像、命令/入口、环境变量、挂载、网络别名、端口（ingress/host）、副本数/global、资源限制、重启策略、更新/回滚配置、placement 约束、healthcheck。
- 按命名空间列出已有服务，新增/更新/移除差异服务。
- 停止 Stack：replicated 服务副本数置 0；global 服务不支持停止，会提示改用删除。
- **限制**：`secrets/configs` 仅支持 `external: true`（先在 Swarm 页面创建同名对象），不支持 `file/content`；不支持 `build`、`privileged`、端口范围。

### 4.4 部署超时

- 后端部署超时 20 分钟；前端 `deployDockerStack` 请求超时同步为 20 分钟。
- 镜像拉取使用 NDJSON 流式进度，前端实时展示 Docker 的 `status/progress`。

---

## 5. 环境连接与安全

| 类型 | 说明 |
| --- | --- |
| `local` | Windows 默认 `npipe:////./pipe/docker_engine`；Linux 默认 `unix:///var/run/docker.sock` |
| `tcp` | 如 `192.168.1.10:2375`，仅建议内网/受控网络 |
| `tls` | 客户端证书 + 私钥 + CA；默认端口 2376 |
| `ssh` | 用户名 + 密码或私钥；通过 SSH 连接远端 `/var/run/docker.sock` |

- TLS 私钥、SSH 密码/私钥使用 AES-GCM 加密后落 SQLite，密钥由 `jwt.secret` 派生（SHA-256）。
- 接口出参永不返回明文凭据，只返回 `hasTls/hasSshPassword/hasSshKey`。
- **迁移 SQLite 时必须同时迁移 JWT secret**，否则已保存凭据无法解密，需要重新录入。
- SSH 当前使用 `InsecureIgnoreHostKey`（内网场景）；对公网主机建议后续增加 known_hosts/指纹配置。
- 私有镜像仓库需先在目标 Docker 主机执行 `docker login`；本系统不保存 registry 凭据。

---

## 6. API 摘要

所有接口需登录；下表省略 `/api/v1` 前缀。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/docker/environments` | 可见环境（含在线状态与访问级别） |
| POST/PUT/DELETE | `/docker/environments[/:id]` | 环境增改删 |
| POST | `/docker/environments/:id/test` | 连接测试 |
| GET/PUT | `/docker/environments/:id/grants` | 资源授权 |
| GET | `/docker/grants/options` | 可授权角色/用户 |
| GET | `/docker/environments/:id/overview` | 概览 |
| GET/POST | `/docker/environments/:id/containers` | 容器列表/创建 |
| POST | `/docker/environments/:id/containers/:cid/actions` | 启停/重启/暂停/恢复/重命名 |
| DELETE | `/docker/environments/:id/containers/:cid` | 删除容器 |
| GET | `/docker/environments/:id/containers/:cid/logs` | 日志（tail） |
| GET | `/docker/environments/:id/containers/:cid/stats` | 资源快照 |
| POST | `/docker/environments/:id/containers/:cid/exec` | 非交互命令 |
| GET | `/docker/environments/:id/images` | 镜像列表 |
| GET | `/docker/environments/:id/images/pull?ref=` | NDJSON 拉取进度 |
| DELETE | `/docker/environments/:id/images/:imageId` | 删除镜像 |
| GET/POST/DELETE | `/docker/environments/:id/networks[/:networkId]` | 网络 |
| POST | `/docker/environments/:id/networks/:networkId/connect|disconnect` | 容器接入/断开 |
| GET/POST/DELETE | `/docker/environments/:id/volumes[/:name]` | 数据卷 |
| GET/POST | `/docker/environments/:id/swarm[/init|leave]` | 集群 |
| GET/PUT/DELETE | `/docker/environments/:id/nodes[/:nodeId]` | 节点 |
| GET/POST/PUT/DELETE | `/docker/environments/:id/services[/:serviceId]` | 服务 |
| POST | `/docker/environments/:id/services/:serviceId/scale` | 扩缩容 |
| GET | `/docker/environments/:id/services/:serviceId/logs` | 服务日志 |
| GET | `/docker/environments/:id/tasks` | 任务 |
| GET/POST/DELETE | `/docker/environments/:id/secrets[/:secretId]` | 密钥 |
| GET/POST/DELETE | `/docker/environments/:id/configs[/:configId]` | 配置 |
| GET/POST | `/docker/stacks` | Stack 列表/新建 |
| GET/PUT/DELETE | `/docker/stacks/:id` | 详情/保存/删除 |
| POST | `/docker/stacks/:id/deploy|stop|rollback` | 部署/停止/回滚 |
| GET | `/docker/stacks/export` | 导出 JSON |
| POST | `/docker/stacks/import` | 导入 JSON |
| GET | `/docker/stacks/backup` | 下载 SQLite 备份（仅超管） |

---

## 7. 备份与恢复

### 7.1 备份

1. 页面「Docker 管理 → 堆栈 → 下载 SQLite 备份」，或直接复制 `data/tomato-docker.db`（建议先停服务）。
2. 同时备份主业务库 `data/tomato.db`（用户/角色/菜单/日志等）。
3. 同时备份 `configs/config.yaml` 中的 `jwt.secret`（用于解密环境凭据）。
4. 需要跨机器恢复 Stack 时，也可用「导出」得到 JSON（不含环境凭据）。

### 7.2 恢复

1. 停止服务，把主库放回 `sqlite.path`（默认 `./data/tomato.db`），Docker 域库放回 `docker.database`（默认 `./data/tomato-docker.db`）。
2. 保持 `jwt.secret` 与备份时一致；否则环境凭据需重新录入。
3. 启动服务，确认「环境管理」中环境在线。
4. 若只导入 Stack JSON：先创建同名环境，再在「堆栈 → 导入」选择 JSON；按环境名称匹配，同名 Stack 可选择覆盖。

> 注意：两个 SQLite 文件职责不同，恢复时都要放回；只恢复 Docker 域库会保留旧的用户/权限数据。

---

## 8. 部署

### 8.1 单二进制

```bash
cd web && pnpm build && cd ..
powershell -ExecutionPolicy Bypass -File scripts/build.ps1
./bin/web-basic-server.exe -c configs/config.yaml
```

数据目录：`./data/tomato.db`（主业务库）与 `./data/tomato-docker.db`（Docker 域），首次启动自动创建并登记本地 Docker。

### 8.2 Docker Compose（源码构建）

```bash
cp deploy/.env.example deploy/.env
# 修改 JWT_SECRET / ADMIN_PASSWORD / 端口
docker compose -f deploy/docker-compose.yml up -d --build
```

`deploy/docker-compose.yml` 已包含：

- `/var/run/docker.sock:/var/run/docker.sock`：让容器内应用管理宿主机 Docker；
- `app-data:/app/data`：持久化两个 SQLite 库；
- `APP__SQLITE__PATH=/app/data/tomato.db`；
- `APP__DOCKER__DATABASE=/app/data/tomato-docker.db`；
- 只有应用容器，没有 MySQL/Redis 边车。

### 8.3 预构建镜像（类似 Portainer 安装方式）

GitHub Actions 在 `v*` 标签构建并推送：

```
ghcr.io/cctomato03/tomato-docker:<版本>
ghcr.io/cctomato03/tomato-docker:latest
```

使用：

```bash
cp deploy/.env.example deploy/.env
# 如 GHCR 包为私有：docker login ghcr.io
TAG=latest docker compose -f deploy/docker-compose.release.yml up -d
```

- `docker-compose.release.yml` 与主 compose 配置一致，仅去掉 build，直接拉取镜像；
- 预构建镜像同样包含前端，不需要在宿主机安装 Go/Node；
- 更新：`TAG=1.0.2 docker compose -f deploy/docker-compose.release.yml up -d`。

> 挂载 Docker Socket 等于把宿主机 Docker 控制权交给该容器，请务必限制后台账号权限并只暴露给可信网络。

---

## 9. 测试

```bash
# 后端编译 + 单元测试
go build ./...
go test ./...

# Docker 页面冒烟（真实登录、逐页渲染、收集控制台/HTTP 错误）
# 先以 APP__AUTH__CAPTCHA_ENABLED=false 启动服务；验证码在内存中，无法外部读取
node scripts/e2e-docker-smoke.mjs http://127.0.0.1:8090 admin <密码>
```

`scripts/e2e-docker-smoke.mjs` 使用真实登录并逐页渲染；验证码关闭时无需 Redis，只需本机 Edge/Chrome。

已验证场景（本机 Docker Desktop 29.6.1）：

- 本地环境登记与连接测试；
- 容器列表/日志/资源/命令执行；
- 镜像流式拉取；
- 网络/数据卷创建删除；
- Compose Stack 创建、部署、更新、重新部署、删除；
- Swarm 初始化、节点、密钥、服务创建/扩缩容/删除、Swarm Stack 部署/停止/删除、退出 Swarm；
- 只读用户：可见环境与容器，容器操作/删除环境/创建网络均返回 403（功能权限或环境级别）。

Docker 部署实测（2026-11-23）：

- `docker compose -f deploy/docker-compose.yml build app` 成功；
- 单容器启动（仅 `tomato-docker-app`，无 MySQL/Redis），`/readyz` 返回 `{"sqlite":true,"cache":true}`；
- 容器内应用通过 `/var/run/docker.sock` 看到宿主机容器（包括自身）；
- 从容器内部署 Compose Stack，宿主机出现 `single-container-e2e-web-1`，随后通过 API 删除；
- `deploy/docker-compose.release.yml` 使用本地预构建镜像启动同样通过；
- 重启容器后 SQLite 中的公告仍在（主库持久化验证通过）；
- 单容器页面冒烟 17/17 通过（含缺失图标外部请求修复）。

---

## 10. 已知限制与后续方向

- 单机 Compose 不支持 `build`、`env_file`、`secrets/configs`、端口范围；后续可增加「上传构建上下文」与多文件 Stack。
- Swarm 仅支持 external secret/config；可增加从 Web 创建 stack 级密钥并自动清理。
- 不支持 Portainer 的 Edge Agent / 多节点代理；远程连接使用 TCP/TLS/SSH。
- 容器命令为非交互式 `/bin/sh -c`，不支持 xterm 终端；可后续增加 WebSocket + xterm。
- 无镜像仓库/Registry 凭据管理、镜像搜索、Stack Git/Webhook 自动更新。
- SSH 未校验 host key；公网场景建议增加指纹白名单。
- 资源授权粒度为环境级；如需单容器/单 Stack 级 ACL，可在 `docker_stack`/容器标签上扩展。
- 进程内缓存只支持单实例：多副本会导致会话/验证码/定时任务租约不共享；需要水平扩容时再引入共享缓存。