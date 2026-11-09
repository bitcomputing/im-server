# 心灵传输三人群原生能力探针 · F0-IM

任务 `DG-20261104-003-F0-IM`；2026-11-05；负责人：43chat im server管理。
**§1—8 保留 F0 历史记录；§9 为 F1-PERSIST 修复与本地验证。权限/独立阅读产品门仍未通过，未部署。**

## 1. 依据、版本和隔离

- 唯一产品原文：相邻 `43chat-soul/产品文档/6-方案/待排期/私聊-产品定稿.md`，本次全文读取，SHA-256 `f5ed9ee038636c81769784a7af8f902806d12723d696dd1e13bf9b9e3ebb9128`。按第二、六、八、十章验证三人群主人只读及不计数/不推送/入口新消息的独立性。
- 任务与专项：知识仓库 `collaboration/tasks/2026/DG-20261104-003-F0.md` 的 F0-IM、`DG-20261104-003-im-server.md`；没有沿用已取代的自聊/观察 API 方案。
- 已对齐业务 `chatbot43/docs/mindlink-conversations-contract.md`，F0-BIZ v1.1，SHA-256 `b0a0b6e818b9629e871f05ac614e8551a63a492874fe5b6e91ba5ad811f2cda9`，源码基线 `0fc1926e270b3e5e03a31e348f1c3a847dc95b66`。在全文读取 v1 后核对了 v1.1 的管理代发写前约束和通知/阅读状态分离条款；候选业务路径未注册，未调用未实现接口。
- IM 源码 `master/b87fc791400dafae0f3fb2e1665863419bad58f7`；运行 Linux/amd64 image ID `sha256:30ac9c9a58866af33b3a0ef1a8c8047ba8a4402f6069e58357ef633694971367`，MySQL 8.0.46。本次未修改任何生产 Go、proto、SQL、迁移或配置。
- 使用忽略的 `.local/worktrees/f0-im` detached 工作区编写和执行探针，最后将相同校验值的新增测试文件交回项目；没有提交、推送、合并或 Git 权限变更。
- DG2-20261105-002 已交二当家审查/待主人批准；其四个准备文件、镜像归档与清单未修改。F0 使用独立目录 `.local/f0-im/20261105/`、Compose 项目 `im-f0-20261105`、全新 MySQL/日志/KV/上传卷及新租户。仅绑定回环 API 19501、导航 19502、WS 19503、后台 18590；MySQL 不发布主机端口。
- 运行前已用 Docker inspect 核对项目标签、镜像 ID、健康及映射；不是默认的本机 9001/9003，也不是 pre/pro。没有远端操作、共享库读写、已有心灵传输升级或 App/业务数据导入。

## 2. 实际测试和证据

可审查源：[simulator/probes/mindlink/probe_test.go](../simulator/probes/mindlink/probe_test.go)。测试只编译到测试程序，不进入服务端制品。默认跳过全部联网用例，不把 skip 当 passed；只有明确 opt-in 的专用回环端口可访问，不能通过环境变量改成远端。
本次测试源 SHA-256：`55cde757ccc93ae7c3d575a0618d81f1d4963e08321045003e7df4f683b126e2`。

本次运行的是原生三人普通群，三个模拟用户对应产品主人/A/B；尚无专用群/内部主体策略投影。模拟“主人”的 Token 仅为隔离用例夹具，不代表向真实分身签发 Token。双端指同一模拟主人同时建立的 **Web 与 PC 协议会话**，不能冒充真实 Web UI、App 或已发布 SDK 验收。

原始结果留在本机忽略目录 `.local/f0-im/20261105/`：

| 文件 | 内容 |
| --- | --- |
| `report.json`、`probe.log` | 当前完整原生探针：14 passed、7 failed、7 not_covered；`go test` 退出 1 |
| `restart-report.json`、`restart.log` | 独立重启后：历史 passed、会话摘要 failed；`go test` 退出 1 |
| `flush-wait.json`、`db-after-flush.txt` | 重启前明确等待超过 60 秒刷新期的 UTC 时间/间隔，以及刷新期后会话 DB 行数；提前运行的重启用例会报 blocked |
| `db-check.sql`、`db-check.txt` | 真实 MySQL 查询与返回：历史/禁言反例的逐消息 ID 核对、摘要表空行、字段宽度 |
| `report.initial.json` 等初轮记录 | 本次追加分页/重启探针前的记录，只作过程证据，不替代当前结果 |
| `fixture.private.json`、`stack.env` | 仅本机夹具身份/Token/数据库凭据，禁止贴入回报或提交 |
| `evidence.json` | 版本、文件散列、退出码、分类计数和真实 DB 数值的脱敏汇总 |

综合是 **15 passed、8 failed、7 not_covered 的能力观察格**，不是 30 个产品流程全部验收。失败来自产品要求尚未满足，未篡改断言让测试变绿；没有把查询失败/空响应当拒发。本次数据库查询成功后对原件逐一核对，得出以下结果。

独立只读审查提出两项证据质量问题，已只修探针并重跑：分页改为严格游标且逐条核对全部三个预期 ID；重启前显式等待 97.8 秒（超过 60 秒刷新期），确认 DB 仍零行后才重启。提前运行的时间闸已实际报 blocked。原生产品缺口未在本包修改或伪装成绿色。

## 3. 探针一：不计数消息

| 能力 | 状态 | 本次真实结果 |
| --- | --- | --- |
| 对照组计数与跨协议清未读 | passed | 普通存储/计数 flags=10 发送后，Web/PC 未读均为 1；清除后两端均为 0 |
| WS 不计数仍送达/存历史/更新摘要 | passed（进程内） | flags=8 和 flags=72 各发一条，两端均实收、历史查询命中同消息 ID、最后一句更新，未读保持 0 |
| HTTP is_count=false | passed（进程内） | 明确 is_storage=true/is_count=false，经签名 API 发送；历史和两个协议会话最后一句均更新，未读为 0 |
| 分页历史 | passed | 每页 1 条，直接使用上一页原始时间作为严格小于游标；三页 ID 逐条等于 HTTP、flags=72、flags=8 的预期 ID，完整且不重复 |
| 不计数与免打扰标志独立 | 已观察，非手机推送验收 | flags=8 的实收仍为 8，没有自动增加 64；flags=72 实收为 72。单独取消计数不等于设置免打扰 |
| 通过 clear_unread 推进不计数消息阅读位置 | failed | 请求携带最新不计数消息 ID/时间和现有 unread index，接口返回成功，但两会话的 LatestReadMsgId 均未推进（本轮为空） |
| 会话摘要真实持久化及重启恢复 | failed | 明确等待超过 60 秒刷新期后，本轮租户 conversations 仍为 0 行。再只重启本测试 IM，原消息历史仍能查到，但 QryConversation 返回成功且无最后一句；未把正常延迟误判成此失败 |
| “分身之间”入口红点/阅读状态同步 | not_covered | 没有真实该入口/已实现独立阅读契约；clear_unread 的失败不能算完整入口反证，更不能拿总未读为零替代红点验收 |
| App 首页/图标未读、手机响铃/通知、离线推送 | not_covered | 无真实 App 设备、APNs/FCM/厂商凭据/系统通知证据；模拟 PC 会话不代替 App |

### 已定位的调用/持久化位置

- API `services/apigateway/apis/msg.go` 的 `handleFlag` 将 is_count 与 is_storage 分开；协议位定义在 `services/commonservices/msgdefines/msgflag.go`（2=计数、8=存储、64=免打扰）。
- `services/message/services/converservice.go:174` 只在 IsCountMsg 时生成 UnreadIndex；`services/conversation/services/converservice.go:313` / `convercache.go:665` 的清未读依赖计数索引递增，因此本次对无新计数的消息不能推进 LatestReadMsgId。
- **摘要持久化缺口：**`services/conversation/services/convercache.go:338` 的 PersistConverV2 向 persistCache 写 `*models.Conversation`；同文件 init 回调（约 40 行）只接受 `*ConverPersistIndex`，类型不符即跳过 Upsert。数据库零行与隔离重启丢摘要均已实际复现；热缓存成功不构成持久化通过。
- `services/message/services/msgservice.go:382` 的 SendPush 以应用开关、push level、64 位免打扰等条件决定推送，并递增 Badge；该函数不检查计数位。这里是代码依据，**不是实际手机已收到或未收到推送的证据**。

## 4. 探针二：三人群主人禁言

先通过 `/apigateway/groups/groupmembermute/set` 对模拟主人单独禁言；直接 WS/签名 HTTP 均实际调用，发送后再查原生历史，并用 MySQL 按消息 ID 核对。

| 路径 | 状态 | ACK/历史/实际数据 |
| --- | --- | --- |
| 原始 WS：文本、图片、语音、视频、文件类型 | passed（拒发） | 五种均返回 13004；相应 ID 在历史查询不存在、MySQL 各为 0 条 |
| 签名 HTTP：上述五类型 | failed | 五种均 code=0，且原生历史命中对应 ID；MySQL 各有 1 条，不能称仅“受理未落库” |
| 原始 WS：stream flag | passed（拒发） | flags=1034 经 g_msg 返回 13004，历史/DB 为 0；仅证明该标志的群发送入口，不代替完整分段流式 SDK |
| 单人禁言＋allowlist | passed（拒发） | 设置 is_allow=1 后仍返回 13004、DB 为 0；allowlist 不是原生群主/管理员角色 |
| 原始 WS 自解禁 | failed | 主人 Token 直接 Query(group_member_mute, isMute=0) 返回 0；随后发送 ACK=0、历史命中，DB=1 |
| HTTP 正常解禁对照 | passed（恢复发送） | 先重新禁言，再经签名 HTTP 解禁，WS 发送和历史/DB=1；这不是“专用策略不可被解除”的通过证明 |
| 原生群主/管理员角色例外 | not_covered | 当前通用群模型/用例没有可核验的群主/管理员角色安装路径；不把 owner 占位用户或 allowlist 冒充该角色 |
| HTTP 群专用流式、已发布 SDK、真实 App/Web/上传控件 | not_covered | 当前仅有 private/stream/send 等路径，未获得群完整流式/真实端与上传链证据；媒体探针是消息类型通道，不是文件上传/播放验收 |
| 业务拒发＋专用 IM 策略投影 | not_covered | F1/F2 尚未实现，此处只测原生通用能力，不能据此声称产品只读已启用 |

### 已定位的调用位置

- `services/group/services/groupmsgservice.go:26` 起以 isFromApi 分支跳过成员/单人禁言检查，WS 执行这些检查；HTTP `services/apigateway/apis/msg.go:126` 返回后异步进入相同上游链。五条 HTTP 原件持久化已核实，故不能只修 UI 或在回调后删除。
- `services/connectmanager/server/imlistener.go` 的 QueryArrived 进入 Actor 路由，`services/group/actors/groupmembermuteactor.go` → `services/group/services/groupmemberservice.go:286` 更新单人禁言。本次普通成员 Token 可调用该链解除自身禁言；通用 signed API 也能更新策略。专用群需保护变更入口，不能把一次设置 mute 当永久约束。

## 5. 真实数据库核对与补项

本轮最后一个新租户的三人群 groupmembers=3；计数对照＋三条不计数消息的 g_hismsgs=4。五条禁言 HTTP 反例各 1 条；五种被拒 WS＋stream flag＋allowlist 各 0；自解禁后发送和正常解禁对照各 1。conversations=0 是**失败数据**，不是查询未成功或“没有未读所以没有会话”。重启后历史仍在、摘要不存在进一步区分了缓存/原件。

information_schema 实查 `users.user_id` 宽度 32、`groupinfos.group_id` 宽度 64；这只是当前字段宽度，未证明所有关联表/协议或 API 都允许同样长度，也未实现内部号。不得直接放入 36 位 UUID 作为用户 ID 或据单表宽度冻结全链路群 ID。早期协作简报误将群字段写为 32，已明确更正给业务负责人，最终以本次查询为准。

最小后续建议（**仅回报，等待 F1 明确派发**）：

1. 修正并测试会话持久化队列写入/消费类型一致性；要求热查询、真实 DB、重启后摘要/已读位置三层证据。
2. 为专用三人群安装不可由普通群 API/原始 Query 解除的 owner/成员/权限版本投影；HTTP/WS/媒体/流式与策略变更链在写入前共用约束。业务再独立按认证 owner/actor 拒发，不能只关某一个入口。
3. 不计数、禁 push/Badge/声音、首页过滤和“入口已读”分别形成契约。按 F0-BIZ §7 补独立 last_seen_seq/阅读确认或证明可复用原生能力，至少让不计数新消息的阅读位置跨端推进；仅保存游标，不复制消息全文。
4. App 同事提供真实首页/图标角标、手机通知与多端阅读证据；群主/管理员/上传/完整群流式路径有可核对象后补测，保持 not_covered 到实际验成。

## 6. 重现与状态约束

只允许先在新项目/数据库配置对应回环端口，检查 Docker 标签、镜像 ID、实际映射和测试夹具；不要把这些环境变量对准已有共享服务。源文件没有 target URL 覆盖项。已准备的本机隔离配置位于 `.local/f0-im/20261105/compose.yml`，与 pre 配置文件无写集重叠。

```powershell
# 先核验 im-f0-20261105 项目及其独立卷/镜像；不是部署到 pre。
docker inspect im-f0-20261105-im-server-1 --format '{{.Image}} {{index .Config.Labels "com.docker.compose.project"}} {{.State.Health.Status}}'
$env:JIM_F0_ISOLATED='im-f0-20261105'
$env:JIM_F0_REPORT='D:/ClaudeCodeProjects/im-server/.local/f0-im/20261105/report.json'
go test ./simulator/probes/mindlink -run '^TestMindlinkNativeProbes$' -count=1 -v
# 基线预期退出 1：缺口必须保留为红例；每次显式运行会创建新隔离租户。
Remove-Item Env:JIM_F0_ISOLATED
# 不可立即重启：会话持久化队列最大刷新期为 60 秒。
# 保留 JSON 原始 UTC 字符串，避免 PowerShell 隐式本地日期转换。
$raw=Get-Content .local/f0-im/20261105/report.json -Raw
$stamp=[regex]::Match($raw,'"timestamp":\s*"([^"]+)"').Groups[1].Value
while (([DateTimeOffset]::UtcNow-[DateTimeOffset]::Parse($stamp)).TotalSeconds -lt 70) {
    Start-Sleep -Seconds 2
}
# 在原测试租户中反查 conversations 并保留结果，区分刷新失败与未等刷新。
# 本次实际查询为 0 行，见 db-after-flush.txt，随后才重启。
docker compose --env-file .local/f0-im/20261105/stack.env -p im-f0-20261105 -f .local/f0-im/20261105/compose.yml restart im-server
# 等 health 恢复后，仅用上一轮私有夹具读取该租户原件与摘要。
$env:JIM_F0_RESTARTED='im-f0-20261105'
go test ./simulator/probes/mindlink -run '^TestRestartedConversationSummary$' -count=1 -v
Remove-Item Env:JIM_F0_RESTARTED,Env:JIM_F0_REPORT
```

