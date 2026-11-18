# DG003 IM 注销屏障与 Rocky 配套安装

> 按 executing-plans 独立实施；沿现有 DG003 worktree，不使用43chat协作。主人已要求补齐并安装新候选，实施细节由当前会话决定。

**目标：** 当前合法注销在 IM 持久化之后，已通过来源检查但迟到的P1不能创建/更新身份；未开通过的身份也能沿原N1得到真实唯一结果。随后配套安装 Rocky 候选，pre暂停、生产不发布。

**正式依据：** BIZ唯一契约§28及主人2026-11-17“继续补齐 安装新候选”。现状为BIZ822d62622、IMb0e7d42；初始空群修复已验但未安装。

## 方案与裁决

Ruling: 采用独立 `mindlink_principal_lifecycle` 持久协调行，不在缺失时插入mindlink_principals/users，也不自举owner grant。原因是principal行同时控制普通协议的身份命名空间，空墓碑可能误伤普通用户/排序别名；独立协调行只约束P1/N1，不参与普通NativeFence身份匹配。代价是一张新增表，必须迁移与整组写节点配套升级。

当前21字段来源继续严格核对，不增加宽松可选字段。新IM明确将“固定当前退休来源已核验”作为负向投影屏障权限：可对该来源绑定的内部主体写永久拒绝记录；不把Native无行当不存在证明，也不把这种记录当聊天/历史/普通号权限。它是新增的负向权限语义，不是旧N1重查策略自然授予的权利。

P1/N1锁序为原operation→协调行→owner→principal→ticket；协调行的唯一键使两个尚无principal的事务也能串行。open保存创建操作完整冻结绑定，采用排他upsert核对，不用1062共享锁升级或affected-row判新建；retired覆盖为N1原绑定。旧N1重查只读协调行。只有严格当前来源核过的N1可在缺owner/principal时建立负向记录，既有显式write=false仍拒绝；存在principal但缺owner、同avatar反向绑定、已有open/旧P1或N1 journal、损坏投影/非明确普通用户仍拒绝。普通users不修改、不占号、不由该协调行封禁。缺投影分支检查旧journal，旧表没有target索引，稀有恢复可能扫描历史；全部历史同时丢失须依赖可信备份。

当前退休来源绑定的N1可使同scope/avatar的现有active投影从不高于原expected的实际policy停到desired，CAS使用锁内实值；高于expected或代际倒退继续拒绝。无来源的底层旧路径仍要求原严格CAS。该增量收敛已跳过的未确认正向修订，但不能覆盖更高Native策略。

缺投影屏障的原ready只能在该运行实例启用固定来源、当前system信任有效且same actor/issuer/scope/owner/原operation/request/body绑定时只读恢复。先验证原journal/严格N1结果和协调行代际/policy；已有read=false仍拒绝。GET不创建任何记录，不扩展P1/C1/F3或聊天历史读权。

## 执行步骤

- [x] Task 1 红例：真实BIZ注销、Native缺投影/缺owner，沿原N1须ready但当前失败；保持原失败及库/F6快照。Expected: 用例因N1无法收敛失败，非夹具错误。
- [x] Task 2 实现：正式新库DDL、有序20261117迁移和NativeFence就绪检查；来源只在完整验证成功后设置私有请求标志；新增协调行共享P1/N1锁，负向效果/原结果同事务；限定原N1查询。
- [x] Task 3 真实测试：P1已过真实来源后停住、注销先赢/正向先赢、丢响应、同键并发、事务回滚、显式read/write拒绝、坏绑定与来源、普通号/别名无副作用、另一新分身可开通、未确认修订跳跃；新库与已有库迁移/重放/缺迁移失败关闭。Expected: 原业务身份保持disabled，Native正向不复活，原N1结果与BIZ唯一回执一致。
- [x] Task 4 回归与独审：原N1/C1/空群真实回归、IM权限integration及项目登记检查，离线skip与真实DB结果分开；一轮整体独立只读审查。提交本项固定代码，不推送/合并。
- [x] Task 5 配套安装：只读核当前实际服务/制品/数据库UUID及原F6，保存测试库与任务/容器配置快照；安装固定BIZ、IM、独立source、已验Web/CLI配套，核实际运行hash与正式账号兼容确认。迁移只作用DG003持久测试IM库，不改旧保护库/默认服务。
- [x] Task 6 页面及DB：实际页面验证正常接入/会话可用，并核本次注销与迟到P1、负向屏障/原结果/群成员及原F6；删除本轮无用临时资源并回查。生产不发布、不重启OS。

