package contextpack

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/mentions"
	"github.com/FernasFragas/Nandocode/internal/tools/fileread"
)

const (
	largeFileBytesThreshold     = 16 * 1024
	defaultHeadLines            = 120
	defaultTailLines            = 220
	defaultMatchBefore          = 20
	defaultMatchAfter           = 40
	defaultHeadingManifestLimit = 32
)

type headingEntry struct {
	Line  int
	Level int
	Text  string
}

type fileEvidenceMetadata struct {
	Path         string
	AbsPath      string
	Bytes        int64
	Lines        int
	Extension    string
	Format       string
	UTF8         bool
	HeadingCount int
	FirstHeading string
	LastHeading  string
	LogLike      bool
	StatusLike   bool
	Headings     []headingEntry
}

type selectedRange struct {
	StartLine int
	EndLine   int
	Kind      string
	Reason    string
	Score     int
}

type fileRangeEvidence struct {
	Meta          fileEvidenceMetadata
	Ranges        []selectedRange
	ContentByKey  map[string]fileread.ReadRangeResult
	HeadingDigest string
	PartialNotice string
	OmittedBytes  int
}

func shouldUseRangePipeline(ref mentionRef) bool {
	if ref.IsDir {
		return false
	}
	if ref.HasLineRange {
		return true
	}
	return ref.SizeBytes >= largeFileBytesThreshold
}

func scanFileEvidenceMetadata(ref mentionRef) (fileEvidenceMetadata, error) {
	st, err := os.Stat(ref.Abs)
	if err != nil {
		return fileEvidenceMetadata{}, err
	}
	if st.IsDir() {
		return fileEvidenceMetadata{}, fmt.Errorf("%s is a directory", ref.Path)
	}
	meta := fileEvidenceMetadata{
		Path:       ref.Path,
		AbsPath:    ref.Abs,
		Bytes:      st.Size(),
		Extension:  strings.ToLower(filepath.Ext(ref.Path)),
		Format:     "text",
		UTF8:       true,
		LogLike:    looksLikeLogOrStatusPath(ref.Path),
		StatusLike: looksLikeLogOrStatusPath(ref.Path),
	}
	if meta.Extension == ".md" || meta.Extension == ".markdown" {
		meta.Format = "markdown"
	}
	f, err := os.Open(ref.Abs)
	if err != nil {
		return fileEvidenceMetadata{}, err
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, 512*1024)
	lineNo := 0
	for {
		chunk, readErr := br.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return fileEvidenceMetadata{}, readErr
		}
		if readErr == io.EOF && chunk == "" {
			break
		}
		lineNo++
		if lineNo == 1 {
			chunk = strings.TrimPrefix(chunk, "\uFEFF")
		}
		if !utf8.ValidString(chunk) {
			return fileEvidenceMetadata{}, fmt.Errorf("%s is not valid UTF-8 text", ref.Path)
		}
		line := strings.TrimSuffix(chunk, "\n")
		line = strings.TrimSuffix(line, "\r")
		if h, ok := parseMarkdownHeading(lineNo, line); ok {
			meta.Headings = append(meta.Headings, h)
		}
		if readErr == io.EOF {
			break
		}
	}
	meta.Lines = lineNo
	meta.HeadingCount = len(meta.Headings)
	if len(meta.Headings) > 0 {
		meta.FirstHeading = meta.Headings[0].Text
		meta.LastHeading = meta.Headings[len(meta.Headings)-1].Text
	}
	return meta, nil
}

func parseMarkdownHeading(lineNo int, line string) (headingEntry, bool) {
	trim := strings.TrimSpace(line)
	if trim == "" || !strings.HasPrefix(trim, "#") {
		return headingEntry{}, false
	}
	level := 0
	for level < len(trim) && trim[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return headingEntry{}, false
	}
	if level >= len(trim) || trim[level] != ' ' {
		return headingEntry{}, false
	}
	text := strings.TrimSpace(trim[level:])
	if text == "" {
		return headingEntry{}, false
	}
	return headingEntry{Line: lineNo, Level: level, Text: text}, true
}