无 opt-in 时 `go test ./simulator/probes/mindlink` 只验证可编译并报告 skip，不是两项探针通过。完整带 opt-in 的原生探针和重启探针本次均已真跑且退出 1；原因、原始数据和未覆盖项如上。本次未进入 F1 修复、迁移、预发布、已上线状态，也未启用任何能力位。

本次 F0 两容器已停止，独立卷/结果保留；原本机 IM/MySQL 仍 healthy 且启动时间未变（11 月 2 日）。停止 F0 IM 时沿既有 run.sh 出现退出 137，本次没有借此改动部署准备或修优雅停机。pre 的四文件 SHA 与封包清单逐项一致；本次新探针不在那个已封包版本内。

## 7. 后续依赖通知核对（2026-11-05，仅文档/源码核验）

对应大当家通知 `pbxp48vpl2jhukeh`、当前批次 `d7b499a2dc3b8afe111250910532f1e150a8f3a4fd0813eb5dbfd72205b8ff3f`。通知引用 F0-BIZ v1、SHA `307c97407f85eb7ff4bca724b1de424b6b6b304ae8d00af6a11ef12c46a789b8`；本批实际读取文件已为 §1 记录的 **v1.1 / b0a0…cda9**，未取得该 SHA 的 v1 原件，不冒称本批核验了 v1。版本差异已回报大当家（`pbxq5hq6u9ehukeh`）。本节只新增源码边界核对，§2—5 的运行结果仍是此前隔离探针证据，**本批未重跑测试、未新增 DB 结果**，15/8/7 分类不变。

| 依赖 | 本批核对与尚未满足的门 |
| --- | --- |
| 主体长度与内部号 | 此前实查 users.user_id=32、groupinfos.group_id=64；源码初始化 SQL 的 groupinfoexts.group_id 仍为32（不是本批 DB 实查）。不能用单表宽度冻结全链路上限、截断36位UUID或由昵称派生号。原生注册 API 返回 Token；内部主体映射、对外不可寻址与不向客户端外发 Token 仍需业务/IM专用约束，不把普通注册当已实现内部号 |
| 建群与幂等 | `/groups/add`、`/groups/members/add` 共用 GroupAddMembers → AddGroupMembers，外部提供 group_id；按租户/群ID查询已有群、过滤已有成员，成员 DAO 使用 insert ignore。这只是普通群按相同ID重用的代码依据，**没有** owner＋规范排序X/Y的pair唯一映射/原发送意图/权限投影提交屏障，也未做并发、进程重启、结果未知或失败恢复验证，均记 not_covered。源码创建群错误仅影响缓存分支、成员批量写错误不改变末尾 success，因此不能单凭 code=0 证明建群和三成员已持久化；本批未故障注入，不声称已复现该错误分支 |
| 持久化结果查询 | HTTP发送先生成/返回消息ID，再异步发送；SaveHistoryMsg 无回执异步发 add_hismsg，群事件订阅在历史提交前触发。不能把受理/事件回调当 persisted。历史查询可核对具体消息；可靠的请求键→持久消息结果及回调重放对账尚未验证。ClientUid 去重为进程内10分钟LRU，不能替代跨重启持久幂等 |
| 拒发与未读 | 沿用 §3—5 的真实反例及 not_covered；is_count=false 与保存、摘要、禁推送/声音/Badge、首页过滤及入口阅读分别验收。原生clear_unread不足，业务last_seen_seq仅保存阅读位置是候选最小补项，尚未实现/多端验收 |
| 业务/API/资产边界 | 候选HTTP路径不是已注册路由；pair只在服务端主动send首次实际发送时内部懒建，不公开ensure、不因主人观察而建群。inform资产授权绑定实际收件avatar/generation及原消息资产集合，接待不是统一授权主体。这些是本批确认的业务语义输入，不是原生IM已提供的能力 |

代码定位：[建群入口](../services/apigateway/apis/group.go)、[建群/成员服务](../services/group/services/groupmemberservice.go)、[成员写入](../services/group/dbs/groupmemberdao.go)、[初始化表定义](../sql/imserver.sql)、[历史入队](../services/commonservices/historyservice.go)、[消息事件](../services/commonservices/eventsubservice.go)、[进程内去重](../services/commonservices/msgfilter.go)。本批只改本报告，没有替业务仓改文件、调用候选接口、启动容器、实施F1或操作pre/pro；能力门维持未通过，等待大当家后续明确派发。

## 8. F1-PERSIST 接单与接口跟进（仅进度，不是修复验收）

本次有效跟进 `pbx5gyjf6nehukeh` / 批次 `042dc656fc993377b7fb0edb47a7dfc635fed821650c85b53dcd8bdd15adcb06` 要求核实原单，未重复派单。负责人已通过 `pbx5jg676nshukeh` **明确接单原 F1-PERSIST，进入实施前核对**；不将原发送回执 `pbxqdf98l3shukeh` 冒称为此前已接单证据。协调账本由大当家维护，本报告不替其改写任务状态。

本批实际核对：HEAD 仍为 `b87fc791400dafae0f3fb2e1665863419bad58f7`，无已跟踪源码差异；本机登记工作区为主仓及原 F0 detached 工作区，未见 F1 专用工作区。`PersistConverV2` 仍入队 `*models.Conversation`，消费回调仍断言 `*ConverPersistIndex`。**本批未发现本机修复实现、修复镜像或新回归证据，未重跑服务/DB**；不据此断言其他机器不存在成果。此前 F0 15/8/7、两次 exit1 仍只是原版本历史观察。

原单计划写集限于 `services/conversation/services/convercache.go`、相关会话持久化回归测试（优先现有 `convercache_test.go`）、必要的 `simulator/probes/mindlink/probe_test.go` 及本报告/本地证据；本批实际仅改本报告。进一步实施仍须红例、真实队列消费/DB、正常更新/删除不复活、租户隔离、错误处理、仅重启 IM 后摘要及合法阅读状态恢复、新制品活体验证和独立审查。**现有探针失败是修复输入，不是禁止开始修复的前置门；摘要修复不等待下表业务接口冻结。** 不修改二当家部署准备、历史封包、pre/pro；原单通过也不产生部署授权。

接口对齐已发送服务端负责人 `01a1a0e1-3618-78b0-a5c7-81a37bff3a31`，回执 `pbx5jhhkwnwhukeh`。唯一契约仍在 `chatbot43/docs/mindlink-conversations-contract.md`；本批文件仍标 v1.1，但实际 SHA 为 `fba8f306e6e96df1c6c9b562c881472fc045116a1af481b74b29e3c9e011ea3b`，不同于 §1 的历史 `b0a0…cda9`。已要求负责人确认当前版本及变更范围，不能假定仅为换行。下表是**待共同冻结的接口登记要求**，不是新增/已上线路由；由服务端负责人在唯一契约补齐具体 IM 路由/RPC、proto编号、错误数字映射及恢复协议，不在本报告另建平行契约。

| 边界、生产者 → 消费者 | 现有实现与待冻结内容 / 后续阻塞 |
| --- | --- |
| 内部主体：业务映射/生命周期 → IM受限主体、业务会话 | 现 `/users/register` 返回普通用户 Token；尚无已冻结的专用 ensure/retire 路由。需 env/tenant/owner/avatar/generation、operation_id → opaque IM ID/状态；最窄长度、唯一性、注销不可复用、不可外寻址/不向端发Token，以及结果未知沿原键查询的规则 |
| pair：业务发送意图 → IM固定成员/权限投影 | 现 `/groups/add` / `GroupMembersReq` 不表达 owner＋排序X/Y唯一键及policy_version。需内部懒建操作、同键异参与者冲突、并发唯一、部分建群/成员投影失败恢复；pair ensure不公开、不因观察建群 |
| 提交屏障：业务可信actor/权限版本 → IM实际持久化点 | 现 `UpMsg`/普通mute及isFromApi不能承担该契约。需冻结真实原actor/generation、请求键、固定成员、policy_version/权限版本和串行提交屏障；过期/未就绪写前拒绝且零历史，HTTP/WS及解禁/扩员不能绕过。具体RPC/proto编号及拒绝码未冻结；业务候选reason不能冒充已注册IM数字码 |
| 持久发送结果：IM真实存储提交 → 业务意图/outbox/reply结算 | 现 HTTP 消息ID、历史异步入队和事件订阅不足。需 request_id＋actor＋摘要绑定最终msg_id/seq/time及accepted/persisted/unknown/rejected；明确由谁在真正存储提交后生产结果、谁查询消费、重复回调幂等及失败重查；unknown不能换键重发 |
| 阅读与通知：业务阅读确认 → 多端入口、页面计数/通知消费者 | 原生clear_unread不足。需业务last_seen_seq展示后单调推进、API/字段/错误/范围校验和重试；与inform_ack、execute完成分离。pair存储/摘要、禁push/Badge/声音/首页过滤各自验收，不能由is_count=false推导 |

以上是后续专用能力的接口缺口，不扩为本 F1-PERSIST 写集。服务端接单/具体接口回报尚未收到，不把“已发送”当已对齐；原生主体边界、可靠持久幂等和权限屏障仍未冻结。完整宿主联调按大当家本次要求使用另一台物理 Win11，不等待同机共存改造；本批未操作第二台机器，也没有端到端验收通过结论。

## 9. F1-PERSIST 实现与隔离验收（2026-11-06）

授权沿 `DG-20261104-003-F1-PERSIST.md`，既有 `ClearTotalUnreadV2` 必要调用方扩展已回报 `pbx8fxkue5qhukeh` 并登记原单。实现基线仍为 `b87fc791400dafae0f3fb2e1665863419bad58f7`；在原隔离 worktree 实施、核对主工作区未被他人改动后，将同 SHA 的源码/测试交回主工作区。本节实施和验收阶段没有提交、推送、合并、迁移、pre/pro 部署或能力位启用；后续主人授权的本地提交见§10，§1—8 的旧版本结果不追溯改变。

修复沿既有延迟刷新机制：队列通知消费最新的冻结快照，深拷贝扩展映射，按租户/用户串行提交，成功后只移除同一快照；暂时、资源和未知存储错误重试，明确无效数据错误记录并丢弃该坏快照。worker 配额满时非阻塞重排，避免逐出回调持全局锁等待 worker。用户缓存键及队列键使用无歧义编码。缓存重载在同一提交锁下读取 DB、合并尚未写入的最新快照再建索引，防止旧 DB 行覆盖更新或删除状态。

既有“全部已读”先执行并检查 SQL，再修改缓存、将清零后的状态入队；与 worker 共用用户提交顺序，覆盖排队中、正在 SQL 写入以及缓存逐出/重载的旧快照。SQL 失败返回错误，缓存不提前清零、调用方不发送成功通知。没有新增阅读接口，也没有使不计数消息的 `clear_unread` 获得独立阅读游标语义。

### 9.1 对应源码与制品

| 文件/制品 | SHA-256 |
| --- | --- |
| `services/conversation/services/convercache.go` | `d1638c461d2d3b200e4e36f139744bd0fd72605ab32b3e4062455685d3edeac5` |
| `services/conversation/services/converservice.go` | `38979c8fe471fe4a93d292f52cdac8720c481214cafeab870e9fb36b6fa5f37a` |
| `services/conversation/services/converpersist_integration_test.go` | `bc50ec9c9be4a284c8b60ff5c67e321f2ce99ab1e3a68723ae6413d41abbe713` |
| `simulator/probes/mindlink/probe_test.go` | `f3cbd3533d2171c7ebf14e31120c419e7eb9adaf00a4e794804f7d255b0b5b0c` |
| `simulator/probes/mindlink/persist_runtime_test.go` | `a427465860848aa80af1f44f9db767da21289e132e83d9442b00f470ee423fa2` |
| Linux/amd64 镜像 `bitcomputing/im-server:f1-persist-d1638c461d2d` | image ID `b9a884df3e36714c37c691f3f97dfaa6adefec1ab95b303466e0dbb63b64f944` |
| 新构建、镜像内及重启后活体 `/opt/imserver` | `a052cece75aa144c7f3f44f475c44de1d6c3b47570c5850f8e108455335d9c51` |

服务端构建 Go1.25.12/Linux，协议客户端 Go1.26.1/Windows。MySQL 实查 8.0.46；复用归属明确的 `im-f0-20261105` 独立栈，loopback 19501/19502/19503/18590，未连接默认共享 IM。旧 `a406…` 镜像及三消息测试是全清修复前的局部证据，保留在 `.local/f1-persist/initial/before-clear-all-fix/`，不计为当前版本通过。

### 9.2 实际执行与证据

下述原件在忽略目录 `.local/f1-persist/initial/`；含凭据的 `runtime-config.yml`、`runtime-private.json`、F0 `stack.env` 和 `fixture.private.json` 不进入 patch/交付记录。

