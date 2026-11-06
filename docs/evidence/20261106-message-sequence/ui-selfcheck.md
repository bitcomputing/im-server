# Browser and database self-check

This supplements the verification of code commit `33d2b9bb1a8aad82d95ed25d890476e9496d5546`; no implementation changed. On 2026-11-06, a real headed Chrome session used the existing isolated IM management UI at `http://127.0.0.1:18790`. The active process binary remained `a4458bccbfd2e05d0a2237fc7adf2757134950575ab33a87b1b7e74505af52fb`. The original 16 source hashes and 24 evidence hashes were rechecked unchanged.

The previous delivery had DB and native WebSocket evidence but no browser run. That gap was not counted as a pass. The browser now actually logged in, showed both retained test tenants, selected the existing final test tenant, and used its API-debug page to send one stored, non-counting private message and one group message to the existing test users/group. It then queried both histories through the same page. No production endpoint or other owner's service was used.

| Browser result | Actual database result |
| --- | --- |
| Private send: code 0, ID `pbyc3b68aachlzrr`; browser history contains that exact ID | One matching private history row, sequence 3; private watermark 3, seen remains 1 |
| Group send: code 0, ID `pbyc3tys2agkbg4m`; browser history contains that exact ID | One matching group history row, sequence 3; group watermark 3, seen remains 1 |

The existing history HTTP response includes message IDs and does **not** include a message-sequence field. Sequence values above come from actual DB queries, not from inferring a field on the page. This verifies the existing management send/history flow with the changed schema; it does not accept the unimplemented business READ page, its entry indicator, or the published App.

Nonsecret captures: [dashboard](ui/dashboard.png), [private response](ui/private-response.png), [group response](ui/group-response.png), [final DB check](ui/final-db-verify.txt), [private browser history result](ui/private-history-result.txt), [group browser history result](ui/group-history-result.txt). Response screenshots contain IDs/status only; request bodies, credentials, secret-unmask actions, auth storage and message-body logs are not included. [ui-manifest.json](ui-manifest.json) records artifact hashes.

Self-check results:

1. Evidence: commits, original source/evidence hashes, current binary, browser responses and matching DB rows were actually checked.
2. Schema workflow: existing browser private/group sends and history queries plus DB checks passed. New READ/business UI, Mongo and published clients remain unverified.
3. Failed attempts remain failed: the first fixture startup raced MySQL readiness and exited 1 with connection refused. The ignored fixture gained a healthy-DB dependency and the same binary then started successfully. One navigation click timed out outside the viewport; it was actually completed after resizing. Neither failure was counted as a completed operation.
4. Residue: no unknown/replayed send occurred; the two new IDs each have exactly one DB row. The temporary sequence-fault table count is still zero. Two test tenants and their data/volumes remain intentionally retained. Browser close succeeded, its original PID no longer exists, and the self-owned IM/MySQL are stopped with exit 0. Idle runner exit 137 and the expected startup-error fixture exit 1 retain their earlier meaning.
5. Time: these are observed snapshots. The original only-IM restart proof was a separate earlier window with unchanged MySQL start time; this browser supplement explicitly restarted the stopped test fixture. No continuous availability or unchanging sequence state is claimed.

Code remains on its isolated branch; no merge, push, release or Rocky switch occurred. READ still requires its owner's integrated regression and independent acceptance.
