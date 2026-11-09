# Mindlink 公共 HTTP 字段收口

DG-20261104-003，IM 公共接口源定义。唯一业务事实源为 chatbot43 的 `docs/mindlink-conversations-contract.md`，本页不替换它。回应 BIZ `pbz32ne2eduhukeh` / 批次 `1d4a6f925381dde9a316bbf3530d7c2f71081ba330c6a805810fd354b7c1d5e5`，本方确认固定 `6d0b3e7f81fe9f6f5ccba0f3fa6c797940bce538` 的 §11.5 N1 字段与技术语义，文件 raw SHA256 `c2b5cce47269976487fd0ddc26a0ac3e1c5bac8fce3a2027b2d06b384505d0dc`。BIZ 候选状态由其收到回执后登记，发信不作已登记证据。

当前 `commons/mindlink/operation.go` 替代 99f7ce5 的旧单数操作名、带 scope 的 body、23字段结果和微秒提案；`principal_disable.go` 实现已验证请求的严格 body/摘要解析。原结果查询核心为 `8f200ea23fe9b48fd558eb14ccf6ed59eebb6449`，公共查询接线为 `077d5dbb91b0c95e6cab527be44be414b7032cdf`。原生拒绝屏障已有下述实际接线与隔离验收；未公开的停用 SQL 事务原语已有隔离验证，真实业务权威写入源和全部适用读取面仍未具备，POST 不能据此启用 ready。

## 原生批量消息分类依赖（c74fc1cf）

Root新增只读Go接口 `bases.FilterNativeMessages(ctx,appkey,[]proto.Message)([]bool,error)`，复用既有递归身份提取；结果按输入序号返回，true只表示所提供身份未命中native拒绝屏障，不授业务owner权限。SQL按有界批次及原生0900排序规则分类，保留用户/群命名空间与app隔离；失败返回nil mask，空候选也检查SQL可用。该接口不会加载缺失的真实history/mentions，也不新增HTTP/proto或替代最终核权。

实现、红绿与查询量反向试验见[共享分类验收记录](evidence/20261109-mindlink-native-filter/verification.md)。READ仍须在其隔离候选中接线并完成分页/聚合、冷态与RPC/重建制品验收；不据此关闭完整READ、公共disable或066/c82。

## 原生 READ 与持久序列整合（2026-11-09，066519ee，Task4）

整合 READ 已交付的固定 `dcdd3a30ba4b802dce11ae396dc8f3711755ed4f`（含 `d336dec4` 的 WithTx 基础）及持久消息序列；未改业务公共 HTTP 契约。原生 `qry_conver_seen` / `set_conver_seen` 经过既有 Actor 拒绝屏障。查询和推进 seen 独立于未读计数；WithTx 只使用调用方事务，尚未接入公共 READ 的原操作快照与 typed 回执生产者。

新增有序迁移 `20261111.sql`，覆盖已处于20261110、会跳过旧006/007文件的数据库；新库同步 seen/sequence 表及版本。私聊、群聊历史的序列列扩大为 nullable BIGINT，保留原 NULL 默认值与注释。实际旧 INT 升级、新库、历史/seen 更大水位保留、重播及2147483648/MaxInt64精确存储已核对。无效版本在 DDL 前拒绝，元数据 SQL 失败不视为首次建库；任一迁移失败停止后续文件及监听器启动。

隔离重建制品已实跑原生 WS 查询/推进、Web/PC 共用 seen、普通与受保护目标、超大序列收取/历史解码、MaxInt64耗尽拒绝、历史物理删除后仅重启 IM 的保留行为。另用 Playwright 操作同一夹具的既有管理页面：普通私聊和群聊的发送、历史查询成功，新 ID 分别匹配唯一 DB 行、接收者43的 Go SDK 收取和持久水位，独立 seen 未变。该页面验证的是 schema 改动后的普通发送/历史流程，不是业务 READ 页面，也未证明已发布 JS/App 的 MaxInt64 精度。

当前 Task4 制品、红绿、失败/跳过及资源清理记录见[整合验收记录](evidence/20261109-mindlink-read-integration/verification.md)。公共 READ/disable、生产权威供给、盲列表/聚合完整过滤、ensure/SendWithKey 仍未完成；本增量不改变 NOT_READY，也不完成父批次。

## 原生拒绝屏障增量（2026-11-09，066519ee）

