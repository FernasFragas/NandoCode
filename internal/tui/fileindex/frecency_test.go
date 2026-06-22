package fileindex

import "testing"

func TestFrecencyTouchScoreDecay(t *testing.T) {
	t.Parallel()
	f := NewFrecency()
	f.Touch("a.txt")
	f.Touch("a.txt")
	if got := f.Score("a.txt"); got < 2 {
		t.Fatalf("score=%v", got)
	}
	f.Decay()
	if got := f.Score("a.txt"); got >= 2 {
		t.Fatalf("score not decayed: %v", got)
	}
}
