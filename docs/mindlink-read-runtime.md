# DG003 主人阅读快照与推进 · 2026-11-10

基线 `f71ce29` 只有原生 WithTx seen 实现，公共 READ 未接线。本轮注册正式 GET/PUT `/apigateway/mindlink/conversations/:conversation_id/seen`，对应 `conversation_seen.query` / `conversation_seen.advance`，默认关闭且须显式列入独立 query/mutation allowlist。仅 owner；不开放分身 inform 对原私聊的读取权。

## 数据与权限

采用既有 `mindlink-read-r1` 21 槽摘要，四份固定 M 向量不变。GET 无 body、request_id=64 hex operation_id；PUT 严格四字段 operation_id/request_id/request_digest/seen_seq，位置为规范非负 int64 字符串。原生 requester/target/channel/成员只能从当前归属与策略投影重建，不能从端输入。

复用公共操作/票据事务。新 GET 只保存原查询回执、快照和 UTC 毫秒观察时间，不创建 seen 行；同原键刷新票据重试仍取原快照。PUT 与原生 seen 更新、原回执同事务；重试不再推进，保留原回执并重新核权查询当前 state。GET data 为 operation/state/state_observed_at/replayed，PUT data 为 operation/state。READ 的 effective_avatar_generation 为 null；query 的 ready 只表示观察记录提交。

新推进核 read_enabled、write_enabled；原结果复放核当前读权，写权撤销不抹去已提交结果，读权撤销拒绝复放。原生调用以 `gorm.Config.Conn=tx` 复用同一 sql.Tx，不另开事务；SQL 失败终止全部结算。非群成员映射 FORBIDDEN，参数错误 INVALID_REQUEST，原生存储失败 UNKNOWN_RESULT。

这里只实现 IM 端；BIZ 独立读取意图持久化、主人 seen API、页面和候选整体仍未完成。不得由前端直接调用此接口或临时签发真实账号票据。普通 `operations.query` 未扩展 READ 包装；READ 以原 GET/PUT 的原意图恢复。

## 检查与未覆盖

日志在协调入口本机 `.local/dg003-transfer/`，Rocky `D:/projects/.evidence/`。

- `im-read-red.log` 缺入口编译红例；`im-read-final-regression.log` mindlink/gateway services/routers 通过。固定 M 摘要四向量通过。
- `im-read-full-db.log`：101 顶层 pass、3 skip、0 fail；随后补入永久 `TestMySQLReadSnapshotAndFailedSettlement`，`im-read-core-db.log` 真实 SQL 验证 query 不写 seen、同键原快照、推进结算失败回滚及撤读权。此永久用例的 native callback 为隔离夹具，不冒充真实原生服务。
- `read-final.log` 为实际 gateway factory → shared native WithTx → SQL：query、advance、原 query 重试、新 query、SQL 结算触发失败回滚、撤写后读原件/撤读后拒绝。另有原消息/回复、inform、两页历史真实 HTTP 链；P1/C1 仍预置。
- `read-replay-red.log` 复现 PUT 重试错误缓存旧 state；修复后原回执不变且 native 重新读取。`read-membership-red.log` 复现非成员错误映射为409；修复后 `read-membership-fixed.log` 真实 HTTP 返回403，前述全链断言继续通过。此测试最后一条新增历史水位是明确 SQL 夹具，不计新消息配送。
- 最终跨仓测试 exe SHA256 `b479bc0a55febb42fee68ff00ef25698a142d2d8960b960d5ec26de6805449e0`。完整套件后仅补严格 state 必填检查/非成员错误映射，最终相关包及跨仓链重跑，不将旧完整套件冒称最终全量重复运行。

未覆盖浏览器/App、已安装宿主、首次接入、长时并发/独立审查。无 schema 变化、pre/pro 部署或发布。所有 scratch 为隔离用例创建并清理；测试通过只描述对应版本的本次结果。
