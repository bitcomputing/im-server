# DG2 原 PERSIST-only 分阶段执行单（待协调，2026-11-09）

来源二当家 `pbz6dc7wwt6hukeh`，有效批次 `4724d50c6b3140e5d6fe3224d7515d7a1d43bf514e71c3753038be6a062e0dcc`。本单交付准备材料；**尚无切换、写入、重启或回退窗口，不可直接执行。** 候选仍是原 `4ecec887fb38d2d81af4bf2773b05d304e8ced97`，不是 POLICY/公共 READ/序列新方案。制品、来源和此前实际检查见 [§22](mindlink-conversations-probes.md#22-rocky-原-persist-only-固定制品与复验稿2026-11-09未切换) 和 [manifest](evidence/20261109-dg2-persist-only/manifest.json)。本单替代 §22 中固定“未来5分钟”的安排；历史提议原件不改写。

## 1. 固定执行对象与证据边界

| 对象 | 审查值 |
| --- | --- |
| 候选 binary / image ID | SHA256 `551de72723dc3c4917c8666f67e8165a094800769f050299e1df53b19838b740` / `sha256:ed9e1addeb5344722faa92dedbe082597655fe3a77720381ca58ef90401f7145` |
| 原运行/回退 image ID | `sha256:09186cb700fdb2e4fe21e163494af2c9dc1e81cb69e5fa1fdf8658b848ef45a7`；binary SHA256 `bfdcaa5d73721472cce9af5b3debcd525f24ab0cfabe2ca1f3cb373540510c9a` |
| Compose 上下文 | project=`dg2-rocky-im`，service=`im-server`；原文件 `D:/projects/.evidence/DG2-20261106-002/im/compose.json`，原 `--env-file D:/projects/.evidence/DG2-20261106-002/im/stack.env`，不得省略或输出凭据 |
| image-only override | 同目录 `persist-only-4ecec887-01c5/candidate.override.json`；回退用 `rollback.override.json`，文件哈希见 manifest |
| 保留边界 | 原 loopback 19001/19002/19003/18090、卷、租户 `dg2c3350172f150`、主人映射 `test_6199931265070805`、原历史及本轮已持久新增消息；BIZ/MySQL/Redis/私有配置/pre 不变 |

本批 SSH 只读采样 UTC `07:10:39.6105597`：运行仍为容器 `d16f0401…e3172a` / 原 image09186cb7，StartedAt=`2026-11-06T07:36:43.635270669Z`；Healthcheck 为 interval5s、timeout3s、retries20、start_period20s，RestartPolicy=no。StopSignal/StopTimeout 均返回 null，**停止、启动及完整回退耗时未实测**，不能把 null 当实际10秒。UTC `07:16:39.7023528` 只读核到 Compose `2.27.0-desktop.2` 的 up 支持 `--timeout/--wait-timeout/--no-deps/--pull`，restart 支持 `--timeout/--no-deps`。参数存在不证明停止能刷净内存。

旧 §9 的真实刷新等待为187.146s和126.919s，只作估算依据，不算新 binary 的测量。以下每个刷新边界暂留240s观察、约5s一次只读检查；240s到期未命中就是未验证，不是数据丢失或通过。每次检查使用新只读事务，避免长期事务重复读取旧快照。健康等待拟为180s，只覆盖健康等待，不覆盖完整切换/恢复，也不保证一定够用。

## 2. 开始前须填写并核实的准入单

以下目前均未齐；二当家统一协调后形成带版本的批准单，不从健康、沉默或旧浏览器关闭推断许可。

1. **参与者释放：** BIZ列全实际参与者（含主人、Web、自动写者/定时任务），附各自停测确认、有效起止、范围与证据。BIZ `pbz5bgrl6lwhukeh` 明确未释放；后来只关闭自己活动不替代全体释放。当前没有可核验的完整参与者名单/释放单。
2. **合法主体及影响集合：** BIZ提供本租户已有合法发送者S、专用接收者R、两个已有小型封闭群G1/G2及完整成员，确定所有受影响会话行/写者；不隐含创建租户、用户、群或替换主人映射。R在测试前须无未读，全部会话仅限明确批准的测试集合，禁止把主人日常用户用于全部已读。全清还影响所有会话的read-time/unread_tag、清空内存MentionInfo，并按app_key/user_id删除全部 `mentionmsgs`，不只处理G2。R的SQL mention行数和全部会话原生MentionInfo须在基线及P4前均为空；本轮消息不得携带mention。若存在其他会话/未读/mention或无法核验，P4阻塞，报告具体影响，不先清理来满足门槛。
3. **Token与客户端：** 由BIZ在获批阶段经真实业务获取S/R Token，进程内交给测试端，禁止落公开文件/日志或复用旧夹具Token。批准两类原生设备连接（拟Web/PC协议类型）及可能的在线/设备审计影响；这两个原生连接不等于浏览器/发布SDK验收。CONNECT前核租户、返回user及业务绑定，任何不符停止。
4. **写入及读操作额度：** G1仅M1/M2/M3各一次，G2仅M4一次；无超时补发。明确允许G1 `clear_unread`（含该会话相应mention清理）、R的全局 `clear_total_unread`（含上项所有副作用）、限定元数据SQL及各段IM-only停机次数。P4缺合法专用用户/全局影响确认则阻塞；不得把跳过P4记成完整PERSIST通过。
5. **测试驱动：** 固定客户端源4ece、驱动文件/hash、实际参数矩阵及证据目录后再核准执行。当前只有下面的协议步骤，**尚无适配Rocky既有身份并经审核的驱动制品**。旧 `simulator/probes/mindlink` 夹具会管理登录/建租户且硬编码F0端口，不得直接运行。新驱动必须检查marshal/unmarshal错误、transport成功、非nil ACK及ACK.Code=0，查询还须非nil合法结果；禁止沿用忽略server ACK.Code的 `ClearUnread` 包装器作成功依据。
6. **停止前数据门：** 默认保留所有需要保留的状态。当前刷新队列仅在内存，尚无受支持的全进程pending/drain证明。已枚举对象的SQL相等不证明其他对象无待刷。须先提供完整受影响范围、停止写者及可核验持久化/排空证据；若无法覆盖，P1即阻塞。任何具体未持久状态的例外只能按主人明确授权处理，不能由本单或二当家默认为可丢弃。
7. **批准范围与结束状态：** 每段确认候选/hash、执行人、绝对UTC起止、最晚新动作时点、必要恢复储备及异常分支。须明确段末“候选继续保留”或“持久化后换回原镜像”，以及分段间能否恢复普通测试；未选择不得开始。尚未批准任何新的具体时间。旧06:00–06:05UTC已过期。

## 3. 相对时长预算（估计，未预订）

| 分段 | 工作量/预计时长 | 另留恢复与记录 | 建议协调额度及停止点 |
| --- | --- | --- | --- |
| P0 准入/只读基线 | 10–15分钟；核上表、固定文件及SQL基线，不登录、连接或发送 | 不含等待各方回执的时间 | 不占停机窗口；缺项不进P1 |
| P1 首次候选启动/恢复检查 | 4–7分钟；停前数据门、image-only切换、健康、获批真实Token CONNECT及旧历史 | 回退/恢复暂留6分钟，记录2分钟 | 15分钟；无新消息，未达门槛不进P2 |
| P2 单会话读/摘要持久化 | 5–7分钟；M1→合法read→M2/M3、双端实收/history、最多240s刷新观察 | 恢复暂留6分钟，记录2分钟 | 15分钟；DB未命中不进P3，不停候选 |
| P3 IM重启/持久结果恢复 | 4–7分钟；只读重核落库、IM-only重启、双端重连/同ID及SQL对照 | 回退/恢复暂留6分钟，记录2分钟 | 15分钟；失败停止新测试，按已批准异常分支处理 |
| P4 全部已读回归 | 8–11分钟；M4、全清、240s刷新观察、落库后IM重启与同ID恢复 | 回退/恢复暂留6分钟，记录2分钟 | 20分钟；需全局影响准入，缺项保持BLOCKED |

建议P1–P3连续协调45分钟；P4另20分钟，且其起点须为同一获准候选。完整P1–P4暂按65分钟，另有P0的10–15分钟。若选择分段回退，下一段须重新包含P1；不能把重新切换藏在原额度中。连续安排也不压缩各段的恢复储备；取得实际耗时后才可重估共享预算。

上述是审查用保守估计，不是实测上界或已承诺恢复时间；健康配置/旧刷新时间无法推出完整停机耗时。首次P1须由协调方接受这一不确定性及已批准的故障处置；P1记录每个动作实际起止，据此重算P2以后所需储备。不足则暂停后续分段重新协调，不自行延长授权。任何新停机/写入动作前须仍有“该动作＋持久化确认＋完整恢复＋记录”的剩余预算；未具备就停止新增动作，不能等到最后一分钟才回退。

## 4. 分段操作与通过证据

**P0：** 固定原container/image/binary/start、候选与两个override hash、原Compose project/env/端口/挂载、参与者及保护对象；只读保存租户/映射/群成员、测试对象已有消息ID和SQL会话元数据，以及按app_key/user_id限定的R mention计数、各会话read-time/unread_tag基线。原生MentionInfo基线在P1获批连接后、任何测试消息之前补核；P0未连接时明确记未核，不提前报通过。需存在的对象为空/查询失败均报告缺口；mention经成功查询确认为0才符合本单要求，不把查询失败当0。不以补建/清理夹具解阻。私有证据只留必需元数据，公开记录不含Token、Secret、正文或原始日志。

**P1：** 再核完整释放有效、原实例仍相符及停止前数据门；按§5切换一次。读回实际image/binary/start、端口/挂载/健康及BIZ/MySQL/Redis身份；获批真实业务Token经导航建立R双端与S连接，逐项核CONNECT ACK、用户、可读旧历史ID，并补核R所有批准会话MentionInfo为空的原生基线，与P0 SQL计数0对应。连不上、版本/端口/卷漂移、保护状态未核、出现mention或窗口耗时超预算，均停止P2。健康或TCP成功不能替代CONNECT/history。连接与查询可能产生在线/会话活动，须包含在授权和后续停前对账中。

**P2：** 每一步只有当前一步实际成功才继续；所有对象锁定本租户及批准群，不把旧独立last_seen方案混入本单。

1. 查G1普通摘要及R两端 `qry_conver` 的 `IsInner=true` 阅读元数据，要求基线未读0，保存原index；这里只读取当前R合法会话，IsInner不授予跨用户权限。
2. S调用 `SendGroupMsg(G1, jg:text, flags=10)` 一次，记录M1的ACK ID；实际双端收到M1，history含同ID，摘要为M1、unread=1且LatestUnreadIndex比基线增加，再取这个真实index/时间。
3. R用原生 `Query("clear_unread", R, marshal(ClearUnreadReq))`，conversations仅G1、channel_type=Group、LatestReadIndex=刚取得的LatestUnreadIndex、LatestReadMsgId=M1、LatestReadMsgTime=M1的真实消息时间。要求transport成功、ACK非nil/Code0；双端元数据readID=M1、readIndex=该index、unread=0。
4. S依次单发M2(flags=8)、M3(flags=72)，每条核双端实收、history同ID及摘要更新；最后摘要=M3，readID仍M1、readIndex不变、unread=0。任何ACK或收取未知先停写，不新增ID重试。
5. 停止测试写者，最多观察240s，核受影响成员各自的 `conversations` 行（app_key/user_id/target_id/channel_type/sub_channel精确限定）、latest_msg_id、latest_read_msg_index、latest_unread_msg_index、latest_read_msg_id/time、is_deleted及批准的既有扩展元数据；R行须与上述原生结果一致。消息历史SQL按批准app/conver/sub_channel/ACK IDs确认M1–M3各一份，不取正文；同时核保护对象/其他成员预期状态。出现其他写者/未知变化就停止并协调，不能只查R一行后称全进程可重启。

**P3：** 重新核准入和停止前数据门，通过才用同一候选上下文IM-only restart一次。重新取真实Token/连接按批准范围进行，不重发M1–M3；双端history仍含原三ID、摘要=M3、readID=M1、readIndex保持、unread=0；同一限定SQL前后对照，其他保护对象/原数据及非IM服务身份一致。逐项有实际结果才能记该项通过；服务恢复失败、NULL/查询失败、未跑均不是通过。

**P4：** 单独再核R仅批准会话、无其他未读/活动写者；SQL `mentionmsgs` 按app_key/user_id成功计数0、各会话原生MentionInfo为空，read-time/unread_tag已取基线且全局副作用已获批准，否则不发送M4或全清。G2先有明确基线，S发M4(flags=10、无mention)一次，双端实收、history/摘要=M4、unread从0增为1；全清前再核mention仍为空。随即R用 `Query("clear_total_unread", R, marshal(QryTotalUnreadCountReq{}))` 核transport/ACK/Code0，双端G2 readIndex=其真实unreadIndex、unread=0，并核G1/批准集合未被错误回写。此命令覆盖R所有会话及全部mention；不能用payload限定G2。全清后SQL mention计数仍0、原生MentionInfo为空，并保存所有批准会话read-time/unread_tag的实际变化；ACK0不能替代这些核对（该源码未消费mention删除的返回错误）。

在延迟刷新期后重新核G2 SQL的readIndex/unreadIndex相等、unread_tag=0和摘要=M4，及G1的M3/M1/readIndex保持；观察预算240s。通过停止前数据门后，再IM-only重启一次，双端重连、原M4历史/摘要/全清状态及同一SQL均一致，mention仍为空，才记本段全清恢复通过；零基线不证明非空mention删除分支已验证。必须证明覆盖了待刷快照与全清相继发生的场景；如果无法观察到该交错，记录仅为一般全清恢复，不把它升级为并发刷新竞态验证。本单不注入DB错误、不改DB状态造竞态。

## 5. 获批后使用的停机命令与回退边界

沿原上下文使用 image-only override，**本批没有执行以下命令**。`--timeout 30` 是拟定的停止等待参数，不是已测时长/排空接口；可能到期强制终止，因此先过数据门。执行前核所选override和当时image ID；每次命令结束立即核exit及实际对象，不把失败或中断当作未执行。

```powershell
$original='D:/projects/.evidence/DG2-20261106-002/im/compose.json'
$prepared='D:/projects/.evidence/DG2-20261106-002/im/persist-only-4ecec887-01c5'
$originalEnv='D:/projects/.evidence/DG2-20261106-002/im/stack.env'
# P1单次切换；只在已批准窗口和全部数据门通过后运行。
docker compose --env-file $originalEnv -p dg2-rocky-im -f $original -f "$prepared/candidate.override.json" up -d --no-deps --no-build --pull never --timeout 30 --wait --wait-timeout 180 im-server
```

P3或P4各自获批且DB核对通过后才可用独立重启命令；restart没有上面的健康等待效果，须另行限时读取健康并完成真实恢复检查。

```powershell
# 变量沿上块；每次重启单独核准，禁止自动循环。
docker compose --env-file $originalEnv -p dg2-rocky-im -f $original -f "$prepared/candidate.override.json" restart --no-deps --timeout 30 im-server
```

仅已批准回退、停止前数据门通过且尚有完整恢复时间时，用独立回退命令，不串成失败自动回退：

```powershell
docker compose --env-file $originalEnv -p dg2-rocky-im -f $original -f "$prepared/rollback.override.json" up -d --no-deps --no-build --pull never --timeout 30 --wait --wait-timeout 180 im-server
```

回退后核原image/binary、实际start、原端口/卷/健康、同一用户真实CONNECT及原消息ID/SQL；保留本轮已持久新增数据。不恢复整库、不删数据/租户/用户、不 `down -v`/prune；不启停BIZ/MySQL/Redis。回到b87会恢复其旧摘要缺陷，也不能找回被停掉的未持久内存。

## 6. 中止、未知及收尾

- **动作前未满足门槛：** 不开始该动作，报缺项和当前实例；时段过期后不借旧单继续。无任意扩大范围、补建账号或修BIZ授权。
- **发送/清读结果未知：** 停止新增测试写入，保留原请求及ID/唯一合成标记，按原对象只读核ACK/实收/history/DB，不重发。无ID也不能另发一条假装原动作未执行。
- **240s未落库、待刷不明或保护对象漂移：** 停止新增写入，保留当前实例/卷和证据，报告二当家协调；不触发新的重启、换镜像或清缓存。开始写入前必须已明确接受此异常分支和负责人，不能到期才临时决定停掉候选；保留实例不等于自行延长停机或取得新操作许可。
- **命令失败/中断：** 只读核实际container ID/image/start、端口/挂载/健康及阶段数据；发生过创建则按本次project/service/image核所有残留实例。不盲目重跑、不删未知资源。无法核实时记录UNKNOWN，保留现场；不能宣称已回退。
- **时间不足：** 提前停止新增动作；只有数据门通过且完整恢复仍在许可内才执行批准回退。健康失败且数据门仍未满足时，不为赶截止强杀实例；立刻报告既定故障处置负责人。任何后续切换/延长须新有效确认。
- **结束记录：** 各步写实际UTC起止/耗时、源/驱动/二进制/image、参与者释放、ACK/实收/history/SQL独立结果、重启前后对象、最终镜像与遗留测试数据。结果分为PASS/FAIL/BLOCKED/UNKNOWN/NOT_RUN，失败/null/未跑不计通过；提交脱敏报告，私有证据不提交。候选保留/恢复原镜像及是否开放手测按批准单实际回报，不作持续健康保证。

## 7. 本批交付状态

本批仅新增本单并更新§22/manifest；实际做了源码/文档核对、SSH只读元数据/CLI帮助检查。没有运行候选binary、登录/CONNECT、发送消息、调用clear、查询业务DB、改配置、启停/回退服务或创建定时任务。上述用户流程、SQL、停止/完整回退耗时、驱动及全部参与者释放仍待验证/补齐；本单不完成原DG2运行验收，也不完成POLICY/RELIABLE任务。

独立材料初审Critical0/Important1/Minor0指出遗漏全局mention清理影响；修订后同一审查方复审Critical0/Important0/Minor0，原问题关闭。审查仅覆盖本单/§22/manifest，没有远程、容器或业务验证，不授予执行许可。
