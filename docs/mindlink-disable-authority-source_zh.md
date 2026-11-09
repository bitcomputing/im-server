# 停用授权供给边界

DG-20261104-003 / F2→F1，有效消息 `pb2kyn5wyvnhukeh`，当前批次 `306b966391af7ddb8c45f056296208ebb5d591ee0fddf8b5eddf2de1eef5e7e0`。只交付接口需求及协作边界，不实现新接口。当前 IM 基线 `e9cab3a53c311b74a67bfbab24e0a5b90d496551`，停用事务实现来自 `028bb2aa5359c96cc67337814b094239ba0b99a4`；唯一业务契约仍由 BIZ 维护。

上述为首次接口交接的历史状态。当前 `8ce3abd7` / `pb2lyhh2gxahukeh` 已交付 BIZ 固定 `75739e949b7c1b5f08bf9f5f7013ed8c01eb4cba`，IM新增下述默认关闭的进程接收/绑定预检组件。实际验证、提供方材料与未覆盖范围见[接收增量证据](evidence/20261109-mindlink-authority-adapter/verification.md)；不据此关闭066/c82。

## 当前可以消费什么

当前新增 `commons/mindlink/authority_source.go` 的 `NewDisableAuthoritySource(DisableAuthoritySourceConfig)` 与 `(*DisableAuthoritySource).Check(ctx, verifier, authorization) error`。服务端构造固定绝对 Program/ConfigPath，Enabled默认false；Check复用同一Verifier的严格N1解析/15槽摘要、限定system原意图及64hex父request ID，调用固定进程并核21字段原件绑定。它不接受上传快照、不缓存allow、不返回grant，尚未接入生产HTTP或SQL效果。当前仍无业务投影writer、mutation grant供给或跨库撤权屏障。

`commons/mindlink/disable_transaction.go:transactPrincipalDisable(ctx, db, auth)` 仍是未公开、未修改的事务原语；它只接受同一 Verifier 验证过的 Authorization，严格解析既有 N1 body，并读取 IM 已存在的两类行：

| 来源 | 定位及消费字段 | 新效果要求 |
| --- | --- | --- |
| `mindlink_owner_barriers` | environment / tenant_id / appkey / owner_id；read_enabled / write_enabled | 同事务锁行，新效果必须 write_enabled；原件重放必须 read_enabled，二者不能互换 |
| `mindlink_principals` | appkey / principal_id 定位；environment / tenant_id / owner_id / avatar_id / avatar_generation / policy_version / state | immutable scope/avatar 相符；state=active，policy=body.expected，body.target generation不得低于当前；不存在为NOT_READY，不自动创建或回填 |

两表见 `20261109.sql`；独立 write_enabled 由 `20261110.sql` 默认 FALSE 增加。这些是当前 IM 存储列，不是准许 BIZ 写 IM 共享库的供给 DTO。原语没有生产调用，公开 POST 仍 NOT_READY。不得先把 IM principal 设 disabled 或把其 policy/generation 预推到 desired；那会绕开原效果或使新操作被拒绝。

## 业务只读来源的固定协议映射

`pb2k9hx8gwahukeh` 提出的受控一次性进程现已在BIZ固定75739落源：`cmd/mindlink_authority_source --config 私有配置 --operation-id 原父ID`；模型为`NewMindlinkAuthoritySource`和`ReadDisableAuthority(ctx,parentID)`，读取060及065原件，不重做W1。实际版本为`mindlink-disable-authority-v1`；076的较早长名字不另开兼容分支。精确21字段定义在该固定模型及唯一业务契约§11.9；下表只映射消费绑定，不另造N1或公共HTTP协议。

实际输入仅原parent ID，不要求caller传owner。BIZ在服务端固定environment/tenant范围按parent定位owner，锁enabled account，再核该owner的原意图/identity/065完整关联；IM接收还逐项核source.owner_id与已验证N1原owner。输入不接收grant、state、代际、policy、目标或trust覆盖，不创建意图或改映射。原请求主体仍须来自实际管理/受限机器所留持久事实，不能因上传同名JSON而授信。

| 快照字段 | 来源和约束 |
| --- | --- |
| version / operation_type | 固定mindlink-disable-authority-v1 / principals.disable，不改mindlink-policy-n1 |
| environment / tenant_id / appkey / issuer / audience / system_actor_id | 原parent/065持久scope和trust绑定；当前固定服务配置必须逐项匹配，不能用新配置重建或覆盖旧绑定，客户端不得选择 |
| owner_id | 原持久父意图及归属核验所得正十进制 uint64；当前账号可执行此生命周期操作，不从临时请求自报 |
| parent_operation_id / parent_digest | 沿W2已有字段，保留原9槽父意图的原 ID/摘要；完整原 payload 的校验及 owner/scope/目标查询须通过；不覆盖原摘要、不冒充子意图摘要 |
| wire_operation_id / wire_request_digest / wire_body_sha256 | 分别对应065的operation_id/request_digest/body_sha256，与本次IM操作和实际N1原body逐项绑定；核持久body与其hash及完整parent/owner/target/generation/policy关联，校验不等于改写或重新生成15槽摘要 |
| principal_id / avatar_id | 060原映射与原父目标、当前归属一致；只引用既有业务映射，预分配ID不证明IM投影已存在 |
| target_avatar_generation | 原已提交退役意图所绑定的目标代际，与当前墓碑及映射一致；不是控制面actor的generation=0 |
| expected_policy_version / desired_policy_version | 从原父意图及其固定payload验证，desired=expected+1，正int64且无溢出；不能用当前IM结果或新造版本倒填 |
| lifecycle_state / retired_at_unix_seconds | 固定retired及正Unix秒，表述业务已退役事实；不是IM当前state、grant或回执时间，更不证明读取后仍有效 |

