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
