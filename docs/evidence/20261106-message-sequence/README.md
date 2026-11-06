# Private/group message sequence dependency

DG-20261104-003-F1-READ / existing F1-RELIABLE scope. Original IM manager implements the shared dependency in `fix/mindlink-sequence-watermark`, based on `8889b39`. READ's own branch and RPC files remain separate. This is a locally verified MySQL dependency, not final READ acceptance or a deployment.

Stored private/group messages reserve a sequence and send time in `message_sequence` before sendbox, history or delivery. Reservations use a MySQL row transaction and survive physical history retention. Allocation failure returns the existing internal-error code before those side effects. Sequence gaps are allowed: a committed reservation is not proof that asynchronous history storage succeeded. Legacy HTTP acceptance, durable request idempotency and outbox/result guarantees are unchanged.

Migration `20261107.sql` seeds binary-scoped retained private/group history and canonical READ maxima before services start, and preserves larger existing watermarks. The unchanged READ `20261106.sql` is included from candidate `008bd1fe` so the newer version cannot skip it. Initialization includes the same SQL. A failed migration or version-record write stops the upgrade; launcher exits 1 on upgrade failure. Old writers must be stopped for the verified upgrade mode.

## Actual checks

Own Docker project `im-f1-sequence-20261106`, Go 1.25.12 Linux runner and MySQL 8.0; API/nav/WS/admin bound only to `127.0.0.1:19701/19702/19703/18790`, with no host DB port. No existing deployment, READ stack, Rocky, pro/pre or other owner's volume was changed.

| Check | Actual result |
| --- | --- |
| Old allocator with real retention and cold generator | RED exit 1, sequence 4 fell to 1 |
| Same retention regression after fix | GREEN exit 0 |
| Real MySQL allocator suite | exit 0: retained-history and READ floors, tenant/private pair/subchannel/channel scope, 24 independent concurrent reservations, injected read/write failures and rollback, overflow; nonstored/other-channel behavior checked separately |
| Migration prerequisite and version-write errors | both RED exit 1 before fix, GREEN exit 0 after fix; later migration does not run |
| Migration seeds | exit 0: history/READ maxima, Group/group binary scopes, private a/a-space canonicalization, rerun never lowers a watermark |
| Version read error / invalid version | exit 0: upgrade rejects both |
| Project's named CI packages, affected vet, Linux launcher build | exit 0; packages with no tests are not runtime coverage |
| New database versus actual upgraded database | initialization exit 0; both metadata tables' SHOW CREATE output byte-identical; strict ONLY_FULL_GROUP_BY enabled; old sentinel retained; version 20261107 |
| Final-source live flow before restart | exit 0: private/group ACK sequence 1, actual Web/PC receipt, exact history IDs/sequences, real retention removes both histories but preserves watermarks and seen=1 |
| Only IM restart | MySQL start timestamp unchanged; watermark/seen dumps byte-identical; same final binary before/after |
| Final-source live flow after restart | exit 0: private/group sequence 2 > retained seen=1, actual Web/PC receipt and history; new IDs differ from expired messages |
| Actual allocation storage fault | both WS sends reject with 10009 and empty ID/sequence; no extra history, inbox/sendbox rows or client receipt observed in the one-second test window; temporary fault table restored |
| Actual startup failure | invalid-version fixture exits 1 before serving |

Final tested image: `sha256:3827b96e8a23b2c927f1e9dd2a7c39173ab465c0378335e31ca035161ce90090`. Disk and both live process binary SHA256: `a4458bccbfd2e05d0a2237fc7adf2757134950575ab33a87b1b7e74505af52fb`. Source and nonsecret evidence hashes are in [manifest.json](manifest.json); text source hashes normalize CRLF to LF for comparison with Git blobs. Earlier intermediate builds are not substituted for this final-source run.

Independent static review found the migration-skipping defect; its concrete error injections and fix are recorded above. Final reread found no remaining blocker for MySQL with old writers stopped. The reviewer did not independently run DB/live checks; its Windows compile attempt hit the known existing metrics Swap fields limitation. Runtime receipt/sendbox checks were subsequently strengthened and actually rerun by the implementer.

## Remaining boundaries

READ owner must integrate this exact dependency in its isolated branch and run `e3957ea` plus READ regression and independent review. Native READ RPC / business §11.3 / real App or browser UI / phone push are not accepted by this patch. No merge, push, Rocky switch or release occurred.

Mongo bootstrap aggregation is retained but not live-tested; MySQL migration seeds do not scan Mongo history. Physical Mongo retention requires backend-aware startup seeding before it is enabled; this patch is not Mongo retention acceptance. History already removed before upgrade, with no remaining READ/history metadata, cannot be reconstructed. Mixed old/new writers and production-size migration/latency/load have not been validated. Each stored private/group allocation now performs a durable row transaction; throughput costs need measurement before production rollout.

Private configuration, credentials, full database and fixtures stay in ignored `.local/f1-sequence/`. Both trial/final test tenants, test databases, volumes, image and binary are retained; own runtime containers are stopped after verification. IM/MySQL exit 0, expected startup-error container exit 1, and idle runner tail exits 137 on stop after all tests have returned; this is not a failed/interrupted test result. Resource state is a recorded snapshot, not a continuing guarantee.
