# Mindlink RELIABLE 文本发送正式源 v1

DG-20261104-003/c82，当前3f7ac87b续办。字段/编码来源是本仓库 `commons/mindlink/message_send.go`、`message_result.go` 及[独立Go编码向量](evidence/20261109-mindlink-reliable-wire/vectors.json)。BIZ负责人将固定差异登记到其唯一业务契约；不改旧N1 disable，旧067摘要/DTO不是这套wire。

独立开发分支已实现 messages.send/query HTTP、原生 MySQL history、持久 outbox 与原始回执的同事务提交；默认配置仍关闭。当前 persisted 只证明保存成功，尚未接通 BIZ F3 wire、客户端投递和页面，不能据此宣称完整消息流程可用。

## 操作与认证

| operation_type | 方法/路径 | 输入/权限 |
| --- | --- | --- |
| messages.send | POST /apigateway/mindlink/messages/send | application/json；下列严格13字段；owner/avatar，无system |
| messages.query | GET /apigateway/mindlink/messages/results/:request_id | 无body/query；路径为原wire request_id；22项JWS中的operation_id仍为原send wire ID，不是新mutation |

沿现有独立Ed25519 JWS完整22项及issuer/kid→audience/scope/operation allowlist，不改N1字段。scope来自可信绑定；owner_id为正uint64规范十进制string。owner actor_id=owner_id/generation=0；avatar actor_id为当前归属分身ID/generation>0。发送时两个policy字段均为当前固定conversation policy，显式正int64且相等；发送不推进policy。GET两个版本为当前独立读授权policy，不覆盖原结果中的旧policy/actor。

target_id精确等于业务conversation_id。Native requester/peer/group/channel/subchannel只从确认的不可变投影重建，body不得自报sender、scope、actor、native坐标或grant；pair主人只读须在最终提交核，不由管理员/system代发绕过。GET按已验证issuer+原operation ID定位并核原audience/scope/目标/request/digest，当前读权限另核，返回原send而非当前状态或新逻辑读取。

## 严格send body（全部显式）

| 字段 | 精确形状及来源 |
| --- | --- |
| version | mindlink-reliable-v1 |
| operation_id | 本wire子意图32随机字节的64小写hex |
| request_id | 原已持久F3业务parent operation ID，64小写hex；不是客户端自由request_id或trace |
| request_digest | 下述新18槽语义SHA256的64小写hex |
| parent_digest | 原业务父意图本地摘要64小写hex，保留旧值；不是新wire摘要 |
| delivery_snapshot_sha256 | SHA256(原父意图已冻结Snapshot原始持久字节)，64小写hex；不在重试时重读接待/名册/重序列化当前状态 |
| conversation_id | 原业务固定会话64小写hex；不等同原生canonical/peer/group ID |
| policy_version | 正int64 JSON十进制整数，与票据两policy字段相等 |
| message_type | 首版仅jg:text；其他能力NOT_READY |
| text | 非空UTF8 string，最多65536 UTF8字节；不trim、不NFC、不丢空白，不接受null |
| mentions | 必填非null array of avatar ID string；[]可，无隐式@all。每项1..100 UTF8字节，至多1000项，按原UTF8字节严格升序、无重复；最终须属该固定会话，不据格式授予权限 |
| reply_to | 必填，仅本字段可null；非null为1..20字节、每字节0x21..0x7e（不含空格）的ASCII原IM message ID。producer须从同scope/原会话/当前可读历史取得引用，不能接受自报引文或借它换目标 |
| asset_ids | 必填非null array of string；首版必须[]，非空NOT_READY，不忽略/降级为纯文本 |

拒绝未知/重复字段（包括转义后同名）、大小写别名、缺字段、非法UTF8/孤立代理、尾随JSON、错误类型及整数的字符串/指数/小数/越界表示。body_sha256为原HTTP字节独立哈希；相同语义不同body字节仍不能覆盖第一次持久wire原件。

原F3本地request_digest并未定义这份完整wire，亦不能代替被冻结受众/接待Snapshot的字节绑定。因此parent_digest与delivery_snapshot_sha256分别保留，父ID/摘要/快照、固定wire版本/ID/body/digest/native映射须在首次网络发送前持久。BIZ投递/原会话回复结算仍只取原父记录及原attempt，不从查询时的新接待重算受众。IM核真实当前归属/代际、固定成员及policy，不能从这两个业务哈希推导权限。

