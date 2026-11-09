# Mindlink 公共 HTTP 字段收口

DG-20261104-003，IM 公共接口负责人提出的具体源定义，供 BIZ 在唯一契约登记确认。Go 字段位于 `commons/mindlink/operation.go`；当前没有注册这些路由，不能调用或据此启用 ready。本提案最初依据 M 版；唯一业务事实源为 chatbot43 的 `docs/mindlink-conversations-contract.md`，本文件不替换它。字段确认后才实现严格 DTO 语义核验与生产权限事务。

后续核到 BIZ N1 工作草案；它尚未联合冻结。与本页提案的差异、建议取舍以及精确可信 RPC/最终事务候选见 [可信调用候选](mindlink-trusted-rpc-candidate_zh.md)。本页和 Go 值类型仍是 99f7ce5 的旧提案，不能与 N1 混作同一个 schema。

## 首个主体操作

| 路径与方法 | operation_type | body 类型 |
| --- | --- | --- |
| POST `/apigateway/mindlink/principals/ensure` | `principal.ensure` | `PrincipalEnsureRequest` |
| POST `/apigateway/mindlink/principals/disable` | `principal.disable` | `PrincipalDisableRequest` |

所有定义字段均显式必填；禁止额外字段、重复字段、null、浮点整数、大小写别名。disable 没有 display_name。body 原字节独立核 `body_sha256`；其中 scope、原操作/请求/摘要、principal target 和版本必须逐项等于已验证票据。

首个入口 actor 仅为 owner 或受信 system，票据 `avatar_generation=0`。目标分身代际另用 body `principal_generation`，不能与签票原 actor 的代际混用。body 的 expected/desired_projection_version 对应票据的 expected/desired_policy_version；首次创建为 0→1，后续为当前版本→当前版本+1，不按代际代替版本。头像代际、稳定分身及内部号均由同一持久绑定核验，不能从显示名派生或覆盖其他主体。

principal_id 为 `ma`+30 位小写 hex；avatar_id 为有效 UTF-8、1..100 字节；principal_generation 为正 int64；display_name 为有效 UTF-8、最多 50 个 Unicode 字符，适配原生 users.nickname，不截断。owner_id 为规范非零十进制字符串。不能向终端返回内部号 Token。

语义摘要的规范数组还须双方确认；现存 F2 九项存储意图没有 appkey、原 actor 和 HTTP 绑定，不可直接冒充 wire 摘要。建议业务适配器持久化 wire 子意图并绑定原 F2 意图，独立固定 wire 的 operation/request/digest/body；不能悄悄重写已存在的 F2 原件。此建议尚未得到 BIZ 确认，当前不新增该业务表。

## 不可变回执

`OperationResult` 所有 JSON 字段均保留，所有版本/代际为 int64 JSON 整数，字符串不做 trim 或 Unicode 归一化。它保存原验证 claims 的静态绑定，不含 jti/iat/exp，并额外包含：

| 字段 | 语义 |
| --- | --- |
| committed_policy_version | 本次事务实际锁定/提交使用的权威 policy 版本；投影更新为 desired，读取或发消息为原 expected=desired |
| desired_state | 该 operation 的严格对象；READ query 为 `{}`，advance 为 `{ "seen_seq": "规范十进制位置" }`，不把 query 伪装为写 0 |
| state | READ query 为 `observed`，advance/真正消息落库为 `persisted`，投影只有全部实际权限约束安装成功才为 `ready` |
| recorded_at | 事务内记录时点，固定 UTC 微秒字符串 `YYYY-MM-DDTHH:mm:ss.ffffffZ`，并非精确 commit 时刻 |

回执只有整个事务提交成功后才能返回；commit 应答异常为 UNKNOWN_RESULT，不能推断没有写。相同原操作重试返回原回执，不能用新的 policy/time 覆盖它。`ready` 不可用一次映射 INSERT 或 accepted 代替；如果后续安装尚未完成，BIZ 仍保持 provisioning。

