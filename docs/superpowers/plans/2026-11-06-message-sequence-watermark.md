# Message sequence watermark implementation plan

> **For agentic workers:** Use superpowers:executing-plans inline, then request an independent review of the complete patch.

**Goal:** Stored private/group messages must remain above acknowledged positions after history retention and a cold IM restart.

**Architecture:** Reserve a tenant/conversation/channel/subchannel sequence and send-time watermark in a short MySQL row transaction before any sendbox, history or delivery side effect. Bootstrap from retained history and existing READ positions; retaining only metadata avoids retaining expired message contents. A reservation is not a history commit: gaps are allowed and the existing asynchronous HTTP response remains an acceptance response.

**Tech stack:** Existing Go, GORM, MySQL; preserve the configured history backend.

**Spec:** DG-20261104-003-F1-READ, current batch f15ae250f146fed63e92f1ca610f30f95b11209e227f24d99366ca97a3d344f7 and docs/mindlink-conversations-probes.md §12.

## Scope and decisions

- Original IM manager owns this fix in an isolated worktree from 8889b39. READ files remain owned by the second manager.
- Modify commonservices/convercache/msgconvercache.go and add msgsequence.go plus MySQL tests. Propagate the new generator error through the six existing sending services; do not change actor/protocol signatures or ordinary HTTP semantics.
- Durable allocation applies to stored private/group messages. Other channel behavior remains compatible, with their callers adapted to the error return. No READ state is reset.
- Reserve migration 20261107. Include READ's unchanged 20261106 migration and identical initialization DDL so applying a higher migration cannot skip it later. No READ RPC implementation is merged by this dependency patch.
- Allocation is committed before side effects. DB failure rejects allocation; an uncertain commit must not be retried inside the allocator. A later fresh allocation can leave a gap. This does not provide durable request idempotency or history/outbox guarantees.
- Existing expired history without either retained messages or READ metadata cannot be reconstructed. New watermarks are never removed by retention. Mixed old/new writers are outside the verified upgrade mode.

## Review focus

History emptied before initialization must still respect a persisted seen position; private IDs use canonical conversation scope. Independent processes must not reuse a sequence. Failed bootstrap, update or commit must never produce a successful allocation. Rollback and retry must preserve durable state. Migration ordering must retain READ 20261106.

## Task 1: Pin and fix cold-start allocation

- [x] Run a real MySQL test against the old generator: RED 4 -> 1 after retention/cold start.
- [x] Add durable metadata table and transactional reservation; adapt all six callers. Error return precedes sending side effects.
- [x] Verify real MySQL purge/cold-start, seen floors, scope, 24 concurrent reservations, failures and overflow.
- [x] Verify fresh/upgrade DDL, Linux tests/vet/build and final-source live flow/only-IM restart. Binary a4458bcc; image3827b96e; full evidence in docs/evidence/20261106-message-sequence.
- [x] Record independent static review and exact evidence for the dependency commit. Return the resulting commit under the active batch; final READ integration/regression remains the READ owner's task.

Ruling: independent review exposed migration skipping after lower migration/version-write failure. Added real RED/GREEN regressions, error propagation to launcher, and pre-service MAX seeding with binary grouping. The verified target is MySQL retention without old writers; Mongo startup seed/physical retention, published clients, load and production release remain unverified. See the evidence report for the exact limitations.
