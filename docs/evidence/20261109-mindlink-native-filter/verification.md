# Shared native-message classification dependency

DG-20261104-003, batch c74fc1cf; READ request pb2etakfx9ehukeh. Root source starts at487fe9447652624fcb6133f3b43cb74a42bc5ce3. This increment supplies a shared read-only helper. It does not implement or accept READ's list/cache/DAO/Actor/model changes, public READ or complete F1.

```go
bases.FilterNativeMessages(ctx context.Context, appkey string, messages []proto.Message) ([]bool, error)
```

The returned bools exactly follow input order; true means the **supplied selectors** did not hit the native rejection fence. This is not owner/requester authorization or proof a target exists. Callers must load actual latest history/mentions, bind the trusted native app/requester and check final authority separately. The helper reuses appendNativeIdentities, including nested messages and mention users, while leaving opaque text/bytes/KV uninterpreted. It does not fetch omitted content.

For READ's028bb2a branch the shared production diff is only commons/bases/native_policy.go plus new commons/mindlink/native_filter.go. Ignored `native-filter-c74-shared.patch` is4781 bytes, SHA256 `6bb620d12e9b1b0b879959e53447c0eec7801e8131304afd75672abb6d613d75`. Actual apply-check and apply in an independent028 index exited0, and both resulting blobs equal Root's staged source; the active index was unchanged. This permits consuming the fixed Root implementation in the isolated READ candidate without merging unrelated Root history or rewriting the permission rule. It is not a candidate build/runtime result.

Nil message elements, typed-nil/invalid messages, nil context and empty app fail. A nil or empty input slice means zero candidates, but still requires an initialized fence, live context and successful zero-row SQL against the classification tables. Unknown schema, closed SQL pool, query/scan/row failure or cancellation returns an error and a nil mask. The bases entry follows the existing5-second native-policy timeout, bounded by the caller's earlier deadline.

NativeFence.Filter accepts per-message NativeIdentities user/group sets. It deduplicates exact raw strings separately by namespace and queries at most256 identities/257 parameters per statement. SQL returns the supplied ordinal, using utf8mb4_0900_ai_ci comparisons against the existing projected native columns and current groupmembers; no prefix heuristic, Go lowercasing or canonical-ID reverse mapping is used. Existing active and permanent disabled projections both deny. All queries are app-scoped, including the principal-to-group-member join.

For U/G distinct nonempty raw user/group selectors the normal SELECT count is ceil(U/256)+ceil(G/256); if both are empty, one zero-row availability query is used. This is a query-size/count bound, not a latency or future-permission guarantee. Each observation can become stale after reading; no new authority, snapshot isolation, route, proto, schema, configuration or projection writer is introduced. Existing Check/CheckNativeMessage bodies and rejection gates are unchanged.

## Current-run verification

New tests first failed compilation because the API did not exist; that is setup evidence, not SQL coverage. A temporary NotReady scaffold then compiled and failed the actual owned-MySQL classification tests and the real bases/SQL entry test. Correct implementation then passed the first targeted Windows SQL race (10.750s for domain) and Linux bases/owned-SQL entry (3.259s). All fixtures are Root-owned disposable schemas, not the READ owner's retained fixture or production.

The10,000-candidate case has20,003 distinct raw selectors and measures actual MySQL session Com_select on one pinned connection. Correct implementation uses80 executed SELECT statements and returns every expected input ordinal, including deny entries in late chunks. This metric is not a count of wire packets or driver prepare/execute protocol exchanges. A Go compile overlay replacing batching with per-message Check fails the same assertion at19,997 SELECTs (exit1,40.17s). The first overlay command was split incorrectly by PowerShell and failed setup; that attempt is excluded. Overlays never changed worktree production source.

An independent review's optional missing tenant regression was added: other-app has the same group/member identity but no projection in that app and must remain ordinary. A second compile overlay removing the principal/member app join fails this assertion (exit1,4.89s). The correct source retains the join. Final source re-review reports C0/I0/M0; reviewer ran pure Windows domain/base tests and vet, not SQL/service/UI integration.

Actual SQL cases cover case/accent aliases and app aliases, repeated selectors, same-ID user/group namespace separation, ordinary ma-shaped IDs, other-app ordinary groups, active/disabled tombstones and current membership removal/reinsertion. Original operation/ticket counts remain unchanged. A failure in the group query after user classification discards the whole partial result; empty batches also reject unavailable member schema, closed SQL pool and cancellation.

