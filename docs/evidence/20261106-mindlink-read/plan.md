# F1-READ implementation plan

Goal: no-count messages retain history/summary while an independent, durable conversation reading position merges monotonically across devices.

Spec: DG-20261104-003-F1-READ; product 私聊-产品定稿; chatbot43/docs/mindlink-conversations-contract.md §8/11.3. Base: 4ecec887fb38d2d81af4bf2773b05d304e8ced97.

Constraints: preserve ordinary unread counting and all old protobuf tags. Only released conversation files, independent mindlinkread proto/generated file, 20261106.sql, new mindlinkread probes and this evidence directory are writable. Original IM manager owns shared SQL initialization, routing/errors/proto and final integration. No production/pre/Rocky/shared fixtures, pushes, merges or deployments.

Approach: use stored history msg_seq_no (not unread_index) as the candidate ordering source; store max seen_seq under tenant/current user/target/channel/subchannel. New query/set RPCs use authenticated requester identity; SQL errors fail, forged or unauthorized positions fail before writes. Existing clear_unread does not mark the entry seen. Additive proto and a separate table avoid sharing the delayed PERSIST snapshot. HTTP/business adapter/contract integration remains with the original managers and is a separate dependency.

## Tasks

- [ ] Storage: failing real-MySQL schema/max-merge tests; additive migration; DAO with synchronous monotonic upsert. Test duplicate/older/concurrent calls, tenant/user/subchannel isolation, error propagation and reconnect/reload.
- [ ] RPC: independent proto (`target_id=1, channel_type=2, sub_channel=3, seen_seq=4`; response `seen_seq=1, latest_seq=2, has_new=3`), registered query/set actors using current requester. Validate history/membership/clean-time visibility and maximum stored sequence; no mutation from query or legacy unread clearing. Demonstrate missing capability before implementation.
- [ ] Runtime: own MySQL/IM/ports/volumes, counted + no-count history/receipt/summary, two actual WS sessions, stale/duplicate/invalid reads, independent ordinary unread, SQL row verification, IM-only restart and version/SHA evidence. Fresh and upgrade schema paths preserve old data. Independent final review, applicable tests/build/vet, local commit only.

## Review focus

Check requester versus client-controlled RPC target; cross-tenant/group/subchannel and restricted history; ordering independent of zero unread index; DB failures versus empty history; asynchronous history persistence and restart ordering; no reset by clear_total_unread or old snapshots. App/push/badge/browser and RELIABLE crash guarantees remain unverified unless actually exercised.

Plan approval: existing owner-approved product and current verified dispatch explicitly authorize implementation after file release; no new product choice is being requested. Interface proposal was returned to the original IM manager; unresolved adapter contracts must remain labeled proposals.
