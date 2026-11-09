# Mindlink 可信调用与最终事务候选

后续 `1d4a6f92` 批次已对固定 BIZ N1 作 IM 技术确认，并替代旧 Go 值类型、实现 disable body/摘要解析，见 [当前字段源](mindlink-http-schema_zh.md)。下文保留 `7f424c2f` 当次候选及差异记录；其中旧23字段/微秒和“尚未接受”不再描述当前字段状态。RPC/生产最终屏障仍是未实现候选。

DG-20261104-003；回应 BIZ 消息 `pbz2k6sm5y8hukeh`，本批 `7f424c2fd98833786aa20f71bf0dca213bbdd8066d97292a65364ccd44685b47`。本文件补充可核定设计，不是已注册的 RPC、生成 proto、生产权限实现或放行依据。

## 当次固定源（7f424c2f）

工作分支 `feat/mindlink-policy`，基线 `99f7ce5383ea5a229e9ed970e7ae45a7512e5f91`。`commons/mindlink/operation.go` 是先前未联合冻结的值类型提案：OperationResult 共 23 字段；avatar_generation、expected_policy_version、desired_policy_version、committed_policy_version 为 int64，desired_state 为 JSON 对象，其余 18 项为字符串。字段不使用 omitempty。READ query 的 desired_state 为 `{}`、state 为 observed，时间为固定 UTC 微秒。它没有实现各 operation 的语义校验。

`commons/mindlink/auth.go` 已实现独立 Ed25519 请求验证；返回对象的私有验证器身份不能由 HTTP DTO 自造。`transaction.go` 已实现原 operation/request/jti、强制核权 callback、效果和原结果同事务；当前生产核权 callback 尚未接入。`commons/errs/errorcode.go` 的 19001–19008 已注册实际 HTTP 状态映射。

## 当次与业务 N1 草案的对应（字段已由后续替代）

本轮只读核到 BIZ HEAD `d0639755e086e2d63d5a4754281ed4dafe492591`，唯一契约为未提交 N1 草案，raw SHA256 `2daf06112b28c4324a1be2fb599af80b06b43cc03c279804d61dfff8a5a2a933`。这与本批正文引用的 M 不同，不把工作区草案写成固定提交或已经联合冻结。

建议下一版直接采用 N1 机器块的 22 字段及 nullable 类型、`principals.disable`、8 字段 body、15 项摘要、READ query 空对象和固定 UTC 毫秒，替代 99f7ce5 的对应提案。issuer/audience/method/path/body hash 继续完整留在原操作内部绑定，结果查询仍须核原 issuer/audience，不能因公共结果不再携带这些字段而省略。ensure 及其名称长度不应阻塞首条 disable，未定操作继续关闭。

两点须同时写入唯一源的语义澄清：

1. committed_at 若保留该字段名，取事务内的提交记录时间，只在 commit 成功后向外返回；它不是存储引擎完成持久化的精确物理时刻。accepted_at 和 state_observed_at 同样明确定义各自记录点。格式建议沿 N1 固定 UTC 毫秒，不再保留另一套微秒候选。
2. 现有 Transact 保存一个不可变终结结果，不实现 accepted→ready 阶段机。首条 disable 只在全部约束同事务落地后产生 ready；参数、权限、能力或提交未知使用非成功错误，不拼造 operation 回执。未来 accepted/failed/unknown 的持久阶段转换需要另行实现，不能把它们交给当前 Transact 后期待重试改写原结果。READ query 可沿 N1 用 ready 表示其快照回执已保存，明确不证明 seen 写入；旧 observed 候选随之替代。

上述为本方具体合并建议，尚未修改 BIZ 唯一源或当前 Go DTO，不宣称 N1 整份已接受。可先核定这些字段语义再落对应代码，避免把两套候选同时投入适配器。

## 跨 Actor 的精确封装候选

首条同进程 SQL 操作直接使用已验证 Authorization 和同一个事务，不为它增加一次 RPC。确需跨 Actor 时，拟新增 `commons/pbdefines/mindlink.proto`、Go 包 pbobjs；下列仅是该新文件的候选 message/tag，不占用或修改任何现有 UpMsg 字段：

```proto
syntax = "proto3";
option go_package = "commons/pbdefines/pbobjs";

message MindlinkCommitReq {
  string actor_jws = 1;
  string method = 2;
  string path = 3;
  bytes body = 4;
}

message MindlinkCommitResp {
  bytes data_json = 1;
}
```