## 验收与风险

- 所有P1/N1写节点须同版本并共用同一持久journal/协调表；不能让旧writer与新屏障混跑。
- 业务刚注销到IM负向事务提交之间仍存在传播窗口；保证从IM屏障提交起开始，不冒称跨库即时原子撤权。
- 数据丢失、坏行、SQL失败不能当首次未确认；正常source失败和Native显式拒绝不被“恢复”吞掉。
- 回退保留新增记录；不得为回退丢弃已生效屏障或启用绕过它的旧写节点，失败时先保持专用服务停止/写入暂停并核原结果。
- 浏览器若故障立即停止该部分，等待主人恢复，不重试/换浏览器；其他可独立验证仍继续。

## 本轮证据索引

证据为本机忽略目录 `.local/dg003-transfer/` 与 Rocky `D:/projects/.evidence/dg003-e2e-20261111/` 中同名固定包/结果；不提交数据库配置、正文或日志。以下只描述对应制品快照。

- 初始行为红例：`p1-fence-red-eb630e01cbfa.json/log`，原N1无法结算；最初两真实绿例 `p1-fence-green-c0768508a3a3.json/log`。均核临时schema无残留、原F6不变。
- 第一轮扩展21项20过，未确认改名夹具尚未结算P1而失败，保留 `p1-fence-final-d362d70daa38`，未将其计通过。已补真实P1结算后正常改名。
- 原权限回归发现旧N1 POST重查的新增策略限制，已修；后续完整套件Native2、Native3分别670/671个pass事件，3项外部配置skip另列。
- 独审四项异常数据发现补六个真实红例 `p1-fence-corruption-red-c480b07cb4db`，均按预期失败，未残留schema或变更原F6。修复后 `p1-fence-all-3c89b0f1e4ac.json/log` 的27屏障、13原N1、11原C1、20空群子例全部通过，零skip。
- 1062共享锁升级额外unknown已真实复现：`p1-native-4a6ef1ae6eda-ready2`，两个不同operation等同一既有open行。此前同包首次网络未就绪原失败保留，非红例；测试引擎就绪检测已改TCP，未将临时初始化MySQL当可用。
- 最终Native4包 `6af0e2ce1f60dd05a3bec9b4bce399bc0d2f8306125112968aeaefa3b67f901c`：`p1-native-6af0e2ce1f60.json/log` 672个pass事件（含父/子项），0fail，3个独立配置skip未计覆盖；新库、09→17升级/重放、缺表/坏列/前缀索引拒绝、既有行双竞争和旧N1重查不建行均有实际数据库结果。临时MySQL容器已移除，scratch无残留，原F6保持。
- 最终跨仓固定包 `17cd2cbd9b795e8446fda5a6259a4026cef95960ef6bfbeea2289efc90ac611f` 最终27屏障、13原N1、11原C1、20空群子例全部通过，零skip，临时schema无残留、原F6保持。独立只读审查见私有 `p1-fence-whole-review-1117.md`，五项实现发现已关闭；安装脚本最终复核无阻断。

- 项目登记的13个自包含包（其中7个无测试）及额外gateway服务包编译/执行通过；`p1-ci-1117.json` 中convercache的4个外部MySQL项为skip，未计数据库验收。受影响core/gateway的go vet与Linux launcher构建通过。安装脚本以-O和未知命令实跑拒绝，未触达服务。