`20261110.sql` 将独立 `write_enabled` 默认设为 false，read_enabled 不授予控制写入；增加与原生 MySQL8 `utf8mb4_0900_ai_ci` 匹配的索引列，N1 的二进制 scope 仍精确比较。已投影 active/disabled 主体及包含其成员的群，均不因关闭公共 Mindlink 配置而恢复旧 Token、管理和消息权限。迁移可在完整 ALTER 步骤之间重跑；结构不符不推进版本。

登录与解码后 Actor 检查已提交 `98b93e5b9c7461f7d120d75aa08fb7e0aee0e5f7`。后续增量补齐业务 ServerPub、发送/导入/流式缓存、最终历史与收发件箱、在线发布和 push 接收者检查；异步 HTTP 在承诺成功或生成消息 ID 前拒绝，批量显式选择器不返回普通部分的假成功。数据库检查错误不伪装成离线回执。用户、群及会话哈希分别解释，消息正文与任意 KV 不作为身份字段。

已重建隔离镜像 `0b385f7cd50807ba97e966afc08fa4c8bca02ea549cd7820b3384f9e995bb09d`（服务二进制 SHA256 `e984fb5b00b1ebb2b7cb4d71875aa82d28ab351ce4c2804008e8d5ac9477fcbf`，Go1.25.12），在公共 API 配置关闭的实际进程中验证 active/disabled/旧会话拒绝、签名 HTTP 和 WS 管控通道/历史拒绝。普通 HTTP 私聊及 WS 同名普通群由 SDK 接收者43收到，并核对同消息ID的唯一历史行；所列非法写入条件为零行。夹具进程和数据库已清理，无长期测试URL；保留专用 MySQL、镜像和忽略的证据目录。

范围限制：这是所列 native 入口/写入路径的实现与验证，不是完整 READ、流式超时实测、实际 push 提供方、真实业务投影供给或 N1 停用 ready。盲会话列表/聚合筛除已交现有 READ 负责人评估。POST disable 继续 NOT_READY；未推送、合并或切换生产。详细红绿、跳过及旧环境失败见[接线计划](superpowers/plans/2026-11-09-mindlink-disable-native-gates.md)。

### 未公开的停用事务原语（Task3A）

`disable_transaction.go` 的 unexported `transactPrincipalDisable` 复用原 journal：锁既有 owner 写权限与不可变 principal/avatar，核目标代际不退和 policy CAS，再在同一事务写 disabled、目标版本及严格22字段原件/随机 token。`created_time` 首次插入显式使用 UTC，回执 accepted_at 取这条原记录；committed_at 为事务内观察时间，只有 commit 成功才返回。原操作重试仅核当前读权并复放原 ready，投影后来推进版本也不改写它；损坏或非终结原件不补成功。请求不能创建 grant 或 projection。

实际自有 MySQL 测试覆盖状态/原件原子提交、回执写失败回滚、读写权限区分、错 scope/avatar/代际/版本、永久墓碑、丢 COMMIT 应答后已提交原件恢复，以及撤权竞争两种顺序。Windows domain/base/db/errs/tools race 为71顶层+326子例 PASS、0FAIL、1可选 gateway-process SKIP；Linux Go1.25.12 的本原语 SQL 测试6顶层+12子例 PASS、0FAIL/SKIP。最初锁等待观测失败保留为失败；改用实际库/表的 performance_schema 锁对象后通过，并记录真实等待1而旧查询文本观测为0。受影响 Linux 编译测试及 vet 通过，独审 C0/I0/M0；原件哈希与命令见接线计划。

原语无生产调用，PolicyHTTP 的 POST 仍硬返回 NOT_READY。测试中的 write_enabled=true、投影和原件来自隔离夹具，不是生产生命周期授权供给；新 SQL pool 也不是 IM 进程重启。尚无此 producer 的公开 HTTP/重启验收，不称完整 F1 或停用 ready；无新路由、配置开关、proto、迁移或权限投影写入接口。

## 本轮查询接线（2026-11-09，066519ee）

沿 BIZ 固定 `7349850f69ca66f3a42e0bff1b303ddaccadb1e8` 的 N1，不修改具名机器块或默认关闭规则。正式注册 `POST /apigateway/mindlink/principals/disable` 与 `GET /apigateway/mindlink/operations/:operation_id`；两者独立于旧 app-secret Signature，只消费当前受信 Ed25519 请求。`mindlink.enabled` 默认 false、`trusts=[]`；无有效配置/数据库时为关闭的 NOT_READY，错误外层始终 `code/msg/data` 且 data=null，不暴露原始 SQL/认证错误。

