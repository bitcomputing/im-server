# READ 原生能力与 BIZ 契约交接

2026-11-06，任务 `DG-20261104-003/F1-READ`。本轮根据业务负责人有效投递 `pbyd8pz48fchukeh` 核读当前源、唯一业务契约与既有验收材料，只交接现有能力及待定绑定，不修改业务契约或公共实现，不重新运行产品测试。

**接受 IM 持久 `conversation_seen` max 为唯一可写阅读权威，BIZ 只认证、映射和适配，不另建业务可写 seen。** 这是 READ 负责人的本次技术确认；仍需业务负责人更新唯一契约并回传新版本/SHA，与原 IM 管理核对后记录共识。它不表示 HTTP 已注册或产品已验收。

本轮基线 HEAD：`932ef61ffc813fd03dd4e4d2c17898bace96d1b8`。其测试源码为 `a35c8779c43d23e42cd6d49fc295f454abf6f2ff`，依赖 `33d2b9bb1a8aad82d95ed25d890476e9496d5546` 整合为 `7ccc6e9`；生产源码及历史证据对应关系见 [integrated-manifest.json](integrated-manifest.json)。本轮核对的 `chatbot43/docs/mindlink-conversations-contract.md` Git blob 与工作区 LF 一致，SHA256 为 `0A117252225B049C15E1B24160ED8D4EB2AB1DA44631070A1328E80EFF2A0480`，最近该文件提交 `b2e1179fcc7260c96932d23503dc116ca8971930`，仍是提案 C。

## 可直接复用的正式原生源

| 项目 | 当前正式源及行为 |
| --- | --- |
| 命令注册 | [conversation/starter.go](../../../services/conversation/starter.go)：`qry_conver_seen`、`set_conver_seen`，均通过原生 Query transport 调用；后者语义是写操作，不因传输名称 Query 而成为只读 |
| Proto 源 | [mindlinkread.proto](../../../commons/pbdefines/mindlinkread.proto)，生成件 [mindlinkread.pb.go](../../../commons/pbdefines/pbobjs/mindlinkread.pb.go)；不改旧 PB 字段编号 |
| 请求 | `QryConverSeenReq`：`string target_id=1, ChannelType channel_type=2, string sub_channel=3`；`SetConverSeenReq` 同前三项并加 `int64 seen_seq=4`。没有 `user_id`、`owner_id`、业务 `conversation_id` 或 `request_id` 字段 |
| 响应 | `ConverSeenState`：`int64 seen_seq=1, int64 latest_seq=2, bool has_new=3`；`has_new = latest_seq > seen_seq`，不是未读条数 |
| Actor / 服务 | [seenactor.go](../../../services/conversation/actors/seenactor.go) → [seenservice.go](../../../services/conversation/services/seenservice.go) 的 `QryConverSeen` / `SetConverSeen` |
| 持久源 | [seen.go](../../../services/conversation/storages/dbs/seen.go)：查询、`INSERT ... ON DUPLICATE KEY UPDATE seen_seq=GREATEST(...)`；[20261106.sql](../../../commons/dbcommons/sqls/20261106.sql) 正式表 `conversation_seen`，唯一键 `(app_key,user_id,target_id,channel_type,sub_channel)`，`seen_seq BIGINT`，二进制排序规则 |
| 共享序列依赖 | [msgconvercache.go](../../../services/commonservices/convercache/msgconvercache.go) 与 [msgsequence.go](../../../services/commonservices/convercache/msgsequence.go)：stored Private/Group 在发送副作用前事务预留序列和时间，水位键 `(app_key,canonical conver_id,channel_type,sub_channel)`；[20261107.sql](../../../commons/dbcommons/sqls/20261107.sql) 种历史/seen max，READ 006 保持原样 |

## 语义、授权和错误边界

原生租户和 requester 来自 RPC context。真实 WS 查询链 [imlistener.go](../../../services/connectmanager/server/imlistener.go) 使用已认证连接的 appkey/userid；服务不使用客户端 routing target 代替 requester。未来 server-only HTTP/actor 入口必须先完成授权后再封装同等可信 context，不能把 body 自报 `owner/user/app` 直接当该事实。已有 [ToRpcCtx](../../../services/apigateway/services/common.go) 的 requester 参数本身不提供业务授权。