| 检查 | 实际结果 / 日志 |
| --- | --- |
| 原类型队列、键碰撞、坏数据阻塞、资源错误误丢 | 分别先复现失败，见 `queue-red.log`、`tenant-key-red.log`、`user-cache-red.log`、`permanent-error-red.log`、`resource-error-red.log`；未把失败记成通过 |
| 全清覆盖排队快照、逐出缺席快照、重载旧 DB 覆盖新快照 | 实际 DB 红例 exit1：`clear-all-red.log`、`clear-all-evicted-red.log`、`clear-all-stale-reload-red.log`；修复后进入完整绿例 |
| `go test ./services/conversation/... -count=1 -v -timeout 60s` | exit0，`queue-regressions-green-v10.log`；真实 MySQL 队列消费、更新/删除、租户、冻结值、交错入队、受控满配额重排、存储重试、全清排队/在途/SQL失败/逐出/旧DB重载均通过 |
| `go vet ./services/conversation/... ./simulator/probes/mindlink`、`go build -o /evidence/image/imserver ./launcher` | 均 exit0，`vet-v6.log`、`build-v6.log`；gofmt、diff 空白检查通过 |
| `TestConversationStateBeforeRestart` / `TestConversationStateAfterRestart` | 均 exit0，`runtime-before-v3.log`、`runtime-after-v2.log`；新制品两群四消息、双端合法阅读/全清、重启恢复 |
| 最新版原生产品探针 / 重启探针 | `.local/f0-im/f1-persist-final/` 中 `probe.log` exit1、`restart.log` exit0；合计 **16 passed / 7 failed / 7 not_covered**，并非完整产品通过 |

活体验收租户 `f17e117fba6bf4`，两群 `f1-state-9aac9e8670cc` / `f1-allread-1e20151c38db`，各三成员。第一群计数消息 `pbx8t9c7jacm2hs3`，两条不计数消息 `pbx8t9eaaagm2hs3` / `pbx8t9fc2ajm2hs3`；第二群全清控制消息 `pbx8t9gpsaljmadh`。第一群 Web/PC 实收不计数消息、历史可查且最后一句更新，已有合法 read ID 仍为计数消息、index=1、unread=0；第二群先实际 unread=1，执行 `clear_total_unread` 后双端 unread=0/index=1。

等待实际刷新后 DB 验得三用户、两群各三成员、六会话行、四消息各一份，主人两会话 readIndex=unreadIndex=1。实际等待运行夹具187.146秒、原生夹具126.919秒（`runtime-flush-wait.json`），均超过60秒刷新期。只重启 IM：启动时间从 `2026-11-06T06:54:23.472721312Z` 变为 `06:58:28.106366958Z`，MySQL 始终为 `03:14:19.976337985Z`；`runtime-db-before.txt` 与 `runtime-db-after.txt` 行内容完全一致。重连核到四条原历史、最新摘要、合法 read ID/index 和全清状态，`runtime-after.json` 的四消息及 all_read 均 passed。

普通 `fillConvers` 本来不返回 ReadMsgId；新探针使用现有、当前用户范围内的 `QryConverReq.IsInner=true` 元数据并交叉核 DB，同时继续验证普通摘要/索引/未读。该观察修正没有改变产品断言；旧不计数阅读游标依然失败。初次普通查询观察错误导致失败的租户 `f119333b69a7f1` 已反查残留并保留，见 `failed-runtime-attempt-residual.txt`；不是未执行。后续各创建意图有 `runtime-attempt-*.json`，测试数据与卷保留。

### 9.3 独立审查与剩余边界

非实施者 `review_f1_final` 核对 v10 源码及真实红绿日志，明确关闭全清常规路径与两条缓存逐出残余缺陷；检查了 commit→user 锁顺序、调用方、不可变快照、SQL 失败返回及索引重建。随后已核最新版两群四消息/IM单独重启原件，并复核主工作区、隔离工作树、manifest五文件同SHA及补丁/日志/镜像一致性，无剩余代码或材料阻断。审查者没有独立复跑DB、race或压力测试；大当家的项目验收与发布仍分别记录，不由本报告代替。

当前 7 红项仍为不计数阅读游标不足、5种签名 HTTP 禁言绕过和 WS 自解禁；对应实际 DB 计数见 `native-db.txt`。App/手机 push、Badge/声音、真实页面、完整群流式/发布 SDK 和业务策略投影仍未覆盖。没有 race detector（当前构建环境无 gcc）或压力测试结果；32配额例是受控占满配额，并非32个真实阻塞SQL。

延迟刷新的 dirty 快照在成功落库前仍只在内存；突然崩溃可能丢失未刷新摘要。本包证明的是实际刷新后的恢复，不声称新增跨崩溃日志或可靠发送事务。明确无效数据错误不会伪装成成功持久化；未知/容量错误保留重试，但没有总 dirty 内存上限保证。

F2-A 映射/操作意图子集已以回执 `pbx7y2nyeyshukeh`、`pbx8palmn64hukeh` 确认；读取唯一契约提案B时 SHA 为 `ead66d3012f1e046b3aaad8857966f94dfd4eba32fea766c07ebe35b889d492a`，真实适配器仍关闭。POLICY/RELIABLE/READ 按串行写集继续，正式接口/认证/事务结果及真实 latest_seq 未因本包自动通过；§11.4 精确票据项已回报 `pbx8uec7n7chukeh`，待双方确认。独立物理 Win11 完整宿主联调仍未执行。

本包收尾已停止所属 IM、MySQL 和 SDK runner，容器/卷/两轮夹具保留，见 `resources-final.txt`、`retained-mounts.txt`、`task-containers-final.txt`。原共享 `juggleim-server` / `juggleim-mysql` 此刻仍 healthy、启动时间仍为11月2日；这是本次快照，不是持续运行保证。预发布文档及三份既有归档SHA与实施前一致。该修复制品尚未被选择为新部署候选；不可自动替换旧封包。

## 10. 完成单元本地提交（2026-11-06）

主人新要求“本项目的某次改动或某个功能完成后就提交下改动文件”，已写入项目 `AGENTS.md` 并独立提交为 `c2edcd5`。PERSIST 按完成单元独立提交生产两文件、三测试文件及本报告；提交编号以 Git 日志为准。部署准备文件和私有证据不纳入该提交，推送、合并、部署仍按各自授权。

提交前实查五份源码/测试仍逐字节匹配§9已审版本，并重新在 Go1.25.12/Linux 及所属真实 MySQL 上执行 `go test ./services/conversation/... -count=1 -v -timeout 60s` 和 `go vet ./services/conversation/... ./simulator/probes/mindlink`，均 exit0，见 `.local/f1-persist/initial/queue-regressions-precommit-v11.log` / `vet-precommit-v11.log`。该记录是本轮新增回归，不把历史测试当作本次新跑；§9活体镜像/刷新/重启流程属于此前同SHA版本证据，本轮未重跑。原生 Windows 测试本轮因既有 metrics 的 SwapTotal/SwapUsed/SwapFree 字段编译问题 exit1，未计为通过。

本轮最初 Docker 引擎不可达，经正常启动 Docker Desktop 后恢复；只为回归显式启动所属测试 MySQL 和 SDK runner，验后停止并保留数据。此前对大当家交付的报告 SHA `f315e9c9945b739e1bfe99148f9b3300897fc3f942d063da9cba5c5cd2039246` 对应快照另存 `.local/f1-persist/initial/reported-v10.md`，旧证据不与本节新增提交记录混版。

## 11. READ 并行交接（2026-11-06）

大当家有效派单 `pbya2kwwflghukeh` 将原 F1-READ 交给同职责“43chat im server二管家” `01a1b020-5d93-76c3-baf2-730ccf75405b`；原管理 `01a1a0e1-b344-7501-bc44-fadc4a29d403` 保留 POLICY、RELIABLE、Rocky 与共享源整合。知识仓库的 `DG-20261104-003-F1-READ.md` 是本次分工入口，不构成永久岗位层级或新增 Git/部署权限。

本轮实查主仓 HEAD 为 `4ecec887fb38d2d81af4bf2773b05d304e8ced97`，tracked/staged 无差异，未跟踪 `deploy/`、`docs/pre-deployment_zh.md` 属旧部署准备，保留。主仓没有未提交 READ 实现。旧 `.local/worktrees/f0-im` 仍是 detached b87，带 PERSIST 的两生产文件改动及未跟踪测试；五份源码/测试 SHA 均与已提交 4ece 相同，仅保留为旧测试副本，不交二管家接管或清理。

交接清单已发送 `pbya6d887mchukeh`，二管家明确回复 `pbya7k4pfm4hukeh` 接管基线、写集、独立 proto/迁移和共享整合安排；其回执作为本次交接问答的结果核对，不把历史消息当新任务重放。后续确认 `pbya9sqkfnlhukeh` 纠正契约快照并说明接口依赖。接管确认不是实现或验收通过。

| 文件集合 | 本包唯一写入人 / 边界 |
| --- | --- |
| `services/conversation/starter.go`、`actors/**`、`convercallers/**`、`services/**`、`storages/**` | 二管家按 READ 必要范围实施，包含 convercache/converservice 及相关测试；原管理不并写该模块，跨包接口变动先协调 |
| `simulator/probes/mindlinkread/**`、`docs/evidence/20261106-mindlink-read/**` | 二管家的独立探针和非敏感证据；凭据与原始私有数据留忽略目录 |
| `commons/pbdefines/mindlinkread.proto`、`pbobjs/mindlinkread.pb.go` | 二管家独立新增；只修改这套生成件，不顺手重生成公共 appmessages 文件 |
| `services/historymsg/storages/dbs/convercleantimedao.go`、新增 `convercleantimedao_test.go` | READ 必要配套临时交二管家独占，限 SQL 条件空格修正及真实 MySQL clean-time/过滤回归；原管理不并写，其他 history 文件仍须另行协调 |
| `commons/dbcommons/sqls/20261106.sql` | 当前独占预留给 READ，尚无已发布迁移；其他包正式编号按实际整合顺序集中协调，若顺序改变先协调重分号并真跑新库/升级，防止高版本先落地而漏掉 READ；已发布脚本不改号 |
| `appmessages.proto` / `pbobjs/appmessages.pb.go`、`commons/errs/errorcode.go`、`services/apigateway/routers/router.go`、`sql/imserver.sql`、`commons/dbcommons/dbmigration.go` 及公共迁移登记 | 原管理集中整合；二管家提供最小接口/差异，不并写。接收已提交交付物后按适用授权处理合入，不自动扩大 merge/push/deploy 权限 |
| 原 `simulator/probes/mindlink/probe_test.go`、`persist_runtime_test.go` 与本报告 | 原管理维护 F0/PERSIST 基线及总报告；READ 不覆盖旧失败或历史证据 |

二管家从 4ece 建独立 worktree/分支，使用自己的 IM/DB、端口和卷，避开旧 F0、共享 IM、Rocky、pre/pro 与原封包。建议先核查 19601/19602/19603/18690；端口仍须启动前确认，不据此承诺持续空闲。本交接不依赖 POLICY/RELIABLE 全包完成，独立红例、存储和普通会话回归可先推进。

已有公开 `Conversation` 字段 1—22、`QryConverReq` 1—5、`ClearUnreadReq` 1—2 未为 READ 改动或占号。二管家提出独立 `qry_conver_seen` / `set_conver_seen`，按当前用户＋租户＋会话持久 max 合并，首页 clear 不推进 seen。已回最小候选编号表：`QryConverSeenReq` 的 target_id/channel_type/sub_channel 为 1/2/3；`SetConverSeenReq` 同 1—3、seen_seq 为4；`ConverSeenState` 的 seen_seq/latest_seq/has_new 为1/2/3。该表仍是正式源登记提案，未生成、未注册或对外生效；字段/命名差异由二管家回报后共同冻结。当前用户与租户须来自真实登录或已验证服务端授权上下文，不能取请求中的 userId/IsInner 作为可信身份。

唯一业务契约本轮已更新为提案 **2026-11-06-C**，SHA `0a117252225b049c15e1b24160ed8d4eb2ab1da44631070a1328e80eff2a0480`；早先交接消息中的 B/EAD66 只代表当时快照。C 已采纳票据精确项，§11.3 的可靠 latest_seq 和业务 seen 接线仍待双方确认。二管家的原生 seen 方案需与业务适配及单一权威存储对齐；原管理集中登记 IM HTTP/授权接线，业务负责人维护唯一契约。分工及依赖已定向通知业务负责人，不能把发送回执当接口冻结。

`DownMsg.MsgSeqNo=6` 与 `unreadIndex=23` 是不同字段。现有序列能否直接复用，须由二管家按同 scope、授权可见历史、真正落库、新旧库与重启证据证明；不假设 RELIABLE 已实现，不把 DB 错误或范围未知记为 latest=0 成功。该关联验收尚未通过，App/push 等仍按实际覆盖单列。本节只交接写权和依赖，本轮未实施或运行 READ 产品测试。

必要调用方范围补充（请求 `pbybhndrxp4hukeh`）：原管理本轮只读核对 DAO 的 `conver_id=?and sub_channel=?`、调用链及二管家 `service-debug2.log` 中 MySQL 1064、query 10009、测试 FAIL；主仓无该 DAO 未提交改动，新增测试文件尚不存在。已将表中两文件释放给二管家补最小修复与真实 DB 红绿例，保留租户/会话/子通道/类型过滤和参数化。旧 `GetCleanTime` 忽略查询错误，不得据此使 READ 降级成功；其服务层及其他 history 文件不包含在本次释放中。本次是写集协调和日志核对，原管理未独立复跑、未修改 DAO，不声明缺陷已修复或回归通过；修复交付与审查另记。

## 12. READ 候选接收与阻断项（2026-11-06）

本次有效批次 `4ccd193bc2a6d5f7584ff6fe9669e29820876c9555109186bb2cc5b97de9d408` 接收二管家的候选 `008bd1fe7e116f006da95cdbf10edfbac47de5db`，前置 clean-time 修复为 `584f9032b8759f63be627c95b5b2336f69f7c4f7`，基线为 4ece。本轮核对两个提交及调用链；15 个变更文件符合上述写集，公共路由、错误码、appmessages 和新库初始化 SQL 未改。

候选已登记独立的 `qry_conver_seen` / `set_conver_seen` Actor、§11 的 proto 编号及 `20261106.sql` 的 `conversation_seen` 表。原管理核对了提交内 manifest 的 12 份源码和 18 份私有证据摘要：11 份源码直接匹配；clean-time DAO 的 manifest 使用工作区 CRLF 字节，转为 Git blob 的 LF 后内容一致；18 份证据摘要均匹配。原件中的 Linux/CI、双端、DB 与重启结果是二管家本轮保留的运行记录，原管理只核对原件及摘要，本轮未独立重跑，不将其提升为全部验收通过。

候选接收期间，二管家分支已出现后继修订，不能把旧候选当最终交付：

