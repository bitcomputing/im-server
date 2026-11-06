# DG-20261104-003-F1-READ

Native MySQL READ passed its applicable local regression and fresh independent review at `a35c8779c43d23e42cd6d49fc295f454abf6f2ff`, with old writers stopped. The previous retention blocker is reproduced in the historical RED evidence and GREEN after the exact shared sequence dependency was integrated. This is a local native-interface result; business HTTP, real clients and release acceptance remain pending.

Dependency: `33d2b9bb1a8aad82d95ed25d890476e9496d5546`, integrated without conflict as `7ccc6e96d09601e1cb19dffd5608907fa0a7feee`. READ implementation: `008bd1f`; clean-time SQL fix: `584f903`; Mongo missing-record error boundary: `80bdd87`; original retention regression: `e3957ea`; enhanced dual-device retention probe: `a35c877`. Base: `4ecec887fb38d2d81af4bf2773b05d304e8ced97`.

## Native interface

`qry_conver_seen`: `QryConverSeenReq(target_id=1, channel_type=2, sub_channel=3)`.

`set_conver_seen`: `SetConverSeenReq` adds `seen_seq=4`.

Both return `ConverSeenState(seen_seq=1, latest_seq=2, has_new=3)`. Sequences are int64. Tenant and requester come from authenticated RPC context. Private/group conversations are supported. Other devices query the same durable max; no new push notification is implemented.

`latest_seq` comes from the latest visible persisted history in the same scope, after membership, hidden-before-join, cleanup, deletion, expiry and portion-recipient filters. It is not the allocation watermark or unread index. Empty visible history returns zero; storage errors and invalid stored ordering return errors. A new acknowledgement above visible latest is rejected; an already acknowledged duplicate remains valid if later deletion reduces latest. `conversation_seen` persists the maximum synchronously and independently of homepage clearing and summary snapshots.

The shared dependency reserves durable private/group sequence and send time before sendbox/history/delivery. `20261107.sql` seeds retained history/seen maxima, preserves higher floors and follows the unchanged READ `20261106.sql`; migration failure stops startup. Reservation alone does not establish asynchronous history durability.

## Current verification

See [integrated-verification.md](integrated-verification.md) for commands, provenance, failures/skips and evidence; [integrated-manifest.json](integrated-manifest.json) records hashes. [integrated-review.md](integrated-review.md) records the independent review and its scope.

| Check | Observed result |
| --- | --- |
| Affected packages and named CI packages | Canonical LF source tests passed; READ real-MySQL cases ran; unrelated fixed-fixture cases skipped |
| Vet and Linux build | Exact affected-package vet/build passed; broad message-tree vet has six pre-existing Mongo literal warnings |
| Fresh/upgrade schema | Both reached 20261107; READ/sequence DDL byte-identical; old seen 3/history 1 seeded floor 3; legacy summary/index sentinel retained |
| Fresh native lifecycle | Three messages matching sender ACK IDs received by simulated Web/PC, matching history and final summary; ordinary unread 1, then cleared to 0 without advancing independent seen |
| Multi-device acknowledgement | Durable max, stale/duplicate/future/nonmember/routing-target cases passed |
| IM-only ordinary restart | Same live binary; MySQL not restarted; scoped DB snapshots byte-identical; history IDs/seq 1..3, summary, unread 0 and seen/latest 3/3 restored |
| Original retention regression | Unmodified `e3957ea` scenario GREEN: latest 4, seen 3, has_new true |
| Final purge/cold-start | Actual owned-tenant retention DAO removed history; seen/floor 3 unchanged across IM restart; next ACK/history seq 4, seen 3, has_new true, both sessions received same ID/summary |
| Final DB persistence | One history row at seq 4 with ACK ID, durable floor 4/seen 3, all three summaries at that ID, owner indexes 1/1 |
| Independent review | No Critical/Important blocker for tested native MySQL mode; one optional regression-assertion Minor |

Tested source archive SHA256: `92313f2557c778632633536a7e246b8e2b52eb231e1d29ed35e7f131d3270ec3`.

Disk and live binary SHA256: `48592a8d46e71929e0d8f062e7b8e456e4969d8af11fddc0134994a6843fec00`.

Image: `sha256:90ce1013e041bb65e4121a849e4938dd3401f3f73724ff90a815f6316784adb9`, revision label `a35c8779c43d23e42cd6d49fc295f454abf6f2ff`. Subsequent evidence-only commits do not change these tested source files.

## Boundaries and retained resources

Business contract section 11.3 and its sole writable authority remain a coordination candidate. Native RPC success does not demonstrate business HTTP route registration or business integration. App/browser UI, phone push/badge/sound, live Mongo retention/bootstrap, mixed old/new writers, production load and RELIABLE idempotency/outbox/crash timing were not accepted by these checks. Mongo missing-record coverage is error injection into the real MySQL consumer, not live Mongo evidence. Supplier-owned sequence failure/concurrency/migration fixture results were audited as supplied evidence, not independently rerun here.

Only the owned `im-f1-read-20261106` Docker project was used, with MySQL 8.0 and Go 1.25.12 Linux runners. API/nav/WS/admin bindings were loopback 19601/19602/19603/18690; MySQL had no host port. Final inspection recorded both IM containers exited 137, OOM=false, and MySQL exited 0 after the stop command. Volumes, images, disposable tenants and private evidence were preserved. This is a resource-state snapshot; graceful IM shutdown was not demonstrated. No push, main merge or production deployment was performed.

Private evidence lives under `.local/f1-read/integrated-33d2b9b/` in the owning managed worktree; credentials and runtime fixtures must not be committed. Historical [manifest.json](manifest.json) describes the earlier `008bd1f` artifact and is not the current artifact. [retention-handoff.md](retention-handoff.md) preserves the original RED reproduction and dependency handoff; its blocked-state notes are historical and superseded by this integrated result.
