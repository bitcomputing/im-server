# STA Mindlink 运行合同 · 2026-11-20

适用于 `32bf6f05ccb18a5880aa5d0813268cfb2d7420da` 后本次 STA 启用提交。IM 只新增独立 `sta` 信任命名空间；`dev/pre/online` 与未知环境拒绝规则保留。跨项目整合负责人固定 IM、BIZ/authority/operator、Web 和 CLI 的提交及实际制品 SHA256 后才部署。本文件是配置及消费合同，业务唯一协议仍为 chatbot43 `docs/mindlink-conversations-contract.md`；历史验收不充当本轮现场证据。

## 配置与制品

正式配置字段源是 [MindlinkConfig](../commons/configures/mindlink.go)，完整占位样例为 [mindlink.example.yaml](../deploy/sta/mindlink.example.yaml)，合入受限 `/opt/conf/config.yml` 的顶层 `mindlink`。默认关闭；实际启用时显式设置 `mindlink.enabled=true` 和 `mindlink.disable_authority_source.enabled=true`，替换全部占位。此键名沿用现有实现，同时供 P1、N1、C1、C1 取消使用，不能改成 `authority_source`。

每条 trust 精确绑定 `issuer/kid → audience/environment/tenant_id/appkey/system_actor_id/public_key_ed25519_base64/query_operations/mutation_operations`。STA 使用自己的 issuer、kid、租户、app、system actor 和公钥，环境恰为 `sta`；不得引用 pre/pro 信任。`appkey` 最多 20 字节。公钥是 32 字节 Ed25519 原始公钥的规范 padded Base64，不能填写 PEM、私钥或无 padding 编码。`system_actor_id` 必须与 BIZ 原 wire 和 authority 固定配置一致；system actor 的 generation 必须为 0。签名通过仍须核当前 owner、代际、策略和原件。

authority 程序由同次固定 BIZ 提交构建 `cmd/mindlink_authority`，Linux/amd64 静态二进制挂载到 `/opt/mindlink-authority`。配置只读挂载到 `/run/43chat-sta/authority.json`，DSN 文件单独只读挂载到 `/run/43chat-sta/authority.dsn`；读取的业务库为现有独立 `chatbot43_sta`，IM 配置的数据库仍为现有 `im_sta`。authority 仅查询业务持久原件，无需 BIZ 私钥；BIZ 私钥留在自身受限文件 `/run/43chat-sta/signing.pem`，不挂入 IM。

[authority.example.json](../deploy/sta/authority.example.json) 给出 authority 的完整字段白名单：`enabled/environment/tenant_id/appkey/issuer/audience/system_actor_id/dsn_file`。实际启用时 `enabled=true`，其 scope/trust 与 IM、原父/子意图一致。`dsn_file` 为容器内绝对路径，不在 JSON 中保存 DSN 值；不能把 account-gate 的 `database_address` 字段混入 authority 配置。实际程序、配置须为普通文件及绝对路径；程序需可执行，私有文件受限。检查主机和容器内同一制品哈希、文件可读/可执行以及静态链接，不能用文件名判断版本。

[compose.mindlink.yml](../deploy/sta/compose.mindlink.yml) 是显式启用的只读挂载 override；基础 [compose.yml](../deploy/sta/compose.yml) 保持原行为。在已核实的受限发布环境中设置 `MINDLINK_AUTHORITY_PROGRAM`、`MINDLINK_AUTHORITY_CONFIG`、`MINDLINK_AUTHORITY_DSN` 为上述三个主机绝对路径，连同原 `IM_SERVER_IMAGE/IM_DB_HOST/IM_DB_PORT` 使用：

```sh
docker compose -p im-server-sta --env-file /ABSOLUTE/sta-release.env \
  -f deploy/sta/compose.yml -f deploy/sta/compose.mindlink.yml config --quiet
```

该命令只验证配置；启动由整合负责人按备份、迁移和发布授权执行。不能打印完整私有 Compose/env/config。镜像工作目录 `/opt`、入口 `/opt/imserver` 使用 `conf/config.yml`；STA 基础 Compose 跳过原 sed 渲染脚本，直接加载受限文件。

## HTTP 和固定 authority 协议

网关工厂 [NewMindlinkHTTP](../services/apigateway/services/mindlink.go) 只接受下表 operation。查询和 mutation 必须分别显式列入 trust 白名单；未知、重复或跨类 operation 拒绝。所有路径均以 `/apigateway/mindlink` 开始，无 query；GET 无 body，写请求为 JSON。方法、原路径和原 body 字节哈希均在 JWS 中绑定。

| operation | 方法与后缀 | 当前角色/用途 |
| --- | --- | --- |
| `principals.ensure` | POST `/principals/ensure` | system，P1 主体投影 |
| `principals.disable` | POST `/principals/disable` | system，N1 停用/永久退休保护 |
| `conversation.ensure` | POST `/conversations/ensure` | system，C1 固定会话/成员快照 |
| `conversation.cancel` | POST `/conversations/cancel` | system，只取消已被替代的旧 C1 原件 |
| `operations.query` | GET `/operations/:operation_id` | owner/system 按原控制操作和当前读权核原件 |
| `messages.send` | POST `/messages/send` | owner/avatar，当前会话写权；pair 主人拒发 |
| `messages.query` | GET `/messages/results/:request_id` | 原 send 的独立结果恢复及当前读权 |
| `history.read` | GET `/history/:conversation_id/:before/:limit` | owner，实际原生历史及当前读权 |
| `conversation_seen.query` | GET `/conversations/:conversation_id/seen` | owner，已读快照 |
| `conversation_seen.advance` | PUT `/conversations/:conversation_id/seen` | owner，读写权及真实已展示位置 |

