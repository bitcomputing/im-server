# Native READ / durable sequence integration evidence

Task DG-20261104-003; active batch 066519ee. This is a local prerequisite increment, not whole-task completion or public readiness. Source starts at `028bb2aa5359c96cc67337814b094239ba0b99a4`; the containing Git commit identifies the integrated source. READ source is fixed at `dcdd3a30ba4b802dce11ae396dc8f3711755ed4f`, including its transaction baseline `d336dec4cae9eff4067938b18e05215a5a0c60fe`. Root alone adapted shared callers, protobuf, migrations and probes; the owner's worktree was not edited.

## Artifact and actual runtime

Linux Go1.25.12, CGO disabled; server binary SHA256 `1a738c38c2eb080c3f9dd73d90877be1139910cce7524747b10869b88a1838e0`. Owned image `im-server:n1-read-066519ee`, image ID `sha256:8a4f037d473e2895b9ce6866534494384696ab56fc34f4d42f496dc71630c5e5`. The runtime base is `sha256:cc835f6309ecad4a1d01cfb774446b1b8faf21759b7556e75d7051f3366e552d`, not the Rocky release image. Image labels inherited from earlier source are not source identity evidence.

`TestNativePolicyRebuiltHTTPWS` ran the rebuilt child service against a new owned scratch database, with public Mindlink disabled and no trusts. Actual Web/PC native queries/advances share seen, reject projected private/group targets, preserve ordinary group/user namespace collisions, receive/history-decode exact private2147483648 and group2147483648/MaxInt64, refuse exhausted allocation without a new history row, and preserve seen/allocator state across an actual IM-only restart. Go SDK history decoding is not visual rendering or released JS/App precision proof. The test stops/waits for both child processes; it does not assert graceful child exit.

The same run then waited for an actual Playwright CLI0.1.19 / Chrome headless management-page check. Browser operations used only the owned loopback proxy and existing fixture management API. Send/history responses contained these new IDs; the Go probe independently required exactly one new scoped DB row, matching receiver43 SDK receipt, allocator watermark and unchanged independent seen:

| Existing page flow | New ID | Exact DB/SDK sequence | Unique history rows |
| --- | --- | ---: | ---: |
| Private send + private history | pb2e4nysjaeghaa8 | 2147483650 | 1 |
| Group send + group history | pb2e469q2ajj4fus | 2 | 1 |

The browser was closed after both fresh IDs were visible in their corresponding history responses. Screenshots contain only the send response panel: [private](ui3-private-send.png), [group](ui3-group-send.png). Raw histories, message bodies, temporary YAML, credentials and browser profiles remain ignored under `.local`; they are not committed evidence.

Final combined runtime `read-runtime-ui-3.log`: PASS, exit0,347.46s, no skip. SHA256 `7c4a04306c6c64b70cb255d562c350225fc92910a563a4c026c27cfbcf49ce32`. Probe binary SHA256 `f4f97d833c432e31255868b419f5ea013dc5c5d3d9046c93a162f8fc19511ad6`. These identify this run rather than the earlier Task2 resource manifest.

## Schema and failure evidence

The actual old10 fixture was forced back to INT history columns before Upgrade11. Both missing/preexisting READ-table cases check BIGINT metadata, exact2147483648 write/read, retained original receipt/seen and non-regressing watermark; reapplication preserves a deliberately larger floor. `read-integration-old-int-upgrade-final.log` passed both cases, exit0; SHA256 `e600c1ed395d00bff2b7fa74a0ab6325bc31214c871c6e42638c56ee923be1f5`.

The preceding final targeted schema/migration race run `read-integration-schema-final-race-2.log` passed, exit0,69.886s. Fresh schema checks require both p/g history columns to be BIGINT, all six applicable tables and current version11. Separate actual SQL cases check2147483648/MaxInt64, closed-pool metadata error propagation, invalid empty/zero/negative/nonnumeric versions with no replayed DDL, and failure of10 preventing11/version advance.

Failures are retained separately and are not passing coverage: version10 initially skipped READ prerequisites; migration10 failure initially advanced through11; closed-pool metadata errors were initially swallowed; invalid versions replayed old DDL; INT overflow produced MySQL1264. A first fresh-SQL edit mistakenly changed broadcast rather than private history; the fresh datatype assertion caught it and the broadcast change was reverted. The corrected schema race and old-INT run above cover the resulting code. A wrong protobuf import mapping failed setup before tests; regeneration with the legacy appmessages import mapping exactly matches the delivered generated blob.

Runtime attempts also retain their limits: old artifact returned11013 for the ordinary READ namespace; an initial probe misclassified mapped application codes and was corrected; UI1 failed because the management module required private relative `conf/config.yml`; UI2 timed out after its bounded proxy exited, and a stale response was rejected by exact-ID checking. Neither UI attempt is counted as acceptance. The later UI3 run above is the successful combined evidence.