`channel_type` 仅 Private=1、Group=2。Private 的 target 是 peer IM user ID，由 [GetConversationId](../../../services/commonservices/conversationservice.go) 对 requester/peer 排序后用冒号连接；Group 的 target 是 IM group ID。seen 是用户自己的 scope；共享 sequence 是会话 scope。BIZ 应从已授权、已持久的映射得到这些值，不让端传任意 IM target/subchannel 选择其他业务会话。当前校验 app 非空且最多20字符、user/target 非空且最多32、subchannel 最多32，UTF-8 合法、seen 非负。空 subchannel 是默认通道。

`latest_seq` 取同 scope 最新**可见且已持久**历史的 `MsgSeqNo`。群先检查 membership，并合并群/成员配置中的隐藏入群前历史边界；还取会话清理和用户清理时间的最大值。查询使用 [GroupHisMsgDao.QryHisMsgsExcludeDel](../../../services/historymsg/storages/dbs/grouphismsgdao.go) / [PrivateHisMsgDao.QryHisMsgsExcludeDel](../../../services/historymsg/storages/dbs/privatehismsgdao.go)，排除全局/用户删除、过期，群消息还检查 portion 收件关系，按 `send_time` 取最新一条。它不读预留水位或 unreadIndex，不用序列差计条数。

成功但可见历史为空时 latest=0；无 seen 行时 seen=0。历史删除/清理后可以 latest < seen，甚至 latest=0、seen>0，seen 不重置，has_new=false。元数据/历史查询失败或已有最新消息的 seq 非正数返回错误，不能伪装成空历史。Mongo 无清理记录错误已归一化，但仅有错误边界注入证据，没有真实 Mongo 验收。

stored no-count 消息仍分配序列、进入历史并影响 latest；count 位与 store 位独立，源见 [msgflag.go](../../../services/commonservices/msgdefines/msgflag.go)。已验证 flags 8/72，不能据此扩成真实 push/badge 验收。首页 `clear_total_unread` / `clear_unread` 走普通 conversation 状态，不写此表。跨设备通过同一持久 max 合并，其他设备重新查询得到状态；没有新增阅读状态广播。

新提交的 seq 若同时高于当前可见 latest 和已持久 seen，拒绝；旧值、重复值按 max 不倒退。若删除使 latest 降低，已确认的旧位仍可幂等重交，不能把这个例外误写成“任何高于 latest 的值都拒绝”。服务确认写语句成功并再次读取后才返回成功；不等待延迟 PERSIST 队列。若写已成功但后续读取/响应失败，调用者可能得到错误/未知结果，不能据此声称未写入；在重新核权后查询或重交同一位置即可，不把 max 幂等扩成 `request_id` 的持久结果协议。latest 与 seen 是本次读取快照，并非跨并发新消息的一次原子全景快照。

| 原生数字码 | 来源与含义 | BIZ 待正式映射 |
| --- | --- | --- |
| `0` | 成功，返回 state | 不等于业务鉴权、execute 完成或 inform 确认 |
| `12004` | `IMErrorCode_MSG_PARAM_ILLEGAL`：空/非法身份参数、类型/长度/负数、超最新的新位 | 业务参数/未来位错误的状态和 payload 尚待正式绑定 |
| `13002` | `IMErrorCode_GROUP_NOTGROUPMEMBER`：群内 requester 非成员 | BIZ 仍需先做 owner/会话/policy 权限；私聊不是群成员模型，其他 requester 会查其自己的 canonical pair，可能成功返回空历史，不能依赖它替代业务拒绝 |
| `10009` | `IMErrorCode_API_INTERNAL_RESP_FAIL`：seen、membership 配套元数据、清理或历史存储错误；非法已存 seq | 失败关闭，不能返回成功的零值或乐观推进 business seen |
| `1` | Actor 默认 `IMErrorCode_PBILLEGAL`，输入不是预期 PB 对象；传输还有独立错误 | 连接/超时/PB/非零 ACK 与成功 state 区分，HTTP 数字码映射不能直接臆定 |

