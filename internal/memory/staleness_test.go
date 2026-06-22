package memory

import (
	"strings"
	"testing"
	"time"
)

func TestStalenessWarningCalendarDays(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	today := time.Date(2026, 5, 3, 1, 0, 0, 0, time.UTC)
	yesterday := time.Date(2026, 5, 2, 23, 0, 0, 0, time.UTC)
	older := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)

	if w := StalenessWarning(now, today); w != "" {
		t.Fatalf("expected no warning for today, got %q", w)
	}
	if w := StalenessWarning(now, yesterday); w != "" {
		t.Fatalf("expected no warning for yesterday, got %q", w)
	}
	if w := StalenessWarning(now, older); !strings.Contains(w, "Before recommending from memory") {
		t.Fatalf("expected action cue warning, got %q", w)
	}
}