- `80bdd8728a16a84208eccce590242bd7ed669551` 补 Mongo 清理记录缺失的判定。旧候选只接受 GORM 的 not-found，而 Mongo 返回 `mongo.ErrNoDocuments`，会把正常空记录报为内部失败；二管家已保留对应红例。本轮未跑真实 Mongo 流程。
- `e3957ea` 新增真实历史清理后的红例；其 `retention-red.log` 记录 IM 重启后新消息 `latest=1, seen=3, has_new=false`，用例 exit 1。现有 `commonservices/convercache.GetLatestMsgTimeSeq` 从剩余历史恢复序列，历史物理清空后从零重新编号，与持久 max 阅读位置冲突。该红例不能算作通过，禁止仅为消除现象而回退 seen；共享序列生产链仍由原管理协调，二管家未获本节之外的新写权。

本次结论：保留 READ 候选，暂不接线或合并。先解决序列复用并取得对应版本的回归、独立审查和业务 §11.3 单一权威存储回执，再由原管理将相同 DDL 整合新库初始化、核对升级顺序并实施已认证 HTTP 适配。主人登录态的业务 `/api/personal/agent-conversations/:conversation_id/seen` 与 IM 原生 RPC 是不同入口；当前未注册的路由和认证不能因候选存在而记为可用。本轮主仓只更新此记录，未修改 schema、运行服务、切换 Rocky 或发布。

## 13. 共享序列依赖交付（2026-11-06）

本次有效批次 `f15ae250f146fed63e92f1ca610f30f95b11209e227f24d99366ca97a3d344f7` 由二管家请求明确共享链分工或交付修复依赖。原管理承担修复，保留二管家的 READ 写集，未转交公共生成/发送文件写权。已在独立分支 `fix/mindlink-sequence-watermark` 提交 `33d2b9bb1a8aad82d95ed25d890476e9496d5546`（base 8889b39），尚未合并主仓；本节是主仓交接记录，不表示主仓运行代码已经含修复。

存储私聊/群聊在发送副作用前事务预留独立 `message_sequence` 序列/时间水位，历史清理不删除该元数据。`20261107.sql` 在服务启动前取历史 MAX/既有 READ 水位，保留二进制 scope 与较大旧值；依赖包包含原样 `20261106.sql` 和新库初始化，不能运行缺少这两个前置的更高编号。六个生成器调用方均处理错误；其他通道原有行为不变。独立审查发现迁移会越过失败脚本或版本写失败，已用真实 MySQL 红绿例补停止/返回错误，启动器在失败时 exit 1。预留可以产生空号，水位不是历史落库结果，异步 HTTP 受理/幂等/outbox 的原缺口仍在。

原管理本轮实际执行：旧清理冷启红例 4→1、修复后转绿；MySQL 24 路分配、租户/会话/私聊双方/子通道/类型边界、失败回滚和耗尽；迁移种子与错误、新旧库结构、严格 SQL 模式、已有哨兵；指定 CI 包、受影响 vet 与 Linux 构建。最终活体私聊/群聊 ACK、Web/PC 实收和历史 ID 一致，清尽历史后仅 IM 重启，新序列 2 大于未改动的 seen=1；前后水位转储相同，MySQL 未在该重启窗口重启。故障注入拒绝两类发送，一秒观察窗无新历史、inbox/sendbox 或实收，故障表残留为零。实际错误版本启动 exit 1。上述适用检查均实际返回预期结果；无测试的包不算运行覆盖。

最终镜像 `sha256:3827b96e8a23b2c927f1e9dd2a7c39173ab465c0378335e31ca035161ce90090`，磁盘及重启前后活体二进制 SHA `a4458bccbfd2e05d0a2237fc7adf2757134950575ab33a87b1b7e74505af52fb`。`git show 33d2b9b:docs/evidence/20261106-message-sequence/README.md` 提供非敏感报告，manifest 的 16 份源码已与提交暂存 blob 逐字节核对，24 份原件摘要保留在同目录。独立静态复核在 MySQL/停止旧写者范围未余阻断；审查者未独立跑 DB/活体，Windows 编译仍受既有 metrics 字段错误影响。

依赖已通过原批次回传二管家（发送回执 `pbycwwhfxy8hukeh`），待其在隔离分支整合确切版本、重建并跑 e3957ea、完整适用 READ 回归及独立复审；发送成功不算其已接单或 READ 验收。业务 §11.3/真实 App/页面/push、Mongo 物理清理启动种子、混合旧写者与生产负载未验。自有测试栈已停止，试验租户/库/卷/制品保留；未动既有开发服务、READ 栈或 Rocky，未推送或发布。

交付自检补充：原交付未跑浏览器，不把该缺口记为通过。本轮使用真实 Chrome 登录隔离管理后台，通过现有 API 调试页实际发送私聊/群聊并查询历史，两个新 ID 分别为 `pbyc3b68aachlzrr` / `pbyc3tys2agkbg4m`；页面历史均命中，DB 各恰一行、seq/水位为 3、seen 仍为 1。现有历史 HTTP 响应不含序列字段，序列证据来自 DB，不能外推为 READ 新页面通过。代码 33d2b9b 未变，原 16 源码及 24 证据摘要复核一致；新增非敏感页面/DB 证据提交为独立分支 `11aa1ac222aaeef50b652cb69cc6715d8811eaff`，见其中 `docs/evidence/20261106-message-sequence/ui-selfcheck.md`。启动时的 DB 未就绪失败及一次视口导航超时保留为失败尝试，条件修正后才继续；未重复发送未知结果。浏览器关闭且原 PID 不存在，故障表为零，自有 IM/MySQL 已停、数据卷保留；均为本轮快照，不承诺持续状态。业务 READ 页面、真实 App、Mongo 等验收门维持待验证。

大当家协调批次 `bfafdeabf509b5fbdb5490e911d705f9958ba5692983df1e11a20c902f580a4b` 去重回核：同一原任务不重复实施。原管理继续独占共享生成/发送链、持久序列、公共迁移/初始化与后续 HTTP/认证整合；二管家负责既有 READ 模块、retention 回归及独立复审。本批实查二管家分支 HEAD 为 `7ccc6e96d09601e1cb19dffd5608907fa0a7feee`，虽不是 33d2b9b 的 Git 后代，但 16 份依赖源码 blob 全部与 33d2b9b 清单一致，证明修复已整合进其源码，不能误记为仍未接依赖。当时仅 retention 探针有未提交加强断言，尚不能据此声明 has_new 红例已转绿或新版本验收。本批只核对当前提交、写集和材料，未重跑前次产品测试；前述实测只作其对应版本的已有证据。

本批核读唯一契约仍为 SHA `0a117252225b049c15e1b24160ed8d4eb2ab1da44631070a1328e80eff2a0480`，§11.3 尚未明确原生/业务游标的单一权威适配。已定向请求业务负责人独占更新候选边界（发送 `pbyc7z4zp2yhukeh`）：IM `conversation_seen` 是唯一可写阅读权威，业务只做认证 owner/conversation 映射与适配；latest 取同授权可见且实际持久的 MsgSeqNo，绝非预留水位或 unreadIndex，允许空号且不能以差值当计数。消息已发送不是契约冻结；业务回执/新 SHA 与二管家的整合活体、e3957ea、完整适用回归及独立复审仍是后续依赖，不扩合并/发布或部署权限。

候选取舍回执（批次 `49c30a4e654eec6204a68314e254e69291d24cf35da7ecb0e0bc7b86878e3ca1`）：二管家早先提出仅在两个发送入口生成后预留 floor 并覆盖 MsgSeqNo，或释放最小写集。该同任务候选到达时，33d2b9b 已实现且其分支已整合；本批核读生成器、事务 helper 和六调用方，决定沿用现方案，由原管理唯一维护共享链/表/helper，不追加第二序列源或转移写权。选择共享生成器同时返回持久序列和时间，并在事务成功后更新缓存，使调用点无需再分别协调覆盖后的缓存与时间水位；六调用方是内部错误签名适配，存储预留只作用于私聊/群聊，其他通道沿原分支，旧协议字段不变。READ 的 20261106 保持原样，独立水位使用 20261107，已应用旧编号的库不靠重写 006 获得新表。本批实查 READ HEAD 已到 `a35c8779c43d23e42cd6d49fc295f454abf6f2ff`（新增双端保留清理测试），仍以其后续真实运行/复审回报裁定整合验收；本批未改生产源、未重跑旧产品用例。

## 14. 真群与私聊改造状态快照（2026-11-06 07:09 -05:00）

按大当家状态查询批次 `644e52c92930b5b4f9e4d0d52f01c647dd347f0a9ad211e4233b6b6b56401c00` 实查源码、分支、工作区、指定契约与材料。本次只查状态，未运行新的产品测试、未启动开发或部署；下表实测均指各自明确版本的既有证据，不把旧 PERSIST 或旧 READ 绿例作为新进度。

| 项目 | 实现状态 | 验证、联调与剩余门 |
| --- | --- | --- |
| POLICY | **代码实施未开始，未实现。** §11.4 actor 票据技术项已在契约 C 确认；专用 principal、固定成员/权限版本投影、受限 Token 及 HTTP/WS 共用提交屏障尚未落源 | 没有 POLICY 实现版验收；当前 `groupmsgservice.go` 仍以 `isFromApi` 跳过普通成员/禁言检查，不具备专用策略不可绕过能力。正式内部路由/19001—19008 错误码仍是候选 |
| RELIABLE | **完整请求幂等、提交结果/outbox 代码未开始。** 已实现的有限子依赖是持久序列 `33d2b9b`，非 RELIABLE 全包 | 子依赖已有真实 DB、24 并发、失败/重启/迁移、双端及管理页+DB 证据（§13、分支 evidence README/UI 补验 11aa1ac）。现 `SaveHistoryMsg` 仍无回执异步入队，HTTP 先给 ID 再异步发送，ClientUid 去重仍为进程内十分钟 LRU；不能报 persisted、跨重启幂等或可靠结果已实现 |
| READ | 原生 qry/set seen 已有候选，Mongo 缺行错误修订 80bdd87；共享修复由原管理唯一负责，二管家继续 READ。二管家分支 a35c877 已含 7ccc6e9 整合及双端 retention 探针 | 固定 7ccc6e9 的 16 份依赖源码已与 33d2b9b 匹配。尚未收到 a35c877 对应的新制品、真实清理/冷启后 has_new=true、完整适用回归和独立复审回报；原始 retention 红例继续保留。未作最终 READ 验收 |
| 公共集成/业务适配 | 006/007 与新库 DDL、迁移失败停止已在依赖分支落源并验证；**尚未合并 master**。正式 IM 心灵传输 HTTP/认证适配尚未实现 | 业务唯一契约仍为 2026-11-06-C、SHA 0a117252…；§11.2/11.3 路由未注册。单一权威候选已发送 pbyc7z4zp2yhukeh，待 BIZ 维护者回 SHA/边界确认，不并写契约、不把消息发送当冻结。DG2 测试配置交接不构成心灵传输业务全链联调通过 |
| PERSIST / 发布 | PERSIST 4ecec88 是已交付的旧验收背景，不计作本轮新进度；新改造未发布 | 未合并、推送或部署新 POLICY/RELIABLE/READ 包，未切换 Rocky/pro/pre。真实 App、READ 新页面、push 与完整宿主业务联调未验收 |

核读工作区：主仓 master 7df1c64 tracked/staged 无差异，未跟踪 `deploy/`、`docs/pre-deployment_zh.md` 是保留的旧部署材料；序列分支 11aa1ac、READ 分支 a35c877 均无未提交改动。旧 detached F0 工作树保留两份 PERSIST 生产文件修改、旧集成测试和 probes，未归为新 POLICY/RELIABLE 实施，不清理。上述状态在本次查询后可能变化。

当前没有可宣称真群/私聊改造整体可用的验收结果。已解除的是共享修复负责人和源码依赖不明；剩余门是 READ 对整合制品的实际回归/复审、BIZ 单一权威与正式 HTTP/认证接线，以及尚未开始的 POLICY/RELIABLE 核心实施。POLICY 未开始不是因为必须等待 READ 全包；后续应由原管理沿既有授权实施专用主体/权限及提交屏障，再推进真实持久幂等/结果链，二管家继续其 READ 回归，各自提交并按对应版本验证。本次状态查询本身不执行这些开发步骤，也不扩大部署权限。

## 15. READ 整合证据核验更新（2026-11-06 07:27 -05:00）

大当家升级请求批次 `cbf09810b1a5292f9e6e8994281c4affb3fd9088aac89300acd92e7c6b194ea8` 到达时，本轮实查二管家已提交新报告 `932ef61ffc813fd03dd4e4d2c17898bace96d1b8`，取代§14当时“尚未取得整合回归回报”的快照。报告对应测试源码 `a35c8779c43d23e42cd6d49fc295f454abf6f2ff`，非旧 008 制品；932 只改四份证据文档。原管理本轮没有启动/重跑二管家的测试栈，只审阅当前材料并执行哈希及结果一致性核对。

- 30 份源码 Git blob、归档内同一 30 份源码、39 份原件摘要均匹配 `integrated-manifest.json`。磁盘二进制 SHA `48592a8d46e71929e0d8f062e7b8e456e4969d8af11fddc0134994a6843fec00` 匹配；测试镜像为 `sha256:90ce1013e041bb65e4121a849e4938dd3401f3f73724ff90a815f6316784adb9`，label 为 a35。
- 未修改的原始 e395 场景原件 `legacy-retention-green.log` 实际 PASS：latest=4、seen=3、has_new=true。最终加强双端原件 `final-runtime-retention.log` 也 PASS，ACK ID 为 `pbydb47ajacltf3b`，两端收到、历史和摘要对应此 ID；原始 latest=1/seen=3/false 的 RED 保留不删。
- 本轮核对持久化原件：恰一条历史 seq=4、floor=4、seen=3，三份摘要均为该 ACK ID，owner 未读/已读 index=1/1。普通及清理后仅 IM 重启的 DB 转储均逐字节相等，新旧库两张表结构相同，版本记录指明 MySQL 在对应重启窗口未重启、活体为上述同一二进制。
- `tests-lf.log` 中 READ schema、merge、service 和 clean-time 四个真实 MySQL 用例均有 PASS 且非 SKIP；同文件另有 19 个固定他人 fixture 等 SKIP，不计覆盖。广泛 message-tree vet 的六个既有 Mongo 字面量警告仍为 FAIL，不描述为全仓 vet 绿；精确受影响包 vet/build 已通过。前期 CRLF、探针编译失败均保留，不归为通过。
- 已读 `integrated-review.md`：新独立审查在原生 MySQL/停止旧写者范围为 0 Critical、0 Important、1 Minor。Minor 是 retention 探针本身未直接断言 seen 恰等于旧值和 latest 恰等于历史序列；本轮原件里的单独精确 DB 断言覆盖了实际运行事实，后续可加强自动断言，未作为新功能验收保证。

