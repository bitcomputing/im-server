# DG-20261104-003-F1-READ

Implemented and locally verified: an independent conversation reading position for stored messages, including messages that do not count as unread. This is a native IM module delivery; business HTTP integration, UI acceptance and release remain separate.

Base: `4ecec887fb38d2d81af4bf2773b05d304e8ced97`. Necessary clean-time SQL fix: `584f9032b8759f63be627c95b5b2336f69f7c4f7` (one SQL-space correction plus real-MySQL regression). No changes to legacy unread counting, old protobuf tags or PERSIST snapshots.

## Native interface

`qry_conver_seen`: `QryConverSeenReq(target_id=1, channel_type=2, sub_channel=3)`.

`set_conver_seen`: `SetConverSeenReq` adds `seen_seq=4`.

Both return `ConverSeenState(seen_seq=1, latest_seq=2, has_new=3)`. Sequences are int64. The tenant and user come from authenticated RPC context, independent of the client-controlled routing target. The API supports private/group conversations and returns a state on each successful query/update. Other devices obtain the same authoritative position by querying; no new push notification is implemented.

`latest_seq` is the stored `msg_seq_no` of the latest history message visible in that exact scope, after membership, hidden-before-join, cleanup, deletion, expiration and portion-recipient filters. Empty visible history returns zero; SQL errors or invalid persisted ordering return an error. New positions above the visible latest are rejected. Duplicate positions already acknowledged remain valid when later deletion/expiry reduces visible latest. `conversation_seen` stores the maximum by tenant/user/target/channel/subchannel synchronously. Homepage unread clearing and delayed summary snapshots cannot overwrite it.

The original IM manager owns shared HTTP authorization/routing, SQL initialization and final migration ordering. `20261106.sql` is reserved for READ. This branch supplies the additive migration, picked up by the existing embedded upgrade path. The business §11.3 adapter and sole-authority contract still require joint registration; native RPC implementation is not evidence those HTTP routes exist.

## Actual verification

Environment: own `im-f1-read-20261106` Docker project, MySQL 8.0, Go 1.25.12 Linux runners; API/nav/WS/admin bound only to `127.0.0.1:19601/19602/19603/18690`, DB has no host port. No shared IM/F0, Rocky, pro/pre or existing deployment archive was used.

| Check | Actual result |
| --- | --- |
| Baseline conversation package tests | exit 0; old PERSIST DB fixtures skipped |
| Missing durable schema | RED exit 1, expected missing-table capability |
| DAO | real MySQL GREEN exit 0; duplicate/older/concurrent updates preserve max; tenant/user/target/subchannel/channel isolation |
| Missing native RPC | RED exit 1, actual WS response 11013 |
| Clean-time SQL | RED exit 1 / MySQL 1064; GREEN scoped/empty/error regression and vet exit 0 |
| Invalid stored ordering | RED exit 1; corrected boundary tested in GREEN suite |
| Module tests, real MySQL tests, vet, Linux launcher build | exit 0; DB read/write/history errors fail rather than return successful empty state |
| Project's named CI package tests | exit 0 |
| New schema | initial SQL plus READ migration executed in own fresh database, table/primary key inspected, exit 0 |
| Existing schema upgrade | startup recorded version 20261106; legacy sentinel retained message ID and unread/read indexes 2/1 |
| Live two-session flow | exit 0; one counted plus flags 8/72 messages actually received by owner Web and PC, all three history IDs/sequences and final summary match; ordinary unread remained 1 until explicitly cleared |
| Independent reading state | native RPC returns max across two sessions; stale/duplicate/future/nonmember/routing-target cases verified; clearing homepage unread leaves entry unseen |
| IM-only restart | exit 0; history, summary, zero ordinary unread and seen/latest=3/3 restored; DB dumps byte-identical and MySQL start timestamp unchanged |

Tested image: `sha256:66f045dd93e2abb3d5190fcadb160dc75685b49637e23b8625570eb00da9133e`. Disk, image process and restarted process binary SHA256: `49d9e6b5f09b4afd382eb11343b7e3b5d73706b08e916c666d1ab736f2d91b87`. These identify the tested dirty source snapshot; [manifest.json](manifest.json) records source and evidence hashes for matching it to commits.

Private raw evidence is `.local/f1-read/` in the owning managed worktree. It includes fixture credentials and must not be committed. The checked-in manifest contains hashes only. Initial test-fixture failures (wrong table names and uninitialized custom debug logger) remain in private logs and were not counted as passing.

Independent code review: pending. No push, merge, production deployment or manual acceptance is claimed. App/browser UI, phone push/badge/sound, MongoDB, history physical-retention sequence reuse and RELIABLE crash guarantees have not been verified. Legacy PERSIST integration tests target another owner's fixed fixture and were skipped; this task instead used its own live history/summary/restart checks.

Own containers/volumes and test tenants are retained for inspection; resource state is a snapshot, not a continuous guarantee.
