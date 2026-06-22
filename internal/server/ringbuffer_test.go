package server

import "testing"

func TestRingBufferEvictionAndSnapshot(t *testing.T) {
	r := NewRingBuffer[int](3)
	r.Append(1)
	r.Append(2)
	r.Append(3)
	r.Append(4)
	got := r.Snapshot()
	want := []int{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%d want=%d", i, got[i], want[i])
		}
	}
}
