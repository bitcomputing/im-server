# F5 首条纯文本 DM 链：IM 实际输入与缺口（2026-11-09）

初次回应 BIZ `pbz8grg466ehukeh`，当时有效批次 `3d83edc11806a23f2880cd4a645f4d0966298998c4846dd0782aa7f6d15ddb77`，沿 DG-20261104-003/c82 原写集核依赖，不另起READ任务；本轮当前补核批次见下方21e33节。唯一业务契约仍是 chatbot43 固定 `46c25b90b0013b6cbb425224f572381692300e7f`；本文是源代码能力盘点和最短接线建议，不是第二份路由/DTO契约。

## 固定源与结论

本批读取 IM F1 `4cde55704cab0448b21d4d073d69c7ed281e0494`、BIZ上述46c25、Web `aa8aa3f15b1507984790b6d242a1bf74c8dfd68e` 的正式源；IM主仓 `e12609146c41150023874d63622fbdd39c76fc62` 的apigateway路由与该F1版本相同。Web依据为固定 `docs/test/2026-11-08-mindlink-web.md` 的唯一交接表及 `src/components/mindlink/client.ts`，其model/client仅是视图/适配边界，生产仍为unavailableClient，不能反推冻结HTTP。

**IM为“owner JWT load/openDM→ensure→send→原键result→授权history”新链注册的路由为0。** apigateway主路由有77条既有group注册，但无 `/mindlink` 新路由；旧私聊、历史和管理能力不计为新链。JWT由BIZ认证，内部签名/密钥不给浏览器。此0只说明IM新接线，不能读成BIZ所有既有JWT/管理API均不存在。

| Web需要 | 当前IM可调用输入 | 新链仍缺什么 |
| --- | --- | --- |
| load | 无新Mindlink加载路由/端到端DTO；旧convers不含完整业务映射/能力 | BIZ正式owner JWT响应、开关、稳定owner/avatar/会话映射及当前权限；IM权威投影与结果对应 |
| openDM / owner-DM ensure | 无新ensure路由；principals.ensure完整body仍关闭，conversation.ensure仍候选 | 合法永久认领的有效avatar、固定principal/owner映射、DM固定参与者/代际/policy及持久原操作；不能降级users/register或普通建群 |
| send / 原键result | 仅有普通 `/apigateway/messages/private/send`；新messages.send/query未注册 | 严格owner/目标/原键/摘要绑定、当前权威/隔离/提交屏障、消息历史＋结果/outbox原子持久、未知沿原键查；普通ACK及code0不是persisted |
| history | 既有 `GET /apigateway/hismsgs/query`，见下节；无新受限history路由/DTO | 当前读权/固定会话/目标绑定、错误与空页区分、正式cursor/顺序及无损真实序列；N1尚无history操作schema |
| seen | 公共HTTP仍0；二管家原生/WithTx交付不等于公共接线 | 原权限/同事务快照/完整公共回执接线；本次不并写其原生源、不建影子seen |
| manage / remove | 原BIZ管理另有其入口；IM新principal disable/query仍未注册；remove不是删历史 | 当前代际/生命周期/原结果适配、IM最终撤权屏障；不能把首条DM当全部管理或F5完成 |

已有 `PrincipalDisableRequest`（8字段）/`OperationResult`（22字段）只是已实现的解析/值类型；它们属于N1首条控制面，不是DM ensure/send/history的完整DTO或producer。错误字典19001–19008及400/403/409/503映射已落源，`ErrorHttpResp`按HttpCode输出；新链条件归因和端到端响应尚未实现。对应源为 `services/apigateway/routers/router.go`、`commons/mindlink/operation.go`、`commons/errs/errorcode.go`、`commons/tools/httpresp.go`。

## 历史序列：正式来源已定位，HTTP两端均丢字段

