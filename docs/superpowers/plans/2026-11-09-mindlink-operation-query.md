# N1 original operation query Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. This is the first code increment of DG-20261104-003 / batch 066519ee, not completion of POLICY/RELIABLE.

**Goal:** Recover an existing N1 disable receipt using the verified issuer and original operation ID, without repeating its effect or rewriting its original identity/version.

**Architecture:** Reuse the operation/ticket journal and the original SQL transaction locking order. A mandatory callback locks and checks current read authority; this increment does not invent production authority from signed claims. Preserve receipt bytes after strict operation-specific validation.

**Tech Stack:** Existing Go database/sql, Ed25519 verifier, strict JSON parser, MySQL8 integration fixture.

**Spec:** `chatbot43/docs/mindlink-conversations-contract.md` at `7349850f69ca66f3a42e0bff1b303ddaccadb1e8`, N1 §11.5/11.7; scheduling `knowledge/collaboration/tasks/2026/DG-20261104-003.md` (2026-11-09).

## Global Constraints

- Exclusive files: this plan, `commons/mindlink/operation_query.go`, `operation_query_test.go`, `operation_query_integration_test.go`. No READ files, proto, migration, configuration or shared environment changes.
- Exact GET `/apigateway/mindlink/operations/:operation_id`, operation `operations.query`, owner/system only, empty body/query. Both query policy versions equal the positive current read version.
- Lookup only `(verified iss, original operation_id)`; no cross-issuer fallback. Original audience/scope/request/digest/target must match. Original mutation remains `principals.disable`.
- Current authority callback is required and executes in the same transaction. Query ticket binding is durable; original request/result are immutable. Missing/corrupt/unreadable records return a non-success error.
- No production `ready` claim: native final gates, concrete authority, HTTP registration and typed producer remain subsequent implementation. Test authority/effects are explicitly fixtures.

## Review Focus

- Renewed key/audience or same operation ID under another issuer cannot read a different original receipt.
- Query current policy differs from the original mutation; do not overwrite original versions or deny solely because disable advanced policy.
- Missing/null/duplicate/unknown result fields, forged desired state or unsupported result states must never look successful.
- Query jti conflict, authority removal, SQL/commit failure and expiry after lock waits must not create an operation or reapply an effect.
- Fixture SQL evidence is not production authority or a true disable/HTTP/WS/restart acceptance.

### Task 1: Transactional original-query core

**Files:** the three Go files above; reuse `transaction.go`, `auth.go`, `strictjson.go` without mutation.

**Interface:** `(*Verifier).QueryOperation(context.Context, *sql.DB, *Authorization, func(*sql.Tx, Claims, Claims) error) (*MutationResult, error)`. Callback arguments are authenticated query claims and immutable original mutation claims. It must lock/check current read authority and the query policy version, never perform the original mutation.

- [x] Add signed query boundary and strict receipt tests, then run API-only stubs and observe valid queries/receipts rejected (RED).
- [x] Implement exact query proof, original binding/result validation and durable query ticket transaction, rechecking TTL after locks and before commit (GREEN).
- [x] Run real owned MySQL tests for changed current policy, wrong issuer/audience/scope/intent, missing/corrupt original, read revocation, jti conflicts, query retries and reopened connections; assert original bytes and effect count remain unchanged.
- [x] Run affected unit/race checks and independent review. Commit exactly this scope after the fresh checks below; CLI state, not this plan, records later delivery.

Execution ruling: existing detailed design and explicit instruction to continue implementation supply the approach and inline method; no new product decision or repeated design approval is needed. Preserve the larger task as in progress.

## Execution evidence (2026-11-09)

- API-only stub RED: unit subset exit1 (valid proof/ready/accepted/failed/unknown all rejected); real MySQL query subset exit1 with expected NOT_READY rather than query behavior. The first DB attempt was unreachable after restarting the owned container with its dynamically allocated new loopback port; this was an environment failure, not a valid RED or a product pass. Only the ignored fixture socket copy was updated; no credentials were displayed.
- Restored implementation: Windows Go1.26.1 `go test -race -json -tags=integration ./commons/mindlink ./commons/errs ./commons/tools -count=1` exit0; **47 top-level / 266 subtests PASS, 0 FAIL / 0 SKIP**. Private raw log `.local/mindlink-policy/query-final-packages.jsonl`, SHA256 `9c0523a6b026ef7eb41c48736ec9ceb154b9d7b516f09226ff2ab94990674dc2`.
- Query-specific SQL coverage includes observed MySQL lock waits on operation/authority/ticket, expiry rejection with no extra effect/ticket, SQL failure, and a packet proxy that drops the actual server COMMIT response. That case returns UNKNOWN_RESULT although the query ticket really committed; the same query recovers the byte-identical receipt without another effect. No packet data is recorded. Reopened connection-pool coverage is not service-process restart coverage.
- Deliberately removing all three query TTL checks and ignoring Commit errors made those regression tests fail (exit1). Production source was restored byte-for-byte, SHA256 `815afb177b67d0713e7176afdc23d0947929e7f6440a5bb809fdd529fb8f7aaa`, before the final race run. Fault-injection failures are retained as test evidence, not current implementation failures.
- Offline Linux Go1.25.12 cached builder, network none, source/module mounts read-only: `go test ./commons/mindlink ./commons/errs ./commons/tools -count=1` exit0. This run did not enable integration and is not Linux SQL evidence.
- Independent read-only reviewer: Critical0 / Important0 / Minor1 (query-specific failure/expiry test gap); its own Windows unit run passed. The planned direct fault tests above subsequently cover that gap, including a real lost COMMIT ACK; no production claim follows. The reviewer did not run SQL or inspect private fixture configuration.
- No schema/proto/config/HTTP/READ file changes. This increment has no concrete production-authority callback or registered HTTP route; producer, native final gates, true disable/WS/message/process-restart acceptance remain open on c82 / 066519ee.
