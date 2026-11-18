# Mindlink authority source adapter implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement inline. Follow the existing authorized c82/028 scope; no publishing or production authorization changes.

**Goal:** Consume the fixed BIZ 75739e949 source process and validate its retirement snapshot against an independently verified N1 system disable request.

**Architecture:** A server-constructed, default-disabled adapter launches only its fixed absolute program/config paths, with a bounded context and stdout. `Check` returns an error or a validated point-in-time observation; it never returns a grant, changes IM rows, caches allow or enables PolicyHTTP. The existing SQL primitive and its read/write barriers remain unchanged because the source releases its account lock before output and supplies no cross-database revocation fence.

**Tech Stack:** Go standard library, existing strict JSON and N1 verifier/parser, Windows/Linux process support.

**Spec:** `docs/mindlink-disable-authority-source_zh.md`; BIZ fixed `75739e949b7c1b5f08bf9f5f7013ed8c01eb4cba:model/agentaccess/mindlink_authority_source.go` and its command. Current batch8ce3abd7, messagepb2lyhh2gxahukeh.

## Global constraints

- Keep N1 body8/digest15/result22 and all public routes/default trust unchanged; source version is exactly mindlink-disable-authority-v1.
- Do not write BIZ/W1/W2/READ files, IM grant/principal/schema/config, or operate shared resources.
- Parse the actual21 required source fields with strict duplicate/null/type/UTF8 checks; independently recompute the N1 digest through ParsePrincipalDisable.
- Source v1 only produces system-authored wire intents; it does not authorize owner/avatar mutations or original-result replay.
- Never include process output/errors/config/DSN/JWS/body in returned errors or logs.

## Review focus

- A child exits nonzero after printing valid JSON: must refuse.
- Slow/flooding/invalid/trailing/duplicate output: bounded and refused, no cached prior success.
- Source matches scope but another wire/body/parent/actor: must refuse before it can become a grant.
- Caller uses foreign/expired Authorization or an earlier context deadline: preserve verifier ownership and cancellation.
- Source succeeds but account is revoked afterward: no freshness/grant/production-ready claim.

## Task 1: bounded source reader and N1 binding check

**Files:** Create `commons/mindlink/authority_source.go`, Windows/non-Windows process setup files, `authority_source_test.go`; update the source-boundary documentation and an evidence record.

**Interfaces:** `NewDisableAuthoritySource(DisableAuthoritySourceConfig) (*DisableAuthoritySource,error)`; `(*DisableAuthoritySource).Check(context.Context,*Verifier,*Authorization) error`. The config has Enabled, Program and ConfigPath; no request can set these. This is an isolated preflight component, not a SQL writer or a production PolicyHTTP integration.

- [x] Write the process protocol/strict binding tests using a real child test executable, including all Review focus cases.
- [x] Run the targeted tests RED; missing symbols count only as the absent-feature red, not runtime coverage.
- [x] Implement fixed process invocation, bounded output/deadline and field binding with reused strict parser/N1 digest validation.
- [x] Run race-enabled targeted and existing mindlink tests; compile source files for Linux. Build/inspect fixed BIZ source and exercise its default-disabled real process if local dependencies permit; report unavailable checks separately.
- [x] Request a fresh review, address Critical/Important findings and verify the resulting scope; record source/command hashes and limitations.
- [x] Commit only the finished adapter increment and its tests/docs; return the fixed result to BIZ and 大当家. Parent066/c82 remains open. Commit/delivery identifiers are recorded in the batch reply rather than altering this plan after its own commit.

## Ledger

- Current Docker /version read-only probe returned HTTP500/exit1; no DB/container mutation followed. Real SQL/HTTP integration is not available from that probe.
- BIZ's SQL/6-exe results are provider evidence only. The source is a valid dependency to implement against, not evidence that IM grants/projections/revocation ordering exist.
- Ruling: implement the isolated receiver rather than bootstrap a grant writer from a retirement snapshot; the latter would invent authority and cross-database ordering not carried by this source.
- Final local evidence: `docs/evidence/20261109-mindlink-authority-adapter/verification.md` and `.json`. Runtime negative overlays prove binding/exit guards; finalmindlink race30top+304sub,0fail/skip, realBIZ disabled process exit3/empty stdout; finalvet andLinuxcrossbuild exit0. No realSQL/fence/HTTP orLinuxruntime claim.