1. `commons/pbdefines/appmessages.proto` 的DownMsg.msgSeqNo是int64/tag6。`services/historymsg/services/hismsgservice.go:QryHisMsgs` 在当前requester/target/channel/sub_channel路径读历史并解PB；PB内序列<=0时从该历史行MsgSeqNo补值。这里的当前授权读取侧和scope必须保留，不能改用发送ACK的seq或另一侧/另一会话的值。
2. `services/apigateway/apis/hismsg.go:QryHisMsgs` 已收到DownMsgSet，但构造 `models.HisMsg` 时没传MsgSeqNo；`services/apigateway/models/msgmodels.go:HisMsg` 也无序列字段。BIZ `lib/imServiceLib/juggleClient.go:HistoryMessage` 同样没有序列字段，因此只改IM一端仍不能交给Web。
3. 旧入口使用租户Signature；from_id从query传入，再由 `services/apigateway/services/common.go:ToRpcCtx` 写RequesterId、IsFromApi=true。这不是由BIZ JWT/已验证actor产生的新主人权限。N1的messages.query是原结果查询，operations.query是原操作查询，conversation_seen.query是READ快照，均不能借名当history授权。须先在唯一源登记受限history操作/严格DTO/签名与当前读权绑定，保留已有删除/可见性等历史规则，最终读取也要经过权威权限门。
4. 原QryHisMsgs私聊路径只在storage err==nil时填充，函数末仍返回SUCCESS；因此新链还须区分DB查询失败与合法空历史，不能沿空页宣称查库成功。这是本批源码观察，未注入故障或复现DB现场。

**可补的最短IM→BIZ输入建议，尚未实现/冻结：** 在正式授权历史响应的每条消息中增加规范十进制字符串序列（暂名 `msg_seq: string|null`，最终字段名由唯一契约登记）；仅取同次授权返回的DownMsg.MsgSeqNo。正值用Go `strconv.FormatInt(value,10)`，全程不经float/JS Number；<=0/缺失/未能证明的值显式未知，不补0、不推进seen。BIZ消费类型须同步用字符串/nullable承载，再适配其正式owner响应；不能靠Web对数字转String补救已损失精度。只补普通HisMsg字段不产生新权限，也不能把旧签名接口直接交浏览器。

该建议需验证1、9007199254740993、9007199254740994、9223372036854775807的精确字符串，0/缺失及跨owner/tenant/会话拒绝、错误不当空页；最后用真实授权历史/SQL与同消息ID对账。本批所持材料只有固定SDK/前端静态映射及合成字符串边界材料，**没有本批SDK真实收取/历史和精度通过证据**，不以model.sequence或fixture替代。ACK/id/time/unreadIndex/摘要floor均不代替该序列。

## 最短可交顺序与必需前置

- BIZ先提供合法owner JWT与有效永久认领avatar、正式load/openDM/纯文本send/result/history的唯一DTO/错误/能力边界、固定映射及wire子意图/原父绑定。此前159/160均无永久认领的报告不能证明有效avatar；160为主人也不能被改作独立avatar。owner授权与avatar认领分别核实，不凭计划补造。本批不启用账号/造密钥。若需首次开号，principals.ensure完整schema、80标量/320字节读写/有序迁移仍是实际依赖；没有现成权威内部号的证据就不能假设已具备。
- IM按原写集完成该DM所需ensure/策略与全部相关入口的最终权限/隔离屏障、可靠send＋原键结果，以及独立受权history/字符串序列承载；同进程复用同一sql.Tx，确需跨Actor才落实受信封装。N1 disable/query的字段确认不自动生成这些DM能力，不重做efcc认证，不借92文档当注册依据。
- 有同一实现版本、迁移/能力配置和合法主体后，再接BIZ owner适配与Web对应动作。先核单向技术纵切的发送ACK、目标实际收到、原键持久结果、授权历史同ID/真实序列和SQL，覆盖未知/重复、撤权/跨scope拒绝及普通聊天回归；读到合法空页与验证失败分别记录。**真正“可用对话”还须BIZ/F3合法execute收件并沿原DM回复**，再核回复的可靠结果、owner实收及同DM授权历史/序列/SQL。原请求、目标avatar/代际、固定DM及回信坐标须一致；唤醒、transport投递、helper code0或单向历史不能当execute收件/回复闭环。
- READ回写、媒体、push另列覆盖门，权限/隔离/可靠持久化仍是技术纵切必需；seen/manage/remove等未交付功能继续失败关闭。技术纵切通过不等于双向可用对话、完整F5、App/push或原c82/c328/READ结案。DG2 PERSIST探针的S/R/G1/G2不是这条单DM链的业务拓扑，不能移用其准备材料证明本链就绪。