数字定义在 [commons/errs/errorcode.go](../../../commons/errs/errorcode.go)；分支源为 `seenactor.go` / `seenservice.go`。这些是现有原生错误，不等于新业务 HTTP 错误已经注册。

## BIZ HTTP/认证仍待新增或确认

| 项目 | 已有 / 本次意见 | 尚待谁确认或实现 |
| --- | --- | --- |
| 唯一权威 | 接受 IM `conversation_seen` max 唯一可写；BIZ 不能双写另一份 seen | BIZ 更新唯一契约 §11.3、新 SHA 回双方；原 IM 管理集中共享接线 |
| 主人侧方法/路径 | 提案 C 已候选 GET/PUT `/api/personal/agent-conversations/:conversation_id/seen`，本 READ 版本没有实现该绑定 | BIZ 注册 handler、登录态鉴权和业务映射；不是本次新增已生效路径 |
| 上游 IM HTTP | 可复用两原生命令及字段；本 READ 版本未交付新的 IM seen HTTP endpoint | 精确方法/路径、server-only schema/proto/context、响应 envelope、超时和码映射，由原 IM 管理与 BIZ 登记；本文件不虚构正式路径 |
| JSON 序列 | PB 是有符号 int64；建议 HTTP `seen_seq/latest_seq` 始终为非负十进制**字符串**，含空状态 `"0"`，`has_new` 为 boolean，以免 JS 数字精度丢失 | 此为建议，尚未双方冻结。PUT 要求显式 seen_seq，拒绝缺字段/null/空串/非十进制/负数/超 int64；不能套用 proto3 缺省0，响应零值也应显式给出；由 BIZ/原管理确认实际 JSON schema |
| request_id | 提案 C 现有候选；原生 proto 不接收/持久化它，只有位置 max 的状态幂等 | BIZ 确认其用途、重复/冲突语义及是否需要可靠操作结果；不能假称现有 RPC 支持 request-id 去重结果 |
| actor/scope/allowlist | BIZ 校验当前 owner、会话参与/可见范围、功能/policy 等，再映射 requester、target、channel、subchannel；原 IM 方确认可信 context | query/write 分离的 operation_type 名称、精确 method/path/body、issuer/kid/env/tenant/app/owner 绑定与代际/policy屏障、可读/可写角色名单均待正式化；沿唯一契约 §11.4，不用旧普通管理签名或可自报 requester 绕过 |
| 展示确认 | 端在成功展示该授权会话的真实消息序列后提交，READ 验证 scope/上界 | 原生不能证明 UI 真展示；实际页面/SDK/32字符映射需真实端验证，不以 unread=0 代替 |

以上建议只服务于契约对齐，不配置密钥、不申请新权限、不替代原管理的共享写权。query 票据不能因原生写命令也使用 Query transport 而取得写权限。

## 已有证据与本轮核对的界限

此前版本对应的 [integrated-verification.md](integrated-verification.md)、[integrated-review.md](integrated-review.md) 与 manifest 已交付：MySQL seen schema/max/隔离、实际 service 错误分支、模拟双端、首页清未读独立、普通 IM-only 重启；真实历史清尽后再冷启，新消息 latest4/seen3/has_new=true，ACK/双端实收/历史/摘要 ID 一致，最终 DB seen3/floor4/history seq4/三份摘要同 ID，MySQL 未重启。独立复审 0 Critical/0 Important，1 Minor 为自动断言覆盖，本轮 SQL 事实另有精确核对。

本轮只核读正式源码/唯一契约，并复核版本和已有证据的对应关系；没有重跑这些产品场景，也不把过去 PASS 当本轮新实测。BIZ HTTP/actor 授权、真实页面/App/push、live Mongo、混合写者/负载及 RELIABLE 异步持久结果/幂等/outbox仍未验收。十九固定fixture SKIP、广义 message vet 六项旧 Mongo 告警和 IM stop137 的局限保持原记录。