The real bases entry covers Conversation.Msg sender aliases, nested MentionInfo target users, protected group targets, ordinary group/user namespace collisions and opaque message/profile/KV values. Both active and disabled projections are tested with public Mindlink disabled. This is an internal Go helper/SQL check, not a new RPC or business page flow.

Final completed checks (logs under ignored `.local/mindlink-policy/`):

| Command scope | Actual result | Log / SHA256 |
| --- | --- | --- |
| Windows Go1.26.1 `go test -race -tags integration -count=1 -json` on commons/mindlink,bases,dbcommons,errs,tools, owned MySQL | exit0;81 top-level+334 subtest PASS,0FAIL;1 optional gateway-process SKIP | `native-filter-final-domain-race.jsonl` / `a3a5ed98f5eae0931d69f939ba5021727ce21573099dbd67bdc666786a447235` |
| Linux Go1.25.12 `go test -p 1 -tags integration -count=1 -json ./simulator/probes/mindlink`, owned MySQL | exit0;7 top-level+18 subtest PASS,0FAIL;5 opt-in process/restart SKIPs | `native-filter-final-native-probes.jsonl` / `7ed1ab908b881a5d308dff47543efcbba4d2b6e6ed0acad3c458fe45fbd8c921` |
| Linux affected unit packages and gateway/launcher compile checks | exit0;35 top-level+254 subtest PASS,0FAIL/test SKIP | `native-filter-final-linux-unit.jsonl` / `7b80c23a8dc81cf91761476cab3637bbf3473759bd6835eedef6c87386b956bd` |
| Linux affected `go vet -p 1 -composites=false` and `go build -p 1 -o /tmp/imserver ./launcher` | both exit0, empty logs | each SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |

Linux unit/vet scope is commons/mindlink,bases,dbcommons, services/apigateway/services, services/apigateway/routers and launcher (launcher has no tests). CGO disabled, existing cached builder, source/dependencies mounted read-only, memory1GiB/GOMEMLIMIT768MiB/GOMAXPROCS2. Native SQL probes used only the owned MySQL network namespace with no new host port or volume. The5 probe skips are TestNativePolicyRebuiltHTTPWS, TestConversationStateBeforeRestart, TestConversationStateAfterRestart, TestMindlinkNativeProbes and TestRestartedConversationSummary; no skip counts as process/RPC/browser acceptance.

Actual behavioral RED `native-filter-behavior-red.log` SHA256 `ce87d32f0e2b6fa544bee35dabbcade314f87672cf1dcc434181eb57457be787`; actual bases/SQL entry RED `native-filter-wrapper-red.log` SHA256 `bf5a193c539432d5917a7e4128797ba8f5f5de4e0a11aab96f8aa95226dae4c5`; entry GREEN `native-filter-wrapper-green.log` SHA256 `283d4e021f72f2b49a1b5248046c4fee99fe429ae11c74a5f125860d2bce1a64`. Expected failing overlays: serial query mutation log SHA256 `3c0f18b47c696cdb90101ac6bb16b1b1ad0b67823bb72423be8e8db8284f77e8`, cross-app join mutation log SHA256 `33ed65f2c2b7a3c1e6e55d015506db735c9b7f3c35ca5b85d0db7b96a206d184`. These are deliberate bad variants, never counted as passing product runs.

At2026-11-09T18:12:18.2173534Z an actual information_schema query found0 owned native/boundary scratch schemas. All temporary task-label containers are absent after completed --rm commands; no service process/browser was created by this increment. Owned MySQL `fa592cac645e829119a48282348da85529c3d15565ba0184e7dd450bd531a023` remains running with StartedAt2026-11-09T11:00:58.972804205Z, unchanged; existing data volume retained. These are point-in-time observations. No credentials, temporary YAML, tokens, message bodies or raw runtime logs are committed.

READ must consume the fixed dependency in its isolated candidate and independently verify cold-source loading, pagination/count/cursor behavior, every RPC error path and the rebuilt candidate. No READ candidate result, browser/App/push test or complete product readiness is inferred here. Parent066/c82 remains open; no push/merge/deployment is part of this increment.