GET 精确查询 verified issuer+原ID，核原 audience/scope/request/digest/target，在原事务中锁 `mindlink_owner_barriers` 与 `mindlink_principals`，核当前 read_enabled/policy 后绑定 query jti。read_enabled 默认 false；缺投影不建号/建授权，撤读权后重放原票据也拒绝。永久 disabled 目标可由当前合法 owner/system 查询，返回的 actor、原版本、desired_state 和时间仍是原 mutation；22字段/null/状态组合及语义摘要检查不合格时不补成功零值。迁移为 `20261109.sql`，同步新库 SQL。

POST 已接验签和严格 N1 parser；生产 authority/grant 写入源、disable producer 与全部适用 native final gate 尚未接齐，合法 body 仍返回 NOT_READY，不插入操作或虚构 ready。新增表及测试夹具中的 grant/原件不代表真实业务权威或实际停用。READ、ensure、消息可靠发送和跨 Actor 接线继续各自保留缺口；不因两条路由已注册称整包可用。

具体写集、执行命令、实际回归与基线失败分列在[本轮接线记录](superpowers/plans/2026-11-09-mindlink-query-http.md)。下面技术确认/旧提案的“尚未注册/GET未实现”是其当批历史状态，当前查询状态以本节为准。

本轮已重建隔离镜像并实际启动完整网关：在上游成功后丢弃HTTP应答，沿原查询恢复；随后重启该夹具网关进程，再以新query票据读取同一原件，均通过且原操作/夹具效果不增。有效当前权威和原件由测试夹具提供，不冒充真实业务投影或disable效果。临时网关已移除；没有长期可用测试URL或生产切换。扩展Windows检查中的既有CRLF源码文本断言失败单列，不称全仓全绿。

## 当前N1技术确认登记（2026-11-09，8323676b）

本批实读 BIZ 固定 `3cd520f4d371e0e351d7f6f1e41a01004c34db14` 的唯一契约，Git blob/LF SHA256 `3690217fa44a4521f24afdd5fe7de45beb2e85a395e1a6c05a7a092cec8f15dc`。§11.7已实际登记本方回执 `pbz5snzq6p2hukeh`；N1机器状态为 `technically_confirmed_runtime_closed`，`enabled_by_default=false`。因此“等待N1同版字段/角色/摘要技术确认”依赖已关闭，不再用99f旧单数/23字段/微秒重开等待。这是读取固定正式源后的登记事实，不再只是此前发信成功的推断。

与46c25固定版直接比对：routing JSON字节未变，N1 JSON只有上述status文本替换；8 body/15摘要/22结果及nullable、向量、默认关闭及其他未定义操作保持。这个status是契约登记状态，不是OperationResult.state，更不等于ready或功能开关。下方旧版本/其他交接文档的hash保留各自原时点，不再据其旧“待确认”描述阻塞当前wire。

只解除技术确认依赖：正式HTTP/原结果GET、生产结果校验与producer、权威投影/最终原生gate及实际撤权、F2 wire存储/适配、公共READ仍未实现或验收；ensure容量未齐继续关闭，不挡已有投影disable按原授权接线。此登记不新增真实密钥、配置、路由、合并/推送/部署或Rocky操作授权。提供方31规格及其独立31、IM旧红绿/SQL报告不作本批重跑；本批仅固定源/文档核对，未改Go/SQL或运行组件。

## 46c25语义澄清记录（2026-11-09，c41835a2）

回应 BIZ `pbz8cpbj65shukeh`，本方确认当前唯一契约固定 `46c25b90b0013b6cbb425224f572381692300e7f`、Git blob/LF SHA256 `c49a953cff22a5006068f3ad5974e4199d7de50a215d0073d693e05141410c80` 的 §11.5 N1 完整技术语义及 §11.6 时间/不可变原件、跨Actor与最终屏障候选边界。两段具名JSON与下节1d671版逐字一致，8 body/15摘要/22结果/nullable/UTC毫秒不变；d063/raw2daf仅是[历史草案观察](mindlink-trusted-rpc-candidate_zh.md)，不能用作当前确认版本。当前bb76类型/解析器也不再是§11.6核92e3120当时的99f旧DTO。

