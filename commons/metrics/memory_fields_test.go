package metrics

import "testing"

func TestMemoryStatBytesOptionalPlatformFields(t *testing.T) {
	stats := struct{ SwapTotal uint64 }{42}
	if got := memoryStatBytes(&stats, "SwapTotal", 0); got != 42 {
		t.Fatalf("swap total = %d", got)
	}
	for _, input := range []any{&stats, nil, (*int)(nil), struct{ SwapUsed string }{"42"}} {
		if got := memoryStatBytes(input, "SwapUsed", 7); got != 7 {
			t.Fatalf("missing or invalid field = %d", got)
		}
	}
}
