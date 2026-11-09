# N1 disable and native rejection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Continue DG-20261104-003 /066519ee and original c82. Commit each verified code increment; do not complete either parent until its applicable acceptance is met.

**Goal:** Make N1 disable a real typed transaction while sealing projected internal identities against legacy Token/login, request, management and persistence paths.

**Architecture:** Native protocols carry no N1 actor proof and always reject projected internal principals and groups containing them, including permanent disabled tombstones. Legitimate new operations use the separately verified N1 transaction; no IsFromApi/admin/allowlist bit bypasses the native fence. Reuse the Actor base's decoded input boundary, then check login and final delivery/storage functions. Control writes require a separate default-false write grant and share the existing owner row lock with query/replay.

**Tech Stack:** Existing Go/protobuf reflection and MySQL8, generated indexed identity columns with the actual supported native collation, existing transaction journal and native F0 probes.

**Spec:** BIZ N1 at7349850f69ca66f3a42e0bff1b303ddaccadb1e8; query/HTTP baseline077d5dbb91b0c95e6cab527be44be414b7032cdf. F0 failures in docs/mindlink-conversations-probes.md and simulator/probes/mindlink/probe_test.go are repair inputs, not new full investigation.

## Global Constraints

- No new public DTO/proto or changes to N1 8body/15digest/22result. ensure and conversation.ensure remain closed pending their actual full schema/authority; no counterfeit projection feed.
- Native identity comparison must match users/groupmembers/app native collation, including case/accent/width aliases. Binary N1 ownership bindings remain exact. Scope user IDs and group IDs separately.
- Denials precede interception/caches/effects; errors/missing schema never count as absent authority. No raw token/JWS/body logging, no false code0/zero receipt.
- read_enabled never grants control writes. New write grant defaults false. Same operation replay uses current read authority and original versions/receipt, not a new effect.
- Runtime POST remains NOT_READY until the native integration and applicable tests are in place. Do not publish ready from an isolated SQL update or unit fixture alone.
- Do not edit READ-owned service/DAO files or introduce shadow seen/history. Root owns shared Actor/HTTP/SQL integration.
- Use only owned disposable MySQL/test image; preserve existing data volumes and actual services. No push/merge/shared deployment or real external messages.

## Review Focus

- Native case/accent aliases must not obtain an internal token, mutate a managed roster or write a message despite binary public identifiers.
- Ordinary users/groups remain usable, including IDs that collide across user/group namespaces and normal ma-shaped IDs with no projection.
- Admin/IsFromApi, stream continuation, import/async history and already-established sessions cannot bypass final checks.
- Disable, current generation/policy CAS, original receipt and query ticket survive response loss/restart without a second effect; revoked/read-only authority stays denied.
- Failures, unsupported schemas, skips and fixture-only results remain separate from actual HTTP/WS/SQL acceptance.

### Task 1: Native identity fence and write-grant schema

**Exact files:** new commons/mindlink/native_fence.go, native_fence_test.go, native_fence_integration_test.go; commons/dbcommons/sqls/20261110.sql; update sql/imserver.sql and commons/mindlink/transaction_integration_test.go (owned fixture helper and migration expectations); this plan. Independent review exposed interrupted ALTER replay; the required repair adds commons/dbcommons/mindlink_migration.go and a targeted guard in dbmigration.go. Each atomic ALTER is retried only when all additions are absent; a fully applied matching step is skipped, while partial/incompatible definitions stop without advancing the version.

**Interfaces:** `NewNativeFence(context.Context,*sql.DB)(*NativeFence,error)` validates native identifier/generator/index schema. `(*NativeFence).Check(context.Context,string,[]string,[]string) error` checks app/user/group identities without grants or DML; existing active/disabled projections both deny. Internal SQL collated columns mirror supported native utf8mb4_0900_ai_ci and have a unique native app/principal index. Migration20261110 is the next ordered version after20261109; its number is a sequence, not a claim about execution date. Fresh initial schema records its actual latest version; the old-schema fixture explicitly retains its older version.

