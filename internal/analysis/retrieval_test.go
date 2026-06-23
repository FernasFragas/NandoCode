package analysis

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/tui/fileindex"
)

type fakeFreq struct {
	scores map[string]float64
}

func (f fakeFreq) Score(rel string) float64 {
	return f.scores[rel]
}

func TestRetrieveTopFiles_RanksByQuestionAndRoot(t *testing.T) {
	entries := []fileindex.Entry{
		{Rel: "docs/PHASE-LOG.md"},
		{Rel: "internal/tui/app.go"},
		{Rel: "internal/agent/agent.go"},
		{Rel: "README.md"},
	}
	got := RetrieveTopFiles(entries, "review tui run status latency", "internal", nil, 3)
	if len(got) == 0 {
		t.Fatal("expected ranked files")
	}
	if got[0] != "internal/tui/app.go" {
		t.Fatalf("expected top match internal/tui/app.go, got %q", got[0])
	}
}

func TestRetrieveTopFiles_FrecencyBoost(t *testing.T) {
	entries := []fileindex.Entry{
		{Rel: "internal/a.go"},
		{Rel: "internal/b.go"},
	}
	freq := fakeFreq{scores: map[string]float64{"internal/b.go": 10}}
	got := RetrieveTopFiles(entries, "internal", ".", freq, 2)
	if len(got) < 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0] != "internal/b.go" {
		t.Fatalf("expected frecency promoted path first, got %q", got[0])
	}
}

func TestRetrieveTopFiles_Deterministic(t *testing.T) {
	entries := []fileindex.Entry{
		{Rel: "internal/tui/app.go"},
		{Rel: "internal/tui/slash.go"},
		{Rel: "internal/agent/agent.go"},
		{Rel: "docs/PHASE-LOG.md"},
	}
	first := RetrieveTopFiles(entries, "review tui app behavior", ".", nil, 3)
	second := RetrieveTopFiles(entries, "review tui app behavior", ".", nil, 3)
	if len(first) != len(second) {
		t.Fatalf("length differs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("non-deterministic result at index %d: %q != %q", i, first[i], second[i])
		}
	}
}

func BenchmarkRetrieveTopFiles_50000Entries(b *testing.B) {
	entries := make([]fileindex.Entry, 0, 50_000)
	for i := 0; i < 50_000; i++ {
		rel := filepath.ToSlash(filepath.Join("pkg", "module"+strconv.Itoa(i%400), "file"+strconv.Itoa(i)+".go"))
		entries = append(entries, fileindex.Entry{Rel: rel})
	}
	q := "analyze module123 agent context memory hooks config"
	freq := fakeFreq{scores: map[string]float64{
		"pkg/module123/file10123.go": 3,
		"pkg/module123/file25123.go": 2,
	}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got := RetrieveTopFiles(entries, q, ".", freq, 20)
		if len(got) > 20 {
			b.Fatalf("limit exceeded: %d", len(got))
		}
	}
}

func BenchmarkRetrieveTopFiles_RootScoped(b *testing.B) {
	entries := make([]fileindex.Entry, 0, 20_000)
	for i := 0; i < 20_000; i++ {
		rel := "internal/" + "sub" + strconv.Itoa(i%120) + "/f" + strconv.Itoa(i) + ".go"
		if i%3 == 0 {
			rel = "docs/section" + strconv.Itoa(i%40) + "/d" + strconv.Itoa(i) + ".md"
		}
		entries = append(entries, fileindex.Entry{Rel: rel})
	}
	q := strings.Repeat("internal ", 8) + "agent context"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got := RetrieveTopFiles(entries, q, "internal", nil, 30)
		if len(got) > 30 {
			b.Fatalf("limit exceeded: %d", len(got))
		}
		for _, p := range got {
			if !strings.HasPrefix(p, "internal/") {
				b.Fatalf("unexpected non-scoped result: %q", p)
			}
		}
	}
}
