package basicrefactor

import "testing"

func TestParseLimit(t *testing.T) {
	got, err := ParseLimit(" 12 ")
	if err != nil {
		t.Fatal(err)
	}
	if got != 12 {
		t.Fatalf("got %d want 12", got)
	}
}

func TestParseOffsetRejectsNegative(t *testing.T) {
	if _, err := ParseOffset("-1"); err == nil {
		t.Fatal("expected negative offset error")
	}
}