- [x] Add fixture/native SQL identity, alias, scope/namespace, permanent tombstone, error and upgrade-preservation tests. Schema/API stubs failed; interrupted-DDL replay failed on actual duplicate constraint before repair.
- [x] Implement ordered write_enabled=false migration and native collated generated index, synchronizing fresh SQL without replaying ALTER against an already-current fresh schema. Actual resume tests preserve old receipt bytes and an existing true write grant; incompatible defaults, generated length and partial index do not advance the version.
- [x] Implement read-only fence with bounded query batches and actual schema checks. Owned MySQL race run: 28 test pass events, zero fail/skip, exit0 (`native-fence-final-race.jsonl`, SHA256 80f5d99e597ab62b3c868891b7875ddab4c82a98e795ee07c31d0bd31bbce347). Linux Go1.25.12 domain/db/config checks exited0. Independent review found no remaining Critical/Important; its minor missing diagnostic was added, then domain/db unit checks exited0. This is an unwired primitive, not public disable readiness.

### Task 2: Native request/login/final storage integration

Task2A exact files: commons/bases/baseactor.go; new commons/bases/native_policy.go and native_policy_test.go; launcher/main.go; services/connectmanager/services/logincheck.go and botlogincheck.go; services/connectmanager/server/imhttpmsghandlers/imhttpmsghandler.go; services/connectmanager/server/imlistener.go and imbotlistener.go; this plan. Install the validated fence after the real migration and before listeners; config.enabled=false must not reopen projected tombstones. Reuse BaseProcessActor after protobuf decode; distinguish request/ACK directions, user/group namespaces and opaque content. Group Actor routing targets are group IDs; private send routing targets are hashed conversations, so use the actual real_targetid extension. StreamMsg has no channel enum and is explicitly a private-user target. Add unit assertions at the real decoded Actor boundary; do not treat them as SQL/runtime acceptance.

Task2's actual boundary fixtures live in new simulator/probes/mindlink/native_policy_integration_test.go. They use only guarded owned scratch MySQL databases, the real native Actor/service and token implementations, and explicit HTTP/WS process phases; unconfigured runtime phases skip and never count as acceptance.

Task2A implemented and verified as an independent increment: Actor selector stubs and uninitialized fence reached business in RED; actual Linux/MySQL legacy login allowed 12 internal identity/state/bot cases in RED. After repair, active/disabled exact and native aliases deny ordinary/bot login without publishing identity, while ordinary owner login succeeds. Actual registration, self-unmute, friend, favorite and global group-config Actors deny; raw native HTTP publish handler rejects its internal token. Review found routing-only device and omitted payload selectors; real device RED returned code0 and created a row, then repaired extraction leaves zero rows. Review also caught an ordinary RTC group/user namespace collision; its RED/green regression preserves group scope. Final Linux Go1.25.12 base+owned native boundary run: 23 pass events/0 fail/0 skip/exit0 (`native-a-final-linux.jsonl`, SHA256 f6f91983e299f013863096b42eaf0734c22b2c69b0d0a05a4bb3d59b1d4161c2). Windows bases race exit0; connected Linux gateway/login/group packages and launcher compiled/tested exit0. Independent re-review C0/I0. Windows service build is unsupported by existing metrics Swap fields and lumberjack Stat_t, not counted as a passing or expected denial test. These are actual in-process handlers/services plus SQL, not rebuilt process/WS acceptance; Task2B and public disable remain open.

Task2B exact files to extend after Task2A: message/services/msgservice.go, dispatchservice.go, messagesave.go, broadcastservice.go; group/services/groupmsgservice.go; historymsg/services/streammsgservice.go and hismsgservice.go; connectmanager/services/publishmanager.go; dedicated owned native runtime integration tests and probe helpers. Cover final direct calls, Token registration, stream/import/group mutations, inbox/history and online recipients. Preserve ordinary behavior and prevent rejected login logging from retaining the supplied Token. Tests exercise actual native service/DB outcomes, not source-string hook checks. Any READ-owned changes remain with READ; do not silently edit their service/DAO files. POST stays NotReady while final gates/runtime coverage remain open.

### Task 3: Typed disable producer and actual public enablement

Lock the existing owner grant and principal in the original journal transaction. Require write authority, immutable scope/avatar binding, positive/non-regressing target generation and policy CAS; preserve permanent disable. Record accepted time from the first journal record, an opaque commit token and committed record time, then validate/save exact N1 ready receipt and return only after commit. Wire POST only after Task2's applicable gates are verified. Exercise query/replay, read-only/revoked authority, unknown commit, concurrency, illegal zero-row/native tests and process recovery on a rebuilt isolated image. Leave later ensure/SendWithKey/READ and release acceptance explicitly open if not implemented.