以上整数/字符串语义沿既有 N1 及持久模型；若额外返回当前业务generation/policy，必须与原target/expected/desired明确区分，不能用当前值替换旧意图版本。本候选未定义外部 JSON 编码、签名、路由、RPC/proto 或额外 claims。不要将快照塞进 N1 的8字段 body；独立 wire 子意图仍由 W2现有源负责。缺065/schema或wire不符必须失败，不能为供给临时生成/修改原wire。

当前IM接收器只运行自身构造时固定的绝对程序/配置路径，不使用shell/PATH或请求给的程序、DSN、快照。stdout上限32KiB；上下文上限10秒并保留调用方更短deadline，WaitDelay1秒；Windows子进程隐藏窗口。仅exit0且完整唯一/严格21字段JSON及全部原绑定符合才返回nil；exit2/3为NOT_READY、4为FORBIDDEN，其余执行/输出异常为UNKNOWN_RESULT，不回传原进程诊断。调用前后检查票据有效期，过期/取消不能变成功。该nil只证明本次已验证的退役观察；调用器不修改SQL、创建或恢复grant、把private数据写日志或启用PolicyHTTP，也不以exit0替代跨库屏障。

源读取在业务账户锁及其一致事务下核当前 user_info 状态、归属、代际、已提交墓碑、identity和原pending/unknown停用父意图的完整关联。BIZ明确在输出前释放账户锁；这不是提交授权租约。新效果必须来自真实退役/禁用生命周期，不能只凭业务映射的desired文字或任意管理密钥。原结果查询独立核当前读权，不要求新的退役效果，也不调用本预检组件来重做原effect。

## 顺序、竞争与恢复

1. 业务在原真实管理/受限机器事务内写退役墓碑、增加目标代际、同步identity并固定原父意图；提交后停止该分身的新业务写入/领取。这一步不声称IM撤权已完成。
2. 供给只读取这份已提交事实；既有 owner grant 与 IM 当前 active principal 的来源仍须独立可信、可追溯且已实际投影。本快照不能自举新 principal、owner grant、read_enabled或write_enabled；缺投影继续NOT_READY。ensure保持关闭。
3. 现有IM锁序为原operation → owner barrier → principal → ticket。新效果锁内核write_enabled、scope/avatar、当前active、expected policy及不倒退目标代际；再同事务CAS写disabled/desired policy/目标代际，保存严格原回执/token，仅COMMIT成功后返回ready。不能由供给预写这次effect。
4. 并发撤owner写权先赢得同一barrier时，新效果FORBIDDEN；不得为了补停号临时恢复write_enabled。停用先提交则保留原结果；后续重放只按当前读权核原件，不再effect、不把原expected当新CAS。终端发送和生命周期更新仍须在相应IM最终持久化点共用权威屏障，不能以业务锁或先前验签代替。
5. BIZ与IM不同数据库不共享事务。一次已核快照只是读取时点；观察时间或短TTL都不能阻止读取后撤权。当前缺可信投影输送/认证、撤权排序与IM提交间的可验证屏障，不能凭这个快照承诺即时撤权。后续适配器必须与所有相关生命周期/撤权路径形成可验证顺序，缺此部分继续关闭真实写入口；本次不虚构epoch/lease或跨库原子保证。
6. COMMIT/响应未知不判定未执行。沿原wire operation/request/digest及当前读权查询原件，不换父/子键、重复effect或把业务退役当IM ready。账户失效时是否仍可读原件按当前独立读权，不能从先前写权推导。

## 写集和验收边界

IM本批写集为新的authority_source/process三个生产Go文件、同名测试及说明/计划/证据；只交付预检接收组件，没有生产writer写集。原SQL事务、HTTP/配置/迁移、W1/W2/READ及BIZ文件不改。BIZ只读源的SQL/实际退役流程结果按其固定材料单独登记，不作IM本轮重跑。

本次实跑Windows race协议/进程测试、固定BIZ制品的默认关闭调用、定向vet和Linux交叉编译；未把协议夹具的成功输出当成真实业务SQL成功快照。只读Docker/version仍HTTP500，未启动或改共享DB/容器；无真实grant/effect/SQL/HTTP/SDK/UI/跨库竞争/重启验收，Linux未运行。仅在自有临时默认关闭配置及测试进程上验证；不读取真实DSN/密钥、不推送/合并/部署，不完成066/c82或完整F1/F3。
