package analysis

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/tui/fileindex"
)

type FrecencyScorer interface {
	Score(rel string) float64
}

// RetrieveTopFiles ranks files for broad analysis prompts.
// Explicit mentions should still outrank this retrieval in caller flow.
func RetrieveTopFiles(entries []fileindex.Entry, question string, rootHint string, freq FrecencyScorer, limit int) []string {
	if limit <= 0 || len(entries) == 0 {
		return nil
	}
	qTerms := queryTerms(question)
	rootHint = normalizeRootHint(rootHint)

	type candidate struct {
		rel   string
		score float64
	}
	cands := make([]candidate, 0, len(entries))
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		rel := filepath.ToSlash(e.Rel)
		if rootHint != "" && rootHint != "." && !strings.HasPrefix(rel, rootHint+"/") && rel != rootHint {
			continue
		}
		score := scorePath(rel, qTerms)
		if freq != nil {
			score += freq.Score(rel) * 3.0
		}
		if score <= 0 {
			continue
		}
		cands = append(cands, candidate{rel: rel, score: score})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score == cands[j].score {
			return cands[i].rel < cands[j].rel
		}
		return cands[i].score > cands[j].score
	})
	if len(cands) > limit {
		cands = cands[:limit]
	}
	out := make([]string, 0, len(cands))
	seen := map[string]struct{}{}
	for _, c := range cands {
		if _, ok := seen[c.rel]; ok {
			continue
		}
		seen[c.rel] = struct{}{}
		out = append(out, c.rel)
	}
	return out
}

func normalizeRootHint(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "./")
	s = strings.TrimSuffix(s, "/")
	s = filepath.ToSlash(s)
	return s
}

func queryTerms(q string) []string {
	raw := strings.Fields(strings.ToLower(q))
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, t := range raw {
		t = strings.Trim(t, ".,;:!?()[]{}\"'")
		if len(t) < 3 {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func scorePath(rel string, terms []string) float64 {
	if len(terms) == 0 {
		return 0
	}
	p := strings.ToLower(rel)
	base := strings.ToLower(filepath.Base(rel))
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(rel)), ".")
	score := 0.0
	for _, t := range terms {
		if strings.Contains(base, t) {
			score += 30
		}
		if strings.Contains(p, "/"+t+"/") || strings.HasPrefix(p, t+"/") {
			score += 16
		}
		if strings.Contains(p, t) {
			score += 8
		}
		if ext != "" && t == ext {
			score += 12
		}
	}
	return score
}
