# READ caller-owned transaction adapter

DG-20261104-003 / F1-READ, authorized continuation `pbz2h8zsdyghukeh`, verified 2026-11-08 local / 2026-11-09 UTC. This is a completed local native adapter increment; public READ HTTP and the complete mindlink product remain pending.

## Exact call boundary

`services/conversation/services` now exports:

```go
QryConverSeenWithTx(ctx context.Context, tx *gorm.DB, req *pbobjs.QryConverSeenReq) (errs.IMErrorCode, *pbobjs.ConverSeenState)
SetConverSeenWithTx(ctx context.Context, tx *gorm.DB, req *pbobjs.SetConverSeenReq) (errs.IMErrorCode, *pbobjs.ConverSeenState)
```

The original `QryConverSeen` / `SetConverSeen` and native topics remain available. Request/response proto tags and error codes are unchanged; no migration or dependency was added.

- Pass a fresh, open GORM transaction from the authoritative IM MySQL database. Nil, ordinary DB and failed-begin handles fail closed with 10009. Finished handles fail through SQL without falling back to the global DB.
- Only `conversation_seen` I/O joins this transaction. Seen reads use `FOR UPDATE` to obtain the current max even if the caller already established an older repeatable-read snapshot. Writes retain `GREATEST`.
- Tenant/requester validation, private canonical pairing, group membership, visible persisted history and future-position rejection still share the native implementation. History/member lookups retain existing storage calls; this increment does not make them part of the transaction snapshot or provide the public policy fence.
- A helper returning storage failure 10009 aborts the supplied SQL transaction. A MySQL 1213 can roll back the server-side transaction while leaving `database/sql`'s handle callable; explicitly ending that handle prevents later auto-committed writes. Do not reuse it. Two first writers of an absent row may deadlock; abort and retry the whole outer operation in a fresh transaction.
- The caller must discard its transaction after **any** SQL failure, including errors in its own statements. The structural guard cannot identify every transaction already rolled back by a server-side error that the caller ignored. The helpers never reopen a transaction or retry only part of the operation.
- Code 0 means the helper staged/read seen successfully. The public owner must atomically commit its actual authority/result work and handle commit failure before reporting operation success. No `request_id`, operation ledger, jti, policy fence or reliable HTTP result was implemented here.

These are same-process Go calls. A `*gorm.DB` is not transported over the old RPC. The original IM owner decides the shared transaction's execution location and supplies any required server-only RPC/proto.

## Real display input

Resolve the current authorized business mapping to `(app_key, requester, target, channel_type, sub_channel)` first. For actual successful display, use the same-scope visible persisted `DownMsg.MsgSeqNo` (tag 6), passed to `SetConverSeenReq.SeenSeq` (tag 4). Do not derive it from an ACK, message ID, `MsgTime`, unread index or allocator floor.

The new targeted runtime probe sent one stored/no-count message, matched its send ACK ID to the recipient's actual receipt and queried history, and used that history's sequence **1** as adapter input. This proves the native fixture obtains the real sequence; it does not prove the final business page or released SDK exposes it. If ordinary HTTP history is chosen, the original IM owner still needs the lossless `msg_seq_no` string in its public model/API.

## Fixed source and current-run evidence

Base `23872613c059f7c181512b5ed8a303c382613c9b`; final source is that canonical LF archive plus exactly the four Go files listed in [the verification manifest](transaction-adapter-verification.json). The local delivery commit includes those exact file hashes; this document and the plan do not change the executable.

| Item | Final value |
| --- | --- |
| Source archive SHA256 | `89620561bfe34b518b9b36690725624fb2424418d029f60e71f34603a1f9a83f` |
| Linux executable SHA256 | `a78d0a99197fd762fa2805d42967578f5dd79ba78ebd1f0e30f2f821f6332ff1` |
| Local image | `bitcomputing/im-server:f1-read-tx-89620561` |
| Image ID | `sha256:82ad77656bd7eb7566e5dd59eb0975616a3d6853a3286cc06f249ca3f6582fa7` |
| Toolchain / database | Go 1.25.12, GORM 1.31.1, owned MySQL 8.0 |

Private raw evidence is retained under `.local/f1-read/tx-adapter-20261108/` in this worktree. Only sanitized metadata and hashes are committed; credentials, restart fixtures and raw logs stay ignored.

