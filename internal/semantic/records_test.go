package semantic

import "testing"

func TestDeterministicRecordID(t *testing.T) {
	t.Parallel()
	idA := deterministicRecordID(RecordKindSymbol, "a/b.go", "Fn", 10, 20, "abc")
	idB := deterministicRecordID(RecordKindSymbol, "a/b.go", "Fn", 10, 20, "abc")
	idC := deterministicRecordID(RecordKindSymbol, "a/b.go", "Fn", 11, 20, "abc")

	if idA == "" {
		t.Fatal("expected non-empty id")
	}
	if idA != idB {
		t.Fatalf("expected stable ids, got %q != %q", idA, idB)
	}
	if idA == idC {
		t.Fatalf("expected ids to differ for line-range changes")
	}
}

func TestSortRecordsStable(t *testing.T) {
	t.Parallel()
	records := []Record{
		{Path: "z/b.txt", Kind: RecordKindChunk, StartLine: 1, EndLine: 2, Name: "chunk-2", ID: "3"},
		{Path: "a/main.go", Kind: RecordKindSymbol, StartLine: 10, EndLine: 12, Name: "B", ID: "2"},
		{Path: "a/main.go", Kind: RecordKindFile, StartLine: 1, EndLine: 30, Name: "main.go", ID: "1"},
		{Path: "a/main.go", Kind: RecordKindSymbol, StartLine: 2, EndLine: 3, Name: "A", ID: "4"},
	}

	sortRecords(records)

	got := []string{records[0].ID, records[1].ID, records[2].ID, records[3].ID}
	want := []string{"1", "4", "2", "3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order at %d: got %v want %v", i, got, want)
		}
	}
}
