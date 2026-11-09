# Native READ blind-list candidate — not accepted

DG-20261104-003/F1-READ；本轮有效消息 `pb2dwmyfx6nhukeh` / batch `2f8c434d`。原生普通规模链路已验证，但一万候选的分页回归仍失败，**不能将本候选或整个 F1-READ 标为通过**。

## Version and ownership

本轮分支 `feat/mindlink-read-lists` 基于原 IM 固定 `028bb2aa5359c96cc67337814b094239ba0b99a4`，使用既有独立 ffc4 worktree。原 `feat/mindlink-read` 仍指向 `dcdd3a30ba4b802dce11ae396dc8f3711755ed4f`；没有把它或原 IM 其他提交合并进本分支。旧 READ 交付不重发，当前代码需与其由原负责人串行整合。

固定执行源为 028 的 canonical LF archive 加 verification.json 中 **15 个 Go overlay**。本地候选 commit 由交付回执记录，其 Go blob 必须逐项对应这些哈希。计划/证据文档不影响执行制品。

| Artifact | Value |
|---|---|
| Source archive SHA256 | `fed7f9a52a0cc89c29485ea10174f97184c9c385893e35cdf0c472e43ae09972` |
| Linux binary SHA256 | `8b1ef62515940a2f6eaf47cde1320c55bfdf49b961da9770343264ba92cc943b` |
| Owned image | `bitcomputing/im-server:f1-read-list-cold-fed7f9a5` |
| Image ID | `sha256:01d61469ff514bdd639594d44b8df5d97739814d2382f2a5b925bd578af59881` |
| Toolchain | Go 1.25.12/Linux, existing GORM and MySQL 8.0; no new dependency |

只写 conversation 读取服务/缓存、四个必要 actor、global DAO/interface、独立测试/probe及本包文档。共享 bases/mindlink、HTTP注册、proto/生成件、迁移/建库SQL、历史服务和最终消息投递文件未修改。全局列表 DAO 的 Go interface 增加 context，四个读取服务返回实际 error code，actor ACK 不再无条件成功；外部 PB 字段及 HTTP 路由不变。

## Implemented behavior

- Ordinary native qry/sync/top lists classify candidates before their page limit; unread totals classify before summing. Active and permanently disabled projections are excluded, native collation aliases are preserved, ordinary prefix lookalikes without a projection remain visible.
- A request-local copy binds cache identity fields, indices, counters and tag filters. Current persisted history is resolved by the copied `LatestMsgId`; obsolete cached bodies are ignored. Cold mentions are resolved in bounded SQL batches before classification, preserving the existing latest-100 and read-index/unread mode. Cache locks protect only copying, not policy/history/mention SQL. This is an ephemeral read view, not another persistent seen authority.
- The global list excludes authoritative projected senders/private or system targets and groups containing such members **before SQL LIMIT**. It uses the verified native fence collation and checks requester/output identities. The tested policy/history/mention/global-query failures are non-success, including empty lists/totals. This does not establish coverage of legacy conversation-cache loader failures or untested backends.
- Native unread/tag/include/exclude/channel behavior, ordinary tombstones and ordinary count indexes remain independent of the new visibility filtering.

The identity classifier still has an important performance defect: after its batch any-forbidden check, it falls back to one lookup per candidate. It is intentionally retained as a visible failing regression until the shared classifier interface is provided; it must not be enabled/deployed as a finished change.

## Actual verification and limits

