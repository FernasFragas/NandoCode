package semantic

import (
	"errors"
	"testing"
)

func TestIsFallbackError(t *testing.T) {
	t.Parallel()
	for _, err := range []error{ErrDisabled, ErrIndexMissing, ErrIndexStale, ErrModelMissing} {
		if !IsFallbackError(err) {
			t.Fatalf("expected fallback error for %v", err)
		}
	}
	if IsFallbackError(ErrSchemaMismatch) {
		t.Fatalf("schema mismatch should not be fallback")
	}
	if IsFallbackError(errors.New("x")) {
		t.Fatalf("unknown error should not be fallback")
	}
}
