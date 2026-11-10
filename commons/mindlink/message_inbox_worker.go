package mindlink

import "context"

// DrainOwnerInbox makes a bounded pass. The cursor moves past failed entries so
// one revoked account cannot starve every later account; the next pass retries it.
func (h *PolicyHTTP) DrainOwnerInbox(ctx context.Context, after string) (string, int, error) {
	if h == nil || h.verifier == nil || h.db == nil {
		return "", 0, ErrNotReady
	}
	if ctx == nil || after != "" && !isLowerHex(after, 64) {
		return "", 0, ErrInvalidRequest
	}
	rows, err := h.db.QueryContext(ctx, `SELECT outbox_id FROM mindlink_message_outbox WHERE state='pending' AND outbox_id>? ORDER BY outbox_id LIMIT 32`, after)
	if err != nil {
		return after, 0, ErrUnknownResult
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return after, 0, ErrUnknownResult
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return after, 0, ErrUnknownResult
	}
	if len(ids) == 0 {
		return "", 0, nil
	}
	n := 0
	var failed error
	for _, id := range ids {
		ok, err := h.verifier.MaterializeOwnerInbox(ctx, h.db, id)
		if err != nil {
			failed = ErrUnknownResult
		} else if ok {
			n++
		}
	}
	return ids[len(ids)-1], n, failed
}