阶段结论：已具备解除“原生 MySQL READ 历史清尽/冷启序列复用”单项阻断的版本对应实测及独立复审证据，原管理材料核验通过；这不是原管理独立重跑，也不是心灵传输产品/生产全验收。正式 BIZ HTTP/原 actor 认证、单一权威契约确认、真实 READ 页面/App/push、Mongo、混合写者及生产负载继续待验。独立权威序列候选已同步 BIZ，唯一契约当次核读仍为 C/0a117252…，未以发送回执当冻结。

写负责人不变：原管理持有共享 `convercache/msgconvercache.go`、`msgsequence.go`及测试、六发送调用方、公共迁移入口/007/新库 SQL/启动器和后续 HTTP/认证；二管家持有 READ 模块、独立 proto/006、clean-time 两配套文件及 READ 探针。006 已原样纳入依赖，整合提交 7ccc6e9 的 16 份共享源码仍与 33d2b9b 相同；不增第二个分配源或转写权。现阶段不再是等待共享依赖交付。

资源仅核快照：本轮 Docker 只读检查显示其两个 IM 和 MySQL 均停止；IM exit 137、OOM=false，MySQL exit 0，不能称已演示优雅退出。数据、卷、制品和私有原件保留；本轮未改生产源、未启动别人的服务、未合并/推送/部署或切换 Rocky。证据入口是二管家 `docs/evidence/20261106-mindlink-read/{README,integrated-verification,integrated-review,integrated-manifest}`，旧 manifest/retention-handoff 保留历史语境。

## 16. READ-R1 契约 E 核对与公共 HTTP 接线候选（2026-11-06）

当前有效批次 `db790ca557f684e9483609d19f6ac542da590a744bc8ccba354e37d94160ba12` 要求核对 E 及补齐公共接线候选。唯一契约仍由 BIZ 独占维护：`chatbot43/docs/mindlink-conversations-contract.md`，固定提交 `ad496155c984b0a3865f07e137025a5447540b73`。本轮直接计算该 Git blob 的 SHA256 为 `0c506044afe2f82d64232b14df85dfdfbddd6518bcce01a511d759aeca5e84b0`，工作区 CRLF 规范化为 LF 后逐字节相同；E 相对 D 的实质变更是合法零 PB 与 HTTP 缺字段的区分，§11.2/§11.4 未变。原管理接受该零状态修订及之前已经回执的单一 seen 权威/原生语义子集，不代表 E 全部接口已经冻结或运行验收。

READ 二管家 `23872613c059f7c181512b5ed8a303c382613c9b` 的交接与现有源一致：`CreateQueryAckWraper` 传递合法空 PB，带正确响应 factory 的 `SyncRpcCall` 调用 `PbUnMarshal`；匹配请求的成功 ACK 可以解出全零 `ConverSeenState`。不能用载荷长度代替有效性判断，HTTP 仍显式输出 `"0"/"0"/false`。上一批原管理在 Go 1.26.1 Windows、关闭模块网络查询后运行既有纯编码检查，实际通过零字节编码及往返；本批只核 E 变更和正式源，不把该前批检查或 BIZ 报告的 21 项规格检查算成本批新运行测试。

### 16.1 可供唯一契约登记的 IM 方候选

本节是原管理提出的设计候选，等待 BIZ 与 READ 对应回执后并入唯一契约。下列方法、路径、枚举和结构**均未注册或实现**；这里不产生第二份已生效契约，也不放行新账号/页面。先把当前状态读取与原写操作结果核查分开：

| 语义 | IM 上游候选方法/路径 | 候选 operation_type | 授权及结果 |
| --- | --- | --- | --- |
| 当前阅读状态 | `GET /apigateway/mindlink/conversations/:conversation_id/seen` | `conversation_seen.query` | 同 scope 当前读权限；无 query/body，空字节 SHA256；返回当前 seen/latest 快照，不证明某次 PUT 已执行 |
| 确认展示位置 | `PUT /apigateway/mindlink/conversations/:conversation_id/seen` | `conversation_seen.advance` | 独立写 allowlist；绑定原持久操作意图、所展示位置与当前策略；不能以 query 票据调用 |
| 原写操作结果 | 复用 §11.2 候选 `GET /apigateway/mindlink/operations/:operation_id` 的受限结果模型 | 精确枚举随公共操作结果 schema 登记，当前不冒称已定 | 核对原操作、摘要、scope、固定目标与当前可读权限；不得用 GET seen 的当前值冒充原请求回执 |

主人侧仍是 BIZ 候选 `GET/PUT /api/personal/agent-conversations/:conversation_id/seen`，不能把客户端直接连到上游服务间接口。首个主人入口仅接受可信 `actor_kind=owner`、`actor_id=owner_id`、`avatar_generation=0`；avatar/system 的阅读授权如另有需求须独立登记，不能从通用票据支持三种 actor 推导本路由都允许。上游 `target_id` 固定为路径中的业务 conversation ID；签名身份及权威投影解析出 IM requester/peer 或 group/type/subchannel，不接受客户端给 IM 身份挑选范围。

**GET 当前状态不是 GET 原操作结果。** 两者需在 §11.4 各自的必填 schema、digest 与 allowlist 中显式区分；前者不能要求该会话必须先有一次 SET，也不能借旧 mutation 票据读取当前状态。原操作查询继续按既有“原持久目标”规则。GET 当前状态的 operation/request 字段是否作为本次查询意图绑定，需在该新 operation 的正式 schema 中登记后再调用；不能默默套用结果查询 schema、缺字段填零或猜测枚举。这一认证细项仍未冻结，不因路径候选确定而解除。

### 16.2 JSON、操作回执与错误候选

PUT 上游候选 body 仅含 `operation_id`、`request_id`、`request_digest`、`seen_seq` 四个必填字段。前三项沿 §11.1：operation ID 为 64 位小写 hex，请求键 1..64 ASCII，摘要 64 位小写 hex；均与 §11.4 票据及原持久意图一致。`seen_seq` 是 JSON 字符串，值只能为 `"0"` 或以 1..9 开头的 ASCII 十进制数字串，且不大于 `9223372036854775807`。数字类型、缺失/null/空串、前导零、空白、正负号、小数、指数和越界均拒绝；未知与重复 JSON 字段拒绝。body 不接受 owner/app/requester/IM target/subchannel 自报授权。BIZ 先固定原始 body 字节再签 `body_sha256`，重试不改字节、目标或语义摘要。

操作摘要必须把 READ advance 的具体 `seen_seq` 纳入原意图，并绑定 operation_type、完整 scope、actor、业务目标、权威参与者/映射及 expected policy；具体确定性数组次序随唯一契约 schema 登记，不允许两边自行猜。新展示位置是新意图；同 operation/request 键改位置/目标/摘要在写前冲突。request_id 不是任意 trace，也不是 seen 表主键。公共持久绑定/结果账本和最终策略提交屏障仍需实现，不能把普通 `SetConverSeen` 外包一层 HTTP 就称为原操作结果已持久。

候选 `SeenState` 固定为三个显式字段：`seen_seq`、`latest_seq` 使用上述十进制字符串，`has_new` 为 boolean，无 omitempty。IM 上游沿现有 `code/msg/data` 成功 envelope，BIZ 再适配自身 `code/message/data/timestamp`，不要把两者按同一 DTO 解码：

```json
{"code":0,"msg":"success","data":{"seen_seq":"0","latest_seq":"0","has_new":false}}
```

GET 的 data 为 SeenState；PUT 候选 data 为 `{operation: OperationResult, state: SeenState}`。OperationResult 沿 §11.2/§11.4 的原操作 ID/类型、request ID/digest、完整 scope、固定目标、desired_state 及 effective policy/result 版本登记；READ 的 desired_state 为 `{seen_seq: "所提交位置"}`。该正式结构、字段类型和事务回执版本尚需公共操作结果源登记，不得省掉它而返回 code0。PUT 成功须具备真实持久操作回执及成功读出的 state；重试返回原操作回执，state 重新按当前权限读取，不缓存旧 has_new 冒充新快照。写已提交但回读失败可返回未知；当前 GET 位置已足够大，也不能证明是该 request 推进的。

| 来源/条件 | 候选 HTTP / reason / 数字码 | 约束 |
| --- | --- | --- |
| 严格 JSON/路径/序列参数错误，原生 12004 | 400 / INVALID_REQUEST / 19001 | 不以 proto3 缺省零代替 PUT 必填 |
| 可信 scope 与路径/投影不符 | 403 / SCOPE_MISMATCH / 19002 | 无写入，不回跨 scope 信息 |
| 同键异摘要/位置/目标 | 409 / REQUEST_CONFLICT / 19003 | 不分配新键，不再次执行 |
| 旧策略或代际 | 409 / STALE_POLICY / 19004 | 最终提交屏障仍核权，不只靠验签时检查 |
| 投影/能力/可信 context 未就绪，原生 11013 不支持 | 409 / NOT_READY / 19005 | 不降级旧管理 API 或 WS 代发 |
| 已禁用主体 | 409 / PRINCIPAL_DISABLED / 19006 | 不因重试获得提交权 |
| 原生 10009、超时、无 ACK、解码失败或缺预期 typed/HTTP 响应 | 503 / UNKNOWN_RESULT / 19007 | 不填成功零状态，不断言写未发生；合法空 PB 不属此类 |
| actor/签名/有效期/可读写权限不符，群非成员 13002 | 403 / FORBIDDEN / 19008 | 私聊返回空历史不能代替业务拒绝 |

以上数字沿已有候选字典，仍须在 `commons/errs/errorcode.go` 和新路径响应测试正式注册后生效。现有框架的路由不存在/方法不支持响应同样不能被适配成成功 state。

### 16.3 可信接线及实施边界

- `routers/router.go` 的现有普通路由 group 使用旧 `Signature`；READ 候选必须挂专用 §11.4 Ed25519 认证/严格 schema group，不能只放进旧 group。`ToRpcCtx` 第二参数虽名 requestId，实际赋 requester，业务 request_id 绝不能传作读者；它也不提供验签或最终事务屏障。
- 公共认证应验证 issuer/kid/env/tenant/app/audience/owner/actor、method/精确 path/body hash、时效与独立 operation allowlist，再解析服务端权威映射并封装不可由终端自报的 RPC context。正式 server-only proto/tag 仍须公共源分配，当前不虚构号码。原生 READ API 没有该上下文、操作账本或事务参数，需协同提供能与最终核权/回执共用事务的最小入口；未完成时禁止通过原 RPC 绕过屏障。
- 最小复用点：READ `starter/seenactor/seenservice/SeenDao`、`mindlinkread.proto`、006/007 以及持久序列；新增 HTTP DTO/handler/route 放 `apigateway/models/convermodels.go`、`apis/conversation.go`、`routers/router.go` 的公共写集。认证、操作结果和必要新迁移由原管理集中实现；不并写二管家文件，不改已定 006/007 偷加新能力。
- 实际展示序列仍有缺口：普通 `models.HisMsg` 与 `apis/hismsg.go` 映射未暴露 MsgSeqNo。如选择该 HTTP 历史作为页面来源，需从相同授权历史的 DownMsg 追加无损 `msg_seq_no` 字符串并验真实数据；不得从发送 ACK、msg ID/time 或预留水位推算，也不得扩大历史收件范围。已有原生 SDK 的 MsgSeqNo 可作为另一实际数据源，但需以最终采用的页面/SDK链证明。

本批只新增这份现有报告内的候选及 E 校核记录，未修改唯一业务契约、生产 Go/proto/SQL、运行栈或密钥。核对 E blob/换行/变更及阅读源码是本轮实际检查；原生 DB/制品/页面、上述新 HTTP schema/鉴权/冲突/提交屏障、历史新增字段均未新跑或实现。完成文档后只提交该报告，不 merge/push/deploy。接下来由 BIZ 把接受的候选及差异写回唯一契约并回固定 SHA，不能把发送回执当作其接受。

## 17. Rocky 真实业务用户连接与回调证据缺口（2026-11-06）

有效批次 `cbe13afa579358f0cb654007ac5d38165081a96daa3d6807896f5067443f7106` 要求回已有正向证据或具体阻塞，不重建租户/用户、发消息或重启。目标固定为 `dg2c3350172f150 / test_6199931265070805`。本轮实际只查 IM 容器/端口元数据、目标租户拦截器计数、既有公开证据和源码，未取得或读取业务 Token、Secret、私有配置或消息日志值。

