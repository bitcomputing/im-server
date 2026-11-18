# Integrated READ verification, 2026-11-06

Tested native source: `a35c8779c43d23e42cd6d49fc295f454abf6f2ff`, branch `feat/mindlink-read`, managed worktree `C:/Users/jogle/.codex/worktrees/ffc4/im-server`. Shared dependency `33d2b9bb1a8aad82d95ed25d890476e9496d5546` was integrated as `7ccc6e9`. Its 16 production sources and 24 nonsecret evidence hashes were audited; `20261106.sql` is unchanged. This audit is distinct from the local runs below.

## Source and artifact correspondence

Export committed source with `git -c core.autocrlf=false archive --format=tar -o ARCHIVE COMMIT`. No persistent Git configuration was changed. The final archive's 30 tracked source hashes were checked against Git blobs. Build/test containers unpacked that archive, rather than the Windows checkout.

Affected-package tests, vet and first build used `7ccc6e9`. Between that commit and `a35c877`, only `simulator/probes/mindlinkread/retention_test.go` changed, adding actual two-session receipt/history/summary checks. The launcher was rebuilt from the final archive; its bytes matched the earlier build. Final lifecycle probes ran the final archive. `source-at-build.json`, `source-final.json`, `final-version-before.json`, `final-version-after.json` and their manifest hashes identify the source, image and live executable.

The Docker artifact inherited the existing local PERSIST runtime image and replaced `/opt/imserver` with the canonical-source Linux build. Its private build context allowed only the binary, Dockerfile and dockerignore. Private fixture/environment files were excluded.

## Commands actually run

In Go 1.25.12 Alpine containers, CGO disabled, shared download/build caches, own Docker network and private environment:

```sh
go test ./services/conversation/... ./services/historymsg/storages/dbs ./commons/dbcommons ./services/commonservices/... ./services/message/... ./services/group/... ./services/broadcast/... ./services/statussubscriptions/... ./simulator/benchmark ./commons/metrics/... ./services/pushmanager/services/httputil -count=1 -v

go vet ./services/conversation/... ./services/historymsg/storages/dbs ./commons/dbcommons ./services/commonservices/... ./services/message/services ./services/group/services ./services/broadcast/services ./services/statussubscriptions/services

GOOS=linux GOARCH=amd64 go build -o /out/imserver ./launcher
```

The test/vet/build commands completed successfully on canonical LF source. `JIM_F1_READ_ISOLATED=im-f1-read-20261106` enabled the owned READ MySQL cases. Original PERSIST tests requiring `im-f0-20261105` and supplied sequence/migration tests requiring their owner's fixed DB fixture remained SKIP. No guard was changed to borrow another owner's resources.

Staged native runtime commands used `go test ./simulator/probes/mindlinkread -run '^NAME$' -count=1 -v` for each name in order:

1. `TestSeenBeforeRestart`: fresh disposable tenant; one counted plus flags 8/72 messages; simulated Web/PC receipt, history/summary, clear-unread independence, max acknowledgement and authorization cases. PASS.
2. Query the real database until the final summary was persisted, capture `final-db-before.log`, restart only the owned IM service, check health and unchanged MySQL start, capture `final-db-after.log` and compare bytes. `TestSeenAfterRestart`: PASS.
3. `TestPurgeSeenHistoryMySQL`: real `GroupHisMsgDao.DelMsgsBaseTime` restricted to this owned tenant; history empty, seen remains 3. PASS. Capture metadata, restart only IM, verify same metadata and binary plus unchanged MySQL start.
4. `TestSeenAfterRetention`: next message seq 4, seen 3, has_new true; both sessions actually receive its ACK ID, query that history ID and summary, unread 0. PASS.
5. Query until all three summaries contain that ACK ID. Independent SQL assertions prove seen exactly 3, floor exactly 4, exactly one history row at seq 4 matching ACK, and owner ordinary unread/read indexes 1/1. All six count assertions matched `1,1,1,1,3,1`.

The unmodified original retention regression was also rerun on the integrated source before strengthening its test, and passed with latest 4/seen 3. The original RED log remains unchanged (SHA256 `7d56fb14b45a6d3e26005dffc766c772eea5e39329c4e1aefb55d3da4d05f450`). A separate enhanced legacy-fixture pass later observed latest 5/seen 3.

## Real schema verification

The existing own database started at version 20261106. Starting the integrated service upgraded it to 20261107 with ONLY_FULL_GROUP_BY enabled; retained seen 3/history 1 seeded floor 3. A legacy summary sentinel kept its message ID and unread/read indexes 2/1.

A separate fresh database was initialized from committed initial SQL and started by an owned sidecar without host ports. Its startup reached 20261107 and its seed sentinel retained floor 7. SHOW CREATE output for `conversation_seen` and `message_sequence` matched the upgraded database byte-for-byte. Evidence: `fresh-init.log`, `upgrade-before.log`, `upgrade-after.log`, `schema-fresh.log`, `schema-upgraded.log`. This local run verifies successful fresh/upgrade behavior; supplier migration-failure tests were not rerun.

## Failures and limits kept visible

- First bind-mounted tests failed `TestExecuteSqlFileReturnsExecError`: Windows CRLF did not match that test's LF source-string expectation. The first ordinary Git archive also used checkout newline conversion. These failures are preserved in `tests.log`, `tests-canonical.log` and the CRLF archive/hash. The corrected per-command archive was verified against committed LF blobs before the successful run; no production code was changed for the test.
- Broad `go vet ./services/message/...` failed six existing unkeyed Mongo `primitive.E` literals in untouched storage files. `vet.log` preserves these diagnostics. The exact affected package vet above passed; broad vet is not reported all-green.
- The first enhanced purge-probe compile failed because its new client type import was missing. Compilation performed no purge. The corrected probe then passed; failed `enhanced-purge.log` is retained separately from `enhanced-purge-fixed.log`.
- Independent review found one Minor: the retention probe itself does not require exact unchanged seen or compare latest directly with history seq. This run's exact SQL assertions and logs cover those facts; future automated coverage could strengthen the assertions. No observed implementation defect was identified.
- Raw failing/null/skipped branches are not counted as successful checks. The old artifact and supplier artifact are not the final integrated artifact.
- Simulated Web/PC sessions are not real App/browser UI acceptance. Live Mongo, push/badge/sound, BIZ HTTP, mixed writers, production load, durable async-result/idempotency/outbox guarantees and production switching remain unverified.

## Resource snapshot

Only the owned `im-f1-read-20261106` project was started/restarted/stopped. After evidence capture, its main IM, fresh-schema IM and MySQL containers were inspected exited; volumes and images remained. IM exit codes were 137 with OOM=false; MySQL exit code 0. The stop command's success is not evidence of graceful IM shutdown. `final-resources.json` records this snapshot without credentials. Private environment and runtime fixture files remain ignored.