正式 BIZ `lib/mindlinkim/transport.go` 的 endpoint 必须为裸 HTTPS origin（不能带 `/`、`/imapi`、query 或 fragment），只允许回环 IP 的隔离 HTTP。当前支持 `https://im-sta.43chat.cn` 加上述原路径。IM 没有 `/imapi/` 前缀路由，BIZ 当前 transport 也不支持此前缀。Nginx 必须原样传递签名路径和 Authorization；不能把 JWS path 改成代理前缀或通过重定向修正。此处不改变 TLS 校验。

[nginx-im.conf](../deploy/sta/nginx-im.conf) 从固定 `827fabc217f122edd105d793b7064a81258cded3` 的同名文件提取，未合入其旧业务源码。`im-sta.43chat.cn` 的原路径转发到 `127.0.0.1:19001`；`wsim-sta.43chat.cn/health` 和 `/im` 转发到 `19003`，保留已有健康探测修复。模板由 ALB 终止 TLS，现场 DNS/证书/转发链需另核；不代表已部署或验证 Nginx。

同一固定程序由 IM 选择如下参数，不从请求接受程序、DSN、owner 或快照：

| 用途 | 参数前缀 | 成功输出 |
| --- | --- | --- |
| P1 | `--principal --config PATH --operation-id ORIGINAL_PARENT_ID` | `mindlink-principal-authority-v1`，active，严格 22 字段含 display_name |
| N1 | `--config PATH --operation-id ORIGINAL_PARENT_ID` | `mindlink-disable-authority-v1`，retired，严格 21 字段 |
| C1 | `--conversation --config PATH --operation-id ORIGINAL_PARENT_ID` | `mindlink-conversation-authority-v1`，active，严格 15 字段 |
| C1 取消 | `--conversation-cancel --config PATH --operation-id ORIGINAL_PARENT_ID` | `mindlink-conversation-cancellation-authority-v1`，obsolete，严格 15 字段 |

快照逐项绑定环境、租户、app、issuer、audience、system actor、owner、原 parent ID/digest、wire ID/digest/body hash 和目标。取消回执沿原 ensure journal，不能换键伪造 ready。退出 2/3 为 NOT_READY，4 为 FORBIDDEN，其他执行/超时/输出错误为 UNKNOWN_RESULT；stdout 上限 32KiB、最长 10 秒、stderr 不外传。成功只证明本次真实读取，仍需 IM 同事务核权及保存不可变回执。

## Owner 屏障、准入和迁移

IM 当前没有直接开放 owner barrier 运维 HTTP/CLI。正式 [P1 事务](../commons/mindlink/principal_transaction.go) 在当前 BIZ `--principal` 原件通过后，仅能创建缺失的 `(environment,tenant_id,appkey,owner_id)` barrier 为 read/write=true；已有行不改值。已撤权的 false barrier 不被配置、签名、P1 重试或 BIZ account-gate 重开。整合负责人先只读核目标 owner 原行；已有撤权或 scope 不符即停止该 owner 的启用并定位原授权来源，不直接 UPDATE。

owner read 控制原件、历史、已读查询和收件箱读取；新发送/会话/主体/已读推进还需适用 write 权。原结果回放重新核当前读权，撤写不改已提交原件，撤读拒绝读取。完整原件、策略、代际、固定成员和 native fence 保护保留；authority、health 和签名白名单均不能替代它们。

BIZ 账户准入正式入口为同提交的 `cmd/mindlink_account_gate`，先使用其 `--config ABSOLUTE_SCOPE_JSON --owner OWNER` 只读查询；既有接入/分身恢复由正式 backfill 入口处理，之后按当前 revision、固定 request-id 和实际能力清单执行 active/paused。`--apply`、`--backfill`、`--ack-legacy-write-freeze`、`--expected-revision`、`--request-id`、`--capabilities`、`--ack-compatible-builds` 的前提以该版本 BIZ 源和交付为准。它不修改 IM barrier；不上传自造 Native 回执，不直接写映射 ready/generation/policy。本轮 `isolated_web` 仅在 BIZ STA 配套验证后启用，App 不冒充已验。

当前 IM 源内最新 `jimdb_version` 为 `20261117`，无需新增 STA schema。现有 `im_sta` 必须先备份、核当前版本和结构，再由 [Upgrade](../commons/dbcommons/dbmigration.go) 仅执行大于当前版本的内嵌有序迁移。禁止执行空库 `db.py init`、重放新库 SQL 或改版本号跳过迁移。MySQL8 的 InnoDB、utf8mb4_0900_ai_ci 原生身份/生成列/唯一索引、owner/principal/conversation/lifecycle 表及 nickname 容量均需满足；可靠消息还需 outbox、history、sequence、seen 等同库事务表。启动器先升级再初始化 native fence，失败则关闭 IM 监听；gateway 构造失败只保留 NOT_READY 处理器，不能因 `/health` 通过就判断能力已启用。

现场核验应从真实业务 sdk-bindings/账号准入、正式 P1/C1 回执、可靠消息原件恢复、收件箱/客户端收取和历史/seen 走通。核对同一消息 ID、序列、唯一 history/outbox/operation/ticket、当前 owner 屏障和实际版本；失败、未知、跳过和未覆盖分列。部署、真实数据库、Linux 运行、浏览器/CLI/App、重启与跨库竞争由整合负责人单独验收，本源码交付不代称已完成。
