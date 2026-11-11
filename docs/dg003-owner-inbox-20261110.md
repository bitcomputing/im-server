# DG003 主人持久收取阶段

## 行为

- 已提交的 message outbox 按原回执、原生历史、环境/租户/主人及当前读屏障校验后，事务内写入主人收件箱（主人自己发送时写发件箱）、会话摘要、未读索引和 `inboxed` 状态。分身 execute/inform 继续由业务投递链处理。
- 同一记录并发或重试不重复入箱、不重复计数；同会话的较早 pending 消息未处理前，较晚消息保留待重试。三人群不增加未读，主人自己发送不增加未读。
- 网关仅在 Mindlink 配置启用且信任构造成功时启动有界消费者；关闭时取消。每轮越过暂时失败的记录并在下一遍重试，失败不伪装成投递成功。
- 原生同步不再仅依赖当前进程的消息时间缓存。内部消息只允许当前主人读取与实际持久收/发件箱内容相同的条目，当前读屏障撤销后过滤，游标仍越过已过滤记录。登录、发送和成员管理拒绝规则没有放宽。
- 收/发件箱查询或消息解码失败返回失败，不返回成功空页，也不推进返回游标。

## 已运行的证据

测试在 Rocky Docker 的独立 MySQL 8 实例，IM 临时库使用测试生成的名称并在结束时删除，结束后查询上述临时库前缀没有残留；业务种子夹具保留供后续隔离联调。没有修改共享测试库或生产库。

- `im-inbox-regression-db.log`：16 个顶层 PASS，0 FAIL/SKIP，包括新事务、四类收/发件箱情形、8 路并发、前序阻挡、正文/收件人/主人/回执篡改、撤读权和退休后历史投递，以及既有可靠发送和原生身份过滤回归。
- `im-inbox-interop-final.log`：真实业务 Store/签名 HTTP → IM 历史与原回执 → 业务结算/回复 → 新消费者 → `SyncMessages` → 原生 SQL 的集成检查通过。两条消息 ID 和顺序一致，游标重连不重复，撤读权过滤，实际重命名收件箱表注入查询失败后不返回成功空页。原 P1/C1 映射预置，不能代表首次开通。Linux 测试二进制 SHA256：`cd0632b0ae1da5414d22de8f6ca3c606cbb237d91d8db579be0ecd0f85d99f5f`。
- `im-inbox-vet.log`：相关 commons、message service 和 gateway 的 Linux 目标 vet 通过；Linux message/gateway 构建通过。
- `im-inbox-unit-final.log`：本机 commons/mindlink、commons/bases、apigateway/services 三包测试通过。
- 失败证据保留：新增方法缺失红例、主人发送误入收件箱、倒序投递、MEDIUMBLOB JSON 查询缺 UTF-8 转换、同步测试缺 RPC 目标字段、收件箱查询失败被误当空页。Windows 编译 message 包因既有 Unix `syscall.Stat_t` 不支持而失败，改用 Linux 验证，不记 Windows 通过。

上述日志保存在本机忽略的 `.local/dg003-transfer/` 和 Rocky `D:/projects/.evidence/`；跨项目测试 harness 位于本机忽略的 `.local/dg003-solo/interop/`，未随仓库发布。

## 仍未覆盖

后续实时通知实现与实际 WebSocket 证据见 [主人实时同步通知](dg003-owner-notification-20261110.md)。下段为本收件箱增量交付时的边界，不替代后续记录。

这不是客户端完整交付：未验浏览器 SDK/WebSocket 实时收取、推送、网关进程重启、普通聊天与新消费者并发分配同步时间、前端真实未读清除。消费者目前只负责持久收/发件箱，尚未产生实时通知；`inboxed` 不表示客户端收到、分身执行或手机推送成功。正式开通和消息正文完整 UI 链、媒体/流式、各宿主安装与 App 仍按总计划续办。没有推送、合并或 pre/pro 部署。
