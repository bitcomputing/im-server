# N1 query HTTP and read projection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Continue DG-20261104-003, batch 066519ee; no parent completion.

**Goal:** Register the two first N1 public paths, allow only authorized original-result reads, and expose an explicit NOT_READY for disable until its producer and native gates exist.

**Architecture:** The gateway constructs one immutable Ed25519 verifier from disabled-by-default public-key configuration. N1 routes do not use legacy app-secret Signature. The existing QueryOperation transaction invokes a concrete SQL reader that locks the owner barrier and target principal, checks immutable scope and current policy, and honors a default-false read grant. Neither HTTP nor a signed ticket creates authority rows or grants. There is no authority projection writer in this increment.

**Tech Stack:** Existing Go net/http, Gin adapter, database/sql, MySQL8, YAML configuration and ordered embedded SQL migration.

**Spec:** N1 contract at chatbot43 `7349850f69ca66f3a42e0bff1b303ddaccadb1e8` §11.5/11.7; query core `8f200ea23fe9b48fd558eb14ccf6ed59eebb6449`; existing owner-barrier design `docs/mindlink-trusted-rpc-candidate_zh.md`.

## Global Constraints

- Exclusive new files: commons/mindlink/{policy_authority.go,http.go,http_test.go,http_integration_test.go}; commons/configures/mindlink.go; services/apigateway/services/{mindlink.go,mindlink_test.go}; services/apigateway/routers/mindlink_test.go; commons/dbcommons/sqls/20261109.sql; this plan.
- Existing edits: commons/configures/configure.go; services/apigateway/{starter.go,routers/router.go}; launcher/scripts/config_template.yaml; sql/imserver.sql; commons/mindlink/transaction_integration_test.go (migration version/DDL inventory only); docs/mindlink-http-schema_zh.md (current-state handoff).
- READ code remains untouched; root alone integrates public routes/proto/migration inventory. No proto change is needed for a same-process query.
- `mindlink.enabled=false`, `trusts=[]`; trust public key is canonical padded Base64 for exactly32 Ed25519 bytes, never a private key. No runtime activation, production key, Compose/service change or deployment.
- Owner barrier key `(environment,tenant_id,appkey,owner_id)`, binary identity comparison, explicit default-false read grant. Principal mapping retains fixed owner/avatar, positive generation/policy and active/disabled lifecycle. Missing/incomplete facts fail closed; disabled targets remain readable to an authorized current owner/system.
- GET returns code/msg/data with exactly operation/replayed; errors contain data=null. Original receipt is immutable. No automatic grant/projection insert, effect, RPC or ordinary management fallback.
- POST disable verifies JWS and strict N1 body/digest, then NOT_READY without an operation/effect while production producer/native gates remain missing.

## Review Focus

- Default-off or malformed trust cannot fall through to legacy Signature or expose an unverified result.
- A permanent disabled principal remains queryable only with current read grant/policy; revoking the owner grant denies even original query-ticket replay.
- Missing barrier/principal or wrong scope is not a valid empty result and cannot insert authority or a receipt.
- New/upgraded DB schemas agree and old user data survive the real migration entry; neither runtime errors nor skips count as coverage.
- A registered disable route is not an implemented disable producer. Configuring trust does not authorize mutation or native message behavior.

### Task 1: Public handler and current SQL read authority

**Interfaces:** `NewPolicyHTTP(*Verifier,*sql.DB) *PolicyHTTP` implements http.Handler. Private `authorizeOperationRead(context.Context,*sql.Tx,Claims,Claims) error` consumes current query/original claims inside QueryOperation; the request context bounds its SQL waits. No projection writer is exported.

- [x] Write default-off/auth/error-envelope and real-loopback HTTP+owned-DB tests; observe 404/schema-stub failures.
- [x] Add owner/principal read projection migration plus matching initial DDL; wire exact N1 validation and original-query handler; keep disable NOT_READY.
- [x] Run real HTTP/DB grant revocation, stale version, missing/cross-scope projection and no-write tests, then journal and migration regressions. Verify old sentinel data and both schemas.

### Task 2: Gateway/configuration integration

**Interfaces:** configures.MindlinkConfig plus trust records; services.NewMindlinkHTTP(configures.MindlinkConfig,*sql.DB) (http.Handler,error); routers.Route gains explicit http.Handler dependency. Starter supplies it before accepting requests.

- [x] Test actual Gin route registration independently of old Signature; test disabled and malformed/duplicate/cross-operation trust configuration.
- [x] Add the configuration factory, disabled template and startup wiring; inspect Compose/entry script defaults without changing existing services.
- [x] Run affected Go1.25.12 Linux packages, Windows domain tests, owned MySQL integration/race, diff checks and independent review. Evidence and one baseline failure are separated below. Commit exactly this increment, then report progress without completing either parent batch.

Execution ruling: the read grant is internal projected authority, not a new public permission API. This increment has no grant writer; absence and the default-false value remain NOT_READY/FORBIDDEN. Fixtures explicitly insert isolated test grants and cannot stand for live business authority.

