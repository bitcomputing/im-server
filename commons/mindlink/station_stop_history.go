package mindlink

import (
	"bytes"
	"crypto/sha256"
)

func stationHistoryCategory(kind, owner, sender string, payload, eventHash []byte) (string, error) {
	if eventHash == nil {
		return "message", nil
	}
	hash := sha256.Sum256(payload)
	if kind != "mindlink_group" || owner == "" || sender != owner || !bytes.Equal(eventHash, hash[:]) {
		return "", ErrUnknownResult
	}
	return "station_stopped", nil
}