| 项目 | 本轮事实及适用范围 |
| --- | --- |
| 当前 IM 版本 | Rocky 仓库 detached `b87fc791400dafae0f3fb2e1665863419bad58f7`；运行镜像 `bitcomputing/im-server:dg2-rocky-b87fc79-diagnostic`，image ID `sha256:09186cb700fdb2e4fe21e163494af2c9dc1e81cb69e5fa1fdf8658b848ef45a7`，不等于本机后续 READ/序列/PERSIST 验收制品 |
| 网络地址范围 | `docker port` 实查 API/nav/WS/admin 只绑定 Rocky Windows 主机 `127.0.0.1:19001/19002/19003/18090`。其他机器的 localhost 不是 Rocky；IM 容器的 localhost 也不是 Windows 业务 API |
| nav 既有记录 | 公开 `handoff-c550-public.json` 于 `2026-11-06T07:50:36.6149341Z` 记录普通测试夹具导航 code0、servers=`["127.0.0.1:19003"]`；不能移作真实业务用户成功记录 |
| 客户端格式 | 同 b87 源 `navigator/apis/checktoken.go` 要求 `x-appkey`/`x-token`，GET `/navigator/general` 返回裸 host:port。`simulator/wsclients/wsimclient.go:Connect` 的 Address 需要 `ws://127.0.0.1:19003`，客户端追加 `/im`，实际 wire URL 是 `ws://127.0.0.1:19003/im`；不把含 `/im` 的完整 URL 再当 Host 输入。此为已读协议客户端源码格式，不是已发布 SDK 的本轮握手结果 |
| 真实业务 Token nav/CONNECT | **not_covered / 未核**。本批没有提供来自业务实际返回 Token 的安全进程输入，未读取业务登录凭据、Redis 或私有文件来获取它；没有发起这条真实用户 nav/CONNECT。需由业务负责人在其凭据范围内取得实际 Token，完成 nav 用户 ID 核对及 SDK/协议 CONNECT，输出仅脱敏 code/用户/地址/版本；不能用注册成功、缓存存在或三模拟用户替代 |
| 目标租户回调配置 | `2026-11-06 14:22:34.791565 UTC` 对自有 `jim_db.interceptors` 仅做目标 app_key 的 COUNT/SUM：全部行 **0**、custom(type0) **0**、业务 `/api/im/callback/message` 路径匹配 **0**，未读取 conf、模板或凭据。该租户尚无这条自定义拦截器的持久注册，不能声称已有正向触发链 |
| IM 容器→业务处理 | **not_covered / 未核**。本轮未发送消息或签名回调，没有实际业务 PASS/处理审计证据。`http://host.docker.internal:16006/api/im/callback/message` 仅为容器访问 Windows 主机的候选地址，监听/防火墙/签名与处理结果未验证，端口或 401 不计正向。业务正式路由为发送前拦截器，不是通用消息订阅 |

静态边界核对：所读 navigator、拦截器 DAO/实现及协议客户端文件与 b87 的 `git diff --exit-code` 为 0。`LoadInterceptors` 按 app_key 加载 `interceptors`；`CustomInterceptor.CheckMsgInterceptor` 对 `isFromApi` 直接 PASS，网络/非200/解码故障也可 PASS。因此以后即使 HTTP 代发 ACK 成功，仍不能证明业务回调被请求、验签或处理成功。此为已有实现的源码事实，本批未改权限或错误策略；真实跨容器请求仍未验证。

用户行复验关联的是**前批** `2026-11-06T12:37:56.256147Z` SQL 结果：目标 scope 恰1行、id9、该隔离库其他租户同 user_id 为0，原发送回执 `pbydq8gfp8lhukeh`。本批未重复该查询，不能称这是本轮重查或证明 Token 握手。该旧证据及本轮计数/版本/格式/未覆盖项已分别写入 Rocky 公开文件 `D:/projects/.evidence/DG2-20261106-002/im/business-crossrepo-cbe13afa-public.json`，SHA256 `df12d5ebeb3952e6bfb4183bfcc16256e700dbed40a9dff6a08f03506c078a26`；本机忽略的对应 JSON 与远端文件逐哈希相同，并已解析回读。原 `validation.md` 仍保留三用户及旧离线/重启失败，不覆盖改写成新业务验收。

首次通过长 EncodedCommand 写证据失败（Windows 命令行长度）；随后回查目标文件不存在，改传上述不含秘密的单个 JSON，读回及哈希匹配后才计作证据交付。没有创建租户/用户/群、发送消息、连接业务用户、回调业务接口、重启服务或改 IM/BIZ/Redis/私有 JSON；仅公开证据文件和本报告新增。下一步所缺是业务 Token 的真实客户端运行及受控的拦截器配置/有效事件/业务审计闭环，不是再发一次模拟消息或把旧记录重标为通过。

## 18. F 字段回执补充：生成时间与接收分发时间分开（2026-11-06）

有效批次 `0b0cb32b31e0721c4bf23984ee8dc5fc660fc5c8c661fb80498a51b36b5403e9`，来源 READ 二管家 `pbyfldxeyu8hukeh`。原管理前次 F 回执 `pbyfjzh38uuhukeh` 只核了发送服务构造赋值等位置，对“私聊转发副本保留 seq/time”的接受范围过宽；本批补读固定 `a35c8779c43d23e42cd6d49fc295f454abf6f2ff` 的接收分发及用户时间状态，作如下限定，不修改原生行为。

- `services/message/services/dispatchservice.go:21`：Private/System 分支直接将 downMsg 交给 `doDispatch`；该文件的 `copyDownMsg` 调用位于 Group 分支。`copyDownMsg:151` 构造副本时确实复制 MsgSeqNo 和当时的 MsgTime，但这仅是复制时点。
- 同文件 `doDispatch:92` 在实际处理前调用 `RegenateSendTime(appkey, receiverId, msg.MsgTime)`，随后以返回值覆盖 `msg.MsgTime`（96—97 行）；该处未重写 MsgSeqNo。不能把复制或发送服务的赋值当作接收投递 MsgTime 不变的证明。
- `services/message/services/userstatus.go:226`：时间状态按 appkey/userId 取出并加锁；已有 latestMsgTime 不小于传入 currentTime 时返回旧值加一。它是接收者时间序列，可能受同接收者其他会话影响，不等同于消息会话的 `message_sequence.send_time`。

因此应将唯一契约 F 的生产者行澄清为：生成阶段返回的 seq/time 写入 DownMsg(6/8)，分发沿用其 MsgSeqNo，MsgTime 可在接收者分发阶段重新生成；不要写“私聊转发保证 seq/time 端到端相同”。READ 的展示位置仍是同授权会话实际持久历史的 MsgSeqNo，不能由 MsgTime、未读 index 或预留水位推导。两张表的键、序列生产/历史消费关系、E 合法空 PB、§16 接线候选及既有验收边界不因此扩大或改变。

本批证据为固定 Git 源码及调用分支核对，**没有**跨会话运行复现或产品/DB 回归，不把静态可发生条件描述成已观测的投递结果。只提交本报告的限定说明，由 BIZ 维护者修改唯一契约后回固定版本；不并写其契约，也不把此处澄清称为原生 bug 修复。

## 19. J 候选补充：GET 自身意图与 READ 摘要参考（2026-11-06）

本批 `f02998be34a28084ff2e52f45fc13e48983c55ada036a9eac733d296aa1dbe9a` 实读唯一契约 J 固定提交 `bd22d3b5983cfefb572441362795ec190aead7bf`，契约 Git blob SHA256 为 `deddaccf6ef8ce2066ba0cafa9ff838fb0546da47cac24dffb407df36179c7b7`，工作区规范化 LF 后相同。J 对 §16 的方法/路径、query/advance 枚举、owner 首个入口与 PUT 四字段登记准确；以下专门补它明确留下的 GET 意图和摘要次序缺口。**仍为原管理提出、待 BIZ/READ 回执后写回唯一契约的候选，不是已注册接口、已实现账本或权限放行。**

### 19.1 GET 独立持久查询意图，不借一次 SET 作前置

建议严格复用 §11.4 的原操作约束，不给当前态 GET 增加“随意复用 trace、跳过持久绑定”的例外。BIZ 在核当前主人/会话/映射/策略后，为一次逻辑读取持久化独立 `conversation_seen.query` 意图；即使会话从未 SET 也可以创建该只读意图，不调用 Set(0) 或创建 seen 行。IM 只从 `conversation_seen`/授权历史读取状态；查询意图和结果是不可变操作证据，不成为第二份可写 seen 权威。

这一选择使逻辑 GET 也产生持久意图/结果写入，复用规划中的公共操作账本，不新增阅读游标表；其保留策略归公共账本统一处理，本轮不另造清理服务。这是保持既有原操作重试约束所取的成本，不应写成无状态 GET 已经接通。

- query 的 `operation_id` 必填，由 BIZ 用32随机字节生成64位小写 hex；`request_id` 同样必填，首个 GET 方案由 BIZ 取与该 query operation_id 相同的64字符值（符合已有1..64 ASCII规则）。同一逻辑读取的重试沿原键，页面明确刷新或下一次逻辑读取才新建 query 意图。PUT 仍用其调用者请求键，不把这一 GET 生成规则套到其他操作。
- 当前态 GET 的 `target_id` 固定为该 query 意图绑定的业务 conversation ID，且与本次路径和当前权威投影一致；不是借某个先前 mutation 的 operation ID 选择目标。查询原写操作结果仍走 §11.2 `operations/:operation_id` 候选及其独立 allowlist/schema，两种 query 不混用。
- 持久绑定至少唯一约束 `(issuer,operation_id)`，并约束同 `(issuer,environment,tenant_id,appkey,owner_id,actor_kind,actor_id,request_id)` 不映射到另一意图；operation_type、代际、目标、policy、映射/成员、摘要和 body hash 为不可替换的绑定内容。不能按 digest 单独合并不同 query 意图，因为同一 scope 的不同读取可以有相同语义摘要而观察到不同时间的状态。
- GET 无 query/body。标识与摘要只由 BIZ 放入经验证的 JWS，不放 URL 参数、body 或可自报 requester。GET 新票据/重试仍完整核验 issuer+jti 及原意图全部绑定；刷新 jti/iat/exp 不改变原 operation/request/digest/目标，失权或策略不匹配仍失败关闭，不因读操作绕过授权。

READ query/advance 的 JWS header 沿 §11.4 固定 `alg=EdDSA`、`typ=mindlink-actor+jwt` 和受信 kid。payload 沿该节已有字段，**本候选要求全部显式必填**：iss/aud/environment/tenant_id/appkey/owner_id、operation_type/operation_id/request_id/target_id、actor_kind/actor_id/avatar_generation、expected_policy_version/desired_policy_version、request_digest、method/path/body_sha256、iat/exp/jti；不新增可自报 IM 身份字段，不允许未知/重复项或缺字段填默认。