func selectRangesForFile(input string, ref mentionRef, meta fileEvidenceMetadata) ([]selectedRange, string) {
	if ref.HasLineRange {
		return []selectedRange{{
			StartLine: ref.LineRangeStart,
			EndLine:   ref.LineRangeEnd,
			Kind:      "explicit",
			Reason:    "explicit mention line range",
			Score:     100,
		}}, ""
	}
	ranges := make([]selectedRange, 0, 8)
	headEnd := min(meta.Lines, defaultHeadLines)
	if headEnd > 0 {
		ranges = append(ranges, selectedRange{StartLine: 1, EndLine: headEnd, Kind: "head", Reason: "document context", Score: 10})
	}
	tailStart := max(1, meta.Lines-defaultTailLines+1)
	if meta.Lines > 0 {
		ranges = append(ranges, selectedRange{StartLine: tailStart, EndLine: meta.Lines, Kind: "tail", Reason: "recent status", Score: 14})
	}
	terms := lexicalSearchTerms(input)
	ranges = append(ranges, scanMatchRanges(ref.Abs, terms, meta.Lines)...)
	merged := mergeSelectedRanges(ranges)
	manifest := buildHeadingManifest(meta, terms)
	return merged, manifest
}

func lexicalSearchTerms(input string) []string {
	defaults := []string{
		"complete", "completed", "implemented", "implementation", "pending", "blocked", "blocker",
		"todo", "remaining", "next", "status", "issue", "risk", "regression", "validation",
		"context", "packing",
	}
	set := map[string]struct{}{}
	for _, d := range defaults {
		set[d] = struct{}{}
	}
	for _, t := range lexicalTerms(input) {
		if len(t) >= 3 {
			set[t] = struct{}{}
		}
	}
	terms := make([]string, 0, len(set))
	for term := range set {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	return terms
}

func scanMatchRanges(absPath string, terms []string, totalLines int) []selectedRange {
	if len(terms) == 0 || totalLines <= 0 {
		return nil
	}
	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 512*1024)
	lineNo := 0
	ranges := make([]selectedRange, 0, 16)
	for {
		chunk, readErr := br.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return ranges
		}
		if readErr == io.EOF && chunk == "" {
			break
		}
		lineNo++
		line := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(chunk, "\n"), "\r"))
		match := ""
		for _, term := range terms {
			if strings.Contains(line, term) {
				match = term
				break
			}
		}
		if match != "" {
			start := max(1, lineNo-defaultMatchBefore)
			end := min(totalLines, lineNo+defaultMatchAfter)
			ranges = append(ranges, selectedRange{StartLine: start, EndLine: end, Kind: "match", Reason: "matched: " + match, Score: 9})
		}
		if readErr == io.EOF {
			break
		}
	}
	if len(ranges) > 12 {
		ranges = ranges[:12]
	}
	return ranges
}

func mergeSelectedRanges(ranges []selectedRange) []selectedRange {
	if len(ranges) == 0 {
		return nil
	}
	sort.SliceStable(ranges, func(i, j int) bool {
		if ranges[i].StartLine == ranges[j].StartLine {
			return ranges[i].EndLine < ranges[j].EndLine
		}
		return ranges[i].StartLine < ranges[j].StartLine
	})
	merged := make([]selectedRange, 0, len(ranges))
	for _, r := range ranges {
		if r.StartLine <= 0 || r.EndLine < r.StartLine {
			continue
		}
		if len(merged) == 0 {
			merged = append(merged, r)
			continue
		}
		prev := &merged[len(merged)-1]
		if r.StartLine <= prev.EndLine+3 {
			if r.EndLine > prev.EndLine {
				prev.EndLine = r.EndLine
			}
			if prev.Kind != r.Kind {
				prev.Kind = "merged"
			}
			if prev.Reason == "" {
				prev.Reason = r.Reason
			}
			continue
		}
		merged = append(merged, r)
	}
	return merged
}

