package mindlink

import (
	"context"
	"errors"
	"testing"
)

func TestNativeFenceWithoutDatabaseFailsClosed(t *testing.T) {
	f, err := NewNativeFence(context.Background(), nil)
	if f != nil || !errors.Is(err, ErrNotReady) {
		t.Fatalf("missing database allowed native fence: %v", err)
	}
	if err := (*NativeFence)(nil).Check(context.Background(), "fixture-app", []string{disablePrincipal}, nil); !errors.Is(err, ErrNotReady) {
		t.Fatalf("zero fence allowed a native request: %v", err)
	}
}
