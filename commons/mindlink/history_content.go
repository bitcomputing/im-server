package mindlink

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"im-server/commons/pbdefines/pbobjs"
)

func historyContent(down *pbobjs.DownMsg) (HistoryMessage, error) {
	var empty HistoryMessage
	var content struct {
		Content string         `json:"content"`
		Assets  []MessageAsset `json:"assets,omitempty"`
		ReplyTo string         `json:"reply_to,omitempty"`
	}
	if down == nil || json.Unmarshal(down.MsgContent, &content) != nil || !utf8.ValidString(content.Content) || len(content.Content) > 65536 || content.ReplyTo != "" && !validMessageAssetID(content.ReplyTo) {
		return empty, ErrUnknownResult
	}
	result := HistoryMessage{Text: content.Content, ReplyTo: content.ReplyTo}
	if down.MsgType == "jg:text" {
		if len(content.Assets) != 0 {
			return empty, ErrUnknownResult
		}
		return result, nil
	}
	if down.MsgType != "jg:image" && down.MsgType != "jg:file" && down.MsgType != "jg:video" {
		return empty, ErrNotReady
	}
	ids := make([]string, len(content.Assets))
	for i, asset := range content.Assets {
		ids[i] = asset.FileID
	}
	if !validMessageAssets(&MessageSendRequest{MessageType: down.MsgType, AssetIDs: ids, Assets: content.Assets}) {
		return empty, ErrUnknownResult
	}
	// Decode media strictly so storage locations cannot become owner-visible history.
	if DecodeStrictObject(down.MsgContent, &content, []string{"content", "assets"}) != nil {
		return empty, ErrUnknownResult
	}
	result.ContentType, result.Assets = strings.TrimPrefix(down.MsgType, "jg:"), content.Assets
	return result, nil
}