READ GET envelope 沿 M：`{operation, state, state_observed_at, replayed}`，其中 state_observed_at 使用上述 UTC 微秒格式，首次读取的 state/time 随原 query 固定，重试不更新。PUT envelope 为 `{operation,state}`，后者重新核权读取当前快照，与原回执区分。下一次明确刷新应先恢复原 query，再产生新的逻辑读取。

操作结果 GET 的精确查询证明、完整 principal desired_state、语义摘要和可信 RPC/原生适配尚未冻结，不能由这些 Go 值类型推导为已有能力。初始主体 SQL 投影可以在 gateway 内直接使用同一事务；跨 Actor 的消息/READ 接入必须明确传递不可伪造证明及最终共享事务屏障，不能给终端 UpMsg 增加自报可信字段。

## 已生效的错误字典

`commons/errs/errorcode.go` 已登记 19001 INVALID_REQUEST/400、19002 SCOPE_MISMATCH/403、19003 REQUEST_CONFLICT/409、19004 STALE_POLICY/409、19005 NOT_READY/409、19006 PRINCIPAL_DISABLED/409、19007 UNKNOWN_RESULT/503、19008 FORBIDDEN/403。仍沿 IM `code/msg` 错误封装，成功为 `code/msg/data`；BIZ 负责适配自己的 `message/timestamp`。

八个错误码从旧实现 HTTP200 的可运行红例修复；实际 `tools.ErrorHttpResp` 经 Gin/httptest 验证响应状态、Content-Type 和 JSON 字节。该验证不代表上面候选路径已注册或部署。

## Web 实施输入核对（2026-11-09）

回应 F5-WEB 消息 `pbz3u6a9wcqhukeh`，批次 `742b2924733db9e8083d076ebd296adff8ba7fdb9308fa142fd6a2d96f086728`。本节只记录固定源码、已存在制品与能力边界；不产生第二份业务契约。核对时 IM 公共实现为 `cd51070f14814395ac3b55196c38131ed2fc9577`，Web 候选为 `aa8aa3f15b1507984790b6d242a1bf74c8dfd68e`，BIZ 为 `dfd2533942799790cde047ed82aaaff0a43434f6`。