本次同版确认覆盖三类时间记录点、首条ready生产约束、READ原快照复放及RPC候选；不是已经联合登记、生成proto/注册procedure、生产效果或运行验收。首条同进程效果沿同一sql.Tx，不加RPC；确需跨Actor时的封装/重新验签及完整四维owner屏障、锁序和全入口gate仍按[候选说明](mindlink-trusted-rpc-candidate_zh.md)，均未实现。BIZ收到本回执后的唯一源登记由其负责，当前发信不证明其登记完成。

## 唯一源1d671登记记录（2026-11-09，a9bcc68a）

回应 BIZ `pbz76f48e4nhukeh`，本方确认唯一契约固定 `1d671dac7e0eeb122a695e420e9c3035fb6c65e5`，Git blob/LF SHA256 `3febb2f532dc091e6bd7c4a70a27ce0f0ffc0fb9ddc0f99018bef4ee432c12d2` 的 §11.5 N1 与新增 §11.6 四点技术边界。它继承前述6d0版，新增历史输入登记和错误字典事实修正，不把旧候选合并成第二套wire，也不等于双边冻结登记、启用或运行验收。BIZ唯一源仍由BIZ维护，本方不据发信声称其登记已更新。

本批直接读取两固定Git blob，`routing-contract-v1` 与 `mindlink-policy-contract-v1` 两段JSON内容分别字节全等；SHA256依次为 `03a85a1d9728dd62176ec764804b91221cc174276f780dd9e4797096a71d6642`、`e7e97b58c5538f0f6421799b019989c178f578ee92724befe12c2da8b9355c5a`。以下严格区分§11.6核过的历史输入与本分支已有替代，不能把时间较晚的历史登记当作旧Go类型仍在使用。

| 项目 | 固定99f7输入（§11.6所记历史） | 当前已提交的bb76f4b / 62da300 |
| --- | --- | --- |
| ensure / disable body | 13 / 12字段，含scope/projection旧命名 | 旧ensure DTO已删除且保持关闭；disable恰8字段，scope仅取JWS/原意图，正目标代际用target_avatar_generation，与actor代际0及policy分开 |
| 操作名 / 结果 / 时间 | principal.*单数、23字段、recorded_at微秒 | principals.*精确复数、22字段及nullable形状、UTC毫秒；已有严格disable解析，仍无生产结果/状态组合校验器 |
| 错误字典与入口 | 19001–19008/HTTP映射已落源 | 字典与ErrorHttpResp沿用；正式disable/query、最终撤权屏障/生产权威、READ/可信RPC仍未接通，不能以字典存在称入口可用 |

四点按本版保持：先已有号disable＋operations.query，ensure关闭；名字沿业务Go规范化后80标量/320字节，不降为50或截断，承载/有序迁移/真实回读未补足前不接线；F2旧ID/九项摘要不动，首次网络前须持久独立wire子意图及完整原父ID/digest、scope/actor/目标/body绑定，unknown不新造child或换键，真实ready再推进；READ的desired_state={}、原query快照/时间复放与新的逻辑读取分开，不以native max代替原请求提交。后两项的适配存储/唯一性/迁移、公共结果生产者和正式query均未实现。

本批实际为固定源、机器块字节及当前DTO字段/摘要顺序/时间格式对照；没有修改Go/唯一业务契约或重跑旧组件、Go/race/DB/HTTP/native。BIZ的31条规格及其独立31pass仅为提供方记录，不相加为62或算本方重跑；26/14、29顶层及容器状态同样不作本批新证据。仅本次同版反馈收口，c82/c328/READ及503仍保留各自任务边界。

## 首个主体操作

| 路径与方法 | operation_type | body 类型 |
| --- | --- | --- |
| POST `/apigateway/mindlink/principals/disable` | `principals.disable` | `PrincipalDisableRequest`，首条；解析器已实现，路由/效果关闭 |
| POST `/apigateway/mindlink/principals/ensure` | `principals.ensure` | 完整 body/权威及存储能力未齐，关闭；已删除旧未冻结 DTO |

disable body 恰八字段：operation_id、request_id、request_digest、principal_id、avatar_id、target_avatar_generation、expected_policy_version、desired_policy_version，全部显式必填；禁止额外/重复（含转义同名）字段、null、错误类型、指数/浮点整数、大小写别名、无效 UTF-8/孤立代理项和尾随第二个 JSON。scope、actor、display_name 不得混入 body。原字节先由 Verifier 独立核 body_sha256；解析器只收同一 Verifier 的已验证 Authorization，固定 POST/path/type，仅 owner 或受信 system。

