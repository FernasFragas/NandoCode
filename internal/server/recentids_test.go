package server

import "testing"

func TestRecentIDsSeenOrAdd(t *testing.T) {
	r := NewRecentIDs(2)
	if r.SeenOrAdd("a") {
		t.Fatal("first add must be unseen")
	}
	if !r.SeenOrAdd("a") {
		t.Fatal("second add must be seen")
	}
	r.SeenOrAdd("b")
	r.SeenOrAdd("c") // evicts a
	if r.SeenOrAdd("a") {
		t.Fatal("a should be evicted and unseen now")
	}
}
