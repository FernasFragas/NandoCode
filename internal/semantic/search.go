package semantic

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	lightMaxCandidates = 512
)

func scoreRecords(
	query string,
	queryVec []float32,
	records []Record,
	vectors VectorSet,
	cfg Config,
	explicitPaths []string,
	currentTurnPaths []string,
	useCurrentPathWeight bool,
) ([]SearchHit, error) {
	if err := ValidateVectorSet(vectors, vectors.Dimensions, len(records)); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDimensionsMismatch, err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	terms := tokenizeQuery(query)
	explicit := map[string]struct{}{}
	for _, p := range explicitPaths {
		p = normalizeRelPath(p)
		if p != "" {
			explicit[p] = struct{}{}
		}
	}
	currentTurn := map[string]struct{}{}
	if useCurrentPathWeight {
		for _, p := range currentTurnPaths {
			p = normalizeRelPath(p)
			if p != "" {
				currentTurn[p] = struct{}{}
			}
		}
	}
	candidateIdx := scoreCandidateIndices(records, terms, explicit, currentTurn, useCurrentPathWeight)

	hits := make([]SearchHit, 0, len(candidateIdx))
	for _, i := range candidateIdx {
		vec := vectors.Vectors[i]
		dot, err := Dot(queryVec, vec)
		if err != nil {
			continue
		}
		rec := records[i]
		lex := lexicalScore(rec, terms)
		score := float64(dot) + cfg.HybridLexicalWeight*lex
		reason := "vector"
		if lex > 0 {
			reason = "vector+lexical"
		}
		if _, ok := explicit[rec.Path]; ok {
			score += 0.35
			reason += "+explicit"
		}
		if useCurrentPathWeight {
			if _, ok := currentTurn[normalizeRelPath(rec.Path)]; ok {
				score += 0.25
				reason += "+current_turn"
			}
		}
		hits = append(hits, SearchHit{
			Record: rec,
			Score:  score,
			Reason: reason,
		})
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			if hits[i].Record.Path == hits[j].Record.Path {
				if hits[i].Record.StartLine == hits[j].Record.StartLine {
					return hits[i].Record.ID < hits[j].Record.ID
				}
				return hits[i].Record.StartLine < hits[j].Record.StartLine
			}
			return hits[i].Record.Path < hits[j].Record.Path
		}
		return hits[i].Score > hits[j].Score
	})
	return hits, nil
}

func scoreCandidateIndices(
	records []Record,
	terms []string,
	explicit map[string]struct{},
	currentTurn map[string]struct{},
	useCurrentPathWeight bool,
) []int {
	if !useCurrentPathWeight {
		return allRecordIndices(len(records))
	}
	if len(records) == 0 {
		return nil
	}

	forced := make([]int, 0, len(explicit)+len(currentTurn))
	related := make([]int, 0, minInt(len(records), 256))
	lexical := make([]int, 0, minInt(len(records), 256))
	relatedDirs := relatedDirSet(explicit, currentTurn)
	needLexical := len(terms) > 0

	for i := range records {
		recPath := normalizeRelPath(records[i].Path)
		if _, ok := explicit[recPath]; ok {
			forced = append(forced, i)
			continue
		}
		if _, ok := currentTurn[recPath]; ok {
			forced = append(forced, i)
			continue
		}
		if inRelatedDir(recPath, relatedDirs) {
			related = append(related, i)
			continue
		}
		if needLexical && lexicalScore(records[i], terms) > 0 {
			lexical = append(lexical, i)
		}
	}

	out := make([]int, 0, minInt(len(records), lightMaxCandidates))
	appendBounded := func(src []int) {
		for _, idx := range src {
			if len(out) >= lightMaxCandidates {
				return
			}
			out = append(out, idx)
		}
	}
	appendBounded(forced)
	appendBounded(related)
	appendBounded(lexical)

	if len(out) == 0 {
		// Keep historical behavior: if narrowing does not find candidates, score the full set.
		return allRecordIndices(len(records))
	}
	return out
}

func allRecordIndices(n int) []int {
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		out[i] = i
	}
	return out
}

func relatedDirSet(explicit map[string]struct{}, currentTurn map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(explicit)+len(currentTurn))
	for p := range explicit {
		if dir := normalizeRelPath(filepath.Dir(p)); dir != "." && dir != "" {
			out[dir] = struct{}{}
		}
	}
	for p := range currentTurn {
		if dir := normalizeRelPath(filepath.Dir(p)); dir != "." && dir != "" {
			out[dir] = struct{}{}
		}
	}
	return out
}

func inRelatedDir(path string, dirs map[string]struct{}) bool {
	if len(dirs) == 0 {
		return false
	}
	dir := normalizeRelPath(filepath.Dir(path))
	if dir == "." || dir == "" {
		return false
	}
	_, ok := dirs[dir]
	return ok
}

func lexicalScore(rec Record, terms []string) float64 {
	if len(terms) == 0 {
		return 0
	}
	path := strings.ToLower(rec.Path)
	base := strings.ToLower(filepath.Base(rec.Path))
	name := strings.ToLower(rec.Name)
	preview := strings.ToLower(rec.TextPreview)
	score := 0.0
	for _, t := range terms {
		if strings.Contains(name, t) {
			score += 20
		}
		if strings.Contains(base, t) {
			score += 16
		}
		if strings.Contains(path, t) {
			score += 8
		}
		if strings.Contains(preview, t) {
			score += 4
		}
	}
	return score / 100.0
}

func tokenizeQuery(q string) []string {
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

func diversifyHits(hits []SearchHit, maxFiles, maxPerFile, maxRecords int) []SearchHit {
	if maxFiles <= 0 {
		maxFiles = 12
	}
	if maxPerFile <= 0 {
		maxPerFile = 4
	}
	if maxRecords <= 0 {
		maxRecords = 40
	}
	filesUsed := map[string]int{}
	fileOrder := map[string]int{}
	fileCount := 0
	out := make([]SearchHit, 0, minInt(len(hits), maxRecords))
	for _, hit := range hits {
		path := hit.Record.Path
		if _, ok := fileOrder[path]; !ok {
			if fileCount >= maxFiles {
				continue
			}
			fileOrder[path] = fileCount
			fileCount++
		}
		if filesUsed[path] >= maxPerFile {
			continue
		}
		filesUsed[path]++
		out = append(out, hit)
		if len(out) >= maxRecords {
			break
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
