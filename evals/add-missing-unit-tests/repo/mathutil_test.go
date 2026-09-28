package addmissingunittests

import "testing"

func TestClampWithinBounds(t *testing.T) {
	if got := Clamp(3, 1, 5); got != 3 {
		t.Fatalf("got %d want 3", got)
	}
}