首版每个F3父意图恰一个固定conversation、一个不可变wire子意图和一个IM message ID。execute/inform/@的多人扇出是对同一消息的业务投递，不生成多个IM send；不支持把一个parent分成多目标、分段流式或外部relay子发送。去重键沿原journal的issuer＋完整scope/owner/actor＋request_id，原operation及body/digest/目标另完整绑定；同parent不能以换wire ID、换版本或换目标绕过未知恢复。未来真需要多条物理消息时各建明确的新业务父意图，不能复用这个原键。

## 精确18槽语义数组

```text
[version,"messages.send",environment,tenant_id,appkey,owner_id,
 actor_kind,actor_id,avatar_generation,conversation_id,policy_version,
 parent_digest,delivery_snapshot_sha256,message_type,text,mentions,reply_to,asset_ids]
```

严格顺序；Go默认json.Marshal，紧凑UTF8、EscapeHTML默认true、无末尾换行，整数不经float，[]保持空数组、reply_to=null保持null，不做Unicode规范化。SHA256输出64小写hex。operation_id/request_id/issuer/audience/method/path/body_sha256/jti/时间不在这18槽内，但仍逐项保留在原JWS/原operation日志绑定；绝不从摘要里移出就丢失绑定。

向量的ASCII/Unicode/MaxInt64-policy三个SHA分别为06a2f852…4abed、c31b5de3…f6322、8d951dea…c6645，完整原语义字节和64位结果见vectors.json。Unicode覆盖HTML转义、U+2028及四字节emoji；MaxInt64向量只是编码/解析，不声称有该实际有效policy或可用消息序列。

## persisted原回执（严格30字段）

现有成功envelope仍为code/msg/data，data恰`{operation: MessageSendResult,replayed:boolean}`；第一次提交replayed=false，复放/原查询true。只有真实native history＋outbox＋原回执同事务COMMIT成功后才返回成功persisted。首版不先存accepted再更新；failed/unknown/accepted/ready不能伪装为此成功原件，参数/权限/能力/未知提交走非成功且data=null。

| 字段组 | 必填约束 |
| --- | --- |
| version, operation_id, operation_type, request_id, request_digest, body_sha256 | 版本固定；原messages.send及其完整原wire绑定，不换成query的ID/type/body hash |
| parent_digest, delivery_snapshot_sha256 | 原send body中的两个独立业务绑定 |
| environment, tenant_id, appkey, owner_id, actor_kind, actor_id, avatar_generation | 原mutation完整scope/actor/代际，不换查询者 |
| conversation_id, policy_version, state | 原业务目标/原policy；state只允许persisted |
| message_id | IM生成的真实native message ID，1..20字节、每字节0x21..0x7e的ASCII；首次调用前不能由BIZ制造或预填 |
| msg_seq_no, msg_time | 规范正十进制JSON string，1..MaxInt64；序列和原history毫秒时间，不收number/指数/前导零/零/空值，不代表消息条数或每用户inbox游标 |
| channel_type, native_conversation_id, sender_principal_id, target_principal_id, sub_channel | 与原已确认native绑定一致；首版Private=1/Group=2，sub_channel必须空。sender/target各1..32 UTF8字节且不含冒号；private两者不同，canonical为二者按原UTF8字节排序以冒号连接，receipt.target为原peer；group canonical=原group/target。不得从query者、opaque ID或收到的receipt推导expected绑定 |
| outbox_id | SHA256(Go-JSON数组["mindlink-message-outbox-v1",原issuer,原operation_id])的64小写hex；原发送冻结事件标识，不是送达证明 |
| commit_token | 原效果/回执同事务生成并保存的64小写hex不透明token；不可排序、不可替代policy/MsgSeqNo或DB提交证据，不在复查时新造 |
| accepted_at, committed_at | 固定UTC毫秒YYYY-MM-DDTHH:mm:ss.SSSZ，committed>=accepted；后者为事务内记录时间，只在COMMIT成功后对外返，不是存储引擎精确物理commit瞬间 |

