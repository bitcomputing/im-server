package mindlink

import (
	"mime"
	"strings"
	"unicode/utf8"
)

// No URL or object-store coordinate is accepted in the durable IM snapshot.
type MessageAsset struct {
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	MIMEType  string `json:"mime_type"`
	Size      uint64 `json:"size"`
	ExpiresAt int64  `json:"expires_at"`
	Width     int64  `json:"width,omitempty"`
	Height    int64  `json:"height,omitempty"`
	Duration  int64  `json:"duration,omitempty"`
}

func validMessageAssetID(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for _, c := range []byte(id) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func validMessageAssets(body *MessageSendRequest) bool {
	maxSize := uint64(0)
	switch body.MessageType {
	case "jg:image":
		maxSize = 10 << 20
	case "jg:file":
		maxSize = 50 << 20
	case "jg:video":
		maxSize = 1 << 30
	default:
		return false
	}
	if len(body.AssetIDs) < 1 || len(body.AssetIDs) > 10 || len(body.Assets) != len(body.AssetIDs) {
		return false
	}
	for i, a := range body.Assets {
		media, _, err := mime.ParseMediaType(a.MIMEType)
		if !validMessageAssetID(a.FileID) || a.FileID != body.AssetIDs[i] || i > 0 && body.AssetIDs[i-1] >= a.FileID ||
			!utf8.ValidString(a.Name) || a.Name == "" || a.Name == "." || a.Name == ".." || len(a.Name) > 1024 || strings.ContainsAny(a.Name, "/\\\x00\r\n") ||
			err != nil || len(a.MIMEType) > 255 || media != a.MIMEType || a.Size == 0 || a.Size > maxSize || a.ExpiresAt < 0 || a.Width < 0 || a.Height < 0 || a.Duration < 0 {
			return false
		}
	}
	return true
}
