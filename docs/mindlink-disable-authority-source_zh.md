# 停用授权供给边界

DG-20261104-003 / F2→F1，有效消息 `pb2kyn5wyvnhukeh`，当前批次 `306b966391af7ddb8c45f056296208ebb5d591ee0fddf8b5eddf2de1eef5e7e0`。只交付接口需求及协作边界，不实现新接口。当前 IM 基线 `e9cab3a53c311b74a67bfbab24e0a5b90d496551`，停用事务实现来自 `028bb2aa5359c96cc67337814b094239ba0b99a4`；唯一业务契约仍由 BIZ 维护。

## 当前可以消费什么

当前没有生产 `AuthoritySource` 接口、HTTP/RPC 快照入口、业务投影 writer 或允许直接消费 BIZ 对象的回调。`commons/mindlink/disable_transaction.go:transactPrincipalDisable(ctx, db, auth)` 是未公开的事务原语；它只接受同一 Verifier 验证过的 Authorization，严格解析既有 N1 body，并读取 IM 已存在的两类行：

| 来源 | 定位及消费字段 | 新效果要求 |
| --- | --- | --- |
| `mindlink_owner_barriers` | environment / tenant_id / appkey / owner_id；read_enabled / write_enabled | 同事务锁行，新效果必须 write_enabled；原件重放必须 read_enabled，二者不能互换 |
| `mindlink_principals` | appkey / principal_id 定位；environment / tenant_id / owner_id / avatar_id / avatar_generation / policy_version / state | immutable scope/avatar 相符；state=active，policy=body.expected，body.target generation不得低于当前；不存在为NOT_READY，不自动创建或回填 |

两表见 `20261109.sql`；独立 write_enabled 由 `20261110.sql` 默认 FALSE 增加。这些是当前 IM 存储列，不是准许 BIZ 写 IM 共享库的供给 DTO。原语没有生产调用，公开 POST 仍 NOT_READY。不得先把 IM principal 设 disabled 或把其 policy/generation 预推到 desired；那会绕开原效果或使新操作被拒绝。

## 业务只读供给的最小候选

后续有效消息 `pb2k9hx8gwahukeh` / 批次 `de450ca8d5aa96e5b5b182702f5648006def619f85e9842ccc55f5b8eb8f761e` 给出一次性受控进程 `cmd/mindlink_authority_source`、只读065 wire原件及输入仅parent ID的实施计划。本方本批接受这些作为Reader候选的收口方向，不要求另建HTTP服务；这是提供方计划，未核其实现/构建/运行。内部候选版本沿其 `mindlink-disable-authority-v1`；`076367a` 中较早建议的长版本名不另开第二条兼容分支。该版本不是新 N1 wire，也不是已存在或已共同冻结的网络协议；精确外部形状仍由BIZ在唯一契约登记。

内部 Go 读取形状可为 `LoadDisableAuthority(ctx, parentOperationID) (snapshot, error)`，名称由 BIZ 决定。不要求caller传owner：在服务端固定environment/tenant范围内，按原parent定位并核其持久owner，再按该owner锁账户和重核原意图/identity/065完整关联；缺失或多义须拒绝，不能跨scope全局扫描或取第一条。owner归属来自真实管理/受限机器链所留下的持久事实，不能因请求自报同名字段而授信。输入不接收grant、state、代际、policy、目标或trust配置覆盖，不创建意图或改映射。

| 快照字段 | 来源和约束 |
| --- | --- |
| source_version | 上述内部候选版本，不改 mindlink-policy-n1 |
| environment / tenant_id / appkey / issuer / audience / system_actor_id | 原parent/065持久scope和trust绑定；当前固定服务配置必须逐项匹配，不能用新配置重建或覆盖旧绑定，客户端不得选择 |
| owner_id | 原持久父意图及归属核验所得正十进制 uint64；当前账号可执行此生命周期操作，不从临时请求自报 |
| parent_operation_id / parent_digest | 沿W2已有字段，保留原9槽父意图的原 ID/摘要；完整原 payload 的校验及 owner/scope/目标查询须通过；不覆盖原摘要、不冒充子意图摘要 |
| wire_operation_id / wire_request_digest / wire_body_sha256 | 分别对应065的operation_id/request_digest/body_sha256，与本次IM操作和实际N1原body逐项绑定；核持久body与其hash及完整parent/owner/target/generation/policy关联，校验不等于改写或重新生成15槽摘要 |
| principal_id / avatar_id | 060原映射与原父目标、当前归属一致；只引用既有业务映射，预分配ID不证明IM投影已存在 |
| target_avatar_generation | 原已提交退役意图所绑定的目标代际，与当前墓碑及映射一致；不是控制面actor的generation=0 |
| expected_policy_version / desired_policy_version | 从原父意图及其固定payload验证，desired=expected+1，正int64且无溢出；不能用当前IM结果或新造版本倒填 |
| desired_state | 固定 disabled，表述业务停用意图；不表述IM当前state、IM ready或写grant |

