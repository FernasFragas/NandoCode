package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ProjectWorkflowOptions struct {
	RootDir   string
	ScopePath string
	Question  string
	Retrieved []string
	MaxFiles  int
	MaxBytes  int
}

type ProjectWorkflowReport struct {
	ScopePath     string
	SelectedFiles int
	CacheHits     int
	CacheMisses   int
	ChunkCount    int
	LedgerRunID   string
	LedgerPath    string
	StageNotes    []string
}

func BuildProjectAnalysisPrompt(opts ProjectWorkflowOptions) (string, ProjectWorkflowReport, error) {
	root := strings.TrimSpace(opts.RootDir)
	if root == "" {
		root = "."
	}
	scope := normalizeRootHint(opts.ScopePath)
	if scope == "" {
		scope = "."
	}
	question := strings.TrimSpace(opts.Question)
	if question == "" {
		question = "Analyze this project and provide a detailed implementation and risk summary."
	}
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 12
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}

	explicit := ExtractMentionedPaths(question)
	selected := mergePaths(explicit, opts.Retrieved)
	if len(selected) > maxFiles {
		selected = selected[:maxFiles]
	}
	// Keep deterministic order while preserving explicit-first behavior.
	explicitSet := make(map[string]struct{}, len(explicit))
	for _, p := range explicit {
		explicitSet[p] = struct{}{}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		_, iExplicit := explicitSet[selected[i]]
		_, jExplicit := explicitSet[selected[j]]
		if iExplicit != jExplicit {
			return iExplicit
		}
		return selected[i] < selected[j]
	})

	runID := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	ledger := NewEvidenceLedger(runID)
	report := ProjectWorkflowReport{
		ScopePath:   scope,
		LedgerRunID: runID,
		LedgerPath:  filepath.Join("state", "analysis", "ledger-"+runID+".json"),
		StageNotes:  []string{},
	}
	stageStart := time.Now()
	report.StageNotes = append(report.StageNotes, "select: start")

	fileSummaries := make([]SummaryRecord, 0, len(selected))
	for _, rel := range selected {
		abs, ok := safeJoinRoot(root, rel)
		if !ok {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			continue
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		if len(b) > maxBytes {
			b = b[:maxBytes]
		}
		content := string(b)
		contentID := SummaryContentID(content)
		ledger.AddFile(rel)
		if cached, hit, err := LoadSummaryFromCache(rel, contentID); err == nil && hit {
			report.CacheHits++
			ledger.AddSummary(rel, "cache", cached.Summary)
			fileSummaries = append(fileSummaries, SummaryRecord{Path: rel, Source: "cache", Summary: cached.Summary})
			continue
		}
		report.CacheMisses++
		mapStart := time.Now()
		chunks := ChunkText(rel, content, 900)
		report.ChunkCount += len(chunks)
		for _, ch := range chunks {
			ledger.AddChunk(ch)
		}
		chunkSummaries := mapChunkSummaries(chunks)
		summary := reduceFileSummary(rel, chunkSummaries)
		if summary == "" {
			summary = "No high-signal symbols extracted; inspect file directly."
		}
		_ = SaveSummaryToCache(rel, contentID, summary)
		ledger.AddSummary(rel, "fresh", summary)
		fileSummaries = append(fileSummaries, SummaryRecord{Path: rel, Source: "fresh", Summary: summary})
		report.StageNotes = append(report.StageNotes, fmt.Sprintf("map_reduce: %s chunks=%d in %s", rel, len(chunks), time.Since(mapStart).Round(time.Millisecond)))
	}
	report.SelectedFiles = len(fileSummaries)
	projectSummary := reduceProjectSummary(question, fileSummaries)
	ledger.AddConclusion(projectSummary)
	_ = SaveEvidenceLedger(ledger)
	report.StageNotes = append(report.StageNotes, fmt.Sprintf("synthesize: files=%d in %s", report.SelectedFiles, time.Since(stageStart).Round(time.Millisecond)))

	prompt := renderProjectPrompt(scope, question, fileSummaries, projectSummary)
	return prompt, report, nil
}

