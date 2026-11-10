# DG003 主人历史读取 · 2026-11-10

基线 `1a34438` 没有正式历史 HTTP 读取；业务 HistoryReader 尚未接线。本轮提供文本纵向链，非全量交付。

## 正式协议

`GET /apigateway/mindlink/history/{conversation_id}/{before}/{limit}` 使用现有 EdDSA actor 票据，必须显式配置 query operation `history.read`。只允许 owner，拒绝分身借 inform 权限遍历原会话。

- conversation_id 为业务 64 位小写十六进制 ID；before 为 0（最新）或正十进制原生序号，limit 为 1–100，均须规范编码。无 query/body。
- request_digest 为完整 Path 的 SHA256。其余信任域、owner、target、当前策略版本及短时票据沿现有校验；签名不创建权限。
- 同一 SQL 事务锁当前主人 read_enabled 和归属/策略映射；原生目标只从映射取得。disabled 会话仍允许主人读取保留历史，不恢复写权。
- 响应沿 `data.operation`，version=mindlink-history-v1；绑定环境、租户、appkey、owner、会话、策略、kind、原生 requester/target、before/limit。messages 是原生消息 ID、sender_id、十进制 sequence、原正文，按序号倒序；next_cursor 是本页最后序号或 null。
- 原生 MySQL history 为事实源，已删消息不返回；每页最多 100 条且原生负载预算 512KiB，序列化超过响应上限失败，不跳过无法提供的消息。不写 seen、ticket journal 或操作结果。

当前只支持已实现发送链的 `jg:text`；媒体/控制分类需要后续完整协议，遇到其他类型返回未就绪，不能称媒体历史完成。Mongo 未接通且失败关闭。

## 证据

协调入口本机 `.local/dg003-transfer/`，Rocky `.evidence/`：

- `im-history-red.log`：缺失正式读取入口的红例；`im-history-unit.log`：mindlink、gateway services/routers 通过。
- `im-history-native-db-fixed.log`：真实签名 HTTP、原生 SQL 正文/游标、注销保留、跨主人及撤权拒绝，GET 不写 seen/操作日志。首次错误工作目录导致 SQL 基线无法读取，`im-history-native-db.log` 保留；scratch 在 cleanup 中删除。
- `im-history-full-db.log`：99 顶层 pass、3 skip、0 fail；跳过沿已有独立来源/网关子进程条件，不计覆盖。
- `history-interop.log`：实际业务签名适配器 → IM HTTP → native SQL，两页各一条，原消息/回复的 ID 与正文一致、无重复，末页无游标；同场景保留发送丢响应后的原结果恢复、execute/inform 分离。P1/C1 仍预置，不是首次接入全链。

未验浏览器、实际客户端收取、媒体、安装或发布。无 pre/pro 部署。

## ML-DG003-HISTORY-VISIBILITY-001 · 提交前反例

首版仅过滤 is_delete，遗漏原生过期、主人单条删除/清空及 portion 可见范围。Rocky `im-history-visibility-red.log` 真实过期消息仍出现，测试失败；补齐原生 MySQL 可见条件后，`im-history-visibility-fixed.log` 验证过期、非本主人 portion、单条删除和清空均不再返回，注销保留/撤权及 GET 无写入仍通过。此缺陷在本 worktree 未提交版本发现，无线上故障判断；之前完整套件并未覆盖这些新反例，不沿用其绿灯代替。

过滤修复后的两页跨仓链重新构建并通过 `history-interop-fixed.log`，无重复、正文保持不变。
