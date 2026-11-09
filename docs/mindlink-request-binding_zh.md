# Mindlink 原请求事务绑定

DG-20261104-003 的独立基础增量，位于 `commons/mindlink`。它要求已验证的 actor 证明，并在同一 MySQL 事务内绑定原 operation、request、jti、权限检查、SQL 效果和原结果。权限 callback 不可缺省；重试核当前读取权限，仅返回已提交的原结果，不重复执行效果。SQL 或提交失败返回未知，不能据此声称没有写入。

这部分没有启用 HTTP 路由，也没有实现具体 POLICY 权威、原生消息投递或可靠回调。测试权限表仅为测试 fixture，不能作为业务权限实现的证据。生产接线还必须从正式 DTO 与权威事实复算语义摘要，并让撤权与消息提交使用相同事务屏障。

## 实际验证

隔离容器 `im-policy-c82-mysql`，MySQL 8.0.46，独有新数据卷及测试库；没有修改原 juggleim、READ 或 Rocky 环境。镜像 ID 为 `sha256:6cd09145362dfe6831b14545de3d5fd6cc75c37cfd6ef8561429c1fc73518b39`。私有凭据、DSN 和原始测试日志留在忽略目录，不提交。

- Windows Go 1.26.1：`go test -race -tags=integration ./commons/mindlink -count=1 -v`，exit 0，24 个顶层测试通过，含 12 个真实 SQL 测试，0 failed / 0 skipped。
- Linux Go 1.25.12 Alpine：同一源树、离线模块、同一隔离 MySQL，`go test -tags=integration ./commons/mindlink -count=1 -v`，exit 0，同为 24 / 12，0 failed / 0 skipped。
- 默认 `go test ./commons/mindlink -count=1` 通过；集成测试无专用 DSN 时明确跳过，不能算 SQL 验证通过。
- 真正执行了新库全量初始化和旧库 `dbcommons.Upgrade`，核对两表结构相同、旧用户哨兵数据保持。没有对已有业务库执行迁移。
- 通过的行为包括旧代际/旧策略/越权/禁用拒绝且三类效果行均为零、原意图或 jti 冲突不改旧数据、8 个并发重试仅一个效果、写失败回滚、连接池更换后恢复原结果、结果缺失不重做、锁等待超时后拒绝。更换连接池不等同服务进程重启验证。
- 撤权测试观察 `performance_schema.data_lock_waits` 中的实际等待；临时移除权限行锁后测试确实失败，恢复源码后通过。自身策略 7→8 后原请求恢复也先观察到失败，再修复重试核权模式。

原始 Windows race 日志 SHA256：`3f84bc88dd3b5134f6c72eb4b86c8be18850ac1c4c9de07a4b4a6f5536e482ee`；Linux SQL 日志：`0cbe82c973084c1834c8fc487b634a4e2aeb57d39b117a49aef82ffb24e2627e`。

## 独立审查及修正

独立 reviewer 对固定 `efcc96c..d3fa119` 只读审查，实际运行 Windows unit/race；没有运行 SQL 或读取私有配置。无 Critical，Important 为 ticket 唯一键锁等待后的 TTL 漏检，Minor 为升级测试在核结构前重放 DDL，可能掩盖 Upgrade 漏执行。两项均已修正，没有留作已知限制。

真实 MySQL 先复现 ticket 锁等待过期后仍执行 effect=1 / err=nil；另一个效果执行期间过期的红例也确实错误提交。现在在 ticket 绑定后及 commit 前复检，两个场景均拒绝，事务三类行均为零。升级测试先核实际 Upgrade 产物，再验证 DDL 重放。

修正后的 Windows race＋MySQL：26 个顶层测试通过，含 14 个真实 SQL，0 failed / 0 skipped，exit0。离线 Linux Go1.25.12 同源同库也为 26 / 14；另加 errs/tools 测试，三个包共 29 个顶层通过，0 failed / 0 skipped，exit0。Linux mindlink 包运行 16.121 秒，结束于 04:06:57 UTC；Windows 已于 04:03:49 UTC 结束，实际数据库用例没有重叠。

修正后日志 SHA256：Windows `35db68a5c9d85a7a9ea72163ba9a621cc5922286fa68b861e2ac315ec8f03eb0`；Linux `5949905a9b9725ed625ec4b8faa90a436138a23ebac0fcb4ad4fb185c299e4a6`。04:09:38.488315 UTC 独立 SQL 回查仍为 operation=1、ticket=1、测试效果=1、generation=4 / policy=7 / disabled=false，限定为当时测试快照。

独立 SQL 回查于 2026-11-09 03:41:43 UTC 得到 operation=1、ticket=1、测试效果=1、测试权威 generation=4 / policy=7 / disabled=false。这是测试结束时的快照，不是实际业务消息验收或持续保证。

## 集成边界

新迁移为 `20261108.sql`，初始 SQL 同步追加定义。本隔离分支没有合入 READ 的 006/007；发布前必须核对完整有序迁移集合与初始化定义，不能直接把本分支迁移到已发布环境。

现有 Windows 原生大包编译受到 `commons/metrics` 的 Swap 字段平台错误影响；本次没有修改该基线问题，也没有据此声称全仓测试通过。尚未验证 HTTP、具体 POLICY 数据、真实消息历史/客户端收取、进程重启或任何部署。
