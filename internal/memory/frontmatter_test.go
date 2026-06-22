package memory

import (
	"strings"
	"testing"
	"time"
)

func TestParseFrontmatterSuccess(t *testing.T) {
	src := `---
name: Testing Policy
description: Use real DB in integration tests
type: feedback
---
Body`
	e, err := ParseFrontmatter("feedback_testing.md", strings.NewReader(src), time.Unix(10, 0), int64(len(src)))
	if err != nil {
		t.Fatalf("ParseFrontmatter returned err: %v", err)
	}
	if e.Type != TypeFeedback {
		t.Fatalf("unexpected type: %q", e.Type)
	}
	if e.Name != "Testing Policy" {
		t.Fatalf("unexpected name: %q", e.Name)
	}
}

func TestParseFrontmatterRejectsMissingFields(t *testing.T) {
	src := `---
name: X
type: user
---
Body`
	if _, err := ParseFrontmatter("user_x.md", strings.NewReader(src), time.Now(), int64(len(src))); err == nil {
		t.Fatalf("expected error for missing description")
	}
}
