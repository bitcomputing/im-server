package bases

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"im-server/commons/mindlink"
	"im-server/commons/pbdefines/pbobjs"
)

func TestFilterNativeMessagesRejectsInvalidPayloads(t *testing.T) {
	for _, tc := range []struct {
		ctx      context.Context
		app      string
		messages []proto.Message
	}{{nil, "fixture-app", nil}, {context.Background(), "", nil}, {context.Background(), "fixture-app", []proto.Message{nil}}, {context.Background(), "fixture-app", []proto.Message{(*pbobjs.DownMsg)(nil)}}} {
		if mask, err := FilterNativeMessages(tc.ctx, tc.app, tc.messages); mask != nil || !errors.Is(err, mindlink.ErrInvalidRequest) {
			t.Fatal("invalid payload became a visibility mask", mask, err)
		}
	}
}

func TestFilterNativeMessagesWithoutInitializedPolicy(t *testing.T) {
	saved := nativePolicy.Swap(nil)
	t.Cleanup(func() { nativePolicy.Store(saved) })
	for _, messages := range [][]proto.Message{nil, {&pbobjs.DownMsg{SenderId: "ordinary"}}} {
		if mask, err := FilterNativeMessages(context.Background(), "fixture-app", messages); mask != nil || !errors.Is(err, mindlink.ErrNotReady) {
			t.Fatal("missing native policy produced a mask", mask, err)
		}
	}
}