以上整数/字符串语义沿既有 N1 及持久模型；若额外返回当前业务generation/policy，必须与原target/expected/desired明确区分，不能用当前值替换旧意图版本。本候选未定义外部 JSON 编码、签名、路由、RPC/proto 或额外 claims。不要将快照塞进 N1 的8字段 body；独立 wire 子意图仍由 W2现有源负责。缺065/schema或wire不符必须失败，不能为供给临时生成/修改原wire。

一次性进程是可信部署实例的Reader适配候选，不是可转发bearer grant。后续IM适配器只能调用自身固定配置的可信程序/实例并绑定本次原操作，不接HTTP调用者给的程序路径、DSN、JSON快照或缓存allow。进程边界至少须有调用方有限deadline、退出状态及完整版本/输出/原绑定检查；失败、不可达、超时、截断/无效输出和成功空对象都不能放行。不得把private DSN、密钥、JWS或原body写入诊断输出。此处没有实现该进程调用器，也不以一次exit0替代跨库屏障。

源读取在业务账户锁及其一致事务下核当前 user_info 状态、归属、代际、已提交墓碑、identity和原 pending 停用父意图的完整关联。新效果必须来自真实退役/禁用生命周期，不能只凭业务映射的desired文字或任意管理密钥。错误状态、缺行、陈旧或不匹配的意图、配置绑定不符和SQL错误均返回错误，不补成功空对象或通用boolean grant。原操作已提交后的结果查询独立核当前读权，不重新要求一次新的退役效果或新的pending父意图。

## 顺序、竞争与恢复

1. 业务在原真实管理/受限机器事务内写退役墓碑、增加目标代际、同步identity并固定原父意图；提交后停止该分身的新业务写入/领取。这一步不声称IM撤权已完成。
2. 供给只读取这份已提交事实；既有 owner grant 与 IM 当前 active principal 的来源仍须独立可信、可追溯且已实际投影。本快照不能自举新 principal、owner grant、read_enabled或write_enabled；缺投影继续NOT_READY。ensure保持关闭。
3. 现有IM锁序为原operation → owner barrier → principal → ticket。新效果锁内核write_enabled、scope/avatar、当前active、expected policy及不倒退目标代际；再同事务CAS写disabled/desired policy/目标代际，保存严格原回执/token，仅COMMIT成功后返回ready。不能由供给预写这次effect。
4. 并发撤owner写权先赢得同一barrier时，新效果FORBIDDEN；不得为了补停号临时恢复write_enabled。停用先提交则保留原结果；后续重放只按当前读权核原件，不再effect、不把原expected当新CAS。终端发送和生命周期更新仍须在相应IM最终持久化点共用权威屏障，不能以业务锁或先前验签代替。
5. BIZ与IM不同数据库不共享事务。一次已核快照只是读取时点；观察时间或短TTL都不能阻止读取后撤权。当前缺可信投影输送/认证、撤权排序与IM提交间的可验证屏障，不能凭这个快照承诺即时撤权。后续适配器必须与所有相关生命周期/撤权路径形成可验证顺序，缺此部分继续关闭真实写入口；本次不虚构epoch/lease或跨库原子保证。
6. COMMIT/响应未知不判定未执行。沿原wire operation/request/digest及当前读权查询原件，不换父/子键、重复effect或把业务退役当IM ready。账户失效时是否仍可读原件按当前独立读权，不能从先前写权推导。

## 写集和验收边界

本方没有在途的生产供给接线写集；本批仅本说明和 `docs/mindlink-http-schema_zh.md`。BIZ所声明的 `model/agentaccess/mindlink_authority_source.go` 及同名测试、新 `lib/mindlinkauthority/` 与独立默认关闭源配置不与本批相交；它们的实施/测试归BIZ，新增旧源交叉改动须先报精确路径。W2的mindlink_wire文件、两.api/handler/logic和READ独占路径不由本批改写。

本次实际为当前源/调用及锁序核对、字段与父意图绑定对照、文档链接及Git差异检查。BIZ可先独立验证只读源的缺行/异owner/代际/墓碑/policy/父摘要/配置/SQL失败及恢复；IM生产消费、跨库撤权竞争、真实HTTP/DB/SDK和重启验收仍须在后续实际接线制品上完成。未改Go/API/proto/SQL/配置，未运行Go或操作Docker/DB/密钥；未推送、合并、部署，不完成066/c82或完整F1/F3。
