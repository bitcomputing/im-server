package mindlink

import (
	"encoding/json"
	"testing"

	"im-server/commons/pbdefines/pbobjs"
)

func TestHistoryContentPreservesMediaAndReferenceWithoutStorageCoordinates(t *testing.T) {
	assets := []MessageAsset{{FileID: "up_a", Name: "photo.png", MIMEType: "image/png", Size: 20}}
	raw, _ := json.Marshal(map[string]any{"content": "caption", "assets": assets, "reply_to": "original_1"})
	m, err := historyContent(&pbobjs.DownMsg{MsgType: "jg:image", MsgContent: raw})
	if err != nil || m.Text != "caption" || m.ContentType != "image" || m.ReplyTo != "original_1" || len(m.Assets) != 1 || m.Assets[0].FileID != "up_a" {
		t.Fatal(m, err)
	}
	for _, bad := range []string{`{"content":"","assets":[]}`, `{"assets":[{"file_id":"up_a","url":"https://public/secret"}]}`, `{"content":"caption","assets":null}`} {
		if _, err = historyContent(&pbobjs.DownMsg{MsgType: "jg:image", MsgContent: []byte(bad)}); err == nil {
			t.Fatal("invalid media history accepted", bad)
		}
	}
	plain, err := historyContent(&pbobjs.DownMsg{MsgType: "jg:text", MsgContent: []byte(`{"content":"old"}`)})
	if err != nil || plain.ContentType != "" || plain.Text != "old" || len(plain.Assets) != 0 {
		t.Fatal("legacy history changed", plain, err)
	}
}
