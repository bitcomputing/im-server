# N1 disable wire alignment

**Goal:** Accept BIZ's fixed N1 wire shape and replace the unused older proposal with the exact disable body/result types and authenticated body/digest validation. This does not enable an HTTP route or claim production revocation.

**Spec:** chatbot43 contract at `6d0b3e7f81fe9f6f5ccba0f3fa6c797940bce538`, raw SHA256 `c2b5cce47269976487fd0ddc26a0ac3e1c5bac8fce3a2027b2d06b384505d0dc`, §11.5. Incoming BIZ message `pbz32ne2eduhukeh`; original implementation authorization DG-20261104-003 continues.

**Scope:** `commons/mindlink/operation.go`, new `principal_disable.go` and `operation_test.go`, existing HTTP/candidate handoff docs. Existing strict JSON and Ed25519 verification are reused. No schema/config/router/native-message changes, no other worktree edits. No callers use the old value types (repository-wide Go reference search).

**Interface:** `(*Verifier).ParsePrincipalDisable(*Authorization) (*PrincipalDisableRequest, error)` accepts only this verifier's authenticated exact POST/path and owner/system actor. It parses eight required fields, validates positive target generation/CAS and fixed IDs, binds all duplicated claims, and checks the N1 15-element Go JSON SHA256. Returned DTO is syntax/authenticated-intent input; final authority, lifecycle snapshot and gates remain mandatory inside Transact.

**Verification:** Real Ed25519 request verification followed by parser tests; published ASCII/MaxInt64/Unicode digest vectors; signed malformed, duplicate/escaped duplicate/null/unknown/alias/overflow/numeric-type, actor and original-binding attacks. Golden result marshaling checks the external 22-field/null/millisecond contract, not an active result producer. Re-run affected default packages; DB integration is not claimed by these wire-only tests.

- [x] Write the boundary tests, run an API-only stub and observe expected failures.
- [x] Replace old value types and implement the parser with existing strict JSON and Go standard library.
- [x] Run fresh tests, independently review the security boundary, fix any supported finding, and prepare only this scope for commit.
- [ ] Send the exact contract acceptance, implementation evidence and remaining closed gates to BIZ; do not complete the overarching F1 task.

Verification results and evidence hashes are in `docs/mindlink-http-schema_zh.md`. Delivery happens after the local commit; this plan does not pre-record a successful send. CLI batch state is authoritative for that receipt.