Runtime verification extension: rebuild the binary and an isolated local image as required by project AGENTS. The labeled fixture gateway shares only the owned MySQL container's network namespace; it publishes no new host ports. Preparation creates full IM schema and private temporary test trust/config only in the owned fixture, never a real account/key. Run the original GET with its HTTP response deliberately dropped after upstream success, recover the same query, restart only that fixture gateway process, then read the original with a new query ticket. `TestMySQLPolicyHTTPGatewayProcessRecovery` skips without its explicit fixture phase; that skip is not runtime coverage. Do not count the prepare phase as HTTP/restart acceptance. Record artifact hashes, container start snapshots and actual phase results before claiming this verification.

## Actual verification (2026-11-09)

- Runtime RED: default handler returned 404 instead of closed JSON; schema stub lacked authority tables; config factory stub accepted invalid trust. Actual Gin RED, after pinned dependencies were resolved, returned404 for both new routes while old users/register still took legacy Signature. The first combined Linux attempt could not compile routers because the read-only cache lacked pinned samber/lo1.52.0; that setup failure is not the Gin RED. `go list -mod=readonly` resolved existing pinned dependencies without manifest edits.
- LinuxGo1.25.12 offline `go test ./services/apigateway/... ./commons/configures -count=1` exit0. Default and integration-tagged `go vet` exit0. Compose configuration check exit0. No actual Compose service was switched.
- WindowsGo1.26.1 expanded race/SQL run: `go test -race -json -tags=integration ./commons/mindlink ./commons/dbcommons ./commons/errs ./commons/tools ./commons/configures -count=1` **exit1**, 53 top-level/275 subtests PASS, 1 FAIL, no test skips at that point. mindlink49 / errs1 / tools2 top-level all passed; dbcommons had one pass and the existing `TestExecuteSqlFileReturnsExecError` failure. Its LF source-text literal fails on the raw CRLF checkout; both dbmigration.go and its test are byte-identical to baseline8f200ea after CRLF normalization. This baseline check was not changed or counted passed. Log SHA256 `2129f9c8d6a8be219fb19ac940bddfa12aac695985782fd9ef5db129b6803da4`.
- Real `TestMySQLFreshAndUpgradeKeepExistingData` exit0: both actual Upgrade runs reach20261109, all four Mindlink table DDLs match, existing user sentinel remains. SQL/HTTP tests cover current grant revocation including ticket replay, default-false grant, missing barrier/principal, stale policy, cross-owner/tenant and valid/invalid closed disable with no operation/effect growth. Migration log SHA256 `45bc70b02159a23519f343f0f777e9d1718534d88f5a94ba8df14714b0e9ce4b`.
- Independent production-code review: **C0/I0/M0**; reviewer independently ran domain/configures unit tests, not SQL/race/Linux. The following explicit gateway probe was added after that read-only review; its prepare phase only prepares fixtures, and its initial/recover phases ran separately rather than being counted from the earlier expanded suite.

### Rebuilt gateway and actual process recovery

Offline Go1.25.12 build from this worktree (`-buildvcs=false -mod=readonly -trimpath -ldflags="-s -w"`), metadata confirmed by `go version` on the output. Binary103837880 bytes, SHA256 `c5ae3a6b434597a6ae00ed514ef645a328b37a951176c60552c68203201b7952`. Local isolated image `sha256:4a929f82e403d946c49eb7f94eb6d330f08250a031ac2b536d27f680aac79ec8`, original runtime base `sha256:cc835f6309ecad4a1d01cfb774446b1b8faf21759b7556e75d7051f3366e552d`; private config/DSN stayed outside the image context. Running binary hash matched. Normal image template remains disabled; only the private isolated fixture config used temporary test trust.

- `TestMySQLPolicyHTTPGatewayProcessRecovery`, phaseinitial: LinuxGo1.25.12, real full gateway+owned MySQL, exit0, no skip. Proxy consumed a successful upstream response then closed the client connection without returning it. Same original query recovered exact fixture receipt; operation1/ticket2/fixture effect1. Log SHA256 `ee2ff56c4e22d1d4dd45052a3ce48b59090f1dd4b301b415b8280726a76279f7`.
- Restarted only labeled fixture container `4b6b1dc59c721a5ab29acf4681d303341448cbe2cf5c08a15f0156032e649ab7`: start `2026-11-09T11:50:22.760269539Z` PID37003 → `2026-11-09T11:52:30.404036089Z` PID38157; same image/binary. This is a real gateway-process restart, not connection-pool replacement or a future-duration guarantee.
- Phase recover: exit0, no skip. New query ticket reads byte-identical original actor/versions/state/timestamps, operation1/ticket3/fixture effect1; no mutation replay. Log SHA256 `ec02008e1b09269aea53b30529a604f4ea9174948588327141db5a518a470293`.
- Cleanup observed after success: gateway exited0 and was removed; no new host-port publication or gateway volumes. Owned MySQL/data and local image/binary retained for continued task work. Private manifests/logs remain ignored under `.local/mindlink-policy/`.

These receipts/authority/effects are deliberate fixtures. No production grant feed or actual disable producer was tested; no native illegal-message/WS/send-key/read-flow acceptance, shared DB, release or production switch occurred. c82 and066519ee remain in progress.
