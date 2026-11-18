# READ transaction adapter implementation plan

> **For agentic workers:** Use superpowers:executing-plans inline, then a fresh independent review. Follow TDD; do not delegate implementation or expand the released write set.

**Goal:** Let the public IM operation owner read/advance the single authoritative seen row inside its caller-owned MySQL transaction.

**Architecture:** Keep native RPC wrappers unchanged in behavior. Add explicit `QryConverSeenWithTx(ctx, tx, req)` / `SetConverSeenWithTx(ctx, tx, req)` helpers, preserving existing request/response types and validation. Only seen-row I/O joins the provided transaction; existing visible-history/group lookups remain their established storage calls. A storage failure aborts the supplied transaction to prevent reuse after MySQL has automatically rolled it back. Public authentication, policy fences, operation results and final commit remain the original IM manager's responsibility.

**Tech stack:** Existing Go 1.25.12, GORM 1.31.1 and owned MySQL 8.0; no new dependencies, proto fields or migrations.

**Spec:** Current delegated task `pbz2h8zsdyghukeh` / batch `7b5d241e`, existing F1-READ scope, unique BIZ contract section 11.3, and the accepted same-transaction requirement. Base is `23872613c059f7c181512b5ed8a303c382613c9b` in the existing managed `ffc4` worktree. This resumes the already authorized implementation; it does not reopen product decisions or L/M acceptance.

## Files and interfaces

- Modify `services/conversation/services/seenservice.go`: two additive exported helpers returning `(errs.IMErrorCode, *pbobjs.ConverSeenState)`; accept only an active GORM transaction, apply caller context, share all existing READ validation.
- Modify `services/conversation/storages/dbs/seen.go`: optional DB handle preserving zero-value DAO behavior; transaction reads lock/current-read the seen row so a caller's older repeatable-read snapshot cannot return an obsolete max.
- Add `services/conversation/services/seenservice_tx_test.go`: meaningful real-MySQL transaction and invalid-handle regressions.
- Add `simulator/probes/mindlinkread/transaction_test.go`: one-message native compatibility, real displayed-sequence input, caller rollback/commit and IM-only restart.
- Update only this evidence directory for plan, handoff and verification. Public HTTP/auth/result/schema, shared proto/migrations and other worktrees stay with their owners.

## Review focus

- A nil, ordinary DB or failed/finished transaction must fail without falling back to global writes.
- Rollback must remove the seen advancement; transaction success is not final commit success.
- An earlier caller snapshot must not hide a newer committed max; older/duplicate devices must not regress the returned state.
- Query must not create a seen row; valid scope/history/future-position checks must remain shared with native RPC.
- Membership/history are not moved into this transaction by these helpers; public caller must supply its real authority fence and final result/commit handling. No request-id ledger or exactly-once HTTP result is claimed.

## Task 1: Adapter and focused regression

- [x] Write invalid-transaction and owned-MySQL tests first; observe the absent exported helpers fail compilation.
- [x] Implement the two small helpers and DAO transaction routing/current reads; gofmt changed Go files.
- [x] Run owned MySQL tests for query-without-write, read-your-write, rollback, commit, stale snapshot, older/duplicate max, future rejection and finished-transaction failure. Read raw results; do not count SKIP as pass.
- [x] Demonstrate global-DB fallback and missing current-read defenses fail in disposable source copies.
- [x] Reproduce the independent review's real 1213 deadlock-victim reuse defect; abort on storage failure; verify Qry/Set rejection, no escaped write and fresh-transaction retry. A first test fixture produced 1205 and was excluded; the corrected gated fixture requires driver error 1213.
- [x] Run final affected conversation package tests/vet, Linux build and source/artifact correspondence. Run rebuilt owned IM for the targeted compatibility/restart check; do not replay the old full retention acceptance.
- [x] Obtain fresh independent review and resolve its Important finding. Prepare only task files for the required local commit.
- [x] Prepare exact signatures, actual displayed-sequence inputs, version-matched cases and missing producer artifacts in the handoff. Final public HTTP same-key/restart tests remain unverified until the original IM producer exists.