func mergePaths(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(a, b...) {
		s = filepath.ToSlash(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func safeJoinRoot(root, rel string) (string, bool) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || strings.HasPrefix(rel, "..") {
		return "", false
	}
	cleanRoot := filepath.Clean(root)
	absRoot, err := filepath.Abs(cleanRoot)
	if err != nil {
		return "", false
	}
	absPath := filepath.Join(absRoot, filepath.FromSlash(rel))
	absPath = filepath.Clean(absPath)
	if absPath != absRoot && !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) {
		return "", false
	}
	return absPath, true
}

func mapChunkSummaries(chunks []FileChunk) []string {
	if len(chunks) == 0 {
		return nil
	}
	out := make([]string, 0, len(chunks))
	for _, ch := range chunks {
		out = append(out, summarizeSingleChunk(ch.Text))
	}
	return out
}

func reduceFileSummary(path string, chunkSummaries []string) string {
	if len(chunkSummaries) == 0 {
		return ""
	}
	seen := map[string]struct{}{}
	lines := make([]string, 0, 6)
	for _, s := range chunkSummaries {
		parts := strings.Split(s, " | ")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			lines = append(lines, p)
			if len(lines) >= 6 {
				return strings.Join(lines, " | ")
			}
		}
	}
	if len(lines) == 0 {
		return "File inspected but no high-signal lines extracted: " + path
	}
	return strings.Join(lines, " | ")
}

func reduceProjectSummary(question string, fileSummaries []SummaryRecord) string {
	if len(fileSummaries) == 0 {
		return "No files were summarized. Additional scope or file selection is required."
	}
	lines := []string{
		"Question focus: " + strings.TrimSpace(question),
		fmt.Sprintf("Evidence coverage: %d files", len(fileSummaries)),
	}
	if len(fileSummaries) > 0 {
		lines = append(lines, "Primary files: "+fileSummaries[0].Path)
	}
	return strings.Join(lines, " | ")
}

func summarizeSingleChunk(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	const maxLines = 4
	lines := make([]string, 0, maxLines)
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !isSignalLine(line) {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
		if len(lines) >= maxLines {
			return strings.Join(lines, " | ")
		}
	}
	if len(lines) == 0 {
		for _, raw := range strings.Split(text, "\n") {
			line := strings.TrimSpace(raw)
			if line != "" {
				return line
			}
		}
	}
	return strings.Join(lines, " | ")
}

func isSignalLine(line string) bool {
	if strings.HasPrefix(line, "func ") || strings.Contains(line, " func(") {
		return true
	}
	if strings.HasPrefix(line, "type ") || strings.HasPrefix(line, "interface ") {
		return true
	}
	if strings.HasPrefix(line, "const ") || strings.HasPrefix(line, "var ") {
		return true
	}
	if strings.HasPrefix(line, "#") {
		return true
	}
	return false
}

func renderProjectPrompt(scope, question string, summaries []SummaryRecord, projectSummary string) string {
	var b strings.Builder
	b.WriteString("You are performing bounded project analysis using precomputed evidence.\n")
	b.WriteString("Scope: ")
	b.WriteString(scope)
	b.WriteString("\nQuestion: ")
	b.WriteString(question)
	b.WriteString("\n\nEvidence summaries (path => summary):\n")
	if len(summaries) == 0 {
		b.WriteString("- none (no files selected; explain what extra input is needed)\n")
	} else {
		for _, s := range summaries {
			b.WriteString("- ")
			b.WriteString(s.Path)
			b.WriteString(" => ")
			b.WriteString(strings.TrimSpace(s.Summary))
			if s.Source == "cache" {
				b.WriteString(" [cache]")
			}
			b.WriteString("\n")
		}
	}
	if strings.TrimSpace(projectSummary) != "" {
		b.WriteString("\nReduced project synthesis:\n- ")
		b.WriteString(strings.TrimSpace(projectSummary))
		b.WriteString("\n")
	}
	b.WriteString("\nRequirements:\n")
	b.WriteString("1) Produce a direct final answer.\n")
	b.WriteString("2) Cite relevant evidence paths from the list above.\n")
	b.WriteString("3) If evidence is insufficient, list concrete missing files/areas.\n")
	return b.String()
}
