# DG2 PERSIST 原生驱动

仅准备和离线自检；当前不具备合法独立S/R、G1/G2、排空证据或执行窗口。二当家 `pbz66b4dey4hukeh` 转述BIZ07:05/`0d8628067`：仅159/160、均无永久认领，160为主人。先闭合503和正常认领，不能填入这些身份来充当独立双端，不能补建群。完整范围见[执行单](../../../docs/dg2-persist-execution-plan_zh.md)。

驱动复用固定 `4ecec887fb38d2d81af4bf2773b05d304e8ced97` 的 `simulator/wsclients`/codec/proto，不改原客户端/服务/DB/依赖，不使用旧建租户测试。构建须在该固定源码副本上叠加本目录；本目录hash和实际编译平台见[制品清单](../../../docs/evidence/20261109-dg2-persist-only/manifest.json)。不从当前后续HEAD重新选一个运行版本。

## 参数与默认自检

`params.example.json` 特意留空真实主体、群、运行号、历史ID和证据引用，缺值会拒绝。S/R及小群完整成员由业务实际返回确认；Web/PC是同一R的两个原生设备类型，并非两个业务用户。所有权威绑定/成员授权仍需BIZ核实，CONNECT只核Token返回的user与参数相同，不能证明永久认领/成员关系。

```powershell
# 默认只解析/校验非敏感参数；不读取Token、不连接、不创建journal。
dg2persist.exe -params params.json -phase g1
```

`-execute` 仅在另有当前有效范围/时段授权后使用，本批禁止。`approved_phase_ref`、`protected_durability_ref`、`zero_mention_sql_ref` 是证据引用，**程序不能核实其真实性，不产生许可或SQL/drain通过结论**。`new_action_cutoff_utc` 是停止新增动作时点，须早于批准窗口末尾并预留执行单全部恢复时间；程序每次连接/原生操作前要求至少30秒，不能撤销已在途操作或保证整个流程在截止前结束。

## 获批后的阶段接口（本批均未执行）

其中baseline也会建立连接，对应执行单P1的获批连接段，不属于仅核非敏感事实的只读P0。

| `-phase` | 原生动作 | 必须单独完成的外部核验 |
| --- | --- | --- |
| baseline | R的Web/PC CONNECT；不带过滤的一页至多100会话且IsFinished=true，两群必须实际存在；拒绝G1/G2外会话、mention或未读；查普通/IsInner元数据及给定旧历史ID | 真实业务Token来源、永久认领/成员、SQL mention基线0及全部保护对象；baseline_history_ids必须覆盖两群真实原ID，不能为过门补发旧消息 |
| g1 | 上述R双端及S CONNECT；M1(flags10)实收/history/摘要→M1真实index/时间清G1→M2(flags8)/M3(flags72)；各条均核双端，合法read不变 | 所有受影响成员SQL/扩展/保护对象，实际刷新，停机前完整待刷证据；本命令不等待或验证DB |
| g2 | 先用 `-expected` 核原G1状态，发送M4(flags10、无mention)，再次核完整批准会话集合/空mention，再全局clear；保留原G1/M4 ID | 全局read-time/unread_tag/mention副作用许可、全清前后真实SQL计数；不证明非空mention删除分支；SQL和安全重启另行执行 |
| verify | `-expected` 只读核原G1及已有M4历史/摘要/合法read，不发送/清读 | 实际container/image/binary/start和SQL前后对照；驱动不知道是否真的重启，不能自行称“重启通过” |

未来执行时显式给 `-journal` 新建私有JSONL文件，g2/verify另给 `-expected` 原g1/g2成功原生观察journal。Token仅由获批BIZ流程在内存中以一份 `{"Sender":"…","Receiver":"…"}` 经stdin传入；不得写进params、命令行、公开证据或Git。驱动不登录业务、不导航获取Token、不创建用户/租户/群。

journal以O_EXCL创建，每条本机意图在发送前写入并Sync；ACK ID在继续前落盘。同一路径已存在即拒绝；未知/中断保留原文件，不换新路径重做。此本机保护不是服务端幂等性：换路径/副本仍可能重复，原wire没有本驱动的持久操作去重。没有ACK ID时亦按原意图/合成标记核真实结果，不判“未执行”。查询失败、非零ACK、nil、坏protobuf或缺原ID均中止后续写入，不自动重试/重连。

journal仅保留ID、索引、观察时间和scope摘要；不保存Token/正文/原始ACK。固定模拟器即使Verbose=false仍打印未知入站topic，所以执行模式丢弃其raw stdout，并只经预留writer输出本驱动脱敏结果。继承客户端的ntf同步/线程行为未修改，且WriteMessage未设置写超时；截止检查只能阻止新动作，不能保证卡住的在途写能按时结束。离线测试不证明真实连接竞争、已发布SDK或页面行为。

成功事件明确标 `native_observed_DB_restart_unverified` 或 `native_reobservation_DB_runtime_identity_unverified`。SQL、全进程drain、真正重启/回退、竞态交错、主人/其他参与者离场不能由本驱动证明。真实写入前缺外部证据就不执行；未落库保留实例，不在本命令内启停/改镜像或修DB。

## 本机离线检查

```powershell
go test ./simulator/probes/dg2persist -count=1
go vet ./simulator/probes/dg2persist
```

测试使用纯内存协议替身，无HTTP/WS监听、真实账号或数据库。覆盖ACK丢失但消息效果已发生、非法参数/其他会话/mention/不完整列表、nil/错误ACK/解码、原ID恢复、重复意图文件和默认不连接。构建及无连接自检另在本机Docker `--network none` 内使用固定4ece副本执行；不进入Rocky。