| Web 所需输入 | 本次核到的能力与限制 |
| --- | --- |
| 固定 SDK | Web 的 package.json 为 `^1.9.17`，package-lock.json 与实际 node_modules 均为 `jugglechat-websdk@1.9.17`；复现安装应使用锁文件。未重新安装或升级，也没有据此认证新 Mindlink 服务端兼容性 |
| 三类会话映射 | 唯一业务契约定义 `mindlink_group`（每 owner 真群）、`owner_avatar_dm`（稳定 avatar 单聊）、`avatar_pair_group`（规范排序 A/B 的三人群）。原生普通群/单聊不自带这些 kind、业务唯一映射、代际及权限投影；当前公共 IM 路由没有注册新的 Mindlink API，不能降级成普通建群后报告 ready |
| 主人服务端拒发 | 普通 WS mute 曾有实际拒发证据，但 F0 已实测签名 HTTP 的 `isFromApi` 绕过及 WS 自解禁后可发送，均查到历史/DB 原件。当前尚无已交付的专用群最终核权屏障；Web 只读 UI 不能关闭这些路径。反例及逐消息核对见 `mindlink-conversations-probes.md` §4、§9 |
| 消息持久化及可靠结果 | PERSIST 固定提交 `4ecec887fb38d2d81af4bf2773b05d304e8ced97` 是已有局部交付；当前通用 SQL 原操作/jti 账本不是消息生产者、可靠 outbox 或业务 SendWithKey。发送 ACK、accepted 及独立映射写入均不能充当新链路 persisted 证据；真实原件、结果恢复和业务投递仍待接线 |
| 真实展示消息序列 | 服务端 `commons/pbdefines/appmessages.proto` 的 `DownMsg.msgSeqNo` 为 int64/tag6；SDK 内嵌协议将同 tag6 命名为 `msgIndex`，格式化函数读取它并生成运行时 `messageIndex`。这是固定源码映射依据，未用本轮真实 SDK 收取/历史证明。SDK Message 类型声明未暴露该字段，不能把声明中 GetMentionOptions 的 messageIndex 当作 Message 字段保证；int64 解码精度及无损字符串传递尚未验证 |
| HTTP 历史与发送应答 | `services/apigateway/models/msgmodels.go` 的 `HisMsg` 没有序列字段；`SendMsgResp.msg_seq` 是发送应答。不能用后者、时间、unreadIndex、摘要 floor 或虚构的序列代替实际展示消息的 tag6；缺可靠展示位置时不推进 seen |
| pair 首页隐藏、零计数、无 push | 原生消息有计数位，但它不定义业务 kind 或所有首页/总未读消费者的过滤。`SendPush` 不检查计数位，其免打扰判断还允许 `PushLevel_IgnoreUndisturb`，随后独立增加 Badge；故 is_count=false 不足以证明禁 push/Badge/声音。当前没有真实 Web/App 与手机 push 的完整验收；页面过滤和局部零未读记录分别保留边界 |
| READ 与跨端 seen | READ 固定 `d336dec4cae9eff4067938b18e05215a5a0c60fe` 的局部 WithTx 与原件已核对，见 `mindlink-read-transaction-handoff_zh.md`。公共认证/typed 结果/最终权限及同事务历史快照仍未接齐，不能据此调用新公共 READ；展示 DTO 及跨端实际流程由 Web 与 READ/BIZ 继续对齐 |
| 已确认契约范围 | BIZ §11.4 的 actor 票据技术子集、L/M 查询意图与摘要规则已有对应技术确认，仍不等于完整 HTTP 上线。最新 §11.5 N1（首条 principals.disable、8 body 字段、15项摘要、22结果字段、UTC毫秒）仍标 `proposal_pending_IM_confirmation`；本页旧23字段/微秒提案不得混用。Web 使用 BIZ 的合法 owner API，不接触内部签名凭据或自行调用内部 IM 管理入口 |

SDK 锁文件的包地址为 `https://registry.npmjs.org/jugglechat-websdk/-/jugglechat-websdk-1.9.17.tgz`，integrity 为 `sha512-J33/j0xzSPy2Jr9lSbTBCM75ojeIrwfQtUmxUKQBhV07cvflItXV0FlP3MbW3WF7mEbM+ivYXpsL16Y2F7CYyw==`。本机实际 index.js SHA256 为 `b2bea5d17cb481d4c6e58c3705c2e4b296c1bb3fbabf992e5329667371e3b114`，index.d.ts 为 `a909d4597469a80ddb79429a1d8868e4d60026c9dfb95a76758a97e0771dac70`；它们不是本轮新发布的 SDK。

唯一业务契约为 chatbot43 的 `docs/mindlink-conversations-contract.md`，本次文件 SHA256 `c2b5cce47269976487fd0ddc26a0ac3e1c5bac8fce3a2027b2d06b384505d0dc`；显示名补充提交 `6d0b3e7f8` 没有把 N1 改成联合冻结。此处不复制或另行冻结其 JSON。现存 READ 局部制品、PERSIST 提交及 Rocky 旧诊断镜像不是统一的 POLICY/RELIABLE/READ Web 可用包；当前没有可宣布完整功能就绪的固定服务端制品。

本批实际执行为源码、锁文件、当前提交、契约状态和文件哈希的只读核对；没有新跑 SDK/浏览器、SQL、跨端 seen、手机 push 或服务端拒发。本节引用的旧实测及提供方材料不计作本批新验证，没有安装依赖、改其他负责人的文件或切换服务。
