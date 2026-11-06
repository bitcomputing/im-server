# Independent integrated READ review

Scope: read-only review of `4ecec887..a35c877`, including shared dependency `33d2b9b` integrated as `7ccc6e9`, and the version-matched private verification evidence. Reviewer: fresh independent agent `/root/review_integrated_read`. The reviewer made no edits, Git mutations, fixture changes or live service calls; inspecting evidence is distinct from independently running the services.

Verdict: **no Critical or Important blocker found for native MySQL READ with old writers stopped**. This is not global product or production acceptance.

The reviewer traced authenticated requester/scope handling, private participant-pair and group membership rules, visible persisted-history filtering, error returns, synchronous monotonic seen persistence, clear-unread independence, pre-side-effect sequence reservation, and fresh/upgrade schema handling. It independently checked all 30 source hashes against Git blobs, disk binary hashes, schema equality, ordinary restart DB equality and the supplied live-version records.

Evidence inspected includes canonical LF package tests with explicit SKIPs; fresh/upgrade metadata; the original retention RED-to-GREEN run; enhanced legacy retention; final fresh native before/restart/purge/retention logs; unchanged MySQL start and live-binary records. Final fresh retention reported latest 4, seen 3 and has_new true, with simulated Web/PC actual receipt and the same history/summary ID. The reviewer also inspected the final persisted-DB log and verification JSON, confirming floor 4, seen 3, one history row at seq 4 with the ACK ID, all three summaries at that ID and owner unread/read indexes 1/1.

One **Minor**, a future regression-coverage gap: `retention_test.go` requires `HasNew` and `LatestSeq > SeenSeq`, but does not itself require `SeenSeq == fixture.Seq` or compare `LatestSeq` directly with the observed history sequence. Current logs and separate exact SQL assertions establish those facts for this run. No current implementation defect was observed. Strengthening those assertions is optional follow-up; the gap is not counted as a failed runtime check or a production guarantee.

The reviewer declined acceptance of real App/browser UI, push/badge/sound, BIZ HTTP adapters, live Mongo/retention/bootstrap, mixed writers, production migration/load/topology, RELIABLE durable-result/idempotency/outbox/crash guarantees, skipped fixed-fixture tests, production switch/rollback and reconstruction after all metadata is erased. Broad vet warnings remain visible. None of these boundaries is counted as passed.