票据 actor avatar_generation=0；body target_avatar_generation 为正 int64 的目标代际。expected_policy_version>=1，desired=expected+1，拒绝溢出；这些值及原 operation/request/digest/principal target 逐项绑定票据。15项摘要按唯一源 N1 顺序使用 Go json.Marshal 后 SHA256，不经 float、trim 或 Unicode 归一化。解析器不证明持久生命周期快照、目标代际不倒退或最终撤权屏障；这些仍须同事务核权，缺 gate 返回 NOT_READY，不写投影。

principal_id 为 ma+30位小写 hex；avatar_id 为有效 UTF-8、1..100字节；operation/digest 为64位小写 hex，request_id 为1..64 ASCII原键。owner_id 为无前导零的正 uint64 十进制字符串。未来 ensure 名称为严格UTF-8、非null string，沿 BIZ `x/text v0.37.0` 的 `norm.NFC.String(strings.TrimSpace(name))` 后1..80 Unicode标量/最多320 UTF-8字节，包含 stream-safe 插入的 CGJ；按规范快照拒绝 Cc，不笼统禁止 Cf，不按UTF-16单元或视觉字素计数。IM核规范快照原字节，不在验签后trim/归一化/截断。当前源码 nickname varchar(50) 不满足完整80标量承载要求；完整读写/新旧库能力未验前保持关闭，不降低业务上限。不能向终端返回内部号 Token。

F2-A 九项旧摘要不等于 N1 wire 摘要；BIZ须先持久化与本地意图关联的固定 wire body/digest，沿原件重试，不能覆盖旧意图。本批不改 BIZ 表/适配器。

## 不可变回执

`OperationResult` 输出值类型恰 N1 的22字段，不使用 omitempty。issuer/audience/method/path/body hash 继续完整留在内部原操作绑定，不因移出公共结果而省略。它不是任意外部结果解析器；当前没有生产者，不能把零值结构或任意 RawMessage 交给通用 Transact 当成合法 ready。后续生产者须校验严格 desired_state、状态组合及真实效果，并将回执/token/效果同事务保存。

| 字段 | 语义 |
| --- | --- |
| desired_state | disable 恰 state=disabled、avatar_id、principal_id、target_avatar_generation；READ query 为 `{}`，advance 恰规范 int64 字符串 seen_seq，未定义操作继续关闭 |
| state | 沿 N1 accepted/ready/persisted/failed/unknown；首条 disable 只在同事务禁用及全部适用 gate 落地后 ready。READ query 的 ready 仅证明原查询快照/回执保存，不证明 seen 写入 |
| effective_policy_version/effective_avatar_generation | nullable int64；disable ready 分别是原 desired 和目标代际；READ 后者为 null；accepted/failed/unknown 的未证明效果字段为 null |
| commit_version | nullable 64小写hex，不透明同事务原回执 token；不排序，不是 policy/MsgSeqNo/全局revision，不在重查时重新分配 |
| accepted_at/committed_at | 固定 UTC 毫秒 `YYYY-MM-DDTHH:mm:ss.SSSZ`；accepted_at取原wire意图首次记录点，与首次持久化一起保存；committed_at为nullable，取效果/终结回执在事务内的记录点，仅整个commit成功后随原件返回，不是存储引擎物理提交瞬间 |
| failure | nullable；N1未知结果仅沿原 operation 查，不换键重做；超时/查询失败不代造确定失败回执 |

回执只有整个事务提交成功后才能返回；commit 应答异常为 UNKNOWN_RESULT，不能推断没有写。相同原操作重试返回原回执，不能用新的 policy/time 覆盖它。`ready` 不可用一次映射 INSERT 或 accepted 代替；如果后续安装尚未完成，BIZ 仍保持 provisioning。

通用state枚举不是首条producer的阶段更新许可：当前Transact只保存不可变原件，不承接accepted→ready；其generic JSON对象检查也不是ready语义校验。首条生产接线仅在全部约束同事务落地后保存/返回ready，参数/权限/能力或提交未知走非成功错误，不拼operation回执。F2父意图的准备/unknown与已提交IM原件分开；未来非终结阶段能力须另行实现，不能改写原终结结果。

