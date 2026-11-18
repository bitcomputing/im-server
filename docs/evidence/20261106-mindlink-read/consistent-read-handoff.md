# READ transaction visibility and original native observation

DG-20261104-003/F1-READ continuation `pbz92gmaxfyhukeh`, batch `82d1c514`. Base is `d336dec4cae9eff4067938b18e05215a5a0c60fe`; verified on 2026-11-09 in the owned local MySQL/IM environment. This increment fixes the local transaction visibility gap and adds a typed native observation for the public producer to consume. It does not register public READ HTTP or implement its original-query journal.

## Behavior and call boundary

`QryConverSeenWithTx` / `SetConverSeenWithTx` now read member, group/member settings, clean boundaries, deletion, expiry, portion recipients and visible persisted history through the supplied GORM transaction. Staged changes in that transaction affect its READ result; outside readers do not see them. Native non-transaction wrappers retain their existing MySQL/Mongo storage paths.

`SeenDao.Latest` reuses existing table models and matches native history visibility, including private/group differences in future-effective deletion and the original send-time ordering. Only transaction calls use it. The existing `FOR UPDATE` seen current read, atomic max, validation and storage-error rollback remain in place. Non-MySQL transaction visibility fails closed instead of silently reading SQL while history lives elsewhere; native Mongo behavior is unchanged and was not live-tested.

All reads being in one transaction is not a production policy fence and does not upgrade caller-owned READ COMMITTED to a repeatable snapshot. Public authority, lock ordering, final authorization and isolation remain explicit caller conditions.

New additive function in `services/conversation/services`:

```go
QryConverSeenObservationWithTx(ctx context.Context, tx *gorm.DB,
    req *pbobjs.QryConverSeenReq) (errs.IMErrorCode, *ConverSeenObservation)
```

`ConverSeenObservation` contains:

- `Coordinates`: copied actual `(AppKey, RequesterId, TargetId, ChannelType, SubChannel)`, excluded from JSON so internal IM identities are not page fields.
- `State`: `SeenState` with explicit `seen_seq`/`latest_seq` decimal strings and boolean `has_new`, including `"0"/"0"/false`.
- `StateObservedAt`: UTC fixed milliseconds, recorded when this logical native observation is obtained, not the physical commit instant.

The helper snapshots the request fields before querying. Later request changes, device advances or new queries cannot overwrite that returned object's coordinates/state/time. Querying does not create seen rows. Invalid stored state, missing/finished transactions, SQL errors and cancellation remain failures; storage failures end the transaction.

This is a partial native observation, **not** the full 22-field `OperationResult`, a complete HTTP data object, or proof that a logical operation committed. It does not contain issuer/operation/request/digest keys. ROOT must bind its original verified intent and authoritative mapping to these native coordinates, save the first state/time once with its query receipt, commit, and replay that original record without calling this helper again. A new logical query may obtain a new observation. PUT's immutable receipt remains separate from its freshly authorized current state.

## Fixed verification

Canonical source is the d336dec archive plus exactly the six Go overlays listed in [the sanitized manifest](consistent-read-verification.json). The delivery commit's blobs must match those hashes. Plan and evidence documents do not change the executable.

| Artifact | Value |
| --- | --- |
| Source archive SHA256 | `ba4039f107e6ed695e45d4ae0fee51f8f3c9cce01dac03b0b1a1ac6232abfec6` |
| Linux executable SHA256 | `7b5cfbf90f013b90b9741d69e00d0723969d7057e05ef16d8a75492b6f3942a6` |
| Owned image | `bitcomputing/im-server:f1-read-consistent-ba4039f1` |
| Image ID | `sha256:605cf86596d214ee028ccb153386e43c4cc95ae491f61f2324010eaa4e60db84` |
| Toolchain | Go 1.25.12 / Linux, existing GORM 1.31.1 and MySQL 8.0 |

Private raw logs, source bundles, binaries and restart credentials remain ignored under `.local/f1-read/consistent-20261109/`; only sanitized metadata and hashes are committed.

