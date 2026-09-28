package bugfixwithtests

import "testing"

func TestInRangeValidIndex(t *testing.T) {
	if !InRange([]string{"a", "b"}, 1) {
		t.Fatal("expected index 1 to be in range")
	}
}
