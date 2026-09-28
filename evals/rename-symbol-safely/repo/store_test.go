package renamesymbolsafely

import "testing"

func TestBuildUsrLabel(t *testing.T) {
	if got := BuildUsrLabel(" Nando "); got != "user:nando" {
		t.Fatalf("got %q", got)
	}
}