首个入口 actor 仅 owner，actor_id 必须等于 owner_id，generation 是显式整数0。policy 两字段是0..MaxInt64整数，均须显式等于权威有效 policy（READ 不修改策略，desired=expected；0只有权威版本确为0时合法）。method/path 必须匹配实际固定路由；GET body hash 为 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`。时间、jti、签发信任绑定仍沿原60秒/5秒及32随机字节hex规则，不改 §11.4。IM 必须从权威投影重建下面摘要所用的 IM reader/target/type/subchannel/参与者；它们不是新增 JWT 自报授权字段。

### 19.2 严格重试返回原查询快照，不把它标成新状态

原 query 已执行时，同键同摘要仅核原持久结果，不再次执行查询来违反原操作规则；异摘要写前冲突。为避免原查询快照被消费成“当前态”，建议将 §16 扁平 GET data 候选补充为 `{operation: 原query回执, state: SeenState, state_observed_at: UTC时间, replayed: boolean}`。首次保存的 state 和其 observation 时间不可被重试覆盖；replayed 是本次返回是否复用了原结果的显式标记。公共 OperationResult 完整类型仍待其正式源登记，缺回执不能补 code0。

BIZ 如向页面提供新的当前态，必须来自本次新 query 意图的首次有效读取。若先重查/重试得到原 query 回执，不能直接把旧 state 当作新状态返回；在原查询结果已确定、重新核当前权限后，另起一次明确的新逻辑读取再提供新快照。**未知的 mutation 仍沿原键查询，绝不因超时创建新 advance 重放。** 这不豁免未知 query 的原操作核对，也不承诺首次查询结果在返回后持续不变。新状态 GET 不证明任何先前 advance 已执行；原 advance 结果查询也不缓存旧 latest/has_new 冒充新状态。

这是本次为满足两种约束共同提出的 GET 回执补充，尚需 BIZ 确认纳入唯一契约；原 §16 扁平 GET data 在该细项确认前不独立启用。没有落地公共意图/结果绑定及当前权限核验时，GET 与 PUT 一起保持未就绪，不用一个裸 native RPC 跳过该门。

### 19.3 request_digest 精确有序数组候选

query 与 advance 采用同一21项数组，顺序严格如下；参与者取权威固定 IM 成员集合，先校验无重复，再按 UTF-8 字节序排序；Private 为固定参与者对，Group 为完整固定成员。发现重复属于无效权威记录，不静默合并来修复坏状态。所有字符串须先通过正式字段格式/UTF-8/长度及权威绑定校验，不 trim、改大小写或作 Unicode 归一化。

| 位置（从0开始） | 值 | JSON 类型 |
| --- | --- | --- |
| 0 | 固定 `mindlink-read-r1` | string |
| 1 | `conversation_seen.query` 或 `conversation_seen.advance` | string |
| 2—5 | environment、tenant_id、appkey、owner_id，依次各一项 | string |
| 6—9 | 业务 conversation_id、conversation kind、actor_kind、actor_id，依次各一项 | string |
| 10 | avatar_generation；首版 owner 为0 | integer/int64 |
| 11—12 | expected_policy_version、desired_policy_version，依次各一项 | integer/int64 |
| 13—14 | 权威 IM requester user ID、权威 IM target ID（peer/group），依次各一项 | string |
| 15—16 | channel_type（1或2）、sub_channel，依次各一项 | integer/int64、string |
| 17 | 排序后的固定成员 IM IDs | string array，禁止null |
| 18—19 | 大写 method、包含固定业务ID的精确 path，依次各一项 | string |
| 20 | query 为 `[]`；advance 为 `[seen_seq]`，其中位置沿H规范十进制字符串 | string array，禁止null |

在已校验类型的 Go 值上使用标准库 **`encoding/json.Marshal` 默认字符串转义**生成这一个数组的紧凑 UTF-8 字节，无 BOM、空白或末尾换行；整数必须是 int64，不经浮点中转，成员/位置空数组必须是已分配的空 slice 而非 nil。保留默认 HTML 转义（例如 `<&>` 编为 `\u003c\u0026\u003e`，U+2028/U+2029也使用 Go JSON 转义）；禁止一侧改用 SetEscapeHTML(false)、Encoder.Encode 的末尾换行或另一侧的默认 JSON 排版。随后 `SHA256` 并输出64位小写 hex。这个摘要是操作语义，不是 HTTP raw body hash；两者仍分别核验。

operation_id/request_id、issuer/kid/audience、jti/iat/exp 不放进该语义数组；合法刷新 jti/时间或同一信任绑定下的 kid 不改变原意图 digest，issuer/audience 等原绑定不允许因此更换。这**不免除**所有这些字段按票据信任配置及原操作/issuer+jti记录的完整绑定和冲突检查。两侧都需从受验证票据及权威事实重建数组，不能只采信 body/request_digest。seen_seq 用字符串进入数组，query 的空位置数组也不能替换为字符串0或null。

合成固定向量见 [digest-vectors.json](evidence/20261106-read-intent/digest-vectors.json)：其中包含完整规范数组字节文本，可直接做两侧序列化比对。样例均非真实账号/租户或凭据。

| 合成向量 | SHA256 |
| --- | --- |
| query | `d77bb09590ad3224c7ea97c703566e27965f4fd753c7bc689d31033a61d1c5c9` |
| advance `"0"` | `c42cfc005e857a1fe12548e0f6df43ba52953a97951ccedca11bcb2239d7b3d8` |
| advance `"3"` | `3524b5a3d9f4e3dfcff1eef5a3991739616773771b0b1f4dbd475fff227138dc` |
| advance `"9223372036854775807"` | `e373d47a707c3727c196d25da00ff2706a208b346b26132194b1cd5d1d3237e1` |

本批先用 Python 独立算上述四个 ASCII 样例，再用 Go1.26.1（GOPROXY/GOSUMDB关闭，标准库程序）对四项黄金值、成员输入顺序变化、Go字符串转义做断言，全部通过，公开向量原件 SHA256 为 `41c93383ba90ef03238080b3c7cb00929bd61875366887052ce65b47c1ec8c36`。这只是合成摘要/编码参考验证，不是 HTTP parser、Ed25519、jti绑定、DB唯一约束/同事务、页面或真实调用的测试。未修改唯一业务契约、Go/proto/SQL生产源或服务；只提交报告与公开向量，忽略目录的离线参考程序不加入生产。后续由 BIZ 把接受项/差异写回唯一契约并给新固定SHA；两个缺口有了具体候选，整体 HTTP 仍待双方确认和实际实现。

## 20. Rocky 真实业务 Token 导航与原生 CONNECT 证据复核（2026-11-06）

有效批次 `c180f8e7e313c9307eb07aa10904bad9b00e9937a1879974498dc6c28f2901a4` 收到 BIZ 负责人稳定身份 `01a1a0e1-3618-78b0-a5c7-81a37bff3a31` 的回报 `pbyhqw95rc8hukeh`。本批接受其补齐 §17 中“真实业务返回 Token 的 nav/协议 CONNECT”这一项的有限证据，不把 BIZ 的实跑记作原管理重新登录或独立运行用户流程。

固定产物为 Rocky `D:/projects/chatbot43` 提交 `d8269a7cac5dec6b380a94638ba48e79dcf5606b`，实际测试 BIZ 源基线 `e0855573bda66ff0db19b60ee5b994ea75fea610`，IM 源 `b87fc791400dafae0f3fb2e1665863419bad58f7`。本轮通过 SSH 实读固定提交的报告增量、六份 `docs/evidence/20261106-im-private-tenant/nav-connect-*` 脱敏产物及其中客户端/驱动源码；提交仅改八份 STATE/文档/证据，未包含旧 `go.sum` 改动。

| 证据 | 实际支持的结论与边界 |
| --- | --- |
| BIZ 成功记录 UTC `17:03:22.790538—17:03:27.130877` | 登录、`/api/im/token`、`navigator/general` 均记录 HTTP200/code0；驱动断言前两者返回同一非空 IM Token，再将这个 Token 用于导航和原生 CONNECT，不重注册 IM 用户或借旧夹具 Token |
| 身份及连接 | app `dg2c3350172f150`，导航、CONNECT ACK 和 client 的 user 均为 `test_6199931265070805`；记录 ACK 存在、client/server code0、connected=true、显式断开及进程 exit0。客户端是固定 b87 的 `simulator/wsclients`，Go1.25.12 Windows、wire SDK1.0.1、Verbose=false，Address 为 `ws://127.0.0.1:19003`，实际路径 `/im`；不是已发布 SDK 或浏览器验收 |
| 原件与归档 | 原管理本轮重算五份指定公开原件/源码的 raw 和 CRLF→LF SHA256，逐一匹配 receipt；成功归档 Git blob SHA256 为 `af81dfe8cd94bb3b5c55210d8465b396f5ab1173b5c7833e8607a8bf92c6cd9f`，receipt blob 为 `11d29cf726a4c4a551ab5f81ce0ed7250d6d8babb4d6fde0f2fc51ff80c81ac2`。固定 b87 客户端源 SHA `c58e4792…` 与 build 记录一致，保留的探针二进制 SHA `29cbfb84…` 亦匹配；未执行该探针 |
| 本轮进程快照 UTC `17:20:53.8409415` | 精确探针路径的进程数0；API PID26644、路径 `D:/projects/chatbot43/tmp/dg2-rocky-biz/chatbot43.im-safe.exe`、启动 UTC `11:08:43.8150240`、二进制 SHA `f2d9fc0c20cdfe1885e14a5b695de731cc871bd9c3d73fe30de0467f66877fe4` 与原记录一致。此为本次读回快照，不是持续运行保证或服务端离线状态证明 |
| BIZ 数据和凭据边界 | 驱动与记录保留业务身份比较、user/reservation/message 的1/1/0计数及 device/request 审计9→10、1→2，合成验证码 NX/300s 与条件清理后不存在；原管理未重跑 SQL/Redis。Token 由 BIZ 进程内获得并经 stdin 交给子进程；其子输出及五份自有应用日志新增字节的凭据扫描仅覆盖那一轮。原管理本批不读取凭据、私有配置或原始日志，也不宣称所有日志出口安全 |

首轮 `nav-connect-attempt1.json` 是探针错误要求 API 存在 Verbose 字段而在 HTTP/夹具写入前失败，不能算通过或业务产品失败；纠正到客户端显式设置后才有上述成功记录，不称产品修复。本轮一次只读复核命令因本机长度预检停止，拆分后的原件哈希和进程复核实际 exit0；未把失败的预检算成已运行远端检查。

§17 的旧 nav 夹具、IM 用户行和 interceptor 零计数仍各保留其原时间；新记录仅取代“该真实用户业务 Token 的 nav/原生 CONNECT 未跑”这一历史缺口。回调仍缺受控注册、有效事件、签名和业务处理审计；发布 SDK/真实页面、消息/历史/离线/摘要/read 重启、服务端离线状态及旧列表/资料问题未因本批通过。IM 运行版本按 BIZ receipt 仍是 `09186cb7…` 诊断镜像，不是后续 READ/序列制品。本批不重启、构建、改配置、发消息、调用回调或部署；独立产品场景、原 bec85、三包和 DG2 整体验收仍未完成。

## 21. Rocky 回调联调前提与当前计数（2026-11-09，只读准备）

来源二当家 `pbz3kp3av9chukeh`，有效批次 `8b7d7417b527dbf62a0e016d1465b66f385a90ab8250586fd49328b4e2388b99`。本批只准备可审查材料，未新增配置、发消息、POST 回调或启停服务；不把旧 CONNECT 当作回调或新方案验收。

### 本轮实际读回

| 观察点 | 结果及边界 |
| --- | --- |
| SQL，UTC 05:09:37.125425 | MySQL8.0.46 / jim_db；指定 app `dg2c3350172f150` 存在1行，interceptors总数0、`intercept_type=0` 的custom数0、ic_conditions数0 |
| 访问方式 | 在原 MySQL 容器内用既有环境凭据运行 mysql，`START TRANSACTION READ ONLY` 后仅 SELECT 并结束事务；exit0。不输出/复制 Secret，不使用二当家此前失败的无密码查询推断零 |
| 网络，UTC 05:08:23 | Windows API16006监听127.0.0.1/PID26644；IM容器执行 `timeout 5 bash -c ':<>/dev/tcp/host.docker.internal/16006'` exit0，仅开闭TCP、不发送HTTP字节。证明该时点TCP可达，不证明路径、签名、PASS或处理审计通过 |
| 版本，UTC 05:13:12.6931418 | IM仍为b87，container d16f0401…e3172a、image09186cb7…ef45a7、活体binary `bfdcaa5d73721472cce9af5b3debcd525f24ab0cfabe2ca1f3cb373540510c9a`；StartedAt仍11-06 07:36:43UTC |
| BIZ对应 | Rocky HEAD `d8269a7cac5dec6b380a94638ba48e79dcf5606b`；PID26644、im-safe.exe SHA `f2d9fc0c20cdfe1885e14a5b695de731cc871bd9c3d73fe30de0467f66877fe4`、启动11-06 11:08:43UTC；源码/制品对应沿既有交接，不称本轮可复现构建 |

SQL只取计数，未读出request_url/conf/template或应用Secret。当前内存 interceptor cache 未直接查询，DB零不能单独证明活体缓存为空。BIZ当前回调表/审计计数未在本批查询，不沿用§17的旧零值。

### 固定请求、签名和结果契约

IM以b87的 `services/commonservices/interceptors/custominterceptor.go`、`interceptorservice.go`、`condition.go` 为准；三文件Git blob依次 `d43f228009a426c9e0ea46a3fdc5555e5e67bda0`、`0ce891e2550c0dfd16748457b8aa84d4026f53b2`、`460ae4f1e45b11b3beb43abc511724c78a32f3ca`。BIZ实际handler/logic blob为 `680dacc2a3a06b44858bd77e27e98b4c1b965238` / `0a00fb04af3b25b257bccdb69e7092a63d5cc245`，Rocky tokenManager blob为 `59468aeefdc50487eca194315bb7443fd1919836`，已直接读取其签名函数，未把本机较新版本替代。

- 正式路径：POST `/api/im/callback/message`；容器内候选URL为 `http://host.docker.internal:16006/api/im/callback/message`。TCP已核，未POST该路径。
- 头：Content-Type=application/json、appkey、nonce（IM生成8字符）、timestamp（Unix毫秒字符串）、signature。签名为小写hex `SHA1(app_secret + nonce + timestamp)`，拼接顺序固定；IM从该租户已有AppInfo取AppSecret，BIZ实际logic走VerifyCallbackSignature。不要因另有ValidateJuggleCallbackHeaders函数就声称本路由已具备它的全部检查，也不把此签名当新原actor票据。
- body固定为 MsgEvent：platform/sender/receiver/msg_type/msg_content为string，channel_type为整数，mention_info/refer_msg为对应对象或null。msg_content是原UpMsg字节转string，不是43Chat响应信封；没有message_id/client_uid/seq。BIZhandler回写X-Request-Id及received/responded审计，但writer是否启用和合法业务前置由BIZ核定。
- `request_template`/`succ_template`/`is_async`/`conf`虽有DB列，当前custom构造器没有用它们转换请求/响应或切换异步；不能靠填“模板”改变上述固定wire。

| 返回/分支 | b87的实际消费规则 |
| --- | --- |
| HTTP200 + `{ "result":"PASS", "custom_code":0 }` | 继续原发送流程；不等于消息已持久化 |
| HTTP200 + REJECT | 非零custom_code透传发送错误；0则12007。私聊/群在历史写入前返回；未来仍须查实收/历史/DB，不能仅按源码报拒绝用例通过 |
| HTTP200 + REPLACE / SILENT | REPLACE按类型/内容/mention字段修改；SILENT直接ACK0且不继续正常投递，不能把ACK0当实收/落库 |
| 网络错误、非200、JSON失败、未知result | 当前实现可PASS；401/超时/空或错误信封都不是成功回调证据 |

### 触发、配置与恢复操作稿（未执行）

1. 先由BIZ给同租户的普通非Admin发送者、有效接收者/会话、合法认领及风险/关系状态、预期PASS与REJECT场景，并确认既有AppSecret匹配，仅回配置一致性结论，不传Secret。系统/白名单发送者不能代替正常用户业务门槛。
2. 登记前重新只读核当前行并保存私有恢复材料；本轮基线虽为0，后续若已变化先停止协调，不能覆盖新行。二当家须另行明确允许受控配置、事件和缓存处理；旧过期窗口、0ae的未确认PERSIST候选时段都不自动授予这些操作。
3. 获批后仅新增一条本任务custom行（app_key固定、intercept_type=0、精确URL、本任务唯一name/sort，模板列空）及一条ic_conditions；同事务提交并记录两行真实ID。条件固定具体sender/receiver、channel_type=1及约定文本msg_type，不用全租户通配；无条件列表时ConditionMatchs为false，不会回调。
4. 普通合法Token建立原生WS，用固定b87 `WsImClient.SendPrivateMsg`（p_msg）发一条已批准有效内容，flags不含cmd/state，并确保条件命中。Admin/IsFromApi及普通签名HTTP代发会绕过custom，不能代替此事件。此原生模拟器场景仍不是已发布SDK/页面验收。
5. BIZ用同一request_id核received/responded、HTTP200/result及实际业务记录；IM独立核ACK、接收者实际消息及history/DB。由于回调发生在消息ID分配/持久化之前且无原消息ID字段，须事先约定单事件串行、唯一合成标记的私有对账方案；不能仅凭X-Request-Id或回调表一行证明消息已持久化，更不能冒充请求级可靠回调。
6. 恢复时在事务内仅删除本次记录的condition ID和interceptor ID，同时限定app_key/interceptor_id并核预期行数；不按app_key清空、不回滚消息历史、不恢复整库。若启用前有原行则须按私有逐行快照恢复，不能凭本轮旧基线直接删。
7. 最后重新核DB行、IM版本/健康、缓存处理及BIZ审计终态；保留新消息、映射、旧镜像和数据卷，异常就停止并按原键查结果，不盲目再发。

缓存由GetMsgInterceptors懒加载，创建后最长10分钟寿命并有读超时回收；本次源码检索未见显式失效接口。为使新增与恢复都确定生效，建议申请包含两次IM-only重启的明确窗口，或先证明现有受支持的缓存失效方式。只删DB行不能立即宣称缓存已恢复。此窗口和配置/事件权限尚未取得，不在本批执行；BIZ/MySQL/Redis及其私有配置不变。

### 新方案制品与未覆盖

