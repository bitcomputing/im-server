# Native conversation list and unread filtering

> **For agentic workers:** Execute inline with superpowers:executing-plans and TDD, then one fresh read-only whole-patch review. No implementation delegation.

**Goal:** Ordinary native conversation lists, sync, pinned lists, global lists and unread totals must omit internal identities and their groups without losing ordinary pagination or reporting policy-storage failure as successful empty data.

**Architecture:** Reuse the fixed owner's `bases.CheckNativeIdentities` / `CheckNativeMessage` fence, never a name-prefix heuristic or another authority table. Classify cached candidates before pagination and unread aggregation. The global SQL list must apply equivalent authoritative exclusion before its SQL limit, preserving current native collation and owner fence readiness. Propagate errors through the existing actors and HTTP callers.

**Tech stack:** Existing Go 1.25.12/Linux, GORM/MySQL 8, protobuf and native RPC.

**Spec:** DG-20261104-003/F1-READ current authorized correction `pb2dwmyfx6nhukeh` (batch `2f8c434d`), original released conversation scope and product IM-D homepage separation; owner fixed native policy at `028bb2aa5359c96cc67337814b094239ba0b99a4`, including `8035fed`. Coordination `pb2dzl6cx7jhukeh` identifies the independent write set.

## Constraints and decisions

- Preserve `feat/mindlink-read` at `dcdd3a3`; this work uses the existing isolated ffc4 worktree on `feat/mindlink-read-lists`, based on 028bb2a. No merge is performed.
- Do not edit shared bases/mindlink, HTTP registration, proto/generated files, migrations, policy projections or message/final-delivery files. Original IM owner serializes those changes and eventual integration.
- The approved product and current continuation authorize implementation; do not reopen a product-design approval round. This plan records implementation choices, not new product features.
- All fixtures, ports, DBs and containers are independently owned. Do not use the running `im-policy-c82` or other owners' services. Private credentials/raw evidence stay ignored.
- Fail closed on fence absence, SQL errors and cancellation. A forbidden list item is omitted; a forbidden requester is an error. Ordinary identifiers that resemble internal names remain ordinary without a projection.
- Cache pagination currently caps its loaded population at 10,000 entries. Filter the existing candidate snapshot before taking the requested page; retain ordering/tombstone semantics. Reuse existing unread filters and marker behavior.

## Files / callers

- `services/conversation/services/converservice.go`, `convercache.go`, a READ-local native filtering file and tests: list/sync/top/unread, ordinary filter semantics and error returns.
- `services/conversation/services/conversationservice.go`, conversation global-list DAO and tests if required: global authoritative exclusion before SQL limit.
- Actors `qryconversationsactor.go`, `syncconversationsactor.go`, `qrytotalunreadcountactor.go`, `qryglobalconversactor.go`: return service error codes, avoid nil logging. Existing top/single actors already return codes; inspect them too.
- Independent tests/probes under conversation and simulator, and task evidence. No public wire changes.

## Review focus

1. Active and permanently disabled identities, collation aliases, groups containing such members, private/system scopes, and ordinary prefix lookalikes.
2. Hidden entries before/inside page boundaries, all-hidden pages, ascending/descending and pinned ordering, sync tombstones, and global SQL limits/timestamps.
3. Unread tags, tags/include/exclude/channel filters, deletion, concurrent cache changes and unchanged stored ordinary indices.
4. Missing/failed authority and canceled reads must produce a non-success ACK/HTTP envelope, including empty lists/totals.
5. Existing identity extraction must also guard returned latest-message/mention identities; no shared-policy bypass or unbounded network operation under a cache lock.

## Task 1: Cached reads

- [x] Add a real owned-MySQL regression that seeds protected and ordinary identities/cache rows. Observe unread/list leakage on 028 before implementation.
- [ ] Implement the smallest shared native classification used before list/sync/top pagination and aggregation; preserve ordinary count/tag rules and propagate errors through every caller.
- [x] Verify the ordinary-size mixed/all-hidden/alias/group/tombstone/pagination/error cases. The separate large-population case remains a real failure; no DB queries while holding cache locks.

## Task 2: Global list and actual RPC

- [x] Reproduce global blind-list leakage and SQL-limit filtering behavior with a real owned DB.
- [x] Implement authoritative exclusion before limit, preserving the owner's gate and current schema; test failures separately from empty success.
- [x] Build the fixed candidate artifact, exercise actual native HTTP/WS list/sync/top/total/global responses and ordinary receipt/history/summary behavior, and verify scoped DB results/unchanged protected state. This is ordinary-size native acceptance only.

## Delivery

- [ ] Run gofmt, supported affected tests/vet/Linux build; record exact source/binary hashes and all failed/skipped branches.
- [ ] Obtain one fresh independent review, resolve material findings with red/green tests, and commit task-only files locally.
- [ ] Send only this new fixed commit/base/precise patch/evidence or concrete remaining integration conflict. No repeated dcdd delivery, push/merge/deploy or shared DB changes.

## Execution record

- Baseline: canonical LF 028 archive SHA256 `39cb50fb75fba5d8aedbfdfa8cb5a17383c6230f34abd6ac867fa413d87300e7`; supported isolated `go test -mod=readonly ./services/conversation/... -count=1` exit0. Fixed-fixture tests were not enabled, so this is no integration acceptance.
- Initial archive extraction attempt used an unavailable Python 3.11 `filter` argument and stopped before extraction. Path/link-checked extraction then succeeded; not counted as a test pass.
- Independent review of source `107f7dea…`: Critical0/Important3/Minor0. Cold/current identity completeness and the error-ACK probe bug were corrected with explicit RED→GREEN evidence. The mixed-population serial-query defect remains open.
- Current normal-flow source `fed7f9a5…`: 16 native component cases passed. Supported routine tests explicitly excluded the known large regression: 53 top-level PASS/11 SKIP/0 FAIL; vet/build/probe compilation exit0. No all-green suite claim.
- The separate 10,000-candidate Count1 test actually failed after SQL3848, 2005ms, code19007; its following aggregate segment never executed. Original IM API dependency requests: `pb2etakfx9ehukeh`, `pb2e9tpz792hukeh`.
- Rebuilt candidate native HTTP/WS/DB runtime exit0 (68.73s): two ordinary recipients matched ACK/history/summary; warm projection/disable and authority failure/recovery passed; only IM restarted, MySQL timestamp unchanged; scoped DB metadata unchanged. Private scratch schemas/runners absent afterwards; only owned MySQL stopped, volume/network/images retained.
- Ruling: pre-resolve current history/cold mentions into a request-local cache snapshot, not a new durable authority; bounded source reads cost additional queries. Keep shared identity extraction in ROOT's package; await its actual batch API or an explicit file release rather than duplicate a prefix/case-fold policy.
- Pending: replace the per-item fallback using that fixed shared API, run the large list and aggregate regressions to GREEN, rebuild and verify that final artifact, then obtain product-level acceptance. Current code is WIP; public owner/page integration is not complete.