## READ原件与双向闭环补核（21e33b30，沿原c82）

回应 BIZ `pbz8lxdsn7ehukeh`，当前有效补核批次 `21e33b3072a2181f8967626d7c70588331c67f00811cdd542967c8a2bb7cf60a`。本批实际只读 `d336dec4cae9eff4067938b18e05215a5a0c60fe` 的 `docs/evidence/20261106-mindlink-read/transaction-adapter-handoff.md` 和seenservice.go：QryConverSeenWithTx/SetConverSeenWithTx仅将conversation_seen I/O加入传入GORM事务，`seenLatest` 的成员/设置/清理/历史仍走既有storage调用，不在该事务快照内。code0只表示helper已读/暂存seen，不能证明公共权限、原query快照/回执已同事务提交。既有helper原件及其边界继续接收，不要求二管家反复重做。

公共producer、owner屏障和journal仍由原IM在原写集整合：须补可靠的同授权历史/latest及当前权威快照，明确同一执行方/同一sql.Tx的效果与回执边界，不能只套WithTx便称全链同快照；当前严格同事务公共producer仍缺。已有SQL→GORM桥接形态和原验证记录见[READ事务交接](mindlink-read-transaction-handoff_zh.md)，本批不把那些旧测试变成新实跑。同进程复用传入sql.Tx，跨Actor只能传原JWS/method/path/body并在最终方重验，不传gorm.DB/sql.Tx或另开池提交。

调用方任何SQL失败都必须终止并丢弃整个caller tx，停止effect/回执生产；不得只重试helper的一句SQL、忽略错误、使用已由服务端回滚但仍可调用的句柄，或回落全局连接。若按规则恢复整个原操作，须在新事务中沿原ID/摘要/子意图先核原结果及当前权限，不能重发未知业务效果或新造键。helper的局部回滚保护不替代调用方遵守此规则。

本批另实读Web固定aa8的ConversationView.tsx：自动IntersectionObserver/seen effect以 `c.kind !== 'avatar_pair_group'` 提前返回，owner-DM不走这条自动seen。故可将owner-DM技术纵切与pair/多端seen分别设验收门，不借此省略当前权限/隔离/可靠持久化，也不把未覆盖的READ行为记为通过；BIZ/F3 execute及原DM回复另有真实闭环证据门。

IM当前sender/history新路由仍0；上一批a86ead9的字段丢失位置和 `msg_seq:string|null` 最短承载建议保持未实现/未冻结。d336交接中的 `msg_seq_no` 与上批暂名 `msg_seq` 都须由唯一源明确一个正式字段，不静默接受多套别名。最小sender输入必须来自已授权业务纯文本原意图和持久wire子意图，经权威映射绑定sender/目标/代际/policy及原键；不采用浏览器自报sender或query from_id授信。BIZ/F3收件和回复适配归其原写集，READ二管家继续只负责其原生范围，不起重复任务。本批未改Go/SQL/READ/Web/BIZ源、未合并分支或执行账号/key/Rocky操作。

3d83原盘点仅核固定源码及正式交接表，未改Go/SQL、BIZ/Web源或定义第二份正式schema，未运行组件/SDK/UI/DB/HTTP/native，也未读取凭据、连接Rocky或创建账号/密钥。其源文件哈希及0路由核对记录保留本机忽略目录 `.local/f5-dm-inputs-3d83/source-audit.json` 原时点；21e33补核另记 `.local/f5-read-boundary-21e33/source-check.json`。两批均无可执行的新链制品或运行通过结论。
