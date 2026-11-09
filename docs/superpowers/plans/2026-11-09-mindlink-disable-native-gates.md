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

Freeze exact files after the decoded Actor/request and final storage call sites are traced. Reuse BaseProcessActor after protobuf decode; distinguish request/ACK directions, user/group namespaces and opaque content. Cover both login variants, Token registration actor, stream/import/group mutations and final inbox/history/online recipient functions. Preserve ordinary behavior and prevent rejected login logging from retaining the supplied Token. Tests exercise real decoded requests and actual native service/DB outcomes; do not merely search for hooks in source.

### Task 3: Typed disable producer and actual public enablement

Lock the existing owner grant and principal in the original journal transaction. Require write authority, immutable scope/avatar binding, positive/non-regressing target generation and policy CAS; preserve permanent disable. Record accepted time from the first journal record, an opaque commit token and committed record time, then validate/save exact N1 ready receipt and return only after commit. Wire POST only after Task2's applicable gates are verified. Exercise query/replay, read-only/revoked authority, unknown commit, concurrency, illegal zero-row/native tests and process recovery on a rebuilt isolated image. Leave later ensure/SendWithKey/READ and release acceptance explicitly open if not implemented.
