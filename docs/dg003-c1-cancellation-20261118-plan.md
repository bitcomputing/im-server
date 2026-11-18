# DG003 旧 C1 确认取消与迟到写屏障

> 使用 executing-plans 在现有隔离 worktree 内实施；本人实施，末尾独立只读审查。

目标：已失效非空名单的 unknown C1 不再永久阻塞。已提交的原操作只取回原 ready；尚未提交的原操作须在 Native 原 journal 中提交 cancelled，再结算业务历史父记录，后继从真实 applied 开始。

## 设计与范围

沿用原 C1 body、语义摘要、operation/request ID 及 journal 的唯一键和事务锁。新增 system-only `conversation.cancel` POST `/apigateway/mindlink/conversations/cancel`，严格解析原 C1 后仅在 journal 绑定中规范化回原 ensure。取消须由固定 BIZ source `--conversation-cancel` 明确证明同一冻结 unknown 已被同目标更高版本取代；来源失败不构成取消权限。Native 不依赖 GET 空结果，不删除历史，不创建权限或改成员。取消与原提交共用一个原 journal 行：先提交的一方决定不可变结果。

取消回执仍为原 ensure 的22字段，原 desired_state 保持原请求，state=cancelled、effective 两字段=null、failure=null，commit_version/时间为持久取消凭证。普通 ready 不变。只有严格绑定的 system 原查询可读 cancelled，不要求不存在的会话投影；仍检查现有 owner read 权限。业务唯一 receipt 和 parent=cancelled 同事务，不推进 applied、不激活当前会话。下游准备排除已取得真实取消回执的 unknown。

Ruling: 复用现有 journal 而非新增取消表或删改原 wire；避免双日志锁顺序与 schema 迁移。代价是新结果状态和配套 writer/source/trust 必须共同安装，旧 writer 不得混跑。新权限只用于拒绝旧请求，不授予发送/建群权限。

Ruling: 主人已要求独立补齐并安装新候选，按已明确目标直接实现，不重新发起设计选项确认。沿用既有专用测试库/端口/托管及保护条件，pre和生产不动。

## 影响位置

- Native：C1解析与事务、原结果验证/限定查询、HTTP/gateway路由及信任白名单、正式配置样例、测试。
- BIZ：固定来源/命令、worker恢复分流、原回执验证和结算、Bug台账、正式内部契约、测试。
- Rocky：固定 IM/BIZ/source 配套及新 mutation trust；Web/CLI无需新操作入口，验证原用户入口及数据库结果。
- 私有 interop：真实双库事务、真实进程 source 与 HTTP、竞态/丢回复/重启/拒绝及旧回归；不提交私有夹具与日志。

## 实施清单

- [x] Task 1：用真实双库构造旧非空 C1 unknown→B退休→新名单，运行先红；保留失败、schema/F6基线。Expected: 现有 worker 无法收敛旧 unknown，非夹具错误。
- [x] Task 2：新增明确负向来源、Native同 journal取消及22字段结果、BIZ严格结算/worker分流。Expected: old cancelled且applied不变，后继ready、B不回群。
- [x] Task 3：覆盖已提交原结果、迟到提交双方顺序、取消丢回复/同键并发、事务回滚/超时、来源与绑定/权限拒绝、初始无投影、停用DM/pair，以及原N1/C1/初始空群/P1回归。Expected: 原结果唯一且无伪ready，失败分支无成员/权限副作用。
- [x] Task 4：登记离线/构建检查、独立整体审查及必要修复；固定提交制品，逐项记录命令/失败/skip。
- [ ] Task 5：核 Rocky 实际运行、保护库/F6及托管，备份后安装同组候选；真实入口与DB验证并回查本轮临时资源。Expected: 配套hash一致、原历史不变，取消和后继可用；未发布pre/生产。

## 审查重点

不将读取失败当作原操作未提交；规范化不能绕过 incoming cancel trust；取消不得覆盖 ready 或与旧提交竞态漏锁；旧非空名单不重新授权；取消回执不得推进 applied 或把业务当前会话标 ready；查询不扩大 owner/跨账号权限；取消后旧 writer 回退和 journal 丢失不安全。

## 本轮证据

本轮起点基线 BIZ08919d16a / IM64fc912；现有 worktree 均 clean。唯一总体进度继续维护根任务表，本文件只维护此变更的实施与证据。

2026-11-18：实际红例与首个绿例分别为私有测试证据目录 `c1-cancel-red-79737b169098`、`c1-cancel-green-05437bc8ca8f` JSON/log。红例按预期失败，绿例通过；两次0skip、schema集合与原F6相同。BIZ受影响4包及登记6包通过；Native core/gateway routers/services三包通过，扩展真实事务与Native integration运行中。安装尚未开始。


### 最终代码验证（安装前）

- 首个完整矩阵 `c1-cancel-all-final-1118.json/log`（包986870cda701…）：23取消＋13原N1＋11原C1＋20首次空群＋27P1，共94子例/99pass事件，0fail/0skip，schema集合和原F6相同。原C1的retire-after-query改为断言真实取消且旧名单无效果，这是§29的预期行为变化，不是把拒绝删除。
- 独立只读审查3项Important均以真实反例复现并修复：来源关闭查询不得落入owner普通读取、首次空群不得获负向授权、后继完整静态绑定/类型/唯一性必须有效。`c1-cancel-review-red-350149f7c033`5个子例全部预期失败。该次库列表快照与另一回归交叠，不能计清理通过；完整矩阵结束后的统一schema比较通过。
- 自查共享回执消费者：ownerDM原ready校验复用通用validator，必须拒绝cancelled回执；同样cancelled父记录不得变为ready。`c1-cancel-terminal-red-1118.log`的两项先红，终态8组合全绿；受影响4包及登记6包绿。仅此4行终态保护位于94例之后；其最终C1/首次空群54子例/57pass事件同包6e9e55aef404…再验，0fail/0skip，schema和F6相同。Native/P1/N1实现及已验固定来源字节保持。
- `c1-cancel-timeout-timeout-a24c6a7773b9.json/log`补真实owner行锁等待后取消HTTP上下文：Native事务退出、无journal/业务回执，再沿原键恢复成功；0skip、schema/F6保持。最终取消共24子例；相关71子例分别按上述固定版本取证，不伪称为同一个二进制的一次95例运行。
- Native最终包129a69e566c7…：673pass事件、0fail、3环境skip（两个独立BIZ配置及gateway进程夹具未供），不计为实测通过；自有临时MySQL容器已删除、scratch为0、原F6相同。真实来源跨库验证由上述interop另取证。
- 本地Native core/gateway三包、登记CI13包（7个无测试、convercache外部库跳过不计DB覆盖）、go vet及Linux launcher构建通过；BIZ受影响4包、登记6包、31契约例和Windows主服务/准入工具构建通过。具体命令与完整日志位于本机私有 `.local/dg003-transfer/c1-cancel-*-1118.log`，远端原件在Rocky证据目录。

Ruling: 首次空群继续沿§27的正向原件恢复；负向路径显式排除，不用临时正向失败改变原权限。取消源无需P1 ready，但完整核静态名册与原固定DM/pair绑定。来源关闭时连system原取消查询也保持NOT_READY，不降级为owner读取。

独审未独立执行运行测试或判定Rocky制品/安装；这些由上述实际命令及下一节的安装证据证明，不以静态审查替代。实现审查已完成，不另重复委派审查。
