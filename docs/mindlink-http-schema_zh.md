# Mindlink 公共 HTTP 字段收口

DG-20261104-003，IM 公共接口负责人提出的具体源定义，供 BIZ 在唯一契约登记确认。Go 字段位于 `commons/mindlink/operation.go`；当前没有注册这些路由，不能调用或据此启用 ready。唯一业务契约仍为 chatbot43 的 `docs/mindlink-conversations-contract.md` M 版；本文件不替换该事实源。字段确认后才实现严格 DTO 语义核验与生产权限事务。

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