内部 procedure 候选为 `mindlink_commit`，只注册服务端调用，不加入 connectmanager 的终端 query/publish topic 表。终端伪造 topic 或 protobuf 不能得到受信证明。procedure 名、IsFromApi、RequesterId、普通管理员身份和 RPC ExtParams 都不作为 actor 授权依据。

actor_jws 为原紧凑 JWS，不带 Bearer 前缀，最多 16 KiB；method/path 为原精确大写方法和路径，不含 query；body 为原字节，最多 1 MiB，GET 必须为空。gateway 仍须验证原请求的 Content-Type；接收方为 mutation 固定构造 `Content-Type: application/json`，不传其他 HTTP 头。接收方按固定 method/path 表选定预期 operation_type，从原封装重新构造请求并运行 Verifier.Verify，再严格解析该操作 DTO、复算语义摘要和权威目标。不能把 RPC 自报 actor/scope 字段或入口已经验过的 boolean 当作证明。缺公钥信任、未知 operation/schema、票据过期或任一绑定不符即拒绝，不转普通 IM 管理 API。

普通 RpcMessageWraper 的 AppKey/RequesterId/TargetId 仅供路由，接收方须与验签及权威事实核对后才能使用。`ToRpcCtx` 第二参数实际写入 RequesterId，不能传业务 request_id 来替代当前 actor 的权威 IM requester。受验证 Authorization 不作 protobuf 序列化，必须在最终执行节点重新产生。

成功 RPC ACK 的 data_json 恰为该路由完整 HTTP data 对象，不包含 code/msg 外层；例如 disable 为 `{operation,replayed}`，READ 为各自原包装，不另在 protobuf 重复 replayed。空值或不符合该路由 schema 的对象不得返回成功。非零错误沿已登记错误码，超时/丢 ACK/响应无效为 UNKNOWN_RESULT，沿原 operation 恢复。禁止把异步发送成功当作 commit。原 JWS、完整 body 和消息正文不写 RPC/错误日志；上线前须核所有封装日志入口。当前两个 message、procedure、重验 handler 及日志检查均未实现或验证。

## 最终事务与撤权屏障候选

建议使用一条独立的 owner scope 屏障行，键为 `(environment, tenant_id, appkey, owner_id)`，候选表名 `mindlink_owner_barriers`。这是新增设计，当前迁移没有这张表。所有该 owner 内部号/专用会话的发送、策略、停用、Token/登录检查和相关管理写入共用它；单个 owner 的相关操作串行，不建立全局计数器。其吞吐影响尚未压测。

新 mutation 的顺序与现有 journal 一致：锁/核原 operation → 锁 owner 屏障 → 核并锁不可变主体映射、当前代际/状态、固定会话成员及 policy → 绑定 jti并复检时效 → 同一事务写实际效果、原终结回执和必要 outbox → 提交前复检时效 → commit 后才发 ACK。跨 Actor 时由最终执行方独占整个事务；不能把事务拆在 gateway 和历史 Actor 两端。GORM 适配必须复用传入的底层 sql.Tx，不能另开池、提前提交或把外部网络调用放入 effect。

同 operation 重试先核不可变绑定，再在屏障内核当前读结果权限，只返回原终结回执，不重做停用，不用自身停用后的新 policy 使旧回执永久丢失。独立 operations.query 沿 N1 先以原 issuer+operation_id 取原件，再核原 audience、完整 scope、固定目标和本次当前读取权限；query 两个 policy 字段等于当前读版本，不能把它们改写进原 mutation。查询 jti另存绑定，不创建第二个 mutation，不调用 mutation effect。该查询入口当前未实现。

停用必须在持有同一屏障时写永久墓碑和适用的代际/policy；消息持有屏障至历史/结果提交。撤权先提交则旧请求失败且无新增；消息先提交则保留原合法结果，停用等待后生效。WS、签名 HTTP、流式、导入及群/用户管理绕过路径必须实际接入 gate；覆盖不齐就 NOT_READY，不能仅凭本候选或一次 disabled UPDATE 返回 ready。可靠消息需把实际历史与回执/outbox同事务保存，原异步 SaveHistoryMsg 不能直接充当该 effect。

当前已完成的 SQL 基础与测试权限表不等于上述生产屏障。需要后续真实 HTTP、原生入口、拒绝后历史/DB 无新增、响应丢失/进程重启及普通聊天回归证据；本文件没有新增运行能力或部署授权。