首条 POST/operations.query 成功 data 为 `{operation,replayed}`。GET `/apigateway/mindlink/operations/:operation_id` 只查已有 disable，按已验证 issuer+路径原ID定位并核原 audience、原 scope/目标/request/digest及当前读权，两 policy 字段相等于当前读授权版本；不能全局扫描或改写原 actor/版本，不新建 mutation、不再 effect。本轮已接查询入口与 SQL 读投影适配，默认仍关闭；找不到原件为 UNKNOWN_RESULT，缺当前权威为 NOT_READY，撤读权为 FORBIDDEN。

READ GET 沿 `{operation,state,state_observed_at,replayed}`，state_observed_at 统一UTC毫秒，与原快照回执复放；PUT `{operation,state}` 的当前 state 与原回执分开。N1 的后续阶段转换尚未实现：当前 Transact 只保存不可变终结结果，首条不会提交 accepted 再期待重试覆盖 ready。控制面仅owner/system；system不得发送消息或扩pair成员/解除主人禁言；READ首个入口仅owner。测试信任只接受 N1 默认关闭结构，使用临时 fixture 公钥替代不可用占位，不新增真实密钥/签发入口。

state_observed_at取该逻辑query首次取得授权阅读快照的记录点，与原快照/回执同事务保存复放，不以重查时间覆盖；query的ready/token只证明快照/回执已保存，不证明创建或推进seen。新逻辑读取另立query意图，复放不是新读当前state。operations.query仍只读原operation，不创建这个逻辑query意图或改原mutation/版本。

新路由、生产权限投影、结果/query生产者、可信RPC及原生入口gate均未具备；HTTP/WS/Token/登录/消息持久化最终检查须共用同一权威屏障，不能由解析成功或一次disabled UPDATE宣布ready。

## 已生效的错误字典

`commons/errs/errorcode.go` 已登记 19001 INVALID_REQUEST/400、19002 SCOPE_MISMATCH/403、19003 REQUEST_CONFLICT/409、19004 STALE_POLICY/409、19005 NOT_READY/409、19006 PRINCIPAL_DISABLED/409、19007 UNKNOWN_RESULT/503、19008 FORBIDDEN/403。仍沿 IM `code/msg` 错误封装，成功为 `code/msg/data`；BIZ 负责适配自己的 `message/timestamp`。

八个错误码从旧实现 HTTP200 的可运行红例修复；实际 `tools.ErrorHttpResp` 经 Gin/httptest 验证响应状态、Content-Type 和 JSON 字节。该验证不代表上面候选路径已注册或部署。

## N1 本批实现与验证

本批固定契约 Git blob SHA256 为 `3a36bc181526cb3e1ac02e67ddbbf9176af4b9abfb8b7fcf75eb657cc279a260`；本机当前文件规范 LF 内容与该固定 blob 一致，raw SHA 与前文区分。生产调用搜索确认旧 DTO 没有调用方；没有改动 auth/Transact、路由、proto、SQL、配置或其他负责人的文件。

| 实际执行 | 结果及原件 |
| --- | --- |
| API-only stub 下 `go test -json ./commons/mindlink -run 'TestPrincipalDisable\|TestN1OperationResultWireShape' -count=1` | exit1；4顶层/61子例 FAIL。有效三向量被stub拒绝，旧结果为23字段，未把失败当通过；`.local/mindlink-disable-wire/red.jsonl` SHA256 `b94695748402b06767e511ce1f4a7e881439f4fefd3a13978ae2b762541b6bfd` |
| 实现后同命令 | exit0；5顶层/64子例 PASS，0 FAIL/0 SKIP；包含追加的 system、uint64/长度边界及原字节保留；`green.jsonl` SHA256 `011f75c13c24cd1194c5419509bf1aa29dbc8eec632d679f5d9c7d8365fe9981` |
| `go test -json ./commons/mindlink ./commons/errs ./commons/tools -count=1` | 三包exit0，20顶层/180子例 PASS，0 FAIL/0 SKIP；`packages.jsonl` SHA256 `a262b159572fe39db16362a742e54dc736f98d16c479185dbbc274c1d35524d4` |
| 独立审查 `review_n1_disable_wire` | Critical0/Important0/Minor0；审查方另实际运行 `go test -count=1 ./commons/mindlink` exit0，不冒称本方重跑其命令 |

