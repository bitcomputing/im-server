package mindlink

import (
	"strings"
	"testing"
)

func mediaRequest(t *testing.T) MessageSendRequest {
	t.Helper()
	r := messageRequest()
	r.Version, r.MessageType, r.Text = "mindlink-reliable-v2", "jg:image", ""
	r.AssetIDs = []string{"up_a"}
	r.Assets = []MessageAsset{{FileID: "up_a", Name: "图片.png", MIMEType: "image/png", Size: 20}}
	_, a := messageProof(t, string(fixtureJSON(t, messageRequest())), OperationMessageSend, "POST", MessageSendPath, nil)
	var err error
	r.RequestDigest, err = MessageSendDigest(a.claims, &r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMessageMediaRequiresVersionedBoundAssetMetadata(t *testing.T) {
	r := mediaRequest(t)
	v, a := messageProof(t, string(fixtureJSON(t, r)), OperationMessageSend, "POST", MessageSendPath, func(c map[string]any) { c["request_digest"] = r.RequestDigest })
	got, err := v.ParseMessageSend(a)
	if err != nil || got == nil || len(got.Assets) != 1 || got.Assets[0].FileID != "up_a" {
		t.Fatal("valid captionless media rejected", err)
	}
	changed := r
	changed.Assets = append([]MessageAsset(nil), r.Assets...)
	changed.Assets[0].Name = "other.png"
	hash, err := MessageSendDigest(a.claims, &changed)
	if err != nil || hash == r.RequestDigest {
		t.Fatal("asset metadata not bound to digest", err)
	}
}

func TestMessageMediaRejectsInvalidAssetsEvenWhenSigned(t *testing.T) {
	for _, change := range []func(*MessageSendRequest){
		func(r *MessageSendRequest) { r.Version = MessageWireVersion },
		func(r *MessageSendRequest) { r.AssetIDs = []string{"up_other"} },
		func(r *MessageSendRequest) { r.Assets = nil },
		func(r *MessageSendRequest) { r.Assets[0].Size = 0 },
		func(r *MessageSendRequest) { r.Assets[0].Name = "../secret.png" },
		func(r *MessageSendRequest) { r.Assets[0].MIMEType = "not a MIME" },
		func(r *MessageSendRequest) { r.Assets[0].Name = strings.Repeat("x", 1025) },
	} {
		r := mediaRequest(t)
		change(&r)
		_, original := messageProof(t, string(fixtureJSON(t, messageRequest())), OperationMessageSend, "POST", MessageSendPath, nil)
		r.RequestDigest, _ = MessageSendDigest(original.claims, &r)
		v, a := messageProof(t, string(fixtureJSON(t, r)), OperationMessageSend, "POST", MessageSendPath, func(c map[string]any) { c["request_digest"] = r.RequestDigest })
		if result, err := v.ParseMessageSend(a); result != nil || err == nil {
			t.Fatal("invalid signed media accepted", string(fixtureJSON(t, r)))
		}
	}
}
