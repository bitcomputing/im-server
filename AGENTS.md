# im-server 开发约定

本仓库由 [bitcomputing/im-server](https://github.com/bitcomputing/im-server) 维护。二次开发的目标是接替 43Chat Pro 当前使用的 JuggleIM 官方部署。兼容性以 43Chat 业务后端的实际调用和已发布客户端的行为为准。提交 PR 时核对目标仓库，仓库内部分链接仍指向上游。

## 开始改动前

- 从仓库根目录执行 `git status --short --branch`，保留已有工作。先找一个同类实现并追到调用方，再确定改动范围。
- 43Chat 业务后端在本机相邻仓库 `../chatbot43`。核对服务端 HTTP 契约从 `../chatbot43/lib/imServiceLib/juggleClient.go` 入手，继续查具体业务调用。需要修改该仓库时，先读它自己的 `AGENTS.md`。
- 架构、消息流程和源码索引见[项目导读](docs/project-guide_zh.md)。本机端口、启停、备份和 43Chat 配置见[本地部署文档](docs/local-deployment_zh.md)。

## 完成后的提交

- 主人于 2026-11-06 明确要求：本项目每次独立改动或某个功能完成并验证后，及时将对应改动文件提交为本地 Git commit，交付时回报 commit 与验证结果。
- 每次只暂存本次工作对应的文件，保留其他人的改动及未完成工作；凭据、私有测试数据和忽略目录内的证据不进入提交。推送、合并和部署沿用各自已有授权。

## 按改动定位源码

| 任务 | 先看哪里 | 修改时一起检查 |
| --- | --- | --- |
| 服务端 HTTP API | [路由](services/apigateway/routers/router.go) → 同模块的 `apis/` → RPC → 业务 Actor | 租户签名、参数、响应结构及错误码是否匹配业务调用方 |
| 客户端连接与协议 | [connectmanager](services/connectmanager/)、[消息定义](commons/pbdefines/appmessages.proto) | 修改源 `.proto` 后重新生成 Go 文件。保留旧字段编号及语义，用旧客户端验证 |
| 私聊、群聊与同步 | [message](services/message/)、[group](services/group/)、[historymsg](services/historymsg/)、[conversation](services/conversation/) | HTTP 与 WebSocket 入口是否都经过被改逻辑，鉴权与数据查询是否限定当前租户 |
| 模块注册与配置 | 模块的 `starter.go`、[启动器](launcher/main.go)、[配置模板](launcher/scripts/config_template.yaml) | 新 Actor 注册在所属模块。增加配置时检查默认值、Compose 及容器启动脚本 |
| 数据表或索引 | [新库初始化 SQL](sql/imserver.sql)、[升级脚本](commons/dbcommons/sqls/)、[迁移入口](commons/dbcommons/dbmigration.go) | 同时覆盖新库与已有库。增加有序的升级脚本，在可丢弃的数据库副本上核对结构和旧数据 |

## 本机运行与检查

在仓库根目录运行。Windows 本机使用两个 Compose 文件，以保留回环地址绑定和现有数据卷。修改服务代码后需要重建镜像才能验证新实现。

```powershell
docker compose -f docker-compose.yml -f docker-compose.local.yml config --quiet
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build --wait --wait-timeout 120
Invoke-RestMethod http://127.0.0.1:9003/health
```

Go 版本以 [go.mod](go.mod) 为准。对改过的 Go 文件运行 `gofmt`，先执行受影响包的测试。以下为 CI 中可单独执行的测试包，Linux 构建和其余检查见 [CI 工作流](.github/workflows/ci.yml)。

```sh
go test ./simulator/benchmark ./services/commonservices/... ./commons/metrics/... ./services/pushmanager/services/httputil
```

`simulator/tests/` 需要运行中的服务及测试租户；运行全仓测试前先确认各集成用例的目标地址和凭据。消息联调使用独立本地租户，现有压测客户端入口为 [cmd/jimbench](cmd/jimbench/)。

## 按业务结果验收

- 改投递时，核对发送 ACK、目标用户实际收到的消息，以及历史查询中的消息 ID。改群聊时核对预期接收成员；改离线同步时，让接收端断开、发送、重连，再检查漏收和重复。
- 改 43Chat 专用能力时，从业务入口发起验证。例如机器人分段回复要覆盖创建、追加、结束及失败重试。线上是否启用某项能力需依据 Pro 的实际配置和调用确认。
- 本机验收和 Pro 切换分开记录。生产部署、数据迁移或入口切换需明确授权，并按[替换验收条件](docs/project-guide_zh.md#154-替换-43chat-pro-官方部署的验收条件)演练回退，包括处理切换后新增的消息。
- 保留开发数据卷，常规停止使用 `docker compose -f docker-compose.yml -f docker-compose.local.yml stop`。本机凭据、租户密钥、Token、数据库备份和含消息正文的日志不得提交或贴入交付记录。`launcher/conf/config.yml` 已被 Git 跟踪，修改本地配置后要检查差异。
- 交付时先说明用户可见的结果，再列实际执行的验证、环境及仍待确认的风险。接口返回、数据库写入、客户端收取各自按实际证据报告。
