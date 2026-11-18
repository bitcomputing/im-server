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
- [x] Task 5：核 Rocky 实际运行、保护库/F6及托管，备份后安装同组候选；真实入口与DB验证并回查本轮临时资源。Expected: 配套hash一致、原历史不变，取消和后继可用；未发布pre/生产。实际结果与清理见下节。

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

### Rocky配套安装与实际入口验收

2026-11-18，实施源码固定BIZ `1700d6f06b0748930a7c0b3aca7cb4c4dabd4725`、IM `b2bad2e2b54fe992e8937922fe6435040deac200`；本节后续提交只更新证据文档，不改变制品。Web927ff9e、CLIa63f536保持。以下原件均位于Rocky `D:/projects/.evidence/dg003-e2e-20261111/`，本机复本在忽略目录 `.local/dg003-transfer/`，不提交备份或私有夹具。

| 实际制品 | SHA256 |
| --- | --- |
| chatbot43-c1-cancel-1118.exe | 00396c29acd367b2ce33bfd91cbaacd95d21928c7f48cf19d09c993e476bcc79 |
| imserver-c1-cancel-linux-1118 | 8708be4d2a550974c551e109fb6b58a047385c0f4b234d4d2f96f6ed2df10ea6 |
| c1-cancel-source-final-1118 | 804c157164eae9c2ddbe0808d7011e2dd5e098bb367c95228de2fb86ed48bda2 |
| mindlink-account-gate-c1-cancel-1118.exe | 1b8dbe98e7a9183ee381514a6ce786c27ca908b423faf05df19246f244cb72d3 |

`install-c1-cancel-pair-1118.py check/apply`最终均退出0；原journal `c1-cancel-pair-install-1118.json`为paired-runtime-verified/passed。停止专用双方写者后备份两个持久库，原件在 `c1-cancel-pair-before-8708be4d2a55/dg003-pair.sql`，SHA256 `3f2353b196aa4d3175f359f86e27dc1e256e395f4ce8dff7249b858493bcc8b7`。旧IM停止且restart=no保留，新IM实际PID1哈希匹配、restart=unless-stopped；BIZ由原DG003-BIZ-Media任务/Job托管新二进制。匿名查询和取消路由均403/code19008。没有schema变更，持久IM版本仍20261117，双库172表。

`verify-c1-cancel-pair-1118.py --apply`只执行一次，三个账号各用新的配套确认意图核对实际四组件与独立来源程序后，162/166/167为active14/6/5；后续无参数只读复核退出0。两个既有Home各一条新代码基站进程，Web构建/原14314入口保持。配套清单/原请求结果分别在 `c1-cancel-pair-capabilities-1118.json`、`c1-cancel-pair-gates-1118.json`、`c1-cancel-pair-live-1118.json`；不能重跑apply或旧安装journal。

安装前没有自然遗留的未结C1，`c1-cancel-live-before-1118.json`的items为空。因此在账号166以实际已安装CLI的generic-run正常登记测试分身，再由正式主人HTTP注销，配合一次受控Native群状态故障构造非空unknown；没有直接伪造任务/回执。`c1-cancel-installed-entry-1118.json`最终normal-entry-cancellation-and-successor-verified/passed：

- 正常注册测试分身产生已确认P1及冻结非空C1。原wire `ad0721cd2956b5b484503df380cc8d1976da16120d050fd8caa7cdb6c9f8477f`，原parent `360bb0829a846e268a477a7f88d3663ba8e29fc302175ef93e42519b45ccc593`，desired30/expected29，body SHA256 `04f05719d82c428fb4baa815ef1bfbeee1be4d8c68f904e7e3f7642dac9933cc`。
- 正式retire请求 `dg003-c1-cancel-live-retire-1118-01` 后，实际worker取得原22字段cancelled、双方回执逐字节相同，SHA256 `bd51ab2f1130ed3489a7a7c35a62a7e35b54102ffe74905e1c0d38a93ea1461f`。取消时业务applied与Native仍29，旧回执只有一份。
- finally恢复临时群状态后，后继parent `128f8a7e2f0c633e79bcfd100453d68ef34459bc0302762b57e47c39375b91da`正常ready31、Native active31。被注销分身不在实际成员表，Native主体disabled2、生命周期retired2；旧名单没有重新授权。原F6和接待17保持。

真实Chrome页面后验：账号166的同一群history200，活动名单无已注销测试分身；账号162正常UI登录，原Codex-A消息article唯一、history200，原50字节附件下载200且SHA256 `7bcb7da4f88a8e7202cf97d360dc62a8ce4aad20d5102d7032642fef659c7c63`保持。截图与 `c1-cancel-browser-postverify-1118.json`在本机私有证据目录；本次取消入口为CLI/正式主人HTTP，不能将页面后验描述成页面按钮执行取消。只读浏览器核验结束后恢复原空标签，并核PID/进程出生/命令行后关闭自有SSH转发，相关四端口本机监听为0。

失败记录保留：两次安装预检分别因测试脚本Windows路径分隔符和CRLF字节推导不一致而拒绝，均在服务变更前，随后按实际字节纠正；首次CLI助手因限定工具名前缀而拒绝，回查零分身/零活动子进程、群仍29并恢复临时故障后，改专用助手沿同一身份继续。页面自动化曾错用menuitem及两个正则选择器，实际DOM确认后改按钮定位；均不是浏览器故障或业务通过记录。

最终运行及清理原件为 `c1-cancel-final-runtime-1118.json`、`c1-cancel-cleanup-1118.json`。固定归属/归档哈希及全部解包文件匹配后才删除本轮9个Windows夹具目录、其中7份Linux临时副本；两个Native临时容器回查不存在，持久库无scratch schema。保留所有归档、失败日志、原journal、备份和在线制品。保护旧实例仍67应用库/2729应用表（含系统共71库）、未停止；原F6、接待17、账号准入14/6/5、取消原回执及后继31再次核对。

本项实现、独立代码审查、构建、真实跨库测试、专用候选安装及本轮入口/页面/DB验收已完成；这不等于DG003全部有效状态组合、各宿主、最终OS冷启动或安装后备份恢复演练通过。pre按主人第6项暂停，生产未发布；旧writer不得与持久取消/退休屏障混跑，不能通过删除journal或直接恢复旧数据库回退。