POLICY/RELIABLE/公共READ尚无同一已实现、已验的commit/image/binary及完整有序迁移包，不能交付新方案可执行切换单。本机已有认证/journal/READ局部候选及其证据不替代统一制品；006/007与后续新迁移仍须在同一候选核顺序、新库/升级及旧写者兼容。原PERSIST-only 4ecec887是另一条旧修复复验候选，无SQL/proto/依赖改动，沿0ae单独确认，不冒充全新方案。

本批交付可达性/计数时点及正式源码契约和操作稿，未实现或验证真实签名POST、PASS/REJECT事件、业务审计/DB关联、缓存新增恢复或新方案部署。需要BIZ身份/审计材料及二当家明确的新执行范围与窗口；当前无需变更监听、配置或服务。

## 22. Rocky 原 PERSIST-only 固定制品与复验稿（2026-11-09，未切换）

来源二当家 `pbz4w6wdnjwhukeh`，有效批次 `01c5c1097c7062b8e4afbe7da6197f6a27c634a3d331abc771684e0dcef6ea5c`。范围只含原 `4ecec887fb38d2d81af4bf2773b05d304e8ced97` 复验准备，不包含本机 POLICY/公共 READ/序列后续 HEAD，不授予启停、部署或迁移。二当家报告主人仍在手测、接入码 POST503 待 BIZ 诊断；这不是本批重现的业务错误。本轮已在本机构建二进制、向 Rocky 本任务新目录暂存文件并构建一个候选镜像，**没有启动候选服务或替换运行容器**。

### 固定候选与本批实际检查

完整机器清单见 [manifest.json](evidence/20261109-dg2-persist-only/manifest.json)。二进制/归档保存在本机忽略目录 `.local/rocky-im/DG2-20261106-002/persist-only-01c5/`；Rocky 暂存目录为 `D:/projects/.evidence/DG2-20261106-002/im/persist-only-4ecec887-01c5`。不包含运行配置、凭据或消息正文。

| 产物 | 固定值 |
| --- | --- |
| 候选提交 | `4ecec887fb38d2d81af4bf2773b05d304e8ced97`；比较基线 `b87fc791400dafae0f3fb2e1665863419bad58f7` |
| 源归档 | `source-4ecec887-canonical.tar` SHA256 `2ebcb383e173bafd58c4165ecce557852f13e3ad2e5da0da7ad5e17eb63a5cbf`；742普通文件，从固定 Git archive 导出，指定文本扩展 CRLF→LF，排除 `launcher/conf/config.yml`，不读取工作树增量 |
| 新 Linux/amd64 二进制 | 103760056字节，SHA256 `551de72723dc3c4917c8666f67e8165a094800769f050299e1df53b19838b740` |
| 传递包 | `persist-only-4ecec887-01c5.tar`，103772160字节，SHA256 `1aae607e309198aba274179646a07287d656f4fcaa7a11265a26fad5eb4bfcfa`；只含二进制、Dockerfile、两个 image-only Compose override 和准备脚本 |
| Rocky 候选镜像 | `bitcomputing/im-server:dg2-rocky-persist-4ecec887-01c5`，ID `sha256:ed9e1addeb5344722faa92dedbe082597655fe3a77720381ca58ef90401f7145` |
| 固定原运行镜像/回退目标 | `bitcomputing/im-server:dg2-rocky-b87fc79-diagnostic`，ID `sha256:09186cb700fdb2e4fe21e163494af2c9dc1e81cb69e5fa1fdf8658b848ef45a7`；活体 binary SHA `bfdcaa5d73721472cce9af5b3debcd525f24ab0cfabe2ca1f3cb373540510c9a` |

本批用缓存构建镜像 `sha256:49be686fa97bc62ee3d408d1f2208c8e6ce613941b16763ece970ff6c4366349` 内 Go1.25.12/Linux/amd64，network=none、GOPROXY/GOSUMDB=off、GOTOOLCHAIN=local、GOFLAGS=-mod=readonly、CGO_ENABLED=0，实际执行 `go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/imserver ./launcher` 和 `go vet ./services/conversation/...` 均 exit0。只挂载固定源码与空输出目录；临时本机构建容器 `--rm`，验后按本任务 label 查得0个保留容器。

SCP 后五个文件 SHA 逐项与本机匹配。Rocky Dockerfile 仅以原诊断镜像为 FROM、添加来源标签并 COPY 新二进制；build exit0，基础层前缀完全相同，仅新增一层，Entrypoint/Cmd/WorkingDir/User/ExposedPorts/Volumes 分别相同。未执行镜像中的新二进制或创建候选容器；06:20:35 UTC 按该候选 image ID 查询容器数0。不是启动、原生消息或 DB 通过证据。

第一次长 EncodedCommand 调用 exit1、返回无法可靠解码的错误。随后只读回查候选镜像0、build log/result不存在、已暂存二进制存在，原运行容器不变；没有把失败当成构建完成。改为上传同范围准备脚本后才有上述成功构建。脚本、文件哈希和前后快照均留在忽略目录；保留候选镜像/暂存目录是本次交付物，不清理其他人的镜像/卷。

### 相对 b87 的变化、迁移与兼容性

生产变更仅两个文件：`services/conversation/services/convercache.go`（SHA256 `d1638c461d2d3b200e4e36f139744bd0fd72605ab32b3e4062455685d3edeac5`）和 `converservice.go`（`38979c8fe471fe4a93d292f52cdac8720c481214cafeab870e9fb36b6fa5f37a`）。其余是 AGENTS、既有报告及三个测试文件；没有 SQL/迁移、proto、依赖、启动器或配置变化。

- 延迟刷新消费最新冻结快照、深拷贝扩展，按用户串行提交；配额满非阻塞重排，暂时/未知存储错误重试，坏数据错误丢弃对应快照；缓存重载合并尚未落库的新状态。内部缓存键改为无歧义编码，不改变数据库键或客户端协议。
- 全部已读先核 SQL 成功，再改缓存及排队快照；与在途 worker 共用提交锁。SQL失败返回既有内部错误，不能发成功通知。这不是独立 READ/last_seen_seq，也不修复专用群权限或可靠发送/回调。
- **无迁移步骤、无 DDL、无数据回填。** 表模型、原 SQL 和 wire 与 b87 相同，具备源码级读写格式兼容依据；新镜像对 Rocky 现有数据及重启后的实际兼容性尚未复验。§9/§10 的旧数据库/原生记录仅作同源背景，不算新二进制本轮通过。
- 刷新队列仍只有内存快照；异常退出可丢未落库状态。不能以“等60秒”代替 SQL 实查。旧 b87 另有摘要落库缺陷，持久数据保留不等于其进程内缓存可恢复；若切换前无法核清待刷状态，应报告影响并停止，不导出/重放消息或伪造摘要补齐。

### 后续获批窗口内的复验稿（本批未执行）

1. 先获 BIZ 明确释放手测，确定同租户现有合法发送者、接收者、可读会话及真实业务 Token 获取路径；不新建/替换现有主人映射，不传 Token/Secret。BIZ 的503诊断与本 IM 候选分开。窗口核准前还须明确“待刷确认＋停止/重建/启动＋健康等待＋真实恢复核验”的完整预算、最晚新动作时点及未持久化的异常退出方式；`--wait-timeout 60` 只限制健康等待，不是整段操作上界。现有材料没有这份实测预算，不能声称全流程适合5分钟；预算不明确时不提出可执行时段，先由二当家缩小步骤或拆成分别确认的窗口。仅这些材料齐备且 BIZ 释放后，才提出新的未来5分钟起止，由二当家确认**同一**候选/范围/回退与执行人。旧06:00–06:05 UTC 提议已过期、未获 BIZ/二当家确认、未执行；当前没有新窗口或定时任务。

   二当家后续 `pbz4zxr6wkjhukeh`（有效批次 `74e672d2`）进一步明确：释放证据须列明所有实际测试参与者，覆盖主人本人及其对应的停止测试确认、有效时段和范围，由 BIZ 协调汇总可核验记录。BIZ无在途请求、容器healthy、旧浏览器关闭均不能代替参与者已停止测试。当前未取得这份完整释放证据。该消息审查的旧 `window-replan-0ae6c577.json` SHA256仍为 `6d01851ff443b542d3f6fccdaeed776f3c9fd44e0dced07b2c60da5df18ee2ed`，作为04:56UTC历史提议原件保留；后续固定制品见本节/清单，不把新制品倒填为旧窗口已批准。
2. 窗口前再次确认原镜像/活体 hash、租户 `dg2c3350172f150`、用户 `test_6199931265070805`、映射、既有历史、卷及 loopback 19001/19002/19003/18090；只记录脱敏元数据。暂停本轮写者并用实际 SQL/摘要核待刷状态，不通过则停止；不得把旧浏览器关闭或容器 healthy 当空闲/数据已刷证明。
3. 只对原 Compose project `dg2-rocky-im` 的 service `im-server` 叠加 `candidate.override.json`。该文件只有 image，不改原 `D:/projects/.evidence/DG2-20261106-002/im/compose.json`、私有配置或挂载。实际容器标签指定原 `stack.env`，且脱敏 Compose 有 DG2_MYSQL_PASSWORD 插值，必须保留同一个 `--env-file`（本批只核标签/文件存在，不读出凭据）；不得在缺少该上下文时执行。以 `--no-deps --no-build --pull never` 限定 IM 重建，BIZ/MySQL/Redis 均不启停。切换后核新 image ID、活体 binary hash、原端口/卷/映射与健康；不符先停止新步骤，只对本次可核定实例按已批准的持久化/回退分支处理，不按旧快照操作他人已变更的实例。
4. 在已获批现有会话用已授权用户发最窄计数/不计数对照，分别核发送 ACK、接收端实际收取、历史消息 ID、最后摘要；普通 read/全部已读的 readIndex/unreadIndex/unread 先取得基线，再核实际 SQL。等候真实刷新并确认 DB 行命中，不把 ACK 或零未读当 last_seen。不要直接运行旧 `simulator/probes/mindlink` 夹具：它硬编码 F0 管理登录/建租户/端口，不适用于 Rocky 既有租户。
5. 只有实际 DB 已命中，且按已核准的完整预算还能容纳重启、核验和安全回退，才按批准范围 IM-only 重启并重连，核同一消息 ID/摘要/合法阅读状态；不重发同一条业务消息“补证”。本次已持久新增消息继续保留，未持久部分明确列未知，不声称已保全。最晚新动作时点必须早于上述完整恢复预算，不能固定写成“第4分钟回退”。预计装不下时提前停止新增测试写入并报告，未覆盖如实记录；拆分/重排必须重新确认，不沿用过期窗口。

**未落库时的安全退出须先获明确批准：** 停止新增测试写入，保留当前实例/卷及原结果，不因时钟触发重启、换镜像、删缓存或重发；报告二当家决定受控等待/新的窗口。该分支不自行延长停机许可，授权到期后不启动新的停机或切换动作。未明确接受这种异常分支，就不开始会产生写入的复验。数据保留优先于赶一个未经验证的恢复时点，不能把“DB未命中”改写成“已安全回退”。

### 保留数据的回退稿（本批未执行）

先暂停新增测试写入、实际确认需要保留的状态已持久化或确认本轮无待刷写入，并核剩余授权时间足够完整恢复；未确认则走上面的安全退出分支，**不得直接停止候选**。满足这些前置后，在同一原 project/service 和原 Compose 文件上只换回 `rollback.override.json`，预检其镜像标签仍对应上述 `09186cb7…`，保留全部原挂载及本轮已持久新增消息；不用旧数据库备份覆盖、不删除数据行/租户/用户、无 `down -v`/volume rm/prune。恢复后再次核旧 image/活体 hash、loopback端口/健康、同一现有用户登录/CONNECT、已确认持久的历史 ID 和数据计数；失败则停止后续业务测试并报告，不自动扩大到 BIZ/DB/配置修复。镜像回退会恢复 b87 的旧缓存/摘要缺陷，不能称 PERSIST 修复仍在，也不能保证未持久缓存可还原。

获批后才可使用的命令骨架，当前没有运行（须先逐项核实当时的 image ID/上下文/期限）：

```powershell
$original='D:/projects/.evidence/DG2-20261106-002/im/compose.json'
$prepared='D:/projects/.evidence/DG2-20261106-002/im/persist-only-4ecec887-01c5'
$originalEnv='D:/projects/.evidence/DG2-20261106-002/im/stack.env'
# 仅获批切换时使用；先核原 project/service/image 与批准单一致。
docker compose --env-file $originalEnv -p dg2-rocky-im -f $original -f "$prepared/candidate.override.json" up -d --no-deps --no-build --pull never --wait --wait-timeout 60 im-server
```

仅获批回退时使用下列独立命令，不与切换块自动串联或失败后盲目重试：

```powershell
$original='D:/projects/.evidence/DG2-20261106-002/im/compose.json'
$prepared='D:/projects/.evidence/DG2-20261106-002/im/persist-only-4ecec887-01c5'
$originalEnv='D:/projects/.evidence/DG2-20261106-002/im/stack.env'
docker compose --env-file $originalEnv -p dg2-rocky-im -f $original -f "$prepared/rollback.override.json" up -d --no-deps --no-build --pull never --wait --wait-timeout 60 im-server
```

本轮前后实际读回运行容器均为 `d16f0401…e3172a`、旧镜像09186cb7、旧活体hash及11-06 07:36:43UTC启动时间；BIZ PID26644/11-06 11:08:43UTC未变。这些是观察时点快照，不保证持续健康或环境空闲。本批未查询/更改业务 DB/Redis、未发原生消息、未重启/回退、未改监听或私有配置、未操作pre；当前缺 BIZ 释放及二当家新窗口，不能执行上述稿件。

材料初审 `review_dg2_persist_material` 为 Critical0/Important1/Minor0：原“第4分钟/末60秒回退”没有覆盖完整恢复耗时，且可能停掉未落库状态。本稿已删除该无条件路径，增加完整预算与未持久化安全退出的先行审批门；原失败审查保留在本任务忽略目录，不把初审计作通过。实际运行/回退仍未验。

同一审查方只读复审为 Critical0/Important0/Minor0，关闭上述材料问题；未执行 SSH/Docker/脚本或测试，不构成运行验收或执行许可。BIZ释放/合法测试身份与Token获取路径的请求已发送 `pbz59zqb6sshukeh`；本批已核材料中尚无其明确释放回执，不提出具体新窗口。
