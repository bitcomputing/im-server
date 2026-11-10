# DG003 · 注销内部号的正式 HTTP 接线

默认关闭。`mindlink.enabled`、信任绑定和 `mindlink.disable_authority_source.enabled` 均须显式配置；本次没有修改运行环境或部署配置。

```yaml
mindlink:
  enabled: false
  trusts: []
  disable_authority_source:
    enabled: false
    program: ""
    config_path: ""
```

程序与配置必须使用绝对路径，由服务端配置选择，HTTP 请求不能指定。业务程序为 chatbot43 的 `cmd/mindlink_authority`，参数协议为 `--config PATH --operation-id ORIGINAL_PARENT_ID`。不通过 shell 拼接参数，进程输出有上限和超时，stderr 不转发到 HTTP。

启用时先检查 NativeFence 所需表、存储引擎、原生身份排序规则和唯一索引。POST `/apigateway/mindlink/principals/disable` 继续验证严格 Ed25519 请求及 N1 原始绑定，再核业务注销事实，最后在原有事务内锁定 IM 当前账号屏障与主体代际/策略，保存停用投影和不可变原结果。新请求不能重启已停用主体，错误来源不能创建授权。

默认构造器及缺配置时仍拒绝写操作。来源退出 2/3 为 NOT_READY，退出 4 为 FORBIDDEN，系统故障或非法输出为 UNKNOWN_RESULT。成功响应的 `data.operation` 是原始 N1 结果，`data.replayed` 区分原操作重试；未知结果查询原 operation_id，不能换键重做。现有 GET 原结果权限检查保留。

此接线只开放限制性的停用操作，不开放建号、建群、消息发送或任何绕过原生拒绝门的通道；它不是跨库完整生命周期完成证明。业务端原意图结算和其他 F1/F2 能力继续独立验收。

本轮在随机独立 MySQL 副本验证 HTTP 来源拒绝无副作用、原始停用及相同请求回放；检查 `mindlink_principals`、`mindlink_operations` 和 `mindlink_tickets` 实际行数和版本。测试入口：`go test -tags integration ./commons/mindlink -run TestMySQLPublicDisable -count=1 -v`。

`TestMySQLPublicDisableRealBusinessSource` 另行调用真实业务 exe 与真实业务测试数据库，再执行签名 HTTP 处理器、核对 IM 提交及原结果回放。请求体使用业务数据库保存的原始 wire 字节，不能按 IM DTO 字段顺序重新序列化后沿用旧 body hash。该测试需显式提供真实程序和私有测试配置；未提供时 skip，不算通过。

2026-11-09：权限模块完整 integration 运行 80 个顶层用例通过、2 个跳过、0 失败；新增跨仓真实来源用例另行通过。原始本机日志在协调入口 `.local/dg003-im-policy-integration.jsonl`、`.local/dg003-disable-cross-repo.log`。验证为真实处理器/子进程/数据库，不包含浏览器、实际服务器部署或全量消息链路。