`MessageNativeBinding`必须来自已确认且属于原actor的不可变投影。`ValidateMessageSendResult`先复核原send严格body/18槽/原字节hash，再检查所有30字段与原请求和expected native绑定。该函数只是形状/绑定校验，不是响应签名或源认证；调用者贴JSON不能凭解析成功成为persisted证据。BIZ只能消费固定受信IM producer/原查询返回，并与自身原父/子意图核对；之后再原子结算067投递/058回复。

## 事务和恢复的后续实现要求

当前已有Go格式代码，不据此称下面效果已实现：最终锁原operation→owner barrier→主体/固定conversation/member权威→ticket；正文/引用/mention与当前读写scope一致后，在同一真实SQL事务预留原生序列，写真正p/g_hismsgs的DownMsg PB、不可变outbox及严格原回执。同键重放不预留序列/重effect；异摘要/目标/快照/body字节冲突，无新消息/outbox。

响应或COMMIT结果未知只沿原wire键查询，重核当前读权、不换ID、重发或刷新受众；原结果查询不是旧operations.query（后者仍限disable），也不由找不到/不可达拼persisted/failed。历史保留或之后撤权不改原committed结果，当前读取权限仍独立核验。

现有native DispatchMsg/SendBoxActor会重写recipient时间并重复写inbox/sendbox，异步pool无持久完成保证，不能直接接收此已提交事件。实际客户端投递还需delivery-only接合、原event去重及每用户cursor/cache/restart验证。outbox durable/persisted不声称客户端已收、execute已领/完成或inform已消费；保持生产配置、信任、HTTP候选关闭到适用验收。

## Rocky 原生发送实现与验收边界

`20261114.sql` 和新库初始化增加 `mindlink_message_outbox`，保存原请求字节、原生坐标、原生 DownMsg protobuf、冻结收件人及 pending 状态。它与 message_sequence、p_hismsgs/g_hismsgs、原 operation 和 ticket 共享同一事务。现有原生序号入口也使用 `commons/messagesequence`，保留历史最大值、已读游标、缓存水位及 Mongo 历史读取；RELIABLE producer 仅支持 MySQL，同库参与表必须为 InnoDB。

最终授权锁主人屏障、会话、成员 principal 和原生用户/群名单；验证 scope、当前策略、代际和 active 状态。配对群主人不能发送；原回执读取仍核当前读权。mentions 只能指向当前会话成员，转换成原生内部号。回复引用按同 app、原生会话及子通道读取非删除历史，去掉递归祖先。DM 历史保留原生接收方视角，receipt 独立保存真实 sender/receiver；配对群不带未读计数标志。

发送原键和完整签名绑定重复时返回同一回执，正文或目标变化冲突；GET 使用原 request ID 路径和原 operation ID，不产生新序号。查询共用原操作 journal 的票据、绑定、TTL 与事务检查，并额外核原 send actor、原 body hash、30 字段 receipt 和冻结 native binding。后台配送尚未消费 pending outbox；不得把这张表存在当作已送达。

Rocky 隔离 SQL 证据（忽略目录 `D:/projects/.evidence`）：

- `dg003-message-sequence-atomic.log`：序号与历史共同回滚、12 路并发唯一、溢出拒绝。
- `dg003-message-sequence-legacy.log`：原生旧入口历史清理、已读水位、scope、24 路并发与失败回归，6 顶层通过。
- `dg003-message-producer-full.jsonl`：mindlink 96 顶层通过/3 跳过，gateway services 2、routers 1 通过；真实 HTTP send/query、群/DM protobuf、重复与撤权、outbox/receipt 故障回滚及新库/升级回归。跳过的独立 BIZ 来源/网关子进程测试不算覆盖。
- `dg003-message-reply.log`：同键改正文拒绝、同会话回复引用、缺失引用拒绝且无多余历史。

早期测试复用了 C1 的 JTI 导致冲突，随后一次新 JTI 格式不合约；这些失败不作为故障回滚证据。修正为独立合法票据并断言各路径具体错误后才得到上面的有效结果。未覆盖浏览器、客户端实际收取、BIZ 新 wire、独立审查及生产启用。