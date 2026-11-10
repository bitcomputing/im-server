# DG003 会话投影 C1

这是独立开发分支的 IM 接收端，尚不代表业务生产者、页面或完整消息链可用。默认关闭；现有配置不自动启用。业务端必须先提供真实 `--conversation` 权限来源，再在服务端信任配置中显式允许 `conversation.ensure`。

POST `/apigateway/mindlink/conversations/ensure` 仅接受已配置的 system 身份。请求严格包含：`operation_id`、`request_id`、`request_digest`、`conversation_id`、`kind`、`native_target_id`、`owner_im_id`、`expected_policy_version`、`desired_policy_version`、`members`。操作、请求、会话 ID 均为 64 位小写十六进制。成员按 avatar_id 严格升序，字段为 avatar_id、principal_id、avatar_generation、principal_policy_version，内部号唯一且不得冒充主人。

语义摘要是 UTF-8 JSON 数组的 SHA-256：

```text
["mindlink-conversation-c1","conversation.ensure",environment,tenant_id,appkey,
 owner_id,actor_kind,actor_id,avatar_generation,conversation_id,kind,
 native_target_id,owner_im_id,expected_policy_version,desired_policy_version,
 [[avatar_id,principal_id,avatar_generation,principal_policy_version],...]]
```

`owner_avatar_dm` 恰有一个分身，目标是其内部号；`avatar_pair_group` 恰有两个分身，实际群含主人及这两个分身；`mindlink_group` 允许空分身名单，主人仍在群内。群号为 mg 加 30 位小写十六进制。首次投影 expected=0；后续以 IM 已提交版本作 CAS，desired 必须更大。C1 提交完整当前成员快照，允许业务尚未投影的中间版本被合并；不能越过尚未确定结果的旧请求。

固定权限程序参数为 `--conversation --config PATH --operation-id ORIGINAL_PARENT_ID`，不得从请求选择程序或路径。成功输出恰含 15 个非空字符串字段：version、environment、tenant_id、appkey、issuer、audience、system_actor_id、owner_id、parent_operation_id、parent_digest、wire_operation_id、wire_request_digest、wire_body_sha256、conversation_id、lifecycle_state。版本为 `mindlink-conversation-authority-v1`，状态为 active。其余字段必须绑定已保存的原请求及当前业务事实。仅签名通过不能授予开群权限。

IM 同事务核对已有主人读写屏障、成员当前代际和策略、原生用户，再写会话投影、群与成员、票据及不可变回执。已有原生群碰撞不得接管；已停用映射不得重开。群 ID 使用原生排序规则的唯一约束，禁止两个主人绑定同一群。即使所有分身退出或映射停用，普通原生接口仍不得访问该群。主人在 pair 群中另有静音标志，但该标志不能替代写入端权限校验。

回执沿用 22 字段操作信封，desired_state 为 `{state,conversation_id,kind,native_target_id,owner_im_id,members}`；effective_avatar_generation 为 0。GET 原结果仍需当前主人读屏障及当前会话策略版本。结果校验重新计算 C1 摘要，不能只比较字段数量。

Rocky 隔离 MySQL 验证入口：

```powershell
go test -p 1 -tags integration ./commons/mindlink -run 'TestMySQLConversation|TestMySQLEmptyAndRetired|TestConversation' -count=1 -v
```

已验证真实 HTTP 建立 pair 群三成员、原结果重放/查询、读写撤权、成员停用/代际过期和末尾 SQL 故障的全事务回滚。来源子进程使用测试夹具，尚未验证真实业务 C1 程序；无浏览器、消息收取、跨库生命周期或生产部署验收。