## Final checks and resource snapshot

Logs below are ignored local evidence in `.local/mindlink-policy`; exit codes were read from the actual completed commands. Commands use the existing fixture configuration file without placing DSN credentials in output.

| Final check | Actual result | Log SHA256 |
| --- | --- | --- |
| Windows Go1.26.1 `go test -race -tags integration -count=1 -json` over commons/mindlink,bases,dbcommons,errs,tools, current owned MySQL DSN | exit0;75 top-level+332 subtest PASS,0FAIL;1 optional gateway-process SKIP | `8661570c9e31e78a667cd674e3052a020d51839d052cdfe4ca08e78b0851d1b5` (`read-integration-final-domain-race-2.jsonl`) |
| Linux Go1.25.12 `go test -p 1 -count=1 -json`, affected packages plus repository CI unit packages | exit0;78 top-level+286 subtest PASS,0FAIL;25 opt-in old READ/sequence/cache/clean-time SQL tests SKIP | `2989a7a7e39149aa8126246cda01575ac77cc2dfd81bc715bf044f320840b893` (`read-integration-final-linux-checks-2.jsonl`) |
| Affected-package Linux `go vet -p 1 -composites=false` | exit0, empty output | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` (`read-integration-final-targeted-vet.log`) |
| Targeted actual schema/migration race | exit0,69.886s | `db6451e151f49383dcf649619863eeed38f744acc2f5a4bc0524abb1c0669ede` (`read-integration-schema-final-race-2.log`) |

Linux packages: commons/{bases,mindlink,dbcommons,pbdefines/pbobjs}; services/commonservices/...; conversation root/actors/services/storages/dbs; message/group/broadcast/statussubscriptions services; historymsg services/storages/dbs; apigateway services/routers; launcher; simulator/benchmark; commons/metrics/...; pushmanager/services/httputil. The skipped old fixture tests are not current SQL acceptance. New schema tests and the separate rebuilt runtime/page run provide only the actual coverage stated above.

The first broad Windows race used an obsolete fixture port and failed reachability; it is not green. The first expanded Linux run reported `cannot allocate memory` while reading mounted source, then was stopped (command exit2 and its auto-removed container was rechecked absent); it is not green or a verified product regression. The successful retry bounded memory1GiB, GOMEMLIMIT768MiB and GOMAXPROCS2. Full-repository vet exited1 because the offline read-only module cache lacks the existing HBase dependency used by testmain/hbtest; only the affected-package vet is passing. No all-repository green claim.

Gofmt was run on all33 changed Go files; no unformatted files remain and `git diff --check` exits0. The22 initially reported files differed by CRLF normalization. Git-filtered hashes of all20 copied READ source files still exactly equal their fixed delivery, including the generated protobuf. Six shared GenerateMsgId callers retain native pre-reservation checks and propagate allocator errors.

Snapshot at2026-11-09T17:08–17:09Z: an actual owned MySQL query found zero schemas matching the native/boundary scratch prefixes (`read-integration-scratch-audit.json`, SHA256 `09396fc27afc18d0fdb45b14859250117542badd72fa8fdc6fc9af2258e2e569`). The successful UI3 container `8dfea7c135b55156fbec1a4a98cbc7cd878a1d5ce05a7844775e80d7367a621b` was inspected exited0 and removed. Proxy3 `f07d2b3b5dd1fcd8ae5d6fd51a46b9cb220f0d7a0631d3c30719cd340158e749` was ownership-checked, stopped, inspected exited0 and removed. The task-label Docker inventory is empty after final checks. The owned browser PID80988 is absent. Earlier failed UI1/UI2 and native test containers were likewise inspected and removed; no volumes were deleted.

Owned MySQL `fa592cac645e829119a48282348da85529c3d15565ba0184e7dd450bd531a023` remains running, StartedAt `2026-11-09T11:00:58.972804205Z`, unchanged across the IM restart and page acceptance. Existing data volume, images and ignored artifacts remain. These are point-in-time cleanup observations, not ongoing monitoring guarantees.

Independent read-only final review: Critical0 / Important0 / Minor0. Reviewer checked source, fixed READ blobs, migration assertions, test cleanup ordering and evidence boundaries; did not independently run the service, browser or SQL integration suite. No push, merge, shared service deployment, production migration, external message or public API enablement is part of this increment.

Open: production authority feed, public READ original snapshot/typed receipt/final authority integration, public disable enablement, blind conversation-list/aggregate filtering, ensure/SendWithKey, real provider push/stream-timeout callback, business Web/App acceptance and released JS/App int64 behavior. The current batch remains open.
