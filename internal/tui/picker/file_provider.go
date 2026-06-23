package picker

import (
	"sort"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/tui/fileindex"
)

const (
	frecencyWeight = 6.0
)

// FileProvider suggests workspace files and directories.
type FileProvider struct {
	index    *fileindex.Index
	frecency *fileindex.Frecency
}

func NewFileProvider(idx *fileindex.Index, freq *fileindex.Frecency) *FileProvider {
	return &FileProvider{
		index:    idx,
		frecency: freq,
	}
}

func (p *FileProvider) Suggest(query string, limit int) []Suggestion {
	if p == nil || p.index == nil || limit <= 0 {
		return nil
	}
	q := normalizeQuery(query)
	if pathEscapesRoot(q) {
		return nil
	}
	entries := p.index.Snapshot()
	if len(entries) == 0 {
		return nil
	}
	if q == "" {
		return p.suggestEmpty(entries, limit)
	}

	candidates := make([]Suggestion, 0, len(entries))
	ql := strings.ToLower(q)
	queryMask, queryASCII := buildASCIIMask(ql)
	for _, entry := range entries {
		if queryASCII && !maskContains(entry.CharMask, queryMask) {
			continue
		}
		display := entry.Rel
		if entry.IsDir {
			display += "/"
		}
		displayLower := entry.RelLower
		if entry.IsDir {
			displayLower += "/"
		}
		score, matches, ok := fuzzyScore(ql, displayLower)
		if !ok {
			continue
		}
		if strings.HasPrefix(displayLower, ql) {
			score += 25
		}
		if p.frecency != nil {
			score += p.frecency.Score(entry.Rel) * frecencyWeight
		}
		detail := ""
		if entry.IsDir {
			detail = "dir"
		}
		candidates = append(candidates, Suggestion{
			Display:    display,
			Insert:     entry.Rel,
			Detail:     detail,
			IsDir:      entry.IsDir,
			MatchRunes: matches,
			Score:      score,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Display < candidates[j].Display
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates
}

func (p *FileProvider) suggestEmpty(entries []fileindex.Entry, limit int) []Suggestion {
	out := make([]Suggestion, 0, limit)
	if p.frecency != nil {
		type scored struct {
			entry fileindex.Entry
			score float64
		}
		list := make([]scored, 0, len(entries))
		for _, e := range entries {
			s := p.frecency.Score(e.Rel)
			if s <= 0 {
				continue
			}
			list = append(list, scored{entry: e, score: s})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].score == list[j].score {
				return list[i].entry.Rel < list[j].entry.Rel
			}
			return list[i].score > list[j].score
		})
		for _, s := range list {
			if len(out) >= limit {
				return out
			}
			out = append(out, toSuggestion(s.entry, nil, s.score*frecencyWeight))
		}
	}
	if len(out) >= limit {
		return out
	}

	top := make([]fileindex.Entry, 0, len(entries))
	for _, e := range entries {
		if strings.Contains(e.Rel, "/") {
			continue
		}
		if !e.IsDir {
			continue
		}
		top = append(top, e)
	}
	sort.Slice(top, func(i, j int) bool { return top[i].Rel < top[j].Rel })
	seen := map[string]struct{}{}
	for _, s := range out {
		seen[s.Insert] = struct{}{}
	}
	for _, e := range top {
		if len(out) >= limit {
			break
		}
		if _, ok := seen[e.Rel]; ok {
			continue
		}
		out = append(out, toSuggestion(e, nil, 0))
		seen[e.Rel] = struct{}{}
	}
	return out
}

func toSuggestion(entry fileindex.Entry, matches []int, score float64) Suggestion {
	display := entry.Rel
	detail := ""
	if entry.IsDir {
		display += "/"
		detail = "dir"
	}
	return Suggestion{
		Display:    display,
		Insert:     entry.Rel,
		Detail:     detail,
		IsDir:      entry.IsDir,
		MatchRunes: matches,
		Score:      score,
	}
}

func normalizeQuery(query string) string {
	q := strings.TrimSpace(query)
	q = strings.TrimPrefix(q, "./")
	q = strings.TrimPrefix(q, ".\\")
	q = strings.ReplaceAll(q, "\\", "/")
	for strings.Contains(q, "//") {
		q = strings.ReplaceAll(q, "//", "/")
	}
	return q
}

func pathEscapesRoot(q string) bool {
	if q == "" {
		return false
	}
	if strings.HasPrefix(q, "/") {
		return true
	}
	parts := strings.Split(q, "/")
	for _, p := range parts {
		if p == ".." {
			return true
		}
	}
	return false
}

func buildASCIIMask(s string) ([4]uint64, bool) {
	var out [4]uint64
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 128 {
			return out, false
		}
		out[b/64] |= 1 << (b % 64)
	}
	return out, true
}

func maskContains(candidate, query [4]uint64) bool {
	return (candidate[0]&query[0]) == query[0] &&
		(candidate[1]&query[1]) == query[1] &&
		(candidate[2]&query[2]) == query[2] &&
		(candidate[3]&query[3]) == query[3]
}
