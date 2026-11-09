# Consistent READ observation implementation plan

> **For agentic workers:** Use superpowers:executing-plans inline with TDD, then a fresh read-only review. Do not delegate implementation or edit shared public files.

**Goal:** Make caller-owned READ transactions observe their own member/history/visibility changes and expose a typed, coordinate-bound native query observation for the shared query producer.

**Architecture:** Preserve native wrappers and d336dec's transaction ownership/error rules. Add a READ-local SQL visibility lookup through the supplied GORM transaction, using the existing table models and native filtering semantics. Add a query-only observation containing explicit decimal-string state, original native coordinates and a UTC millisecond observation time. The original IM owner still supplies verified authority, original operation binding, immutable query/result persistence, final commit and HTTP registration.

**Tech stack:** Existing Go 1.25.12, GORM 1.31.1, owned MySQL 8.0. No dependency, schema, proto or migration changes.

**Spec:** Current authorized batch 82d1c514 / message pbz92gmaxfyhukeh; product final chapter 8/10; unique BIZ contract at b6f36cf7a5d308c2c800029258f4c9169b307208, N1/11.6; ROOT source inspected at e19538e04f9c4816345df9f309e20e78aa56da7f. Starting READ HEAD d336dec4cae9eff4067938b18e05215a5a0c60fe in the existing managed ffc4 worktree.

## Design and scope

- Editing shared group/history DAO interfaces would expand the active write set. Keeping the global lookups cannot see staged changes. The selected approach is a transaction-local READ lookup under conversation storage, reusing the existing models/table names and matching deletion, expiry, portion, clean-time and pre-join visibility.
- Transaction-local reads do not by themselves create the production owner/policy fence or upgrade READ COMMITTED to a repeatable snapshot. Those remain explicit public integration conditions; transaction isolation is caller-owned.
- MySQL transactions cannot provide an atomic view of Mongo history. Unsupported configured history engines fail closed on the transaction path; native storage selection remains unchanged.
- A native observation is not the 22-field public OperationResult and does not invent a query ledger or another seen table. Its original coordinates come from validated context/request, not a current UI selection. Coordinates stay out of its public JSON; ROOT can bind them to its existing original operation/digest and persist the returned state/time once.
- Native seen and regular homepage unread remain separate. No-count history/summary and ordinary DM behavior must remain usable. App/push, public actor authorization, original-key journal replay and page/SDK precision are separately unverified until their actual producers are integrated.

## Files and interfaces

- Modify `services/conversation/services/seenservice.go`: supplied-transaction visibility branch, native wrapper behavior retained.
- Create `services/conversation/storages/dbs/seenlatest.go`: READ-local MySQL member/settings/clean/history lookup through `SeenDao.DB` only.
- Create `services/conversation/services/seenconsistent_test.go`: owned-MySQL staged visibility and native parity regressions.
- Create `services/conversation/services/seenobservation.go` and `_test.go`: `QryConverSeenObservationWithTx(ctx, tx, *pbobjs.QryConverSeenReq) (errs.IMErrorCode, *ConverSeenObservation)`, explicit string state, original native coordinates, UTC milliseconds; no query-to-set substitution.
- Add targeted cases only under `simulator/probes/mindlinkread/` and evidence only in this directory. Shared HTTP/JWS/OperationResult/proto/generated sources/history/group files/migration manifests remain with ROOT.

## Review focus

- A transaction's staged membership, settings, clean times, deletes, portion recipients, expiry and persisted history must affect its READ result without global fallback.
- Empty history is a valid latest=0; missing group membership is forbidden; SQL errors must never turn into a valid zero state and must end the transaction.
- No scope leakage across tenant, user, target, channel or subchannel; duplicate/older positions retain max after the visibility change.
- Original coordinates and observed state/time must not drift when the caller changes its request or performs a later query; nil/missing transaction and nil requests remain errors.
- Same transaction does not imply public commit success or production authorization. Incomplete producer paths remain closed and explicitly identified.

## Task 1: Same-transaction visibility

- [x] Add real-MySQL tests using existing WithTx functions; observe staged member/history/clean/delete visibility fail on d336dec.
- [x] Implement the smallest READ-local SQL lookup and route only supplied-transaction calls to it; gofmt.
- [x] Run staged-state and committed native-parity cases for private/group scope and every visibility filter, including error rollback and unsupported engine rejection.

## Task 2: Typed original query observation

- [x] Add tests first for explicit zero/int64 state, original coordinates, UTC millisecond time and query-without-seen-write; observe the missing helper fail.
- [x] Implement the additive query observation. Do not duplicate or fabricate public operation receipts.
- [x] Verify query errors stay errors and previous observations remain unchanged after later reads. Existing INT history columns cannot store MaxInt64; use the serializer boundary and retained BIGINT seen for that assertion, and disclose the schema limitation separately.

## Verification and delivery

- [x] Run affected conversation tests/vet and Linux build, keeping SKIP/unavailable fixtures out of passed counts.
- [x] Rebuild the owned artifact and run targeted actual no-count history/summary, typed observation/rollback/commit, out-of-order devices and ordinary DM checks, then restart only owned IM and verify scoped DB/results and executable hashes. Do not replay the old full retention acceptance.
- [x] Obtain independent read-only review (no source findings); verify the fixed six-file source/archive/artifact tuple. Prepare task-only files for the required local commit.
- [x] Stop only owned test resources and preserve volumes/evidence. Prepare exact public producer/query-coordinate binding gaps for 大当家/ROOT. No push/merge/production deployment or production messages.