| Check | Actual result |
|---|---|
| Initial real MySQL RED | unread returned 204 instead of ordinary 4; native list/pinned/sync/global exposed protected records |
| Cold-state RED | current-ID body not inspected, obsolete cached body hid an ordinary row, cold mention changed total to 11, missing mention source returned successful 4 |
| Corrected native component cases | 16 top-level PASS, including cold/current-body/mention errors, aliases/disabled/requester/filters/paging/tombstones and pre-canceled reads |
| Supported routine packages | 53 top-level PASS, 11 F0 fixture SKIP, 0 FAIL **with the known large regression explicitly excluded**; skips and exclusion are not passes |
| vet / launcher build / probe build | exit0 under explicit two-worker limits; exact source/artifacts above |
| Large regression, separate real MySQL run | **FAIL:** 10,000 cached candidates, Count1, SQL_delta3848, 2005ms, code19007. Its following aggregate segment did not execute |
| Actual rebuilt native HTTP / WS | corrected probe exit0 in 68.73s; signed native list/global HTTP, two authenticated owner Web/PC sessions, pinned/sync/list/total; ordinary peer ACK ID actually reached both sessions and matched persisted private history/summary |
| Warm authority changes | inserting a projection after the cache was read changed native visible list/unread 2→1; changing it to disabled kept it hidden |
| Authority failure and recovery | only the owned scratch authority table was temporarily renamed; signed HTTP and WS returned non-success/no data; restored table and real reads passed |
| IM-only restart / real DB | fresh process queried list/unread1; scoped conversation metadata identical, protected two rows retained unread200, ordinary summary matched ACK; disk/live binary SHA matched before/after; MySQL StartedAt unchanged |
| Resource audit | no scratch schemas or transient runners remained; owned MySQL subsequently stopped exit0; its data volume/network/images/private evidence retained |

These are synthetic, independently owned native HTTP/WS fixtures, not owner business API or real browser/App page acceptance. The 200 protected unread fixture is deliberate legacy dirty-state coverage, not a statement that new no-count messages should increment it. The new source does not include the old native seen/proto/sequence branch; a combined fixed artifact still requires the original integrator.

## Independent review and decisions

One fresh read-only review of predecessor archive `107f7dea…` reported Critical0/Important3/Minor0. It did not run runtime/DB tests. Important1 (incomplete cold identities/stale body) has explicit real-MySQL RED→GREEN coverage in the new source; Important3 (probe interpreting a returned error ACK as transport absence) has the failed predecessor runtime and successful corrected runtime. Important2 remains open, independently confirmed by the measured large regression. This is not a second independent review or a claim of zero remaining findings.

Ruling: keep shared identity extraction/authority with the original IM owner rather than duplicate a prefix/Go-case-fold policy in READ. Cost: full delivery depends on that exact interface.

Ruling: use a request-local cache/data snapshot so the aggregation and identity view cannot change merely because another read warms a lazy cache. Cost: bounded pre-resolution queries; direct history factory in-flight cancellation and live Mongo were not validated.

Ruling: distinguish the client's returned ACK status from transport absence; a nonzero ACK is a valid observed failure, not proof that nothing ran. The first runtime failed at the faulty helper and was never counted as a pass.

Ruling: run routine checks excluding the known large failure only to assess other regressions, while retaining that actual failure as a release blocker. Cost: no all-green/full-acceptance claim.

## Concrete dependency / next work

Original IM stable owner `01a1a0e1-b344-7501-bc44-fadc4a29d403` has requests `pb2etakfx9ehukeh` and measured follow-up `pb2e9tpz792hukeh` for:

```go
bases.FilterNativeMessages(ctx context.Context, appkey string,
    messages []proto.Message) ([]bool, error)
```

A bool must correspond to each input ordinal, true only for an ordinary native-visible record. Reuse its existing recursive identity extraction and authoritative native projection/current group membership, including active+disabled, native collation aliases, entity namespaces and cross-app separation. Unknown/not-ready/SQL/cancel errors fail the whole batch; bounded lookups must replace per-candidate round trips. This API is not an owner grant or a new authority writer. The exact fixed provider commit/signature is still **not supplied / unverified**. Shared package files were not taken over.

Once the original owner supplies the API or explicitly releases its precise independent implementation files, wire the read classifier, make the large list and previously unexecuted aggregate assertions pass, and rebuild/re-run the affected final artifact. Until then this candidate is WIP, not F1-READ completion. Owner mapping/page acceptance, App/push/badge, live Mongo, combined seen/proto/sequence/forward-migration integration and production authority/release remain unverified. No push, merge, deployment, shared-DB operation or historical delivery retry was performed.

Private raw logs/config/credentials and the prior failed artifacts remain ignored in `.local/f1-read/blind-lists-20261109/`. Initial logger/cluster fixture faults, unsupported Python tar filter, Windows CRLF shell setup, Windows checkout normalization guard and package-enumeration ENOMEM are excluded setup attempts, not passed branches. The bounded rerun does not erase those failures.
