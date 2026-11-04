# im-server 本机部署与 43Chat 对接

本文记录 2026-11-02 在 Windows + Docker Desktop 上完成的本机部署。工作目录为 `D:\ClaudeCodeProjects\im-server`，维护仓库为 [bitcomputing/im-server](https://github.com/bitcomputing/im-server)。项目原理见[项目导读与维护指南](project-guide_zh.md)。

## 已部署的服务

本机使用 MySQL 存储消息，由一个 IM 进程启动 API、导航、WebSocket 和管理后台。此部署不依赖 Redis、RabbitMQ 或 MongoDB；43Chat 业务服务自身的依赖需要在其项目中单独启动。

| 服务 | 本机地址 | 说明 |
| --- | --- | --- |
| 管理后台 | http://127.0.0.1:8090 | 本地默认账号 `admin`，密码 `123456` |
| 服务端 API | http://127.0.0.1:9001 | 实际接口前缀 `/apigateway`；需租户签名 |
| 导航 | http://127.0.0.1:9002 | `/navigator/general`；需有效租户与用户 Token |
| WebSocket | ws://127.0.0.1:9003 | SDK 使用此基础地址；实际协议路径 `/im` |
| 健康检查 | http://127.0.0.1:9003/health | 容器健康检查使用此地址 |
| 性能诊断 | http://127.0.0.1:6060/debug/pprof/ | 仅供本机开发诊断 |
| MySQL | `127.0.0.1:13306` | 数据库 `jim_db`，用户 `root`；密码配置见 Compose |

容器名称继续使用 `juggleim-server` 和 `juggleim-mysql`，避免另起一套环境。本次从当前源码构建了 `bitcomputing/im-server:local` 镜像，基线提交为 `cb7297860c9e3064956274d75775cbd2a2f4c060`，另包含本地 Dockerfile 启动脚本换行修复。

所有发布端口绑定 `127.0.0.1`。同一台 Windows 上的业务后端和浏览器可访问；手机、局域网其他设备不能直接连接。后续真机联调需要一起调整端口绑定、导航公布地址和客户端连接地址。

## 启动、停止与更新

要求 Docker Desktop 运行 Linux 容器，Docker Compose 至少为 2.24.4（本地覆盖文件使用 `!override`）。本次验证环境为 Docker Engine 27.2.0、Compose 2.29.2、MySQL 8.0.46。

以下命令在 PowerShell 中执行。**始终同时指定两个 Compose 文件**，以保留本机端口限制与 IM 数据卷。

```powershell
Set-Location D:\ClaudeCodeProjects\im-server

# 启动已有镜像，等待 MySQL 与 IM 健康检查通过
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --no-build --wait --wait-timeout 120

# 查看状态与日志
docker compose -f docker-compose.yml -f docker-compose.local.yml ps
docker compose -f docker-compose.yml -f docker-compose.local.yml logs --tail 100 im-server

# 停止服务，保留容器和数据
docker compose -f docker-compose.yml -f docker-compose.local.yml stop
```

源码变更后重新构建并启动：

```powershell
Set-Location D:\ClaudeCodeProjects\im-server
$env:IM_SERVER_REVISION = (git rev-parse HEAD).Trim()
docker compose -f docker-compose.yml -f docker-compose.local.yml build im-server
if ($LASTEXITCODE -ne 0) { throw 'IM 镜像构建失败' }
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --no-build --wait --wait-timeout 120
if ($LASTEXITCODE -ne 0) { throw 'IM 服务未就绪，请检查日志' }
```

镜像内编译 Go，无需为了启动服务在宿主机安装 Go。首次构建需要下载基础镜像和 Go 模块。镜像标签中的提交号表示源码基线，不代表工作区没有未提交修改。

容器配置了 `restart: unless-stopped`。Docker 引擎运行时可自动恢复未被手动停止的容器；Windows 开机后仍需确保 Docker Desktop 已启动。

## 与 chatbot43 的连接配置

43Chat 业务仓库位于 `D:\ClaudeCodeProjects\chatbot43`。已核对其本地配置 `app/etc/dev/chatbot-api-local.yaml`：

```yaml
Im:
  AppKey: "ueNSmIjnyoj5vz6F"
  # AppSecret 保留业务项目已有值，本文不复制密钥。
  ApiURL: "http://127.0.0.1:9001"
  WsUrl: "ws://127.0.0.1:9003"
  WebWsUrl: "127.0.0.1:9003"
  Timeout: 30
  RetryCount: 3
  Prefix: "dev"
```

这些地址已经匹配本次部署，未修改业务仓库。业务 SDK 会补上 API 路径，`ApiURL` 不要重复添加 `/apigateway`。原有 IM 租户及其凭据保存在原 MySQL 数据卷中，本次保留了该租户原有的 7 个用户和 7 条私聊历史消息。

上面的地址适用于业务服务直接运行在 Windows 上。若后续把业务服务放进容器，容器内的 `127.0.0.1` 指向自身，需要按网络部署方式改用容器服务名或 `host.docker.internal`。

### 业务回调尚未联调

本次检查时，43Chat 本地业务服务的 `6006` 端口未能响应，因此没有完成业务登录、机器人回复、业务消息回调的端到端验证，也没有启动整个业务项目。

业务项目的 `/api/im/callback/message` 是发送前的自定义消息拦截器，使用 `PASS` / `REJECT` 等响应语义，不能直接当成普通消息事件订阅接口。原本地 IM 租户未配置该自定义拦截器，本次保持原状。

后续业务服务在 Windows 的 `6006` 端口就绪后，从 IM 容器访问它的候选地址是 `http://host.docker.internal:6006/api/im/callback/message`，还需确认业务服务监听地址、防火墙及接口鉴权，并按拦截器配置流程接入。不要在容器回调配置中填写 `127.0.0.1:6006`。

## 数据保存与备份

| Docker 数据卷 | 用途 |
| --- | --- |
| `im-server_mysql-data` | 原有 MySQL 数据，继续复用 |
| `im-server_im-logs` | IM 日志及 `logs/kvdb_data` 下的 LevelDB 数据 |
| `im-server_im-uploads` | 管理模块使用的本地上传文件目录 |

采用 Linux 命名卷保存日志，避免把日志符号链接复制到 Windows 目录时失败。升级前已将旧容器日志和 LevelDB 数据归档，再恢复到日志卷。

本次升级将数据库版本从 `20260813` 更新到 `20260908`，新增 iOS 推送证书配置相关的 6 个字段。已核对迁移版本和字段数量。容器重建不会清空上述数据卷；不要执行 `down -v` 或删除这些卷，否则会丢失本机数据。

升级前备份保存在本机 `.local/backups/`，不提交到 Git、不进入 Docker 构建上下文：

- `mysql-before-local-deploy-20261102.sql`：完整 `jim_db` 逻辑备份。
- `logs-before-local-deploy-20261102.tar`：旧容器日志与 LevelDB 数据归档。
- `conf-before-local-deploy-20261102/`：旧容器运行配置。
- 旧镜像已保留为 `juggleim/im-server:backup-20261102`。

SQL 备份 SHA-256：`5CE1B0486E1DAB5B1B9A584CB901593E57720CA90F736AAB0774C16EA8AAB662`。

备份含本机凭据和业务数据，应只保存在受控位置。它们仅是本次升级前快照，不会自动更新；本次未做备份恢复演练。需要回滚时，先停止写入并另存最新数据，再评估数据库兼容性和恢复范围，避免直接覆盖新增消息。

## 已执行的验证

测试使用独立的本地租户和模拟用户，没有向现有 43Chat 用户发消息。最后一次完整测试结果如下：

| 检查 | 结果 |
| --- | --- |
| 镜像构建、启动脚本语法 | 通过；镜像构建时将 `run.sh` 的 CRLF 统一为 LF |
| 管理后台登录、创建测试租户、用户注册 | 通过 |
| 鉴权导航 | 返回 `127.0.0.1:9003` |
| 私聊：3 个客户端，约 4 秒 | 19 条发送确认成功，19 次接收，0 个发送失败 |
| 群聊：3 个客户端，1 个发送者 | 19 条发送确认成功，另外 2 人共收到 38 次，0 个发送失败 |
| 接收者离线后重连 | 能补收到离线期间发送的消息 |
| 双方历史消息查询 | 发件人与收件人均查询到相同消息 ID 和正文 |
| 数据库保存 | 测试租户有 20 条私聊历史（含离线消息）、19 条群聊历史 |
| 原租户数据 | 7 个用户、7 条私聊历史仍在 |
| IM 容器重建 | MySQL 与 IM 均恢复 healthy，重建前后上述用户和消息计数一致 |

本机验证脚本、模拟客户端和 JSON 结果位于 `.local/verification/`，包括 `private.json`、`group.json`、`offline.json`、`navigation.json` 及容器重建前后的数据计数。该目录被 Git 忽略，只属于本机验证记录；通用测试客户端源码仍在仓库的 `cmd/jimbench` 和 `simulator/wsclients` 中。

测试过程中创建的两个隔离租户保留在本地管理后台，名称均为 `Local deployment smoke verification`。这些是短时间功能冒烟测试，不能作为容量或性能结论。

## 常见问题

- **容器显示运行但业务不可用**：查看 `docker compose ... ps` 中的健康状态、日志和 `/health`。本地覆盖文件已增加 IM 健康检查，`up --wait` 会等待服务实际就绪。
- **`env: can't execute 'bash\r'`**：旧镜像可能直接复制了 Windows CRLF 脚本。当前 Dockerfile 已在镜像构建时规范化换行，重新构建镜像即可。
- **9001/9002 的 `/health` 返回 404**：这两个服务没有该探针；健康检查使用 `9003/health`，API 和导航通过实际鉴权请求验证。
- **端口占用**：先检查本机进程和 `docker ps`，不要停止不属于本项目的容器。修改端口时同步调整业务配置及导航公布地址。
- **数据库连接失败**：检查 MySQL 健康状态及两项服务的密码配置是否一致；已有数据卷不会因为修改初始化环境变量而自动重置数据库密码。

下一步是在 chatbot43 项目启动本地业务服务，用测试账号走通登录、收发消息、拦截回调和机器人分段回复。后续二次开发以接替 43Chat Pro 当前的 JuggleIM 官方部署为目标，兼容性、数据迁移和回退的验收条件见[项目导读第 15.4 节](project-guide_zh.md#154-替换-43chat-pro-官方部署的验收条件)。