| Actual current-run check | Result |
| --- | --- |
| Visibility RED on d336dec | Staged history/clean/delete/expiry/member/settings/portion changes returned global old state; corrected fixture reproduced the expected failures |
| Typed helper RED | Failed compilation for missing `QryConverSeenObservationWithTx` |
| Global-visibility mutation of final source | Returning the new lookup to the global DB reproduced staged visibility failures |
| Float conversion mutation | MaxInt64 string assertion failed; the regression detects lost precision |
| `go test ./services/conversation/... -count=1 -v` | Exit 0: 19 top-level PASS, 11 F0-fixture SKIP, no FAIL; skips are unverified |
| Targeted real MySQL tests | Staged private/group visibility and advancement, staged member creation/removal, settings precedence, strict clean boundaries, deletion/expiry/portion, native committed parity, tenant/subchannel isolation, unsupported-store rejection and cancellation rollback passed |
| Typed observation | Explicit zero fields, MaxInt64 serializer/retained seen, stable original native coordinates/time, query without seen creation and invalid-state rejection passed |
| vet / Linux build | Both exit 0; final artifact above |
| `TestObservationBeforeRestart` | Two stored/no-count group messages actually reached Web/PC devices; ACK/history IDs matched, actual history seq1/2 supplied the advances. Original typed observation stayed 0/2/true in memory; uncommitted/rolled-back seen remained 0 externally; partial commit1 followed by older/duplicate device advances ended at seen2. Group summary/unread0 and ordinary owner-to-peer DM history/summary/unread1 passed and persisted |
| `TestObservationAfterRestart` | Exit 0: both devices seen/latest2/2, no-count history IDs/seq and summary/unread0, ordinary DM history/summary/unread1 survived IM-only restart; a new typed query read the restored current state |
| Actual DB/artifact checks | Scoped seen, allocator, group/private history metadata and conversation records identical before/after restart; MySQL start timestamp unchanged; disk and running executable hashes match before/after |

The restart test obtains a **new** observation. Original-observation stability was checked on the in-memory object; this is not evidence of original-operation/query-journal persistence or replay. These native clients and direct Go helper calls against the owned real DB do not exercise the public owner API, business mapping/gate or final page.

Fresh independent read-only source review reported Critical0/Important0/Minor0 and verified all six source/archive hashes and the runtime probe's scope. Its four pure Windows Go1.26.1 tests did not execute because pre-existing `commons/metrics` compilation failed on unavailable `memory.Stats` Swap fields. That failed attempt is not counted as a pass; supported Linux Go1.25.12 results above are this implementer's actual runs. Old full retention acceptance and earlier task results were not replayed or counted as new verification.

Invalid setup attempts were retained and excluded: initial settings row used zero `updated_time` (1292), the first large-sequence fixture exceeded message-ID length (1406), and MaxInt64 history insertion exceeded the actual column range (1264). The corrected visibility RED and final fixed-source tests are separate logs. No failed branch was converted into a passed assertion.

## Concrete remaining producer dependencies

At final inspection ROOT was `2713/feat/mindlink-policy` at `8f200ea23fe9b48fd558eb14ccf6ed59eebb6449`, with in-progress shared HTTP/config/authority/migration changes. They were not edited or incorporated here. `Verifier.QueryOperation` now exists for retrieving existing `principals.disable` receipts; its `validateDisableResult` cannot validate a READ result, and it does not create a new `conversation_seen.query` snapshot. `Verifier.Transact` still rejects GET. The in-progress `PolicyHTTP` dispatch is for disable/operations.query, not public READ.

ROOT still owns these actual READ integration points:

1. Verified owner authority/mapping, member/policy/disable final fence and original intent binding `(issuer, operation_id, request_id, request_digest, full scope)` to the observed native coordinates.
2. `conversation_seen.query` original-query producer/strict N1 result validator, first state/time persistence and replay; query must not become Set(0). Existing disable result lookup is a distinct operation.
3. Advance effect joining seen and typed original receipt in the same transaction; current-state reread and public envelope/error/RPC/registration integration. No public shared file was co-written.
4. A fixed integrated artifact containing this READ patch and ROOT's shared code, plus BIZ owner API and an actually displayed lossless sequence consumer. App/push and the final business page remain untested.

The owned fixture's actual schema inspection found `p_hismsgs.msg_seq_no` and `g_hismsgs.msg_seq_no` are signed **INT**; `conversation_seen.seen_seq` and `message_sequence.seq` are **BIGINT**. This is a verified local-fixture schema snapshot, not a claim about every deployment. Typed strings preserve the full int64 range, but this run does not prove full-range history storage or SDK transport. The 1264 case and metadata were sent to ROOT (`pb2acdg7pkuhukeh`) for its formal schema/new-migration responsibility; this patch changes no schema or migration list.

Only the owned local IM/MySQL were started/restarted/stopped; final snapshot: IM exit137/OOMKilled=false, MySQL exit0, fresh sidecar kept its original stopped state. This is not a graceful-IM-exit claim. Volumes, image and the isolated runtime fixture were retained; temporary runners were absent and selected generated test history/seen rows were checked zero. No push, merge, production deployment or real external test messages occurred.
