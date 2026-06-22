package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/paths"
)

type EvidenceLedger struct {
	RunID      string          `json:"run_id"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Files      []string        `json:"files,omitempty"`
	Chunks     []FileChunkRef  `json:"chunks,omitempty"`
	Summaries  []SummaryRecord `json:"summaries,omitempty"`
	Conclusions []string       `json:"conclusions,omitempty"`
}

type FileChunkRef struct {
	Path      string `json:"path"`
	Chunk     int    `json:"chunk"`
	StartByte int    `json:"start_byte"`
	EndByte   int    `json:"end_byte"`
}

type SummaryRecord struct {
	Path    string `json:"path"`
	Source  string `json:"source"` // cache|fresh
	Summary string `json:"summary"`
}

func NewEvidenceLedger(runID string) EvidenceLedger {
	now := time.Now().UTC()
	return EvidenceLedger{
		RunID:     strings.TrimSpace(runID),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (l *EvidenceLedger) AddFile(path string) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return
	}
	l.Files = appendUnique(l.Files, path)
	l.touch()
}

func (l *EvidenceLedger) AddChunk(c FileChunk) {
	if strings.TrimSpace(c.Path) == "" {
		return
	}
	l.Chunks = append(l.Chunks, FileChunkRef{
		Path:      filepath.ToSlash(c.Path),
		Chunk:     c.ChunkIndex,
		StartByte: c.StartByte,
		EndByte:   c.EndByte,
	})
	l.touch()
}

func (l *EvidenceLedger) AddSummary(path, source, summary string) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	source = strings.TrimSpace(source)
	summary = strings.TrimSpace(summary)
	if path == "" || summary == "" {
		return
	}
	l.Summaries = append(l.Summaries, SummaryRecord{
		Path:    path,
		Source:  source,
		Summary: summary,
	})
	l.touch()
}

func (l *EvidenceLedger) AddConclusion(conclusion string) {
	conclusion = strings.TrimSpace(conclusion)
	if conclusion == "" {
		return
	}
	l.Conclusions = appendUnique(l.Conclusions, conclusion)
	l.touch()
}

func SaveEvidenceLedger(ledger EvidenceLedger) error {
	ledger.Files = canonicalizeList(ledger.Files)
	ledger.Conclusions = canonicalizeList(ledger.Conclusions)
	if ledger.CreatedAt.IsZero() {
		ledger.CreatedAt = time.Now().UTC()
	}
	ledger.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	path := evidenceLedgerPath(ledger.RunID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func LoadEvidenceLedger(runID string) (EvidenceLedger, error) {
	b, err := os.ReadFile(evidenceLedgerPath(runID))
	if err != nil {
		return EvidenceLedger{}, err
	}
	var l EvidenceLedger
	if err := json.Unmarshal(b, &l); err != nil {
		return EvidenceLedger{}, err
	}
	return l, nil
}

func evidenceLedgerPath(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = "latest"
	}
	return filepath.Join(paths.StateDir(), "analysis", "ledger-"+runID+".json")
}

func appendUnique(in []string, v string) []string {
	for _, existing := range in {
		if existing == v {
			return in
		}
	}
	return append(in, v)
}

func (l *EvidenceLedger) touch() {
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	l.UpdatedAt = time.Now().UTC()
	sort.Slice(l.Chunks, func(i, j int) bool {
		if l.Chunks[i].Path == l.Chunks[j].Path {
			return l.Chunks[i].Chunk < l.Chunks[j].Chunk
		}
		return l.Chunks[i].Path < l.Chunks[j].Path
	})
}

func ExtractMentionedPaths(prompt string) []string {
	if strings.TrimSpace(prompt) == "" {
		return nil
	}
	parts := strings.Fields(prompt)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if !strings.HasPrefix(p, "@") || len(p) < 2 {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(p, "@"))
		path = strings.Trim(path, ".,;:!?()[]{}\"'")
		if path == "" {
			continue
		}
		out = append(out, filepath.ToSlash(path))
	}
	return canonicalizeList(out)
}
