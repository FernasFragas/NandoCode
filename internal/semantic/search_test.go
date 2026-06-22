package semantic

import (
	"reflect"
	"testing"
)

func TestScoreCandidateIndicesLightModeKeepsExplicitAndCurrentTurn(t *testing.T) {
	t.Parallel()
	records := []Record{
		{Path: "pkg/auth/service.go", Name: "Service", TextPreview: "auth service"},
		{Path: "pkg/auth/token.go", Name: "Token", TextPreview: "token parser"},
		{Path: "pkg/payments/pay.go", Name: "Pay", TextPreview: "payment flow"},
		{Path: "pkg/payments/refund.go", Name: "Refund", TextPreview: "refund flow"},
		{Path: "pkg/ui/view.go", Name: "View", TextPreview: "render ui"},
		{Path: "docs/readme.md", Name: "README", TextPreview: "authentication bug notes"},
	}

	explicit := map[string]struct{}{
		normalizeRelPath("pkg/payments/pay.go"): {},
	}
	currentTurn := map[string]struct{}{
		normalizeRelPath("pkg/auth/service.go"): {},
	}

	got := scoreCandidateIndices(records, tokenizeQuery("fix authentication bug"), explicit, currentTurn, true)
	if len(got) == 0 {
		t.Fatalf("scoreCandidateIndices() returned no candidates")
	}
	if len(got) >= len(records) {
		t.Fatalf("scoreCandidateIndices() len=%d want < %d in light mode", len(got), len(records))
	}
	if !containsInt(got, 0) {
		t.Fatalf("missing current-turn candidate index 0")
	}
	if !containsInt(got, 2) {
		t.Fatalf("missing explicit candidate index 2")
	}
	if containsInt(got, 4) {
		t.Fatalf("unexpected unrelated candidate index 4")
	}
}

func TestScoreCandidateIndicesLightModeFallsBackToAllRecords(t *testing.T) {
	t.Parallel()
	records := []Record{
		{Path: "pkg/a.go"},
		{Path: "pkg/b.go"},
		{Path: "pkg/c.go"},
	}
	got := scoreCandidateIndices(records, tokenizeQuery("hi"), nil, nil, true)
	want := []int{0, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scoreCandidateIndices()=%v want %v", got, want)
	}
}

func TestScoreCandidateIndicesLightModeCapsCandidatesButKeepsForced(t *testing.T) {
	t.Parallel()
	records := make([]Record, 0, 700)
	for i := 0; i < 700; i++ {
		group := "000"
		if i%2 == 1 {
			group = "001"
		}
		records = append(records, Record{
			Path: "pkg/group_" + group + "/file_" + intToString(i) + ".go",
		})
	}

	explicitA := normalizeRelPath(records[698].Path)
	explicitB := normalizeRelPath(records[699].Path)
	explicit := map[string]struct{}{
		explicitA: {},
		explicitB: {},
	}
	got := scoreCandidateIndices(records, nil, explicit, nil, true)
	if len(got) != lightMaxCandidates {
		t.Fatalf("candidate len=%d want %d", len(got), lightMaxCandidates)
	}
	if !containsInt(got, 698) || !containsInt(got, 699) {
		t.Fatalf("forced explicit candidates missing from bounded output")
	}
}

func TestScoreRecordsDeterministicInLightMode(t *testing.T) {
	t.Parallel()
	records := []Record{
		{ID: "a", Path: "pkg/auth/a.go", Name: "AuthA", StartLine: 20, TextPreview: "auth"},
		{ID: "b", Path: "pkg/auth/b.go", Name: "AuthB", StartLine: 10, TextPreview: "auth"},
		{ID: "c", Path: "pkg/other/c.go", Name: "Other", StartLine: 10, TextPreview: "none"},
	}
	vectors := VectorSet{
		Dimensions: 2,
		Vectors: [][]float32{
			{1.0, 0.0},
			{1.0, 0.0},
			{1.0, 0.0},
		},
	}
	queryVec := []float32{1.0, 0.0}
	cfg := DefaultConfig()

	gotA, err := scoreRecords(
		"fix auth bug",
		queryVec,
		records,
		vectors,
		cfg,
		nil,
		[]string{"pkg/auth/a.go"},
		true,
	)
	if err != nil {
		t.Fatalf("scoreRecords() first error = %v", err)
	}
	gotB, err := scoreRecords(
		"fix auth bug",
		queryVec,
		records,
		vectors,
		cfg,
		nil,
		[]string{"pkg/auth/a.go"},
		true,
	)
	if err != nil {
		t.Fatalf("scoreRecords() second error = %v", err)
	}
	if !reflect.DeepEqual(gotA, gotB) {
		t.Fatalf("scoreRecords() produced non-deterministic ordering")
	}
}

func containsInt(values []int, target int) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
