# DG2 停前排空证据的具体缺口（只读源码审计，2026-11-09）

来源二当家 `pbz66b4dey4hukeh`，有效批次 `9e0474dff8b8c9dffecd7f4c708805c2f0a041cc82c1539ecf7bf9610413ce24`。本批只读本机固定源码，不连接Rocky、不取heap/私有日志、不加观测接口或改Shutdown。P1首先要停的原版本是b87；4ece的候选实现不能倒算成原进程已有排空能力。

| 必须知道的事实 | b87原版本 / 4ece候选源码依据 | 当前具体缺口、不能作的推断 |
| --- | --- | --- |
| 原进程哪些会话状态尚未写库 | b87 `services/conversation/services/convercache.go`：PersistConverV2入队值为 `*models.Conversation`，初始化回调却只消费 `*ConverPersistIndex` | 这是原摘要落库缺陷的源码依据；60秒到期可能只移除队列项而不Upsert。既不能“等够就刷净”，也不能以队列空推断旧状态已保留。本批没有读取活体内存或重现它 |
| 候选全部待刷key及最新版本 | 4ece同文件 `persistLatest`、`queueConverSnapshotLocked`：保存冻结快照，再投EphemeralCache通知 | 未导出的进程内sync.Map，没有现成受支持的非敏感API/指标给出所有key、版本及保护范围覆盖度；仅查询两个测试群不能枚举其他待刷对象 |
| 通知、在途worker、等待锁是否为零 | 4ece `persistConversationSnapshot`：32个persistSem名额，异步goroutine，按用户commit lock串行Upsert | 没有全局在途/等待锁/成功提交水位观测。`commons/caches/ephemeralcache.go` 先删过期通知再执行回调，所以Len=0不等于worker完成；Len本身也不是现成对外排空接口 |
| 是否还有延后重试或被丢弃的坏快照 | 4ece队满或未知/暂时DB错误通过 `time.AfterFunc` 重排；特定MySQL坏数据错误CompareAndDelete该快照 | 缺可核验的待触发timer集合、最后错误/重试/坏快照丢弃记录和成功提交版本。即使单次pending读成0，也不能证明没有稍后工作，更不能证明已丢弃状态成功落库 |
| “最后一次写入”之后是否确实无新写者 | 会话更新/清读/全清仍由原RPC/Actor路径进入；没有本任务的全写者冻结屏障 | 缺写入口/后台活动完整清单、各实际参与者停测证据与可验证的统一边界；BIZ仅核自己能核实的参与者，主人占用由二当家协调。零inflight、关闭一方浏览器或停止驱动不是全局静止证明 |
| SQL与内存是否覆盖相同保护集合/版本 | SQL能读已提交的会话/历史；native普通/IsInner查询仅返回调用用户可见状态 | 缺全保护集合的权威枚举及内存→成功SQL提交的一致版本对应。统计行数相同不证明字段相同；一次字段相等也不能证明其他用户/其他模块无待刷。缺失会话可返回Code0空壳，native mention懒加载错误也未完整上抛，不能把空值当成功的全状态观测 |
| 正常停机是否等待上述全部完成 | b87/4ece `services/conversation/starter.go` 的Shutdown仅打印；`launcher/main.go` 信号路径调用模块Shutdown等，没有会话flush/wait步骤 | 没有本任务可调用并确认完成的drain/shutdown握手；stop timeout不是排空接口，强制结束会销毁内存。候选镜像回退也不能恢复这些状态 |

本批检索固定4ece生产Go源中上述persist变量的引用，只定位到会话缓存实现，没有找到它们的公共观测/排空注册；这不等于运行中数值为零，也不外推其他模块已排空。固定源文件/客户端哈希及本批构建、自检记录见[manifest](evidence/20261109-dg2-persist-only/manifest.json)。源码证据不是当前Rocky运行/DB证据。

当前能完成的是固定驱动、离线协议/失败分支测试和上述缺口定位。不能据此提请可执行窗口；P1的停止前数据门仍未满足。完整非敏感观测/写者屏障/成功持久化核对若需要实现，属于新的生产行为改动，需另定明确范围和验收；本批不加一个“drain=true”标志来替代这些事实，也不通过导出/重放消息或清缓存造证据。

未来针对已明确授权的小范围可逐行对账，但不得称全进程排空。原b87已有未持久状态是否需要保全、如何恢复，目前未核；任何例外都须有具体主人授权，不能从二当家接受预算/驱动材料推出可丢弃。没有排空证据就不切换，未持久化时停止新增测试写入并保留实例；本批没有启停、写入或恢复动作。
