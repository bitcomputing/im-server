# Mindlink reliable text send implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans. Continue the existing c82 authorization inline; use a fresh independent reviewer at each externally delivered interface boundary.

**Goal:** Supply a fixed strict send/result contract and implement atomic native MySQL history, immutable original receipt and durable outbox, with original-key recovery and final authority checks.

**Architecture:** Reuse Ed25519 request authentication and the original-operation journal, but define a separate mindlink-reliable-v1 wire/digest/result, never disable8/15/22. Resolve native coordinates from a fixed conversation/member projection, lock owner/principals/conversation in the committing transaction, persist actual DownMsg history plus outbox and receipt, then return persisted after commit. Public activation and production projection/bootstrap remain closed. Existing dispatch rewrites times and repeats inbox writes, so this increment must not pretend its asynchronous enqueue is delivery completion.

**Tech Stack:** Go1.25.12 module, Go1.26.1 Windows tools, database/sql/MySQL8, native protobuf and existing sequence/history formats.

**Spec:** Current task3f7ac87b / pb2m3jgqg2ehukeh; BIZ unique contract at c6941661 and existing F3 model2a43; IM base9141c5f. Formal exact source will be new `docs/mindlink-reliable-send_zh.md` plus Go DTOs/vectors.

## Global constraints

- Only text jg:text in v1; exact content bytes, sorted unique avatar mentions, same-conversation reply reference; asset_ids must be explicit[] until its real authorization/producer is implemented.
- Every field required; reply_to alone nullable. Wire request_id is the persisted F3 parent operation64hex, parent_digest remains its old local digest; separate delivery_snapshot_sha256 binds the immutable parent snapshot bytes. Neither is the new wire digest.
- messages.send POST /apigateway/mindlink/messages/send; messages.query GET /apigateway/mindlink/messages/results/:request_id, no body/query. Owner/avatar only, no system send; fixed current policy in both ticket versions.
- Do not write BIZ/lib/mindlinkim, READ services/conversation files or shared resources. Shared sequence changes preserve the legacy wrapper and all its floors/Mongo refusal boundaries.
- No public enablement, grants from fixtures, producer self-authorized JSON, merge/push/deploy or production configuration changes.
- No scope expansion to streaming/media/directed sends or new delivery platform. Persisted means the original native history/outbox transaction committed, not client receipt or completed execution.

## Review focus

- Same request/different native target or content, body aliases/Unicode/numeric coercion: reject before effect.
- Valid request at ingress, revoked owner/actor/member/policy before commit: serialize on current authority and reject.
- Lost commit/HTTP response and restart: query original immutable result, never new message/sequence/outbox.
- History/outbox/receipt failure mid-transaction: no partial effect, original lookup cannot fabricate success.
- Native history/sequence semantics and recipient cursors: do not call the old persistence+dispatch pipeline on an already committed event.

## Task 1: fixed wire and receipt

**Files:** commons/mindlink/message_send.go, message_result.go and tests; docs/mindlink-reliable-send_zh.md and evidence/vectors.

**Interfaces:** ParseMessageSend(auth); MessageSendDigest(claims,request); ValidateMessageSendResult(auth,raw,nativeBinding). Reuse existing Authorization identity, strict parser and Go-json encoding.

- [x] Write explicit Go reference encoding vectors and failing strict/body/receipt tests; observe RED.
- [x] Implement separate strict text DTO,18-slot digest, original receipt and native binding validation.
- [x] Run affected tests/vet and independent review, then commit the consumer-usable contract/source and send it to BIZ while keeping this batch/parent open. Commit/receipt IDs live in the batch handoff; no claim that runtime producer already exists.

## Task 2: atomic native producer and original query

**Files:** new mindlink send authority/store/query files and tests; commons/dbcommons/sqls/20261112.sql plus sql/imserver.sql; supplied-transaction sequence hook without changing READ-owned files; private HTTP registration/closed readiness only as applicable.

**Interfaces:** Verifier.SendMessage/QueryMessageResult use verified Authorization, actual SQL transaction and native projection. No caller-supplied arbitrary effect or grant callback.

- [ ] Write real SQL tests for native private/group PB history, original replay/conflict, rollback, response-loss query and authority races; observe relevant failures.
- [ ] Add ordered/new-init schema and schema readiness; implement authority+native checks, shared sequence reservation and immutable native history/outbox/receipt in one transaction.
- [ ] Implement separate messages.query validator/read authority, preserving old disable-only operations.query.
- [ ] Verify new/upgrade schema and real data, errors, response loss/restart and scoped authority races on the owned isolated fixture. Flag UI/WS/online worker and actual business projection limitations explicitly.

## Task 3: delivery/recovery review and handoff

- [ ] Inspect frozen outbox recovery/delivery prerequisites. Any native delivery-only hook needs durable per-user cursors/cache coordination; do not silently use DispatchMsg/SendBoxActor or count an enqueue as delivered.
- [ ] Run applicable Go/race/vet/build and independent review; fix Critical/Important, commit completed changes only.
- [ ] Verify owned resource identity and stop only the owned test process; retain ignored data/evidence rather than broad deletion.
- [ ] Return exact code/version/evidence and uncovered paths to 大当家/BIZ. Public capability remains closed, original066/c82/F1/F3 not globally complete.

## Ledger

- Native plan review: existing history/inbox/sendbox use global DB handles; reserveSequence opens its own transaction. DownMsg history and shared durable sequence must join the original journal transaction.
- Existing DispatchMsg mutates recipient MsgTime/reinserts inbox and asynchronous pools give no durable completion. Direct transactional inbox insertion also needs per-user monotonic cursors and cache updates; not part of initial persisted-only receipt.
- Own standalone MySQL8.0.28 at loopback33606, schema mindlink_reliable_3f7, private fixture/owned-process metadata under ignored .local/reliable-sql-3f7. Docker untouched. Initial root-host1130 and monitor-child restart failures were preparation failures; exact own process residuals checked before console-mode/private fixture-account bootstrap succeeded. Never print or commit its DSN/startup file.
- Ruling: first complete wire exposes parent_digest and delivery_snapshot_sha256 separately. The existing F3 local fingerprint does not itself define IM wire semantics or authorize snapshot recipients; fixed native membership and current policy are independent IM checks.
- Task1:4top+135sub targeted racePASS,0fail/skip; explicit group receipt positive added. Independent reviewC0/I0, native-format documentation and one-parent-one-message invariant fixed. One initial receipt test had an invalid expectation about an unknown generated messageID; preserved failing log, changed test to true schema violation, no production relaxation.
