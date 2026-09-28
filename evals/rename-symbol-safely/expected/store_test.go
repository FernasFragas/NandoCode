package renamesymbolsafely

import "testing"

func TestBuildUserLabel(t *testing.T) {
	if got := BuildUserLabel(" Nando "); got != "user:nando" {
		t.Fatalf("got %q", got)
	}
}