| Current-run check | Observed result |
| --- | --- |
| Initial missing-helper RED | Compilation failed for absent new functions, as intended |
| Global-DB fallback mutation | Both scopes failed because uncommitted seen escaped to the global DB |
| Missing `FOR UPDATE` mutation | Both scopes detected the obsolete max hidden by an older RR snapshot |
| Real MySQL deadlock RED | Driver error 1213; old handle Qry/Set succeeded and rollback left seen=3 |
| Final no-abort mutation | Removing only the abort defer reproduced that defect in both scopes |
| Final `go test ./services/conversation/... -count=1 -v` | Exit 0; 12 top-level tests passed, 11 F0-fixture tests skipped; skips are unverified |
| Final `go vet ./services/conversation/... ./simulator/probes/mindlinkread` | Exit 0 |
| Final `go build -trimpath -buildvcs=false ... ./launcher` | Exit 0; artifact above |
| Final `TestSeenTxMySQL` | Peer/group query creates no row; own-write visible only in tx; rollback=0, commit=1, another device's max=3 survives old snapshot/older/duplicate inputs; future rejected; finished handle rejected |
| Final 1213 case | Failed handle Qry/Set rejected; global seen remains 0 after rollback; fresh whole transaction can commit 3 |
| Final `TestTransactionBeforeRestart` | Actual receipt/history seq1; native sees 0 during uncommitted update and after rollback; commit gives seen/latest1/1, has_new=false, no-count unread0; real summary row persisted |
| Final `TestTransactionAfterRestart` | Exit 0; same history ID/seq, summary and seen/latest1/1 survive IM-only restart |
| Scoped DB verification | Seen, allocator, history metadata and conversation rows identical before/after restart; MySQL start timestamp unchanged |
| Artifact correspondence | Disk and running `/proc/.../exe` SHA256 match the final build before and after restart |

Fresh independent read-only review found one Important issue (1213 victim reuse); the real red/green regression and abort fix closed it. Final fixed source review reports no Critical, Important or Minor findings. The reviewer did not independently rerun the runtime; runtime and DB assertions above are the implementer's actual runs.

The first deadlock-test fixture produced lock timeout **1205** because it started a writer too early. Its log is retained as `deadlock-fixture-invalid-1205.log` and is excluded from valid 1213 evidence. Old pre-task retention/fresh-schema acceptance was not replayed or counted as current-run verification.

Only the owned `im-f1-read-20261106` IM/MySQL were started/restarted/stopped. Final snapshot: both exited, IM 137 with OOMKilled=false, MySQL 0; this is not a graceful-IM-shutdown claim. Fresh sidecar remained stopped with its original November 6 timestamps. Volumes, images and the two isolated runtime tenants were preserved. Temporary runners are gone; generated transaction-test seen/history rows were checked absent.

## Handoff to the public producer

As of inspection, original IM worktree `eb29` is clean at `11aa1ac222aaeef50b652cb69cc6715d8811eaff`; main is `fca18328483571fd1399f353fc2bdfcfde11668a`. The scoped `services/apigateway` search found no real READ transaction producer. Unique BIZ contract is M, `6af22e5ab620f64f8775afbf23d0a323e4fb2573`; it still marks these public components pending. These are inspected snapshots, not a promise that other worktrees will remain unchanged.

| Producer | Missing concrete artifact / remaining acceptance |
| --- | --- |
| Original IM manager `01a1a0e1-b344-7501-bc44-fadc4a29d403` | Registered READ router/API/types/errors; verified actor and trusted context; authoritative projection/policy/disable fence; persistent jti/operation result and final commit integration; typed `OperationResult`, GET observation/desired-state schema; actual execution location/server-only RPC/proto if required |
| BIZ manager `01a1a0e1-3618-78b0-a5c7-81a37bff3a31` | Authorized owner/conversation mapping, actual operation-intent persistence/signing and HTTP adapter consuming that producer; no duplicate business seen |
| Original IM plus final UI owner coordinated by 大当家 | Chosen HTTP history field or SDK-to-page actual displayed sequence, with a real page acceptance fixture |

Once those artifacts have fixed commits and a valid isolated fixture, run these **public** cases against that exact version: display/advance on two devices never regresses; same operation key and same digest reuses its persistent result; same key/different position conflicts; lose the PUT response then query/retry the original key; restart IM and query/retry without another execution; prove GET replays its original observed snapshot; revoke/change policy before final commit and verify no unauthorized write. Inspect both the actual response and DB ledger/seen, and match actual page/history IDs and sequences.

Those public same-key, authorization/final-commit and page cases are **unverified**, pending the named producers. Native max, fresh-transaction retry and the IM-only restart above do not substitute for them. No push, merge, deployment, production data or production entry was changed.