Go1.26.1/Windows；gofmt 检查及 Git whitespace 检查无差异。以上为无 integration tag 的 wire/默认包测试，不含 DB 用例；本批未运行 SQL、HTTP 页面、WS/SDK、Token/登录或撤权后原生提交拒绝。结果测试覆盖22字段/nullable零值及毫秒格式常量，没有验证实际生产者的非空结果/状态组合；没有可运行的生产者，不以结构类型通过报告停用生效或 ready。

## 显示名补充的同源核对（批次 b99bf22a）

回应 BIZ `pbz4yeatwj2hukeh`，本方确认固定 `6d0b3e7f81fe9f6f5ccba0f3fa6c797940bce538` 中显示名补充，Git blob SHA256 `3a36bc181526cb3e1ac02e67ddbbf9176af4b9abfb8b7fcf75eb657cc279a260`。本批实际比较：移除唯一新增 display_name_policy 后，整个 N1 机器块与 `7272e1afb65ba592b996f9ee1f3d83626abf0a7e` 一致；disable八字段/结果22字段/摘要15槽及其角色/代际/版本/原结果query规则不变。

本批从固定提交取出并逐份核对显示名证据清单19份 Git 原件的规范 LF SHA，全部匹配；失败草稿、KeyError准备失败、Go/Python红例及初审问题继续保留，不计作通过。BIZ的31项规格/独立复审属于其既有记录，本批没有重跑或改称本方验证。

已经检查所提供参考源码，仅用字符串/Unicode/JSON，无业务/DB调用。在 IM 当前模块（同为 x/text v0.37.0）实际运行该固定 `name_bounds.go.txt` 原件：Go1.26.1/Windows，GOTOOLCHAIN=local、GOWORK=off、GOPROXY/GOSUMDB=off、GOFLAGS=-mod=readonly，`go run .local/mindlink-name-b99/name_bounds.go` exit0。七个向量与固定 BIZ 输出全部字段相同，且两条机器块 canonical hex/计数/allowed 逐项一致；原程序的 Cc/Cf 边界断言亦执行。

| 合成向量 | 本批 Go 实际标量/UTF-8字节/CGJ | 契约含义 |
| --- | --- | --- |
| 51个ASCII / 80个汉字 | 51/51/0；80/240/0 | 超过旧50字符存储能力的合法名字输入，未运行实际保存 |
| 80 / 81个四字节emoji | 80/320/0；81/324/0 | 前者在上限内，后者超限 |
| 空白包围的 e+组合重音 | 1/2/0 | BIZ持久意图前处理，IM接收并保存其规范原字节 |
| a+31 / a+78个U+0305 | 33/65/1；81/161/2 | 后者因处理后81标量应拒绝；不以 Python 普通NFC代替Go字节oracle |

参考源码 SHA256 `598a902363d9fbb8588b48258a3e58607fa8ca1b859eca15f140a285dfd2ddb4`；本方新执行输出 `.local/mindlink-name-b99/actual-vectors.json` raw SHA256 `20a3c8a24f7d9f54f0ca39e56758ce02348395f03f416f131ab2e02ac9bdaee3`，规范LF内容与提供方固定输出相同。核查及比较结果见同目录 input-audit.json/verification.json，均为忽略的合成材料。

本轮只补交接说明和字符串参考验证，未新增生产名字校验器、ensure DTO/路由/迁移或改依赖/SQL。当前 sql/imserver.sql 的 nickname varchar(50) 是源码观察，live DB未核；未来还须按实际资料/标题调用链补足无损承载，并分别实查新库/升级/真实保存回读。此前不注册 ensure，不用截断回显或普通users/register代替。disable不接display_name且定义不受该后续缺口阻塞；本批不宣称禁用效果、原结果GET或完整POLICY已实现。

## Web 实施输入核对（2026-11-09）

后续3d83批次沿原c82分工给出[首条纯文本DM链的实际输入与最短缺口](mindlink-f5-dm-inputs_zh.md)：IM新链路由仍0，普通历史在IM/BIZ两端都未承载序列；新history操作/授权schema尚缺，字符串输入仅建议，不把前端model或旧签名接口当冻结接线。

以下保留上一批 `742b2924` 查询时点的版本与缺口；本轮 N1 同版确认及 Go 源替代以本页开头为准，不追改历史回传为当时已确认。

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