func buildHeadingManifest(meta fileEvidenceMetadata, terms []string) string {
	if meta.Format != "markdown" || len(meta.Headings) == 0 {
		return ""
	}
	containsTerm := func(s string) bool {
		low := strings.ToLower(s)
		for _, t := range terms {
			if strings.Contains(low, t) {
				return true
			}
		}
		return false
	}
	matches := make([]headingEntry, 0, defaultHeadingManifestLimit)
	for i, h := range meta.Headings {
		if i < defaultHeadingManifestLimit/2 || i >= len(meta.Headings)-defaultHeadingManifestLimit/2 || containsTerm(h.Text) {
			matches = append(matches, h)
		}
		if len(matches) >= defaultHeadingManifestLimit {
			break
		}
	}
	if len(matches) == 0 {
		return ""
	}
	var b strings.Builder
	for _, h := range matches {
		fmt.Fprintf(&b, "L%d %s %s\n", h.Line, strings.Repeat("#", h.Level), h.Text)
	}
	if len(meta.Headings) > len(matches) {
		fmt.Fprintf(&b, "... omitted %d headings ...\n", len(meta.Headings)-len(matches))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func renderNumberedContent(content string, startLine int) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	var b strings.Builder
	for i, line := range lines {
		if line == "" && i == len(lines)-1 {
			continue
		}
		fmt.Fprintf(&b, "%5d  %s\n", startLine+i, line)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func buildRangeEvidence(ref mentionRef, input string, maxChars int) (fileRangeEvidence, int, []agent.OmittedEvidence, error) {
	meta, err := scanFileEvidenceMetadata(ref)
	if err != nil {
		return fileRangeEvidence{}, 0, nil, err
	}
	selected, headingManifest := selectRangesForFile(input, ref, meta)
	contentByKey := make(map[string]fileread.ReadRangeResult, len(selected))
	omitted := make([]agent.OmittedEvidence, 0)
	consumed := 0
	contentBytesRead := 0
	if maxChars < 1 {
		maxChars = 1
	}
	remaining := maxChars
	for _, r := range prioritizeRangesForBudget(selected, meta) {
		if remaining <= 0 {
			omitted = append(omitted, agent.OmittedEvidence{Path: ref.Path, Kind: "file", Reason: "budget", BytesOmitted: int(meta.Bytes)})
			break
		}
		startLine, lineLimit := rangeReadWindow(r, meta, remaining)
		res, readErr := fileread.ReadRange(fileread.ReadRangeRequest{Path: ref.Abs, StartLine: startLine, LineLimit: lineLimit, MaxBytes: remaining})
		if readErr != nil {
			omitted = append(omitted, agent.OmittedEvidence{Path: ref.Path, Kind: "file", Reason: "read_error", BytesOmitted: int(meta.Bytes)})
			continue
		}
		if strings.TrimSpace(res.Content) == "" {
			continue
		}
		key := fmt.Sprintf("%s:%d-%d", r.Kind, r.StartLine, r.EndLine)
		contentByKey[key] = res
		consumed += len(res.Content)
		contentBytesRead += len(res.Content)
		remaining -= len(res.Content)
	}
	partialNotice := ""
	if ref.HasLineRange && len(contentByKey) > 0 {
		for _, v := range contentByKey {
			if v.Truncated {
				partialNotice = "The requested file range was too large for the current context budget and has been truncated. Use a smaller line range or search for specific content if more precision is needed."
				break
			}
		}
	}
	if partialNotice == "" && meta.Bytes >= largeFileBytesThreshold {
		partialNotice = "This file was too large to include fully. The included ranges are partial evidence. Use FileRead/Grep-style tools to inspect omitted ranges before making claims that depend on them."
	}
	if headingManifest != "" {
		manifestCost := len(headingManifest)
		if manifestCost <= remaining {
			consumed += manifestCost
			remaining -= manifestCost
		} else {
			headingManifest = ""
		}
	}
	omittedBytes := max(0, int(meta.Bytes)-contentBytesRead)
	return fileRangeEvidence{
		Meta:          meta,
		Ranges:        selected,
		ContentByKey:  contentByKey,
		HeadingDigest: headingManifest,
		PartialNotice: partialNotice,
		OmittedBytes:  max(0, omittedBytes),
	}, consumed, omitted, nil
}

func prioritizeRangesForBudget(ranges []selectedRange, meta fileEvidenceMetadata) []selectedRange {
	out := append([]selectedRange(nil), ranges...)
	sort.SliceStable(out, func(i, j int) bool {
		pi := rangePriority(out[i], meta)
		pj := rangePriority(out[j], meta)
		if pi == pj {
			return out[i].StartLine < out[j].StartLine
		}
		return pi < pj
	})
	return out
}

func rangePriority(r selectedRange, meta fileEvidenceMetadata) int {
	if meta.LogLike || meta.StatusLike {
		if isTailLikeRange(r, meta) {
			return 0
		}
		if r.Kind == "match" || strings.Contains(r.Kind, "match") {
			return 1
		}
		if r.Kind == "head" {
			return 2
		}
		return 3
	}
	if r.Kind == "head" {
		return 0
	}
	if isTailLikeRange(r, meta) {
		return 1
	}
	if r.Kind == "match" || strings.Contains(r.Kind, "match") {
		return 2
	}
	return 3
}

func rangeReadWindow(r selectedRange, meta fileEvidenceMetadata, remainingBytes int) (int, int) {
	lineLimit := r.EndLine - r.StartLine + 1
	startLine := r.StartLine
	if lineLimit < 1 {
		return startLine, 1
	}
	if isTailLikeRange(r, meta) && remainingBytes > 0 {
		approxLines := max(1, remainingBytes/80)
		if approxLines < lineLimit {
			lineLimit = approxLines
			startLine = max(r.StartLine, r.EndLine-lineLimit+1)
		}
	}
	return startLine, lineLimit
}

func isTailLikeRange(r selectedRange, meta fileEvidenceMetadata) bool {
	if meta.Lines <= 0 {
		return false
	}
	if r.Kind == "tail" || strings.Contains(strings.ToLower(r.Reason), "recent status") {
		return true
	}
	return r.EndLine >= meta.Lines
}

func renderRangeEvidenceBlock(ev fileRangeEvidence) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n<file_metadata path=\"%s\">\n", ev.Meta.Path)
	fmt.Fprintf(&b, "bytes: %d\n", ev.Meta.Bytes)
	fmt.Fprintf(&b, "lines: %d\n", ev.Meta.Lines)
	fmt.Fprintf(&b, "format: %s\n", ev.Meta.Format)
	fmt.Fprintf(&b, "strategy: range_pipeline\n")
	if ev.HeadingDigest != "" {
		b.WriteString("heading_manifest:\n")
		b.WriteString(ev.HeadingDigest)
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "omitted_bytes: %d\n", max(0, ev.OmittedBytes))
	b.WriteString("</file_metadata>\n")
	for _, r := range ev.Ranges {
		key := fmt.Sprintf("%s:%d-%d", r.Kind, r.StartLine, r.EndLine)
		res, ok := ev.ContentByKey[key]
		if !ok || strings.TrimSpace(res.Content) == "" {
			continue
		}
		endLine := res.StartLine + res.LineCount - 1
		if endLine < res.StartLine {
			endLine = res.StartLine
		}
		fmt.Fprintf(&b, "\n<file_range path=\"%s\" kind=\"%s\" lines=\"%d-%d\" reason=\"%s\">\n", ev.Meta.Path, r.Kind, res.StartLine, endLine, r.Reason)
		b.WriteString(renderNumberedContent(res.Content, res.StartLine))
		b.WriteString("\n</file_range>\n")
	}
	if strings.TrimSpace(ev.PartialNotice) != "" {
		fmt.Fprintf(&b, "\n<partial_file_notice path=\"%s\">\n%s\n</partial_file_notice>\n", ev.Meta.Path, ev.PartialNotice)
	}
	return b.String()
}

func looksLikeLogOrStatusPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	hints := []string{"log", "changelog", "status", "phase", "todo", "task", "progress", "summary"}
	for _, h := range hints {
		if strings.Contains(base, h) {
			return true
		}
	}
	return false
}

func sanitizeInputForExpansion(input string) string {
	runes := []rune(input)
	var b strings.Builder
	for i := 0; i < len(runes); i++ {
		if runes[i] != '@' {
			b.WriteRune(runes[i])
			continue
		}
		start := i + 1
		j := start
		for j < len(runes) && !unicode.IsSpace(runes[j]) {
			j++
		}
		tok := string(runes[start:j])
		if strings.Contains(tok, "#L") {
			path, _, _, ok := mentions.ParseLineRangeToken(tok)
			if ok {
				tok = path
			}
		}
		b.WriteRune('@')
		b.WriteString(tok)
		i = j - 1
	}
	return b.String()
}
