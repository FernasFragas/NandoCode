package bugfixwithtests

import "testing"

func TestInRangeValidIndex(t *testing.T) {
	if !InRange([]string{"a", "b"}, 1) {
		t.Fatal("expected index 1 to be in range")
	}
}

func TestInRangeRejectsUpperBound(t *testing.T) {
	if InRange([]string{"a", "b"}, 2) {
		t.Fatal("expected upper bound index to be out of range")
	}
}