## Rocky 配套安装与实际旅程

实现固定提交为 `60387d1`，正式BIZ契约为 `512a3d6c3`（后续说明只作交接澄清）。运行制品、完整源码指纹及组件能力以私有 `p1-pair-candidate-1117.json` 为准：BIZ运行源码822d62622、IM本项实现，Web927ff9e、CLIa63f536保持原已验制品，未对App做兼容确认。Native二进制由提交前的相同源码构建，已按逐文件SHA对应固定提交，不冒称其内嵌VCS元数据为60387d1。

`p1-pair-install-1117.json` 记录实际切换成功：旧BIZ父子进程退出；旧IM容器保留、停止且restart=no；两座专用持久库在停写后生成一致性dump并保存SHA；新增表迁移到20261117。新BIZ实际进程、父启动器及计划任务Running、IM /proc/1/exe和独立来源程序摘要均核对。新IM先restart=no，通过策略入口403/19008及运行检查后才设unless-stopped。备份保留于Rocky证据目录的 `p1-pair-before-97ed551eb4a2/`，不作为覆盖安装后退休屏障的回退授权。

首次apply在停服务前因保护库自定义socket未指定而失败；原BIZ PID/birth、旧IM started/hash、任务状态与空备份目录逐项回查，空目录已删除，失败留在 `p1-install-pre-socket-failure-1117.json`。之后预检和结束均通过专用socket核对旧保护库全列表：71座总库包含67座应用库，不将4个系统库混计。旧保护实例未停止。最终安装脚本的socket及计数修正经独审复核。

`p1-live-after-1117.json` 核验现场原owner166缺投影N1自动沿原ID结算：父ready、真实原结果及业务回执各一且字节相等，协调记录retired/目标代际2/policy2/projection_absent=true，没有创建principal或users。未手写回执、重建操作或重放原保护F6。

`p1-pair-live-1117.json` 核对实际四组件及双Home各一基站，正式新制品确认后162/166/167分别active13/5/4；接待17、原F6保持。`p1-normal-entry-1117.json` 与 `p1-normal-retire-1117.json` 补真实安装CLI的全新测试分身开通和群ready、一条主人消息持久化，再经网页“聊天→分身→该测试分身更多操作→注销”完成真实停用。最终Native principal disabled2、life retired2且projection_absent=false，原N1双方回执一致，当前群名单已移除，业务发送及原生消息各唯一一条。测试分身已正式注销，记录保留。

真实浏览器从正常登录进入旧会话，原文件50字节及SHA匹配；新分身消息正文恰一article，注销后名册不再显示活跃分身，历史标“已注销”，提示不可再写，消息仍可读取。`p1-browser-1117.json` 与同名前缀PNG保存事实和入口。浏览器通过回环SSH访问Rocky服务，不冒称Rocky桌面人工点击验收；脚本的URL对象、尚未创建DM、面板入口和退休URL观察器错误保留，均未算通过或归为浏览器故障。实际退休请求是percent-encoded原avatar ID，只点过一次确认；未因观察器超时重复注销。首条发信503后先确认没有发送意图，保持原键；accepted阶段继续原命令查原结果，最终一条persisted，不把accepted当完成。

`p1-cleanup-1117.json` 核对本轮13个已解压测试目录及容器内对应临时程序已删除，临时Native MySQL容器无残留；固定包、结果、备份、旧停用容器及实际运行制品保留。本机过时的14个裸二进制及CI解压目录已清理。之后用只读配套检查复核服务，不以安装前快照当持续保证。

本项实现、配套安装、页面及数据库验收完成。DG003整包仍有非空旧C1取消/其他在途组合、特定宿主Monitor等待验项；App排除，macOS真机单列。pre按主人要求暂停，本轮未部署pre或生产、未重启操作系统，未执行安装后备份恢复演练。
