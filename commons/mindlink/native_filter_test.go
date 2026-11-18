package mindlink

import (
	"context"
	"errors"
	"testing"
)

func TestNativeFilterMissingFenceFailsEvenForEmptyBatch(t *testing.T) {
	for _, fence := range []*NativeFence{nil, {}} {
		for _, rows := range [][]NativeIdentities{nil, {{Users: []string{"ordinary"}}}} {
			mask, err := fence.Filter(context.Background(), "fixture-app", rows)
			if mask != nil || !errors.Is(err, ErrNotReady) {
				t.Fatal("uninitialized classifier became a mask", mask, err)
			}
		}
	}
}
