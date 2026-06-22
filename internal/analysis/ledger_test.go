package analysis

import (
	"os"
	"strings"
	"testing"
)

func TestEvidenceLedgerSaveLoad(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	ledger := NewEvidenceLedger("run-1")
	ledger.AddFile("internal/tui/app.go")
	ledger.AddChunk(FileChunk{Path: "internal/tui/app.go", ChunkIndex: 0, StartByte: 0, EndByte: 120})
	ledger.AddSummary("internal/tui/app.go", "fresh", "TUI message flow")
	ledger.AddConclusion("Checkpoint resume is pending.")
	if err := SaveEvidenceLedger(ledger); err != nil {
		t.Fatalf("save ledger: %v", err)
	}
	got, err := LoadEvidenceLedger("run-1")
	if err != nil {
		t.Fatalf("load ledger: %v", err)
	}
	if len(got.Files) != 1 || got.Files[0] != "internal/tui/app.go" {
		t.Fatalf("unexpected files: %#v", got.Files)
	}
	if len(got.Summaries) != 1 {
		t.Fatalf("unexpected summaries: %#v", got.Summaries)
	}
}

func TestExtractMentionedPaths(t *testing.T) {
	got := ExtractMentionedPaths("review @internal/tui/app.go and @docs/PHASE-LOG.md then @internal/tui/app.go")
	if len(got) != 2 {
		t.Fatalf("unexpected paths: %#v", got)
	}
	if got[0] != "docs/PHASE-LOG.md" && got[1] != "internal/tui/app.go" {
		t.Fatalf("unexpected sorted paths: %#v", got)
	}
}

func TestLoadEvidenceLedgerMissing(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	_, err := LoadEvidenceLedger("missing")
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestEvidenceLedgerConclusionDedup(t *testing.T) {
	l := NewEvidenceLedger("x")
	l.AddConclusion("same")
	l.AddConclusion(strings.TrimSpace(" same "))
	if len(l.Conclusions) != 1 {
		t.Fatalf("expected deduped conclusions, got %#v", l.Conclusions)
	}
}
