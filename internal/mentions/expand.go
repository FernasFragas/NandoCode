package mentions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/tools/dirwalk"
)

// ResolvedFile describes one @file mention resolved into prompt context.
type ResolvedFile struct {
	Path      string
	AbsPath   string
	Truncated bool
	SizeBytes int
}

// ResolvedDirectory describes one @directory mention resolved into prompt context.
type ResolvedDirectory struct {
	Path            string
	AbsPath         string
	Mode            string
	Tree            string
	FileCount       int
	DirCount        int
	SkippedCount    int
	TotalBytes      int
	Truncated       bool
	Reason          string
	DiscoveredFiles int
	DiscoveredDirs  int
	IncludedFiles   int
	IncludedDirs    int
	IgnoredByGit    int
	ExpansionSource string
	OmittedReasons  map[string]int
}

// Token represents an @mention token in a line of text.
// Start and End are rune offsets with End exclusive.
type Token struct {
	Active bool
	Start  int
	End    int
	Raw    string
}

type resolvedMention struct {
	Path  string
	Abs   string
	IsDir bool
	Mode  MentionMode
}

type MentionMode string

const (
	MentionModeAuto    MentionMode = "auto"
	MentionModeTree    MentionMode = "tree"
	MentionModeContent MentionMode = "content"
	MentionModeAll     MentionMode = "all"
)

type parsedMention struct {
	RawPath      string
	Path         string
	Mode         MentionMode
	HasLineRange bool
	StartLine    int
	EndLine      int
	ParseError   string
}

// ExpansionReport summarizes mention expansion decisions for callers that
// need to reason about prompt-shape correctness.
type ExpansionReport struct {
	Intent            IntentReport
	ListingIntent      bool
	ModeByMention      map[string]MentionMode
	IncludedFileBodies int
	DiscoveredFiles    int
	Warnings           []string
}

type promptBudget struct {
	MaxFiles int
	MaxBytes int64
	Files    int
	Bytes    int64
}

func (b *promptBudget) exhaustedReason() string {
	if b.MaxBytes > 0 && b.Bytes >= b.MaxBytes {
		return "prompt-byte-cap"
	}
	if b.MaxFiles > 0 && b.Files >= b.MaxFiles {
		return "prompt-file-cap"
	}
	return ""
}

// ExpandPrompt appends the contents of any @path mentions (files or directories) to the prompt.
func ExpandPrompt(input string, ctx tools.Context) (string, []ResolvedFile, []ResolvedDirectory, error) {
	expanded, files, dirs, _, err := ExpandPromptDetailed(input, ctx)
	return expanded, files, dirs, err
}

// ExpandPromptDetailed returns expanded prompt content plus a report describing
// how mention intent mapped to final context shape.
func ExpandPromptDetailed(input string, ctx tools.Context) (string, []ResolvedFile, []ResolvedDirectory, ExpansionReport, error) {
	paths := extractMentionPaths(input)
	if len(paths) == 0 {
		return input, nil, nil, ExpansionReport{}, nil
	}
	report := ExpansionReport{
		ModeByMention: map[string]MentionMode{},
	}

	mentions, err := resolveMentions(paths, ctx)
	if err != nil {
		return "", nil, nil, ExpansionReport{}, err
	}
	report.Intent = ClassifyPromptIntent(input, paths, mentions)
	report.ListingIntent = report.Intent.ListingLike()
	if report.Intent.AttachmentPolicy == AttachListingTreeOnly {
		for i := range mentions {
			if mentions[i].IsDir && mentions[i].Mode == MentionModeAuto {
				mentions[i].Mode = MentionModeTree
			}
		}
	}
	for _, m := range mentions {
		if !m.IsDir {
			continue
		}
		report.ModeByMention[m.Path] = effectiveMentionMode(m.Mode)
	}
	mentions, notes := pruneOverlaps(mentions)
	if len(mentions) == 0 {
		return input, nil, nil, report, nil
	}

	budget := &promptBudget{
		MaxFiles: ctx.EffectiveMaxPromptFiles(),
		MaxBytes: ctx.EffectiveMaxPromptBytes(),
	}
	maxRead := ctx.EffectiveMaxReadChars()
	resolvedFiles := make([]ResolvedFile, 0, len(mentions))
	resolvedDirs := make([]ResolvedDirectory, 0, len(mentions))
	blocks := make([]string, 0, len(mentions))
	hasDirs := false

	for _, m := range mentions {
		if m.IsDir {
			hasDirs = true
			if reason := budget.exhaustedReason(); reason != "" {
				resolvedDirs = append(resolvedDirs, ResolvedDirectory{
					Path:      m.Path,
					AbsPath:   m.Abs,
					Mode:      string(effectiveMentionMode(m.Mode)),
					Tree:      renderTree(displayPath(m.Path, m.Abs, ctx), nil, nil),
					Truncated: true,
					Reason:    reason,
				})
				blocks = append(blocks, renderDirectoryBlock(renderDirectoryArgs{
					Path:            m.Path,
					AbsPath:         m.Abs,
					Tree:            renderTree(displayPath(m.Path, m.Abs, ctx), nil, nil),
					Files:           nil,
					FileCount:       0,
					Bytes:           0,
					Truncated:       true,
					Reason:          reason,
					NoteCount:       notes[m.Abs],
					Mode:            string(effectiveMentionMode(m.Mode)),
					Source:          "budget",
					FilesDiscovered: 0,
					DirsDiscovered:  0,
				}, ctx))
				continue
			}

			block, files, dir, dirErr := expandDirectory(ctx, m, budget, notes[m.Abs])
			if dirErr != nil {
				return "", nil, nil, ExpansionReport{}, dirErr
			}
			blocks = append(blocks, block)
			resolvedFiles = append(resolvedFiles, files...)
			resolvedDirs = append(resolvedDirs, dir)
			continue
		}

		fileBlock, rf, fileErr := expandSingleFile(ctx, m, budget, maxRead)
		if fileErr != nil {
			return "", nil, nil, ExpansionReport{}, fileErr
		}
		blocks = append(blocks, fileBlock)
		resolvedFiles = append(resolvedFiles, rf)
	}

	var appendix strings.Builder
	if hasDirs {
		appendix.WriteString("\n\nReferenced files and directories:\n")
	} else {
		appendix.WriteString("\n\nReferenced files:\n")
	}
	for _, block := range blocks {
		appendix.WriteByte('\n')
		appendix.WriteString(block)
		if !strings.HasSuffix(block, "\n") {
			appendix.WriteByte('\n')
		}
	}

	for _, d := range resolvedDirs {
		report.DiscoveredFiles += d.DiscoveredFiles
	}
	report.IncludedFileBodies = len(resolvedFiles)
	if report.ListingIntent && report.IncludedFileBodies > 0 {
		report.Warnings = append(report.Warnings, "listing-intent-with-file-bodies")
	}
	if report.Intent.AttachmentPolicy == AttachListingTreeOnly && len(resolvedDirs) > 0 {
		return buildListingScopedPrompt(input, resolvedDirs), resolvedFiles, resolvedDirs, report, nil
	}
	return input + appendix.String(), resolvedFiles, resolvedDirs, report, nil
}

func resolveMentions(paths []parsedMention, ctx tools.Context) ([]resolvedMention, error) {
	out := make([]resolvedMention, 0, len(paths))
	for _, p := range paths {
		abs, err := tools.ResolvePath(ctx, p.Path, tools.PathRead)
		if err != nil {
			return nil, fmt.Errorf("resolve @%s: %w", p.RawPath, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("stat @%s: %w", p.RawPath, err)
		}
		out = append(out, resolvedMention{
			Path:  p.Path,
			Abs:   abs,
			IsDir: info.IsDir(),
			Mode:  p.Mode,
		})
	}
	return out, nil
}

func pruneOverlaps(in []resolvedMention) ([]resolvedMention, map[string]int) {
	keepDir := make(map[string]bool, len(in))
	dirs := make([]resolvedMention, 0, len(in))
	for _, m := range in {
		if !m.IsDir {
			continue
		}
		keepDir[m.Abs] = true
		dirs = append(dirs, m)
	}

	for _, d := range dirs {
		for _, other := range dirs {
			if d.Abs == other.Abs {
				continue
			}
			if isWithinPath(other.Abs, d.Abs) {
				keepDir[d.Abs] = false
				break
			}
		}
	}

	notes := make(map[string]int, len(dirs))
	out := make([]resolvedMention, 0, len(in))
	for _, m := range in {
		if m.IsDir {
			if keepDir[m.Abs] {
				out = append(out, m)
				continue
			}
			if parent, ok := nearestCoveringDir(m.Abs, dirs, keepDir); ok {
				notes[parent.Abs]++
			}
			continue
		}
		if parent, ok := nearestCoveringDir(m.Abs, dirs, keepDir); ok {
			notes[parent.Abs]++
			continue
		}
		out = append(out, m)
	}
	return out, notes
}

func nearestCoveringDir(path string, dirs []resolvedMention, keepDir map[string]bool) (resolvedMention, bool) {
	var best resolvedMention
	bestDepth := -1
	for _, d := range dirs {
		if !keepDir[d.Abs] {
			continue
		}
		if !isWithinPath(d.Abs, path) {
			continue
		}
		depth := strings.Count(filepath.Clean(d.Abs), string(filepath.Separator))
		if depth > bestDepth {
			best = d
			bestDepth = depth
		}
	}
	return best, bestDepth >= 0
}

func expandSingleFile(ctx tools.Context, m resolvedMention, budget *promptBudget, maxRead int) (string, ResolvedFile, error) {
	content, err := os.ReadFile(m.Abs)
	if err != nil {
		return "", ResolvedFile{}, fmt.Errorf("read @%s: %w", m.Path, err)
	}
	if !utf8.Valid(content) {
		return "", ResolvedFile{}, fmt.Errorf("@%s is not valid UTF-8 text", m.Path)
	}

	truncated := false
	if len(content) > maxRead {
		content = content[:maxRead]
		truncated = true
	}
	if budget.MaxBytes > 0 && budget.Bytes+int64(len(content)) > budget.MaxBytes {
		content = nil
		truncated = true
	}
	if budget.MaxFiles > 0 && budget.Files >= budget.MaxFiles {
		content = nil
		truncated = true
	}

	if len(content) > 0 {
		budget.Files++
		budget.Bytes += int64(len(content))
		if ctx.RecordFileSnapshot != nil {
			ctx.RecordFileSnapshot(m.Abs, content)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<file path=\"%s\"", displayPath(m.Path, m.Abs, ctx))
	if truncated {
		b.WriteString(" truncated=\"true\"")
	}
	b.WriteString(">\n")
	b.Write(content)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString("</file>")

	return b.String(), ResolvedFile{
		Path:      m.Path,
		AbsPath:   m.Abs,
		Truncated: truncated,
		SizeBytes: len(content),
	}, nil
}

type inlineFile struct {
	Path      string
	Content   []byte
	Truncated bool
}

type renderDirectoryArgs struct {
	Path            string
	AbsPath         string
	Tree            string
	Files           []inlineFile
	FileCount       int
	Bytes           int
	Truncated       bool
	Reason          string
	NoteCount       int
	Mode            string
	Source          string
	FilesDiscovered int
	DirsDiscovered  int
}

func expandDirectory(ctx tools.Context, m resolvedMention, budget *promptBudget, noteCount int) (string, []ResolvedFile, ResolvedDirectory, error) {
	sourceMode := strings.ToLower(strings.TrimSpace(ctx.MentionDirectorySource))
	if sourceMode == "" {
		sourceMode = string(dirwalk.SourceAuto)
	}
	walkSource := dirwalk.SourceMode(sourceMode)
	if m.Mode == MentionModeAll {
		walkSource = dirwalk.SourceFilesystem
	}
	if ctx.MentionIncludeGitignoredOnExplicit && walkSource == dirwalk.SourceAuto {
		walkSource = dirwalk.SourceFilesystem
	}

	walked, walkStats, err := dirwalk.Walk(ctx.EffectiveContext(), m.Abs, dirwalk.Options{
		Excludes:      dirwalk.DefaultExcludes(),
		MaxDepth:      ctx.EffectiveMaxDirDepth(),
		Source:        walkSource,
		DetectIgnored: true,
	})
	if err != nil {
		return "", nil, ResolvedDirectory{}, fmt.Errorf("walk @%s: %w", m.Path, err)
	}

	maxRead := ctx.EffectiveMaxReadChars()
	maxDirFiles := ctx.EffectiveMaxDirFiles()
	maxDirBytes := ctx.EffectiveMaxDirBytes()
	dirFiles := make([]inlineFile, 0, 32)
	resolvedFiles := make([]ResolvedFile, 0, 32)
	skipped := make(map[string]string, 32)
	skippedCount := 0
	totalBytes := 0
	fileCount := 0
	truncated := walkStats.Truncated
	reason := walkStats.Reason

	for _, e := range walked {
		if e.Skipped {
			skippedCount++
			skipped[e.RelPath] = e.SkipReason
			continue
		}
		if e.IsDir {
			continue
		}
		if m.Mode == MentionModeTree {
			continue
		}

		if maxDirFiles > 0 && fileCount >= maxDirFiles {
			truncated = true
			reason = dirwalk.ReasonFileCap
			break
		}
		if budget.MaxFiles > 0 && budget.Files >= budget.MaxFiles {
			truncated = true
			reason = "prompt-file-cap"
			break
		}

		resolved, resolveErr := tools.ResolvePath(ctx, e.AbsPath, tools.PathRead)
		if resolveErr != nil {
			skippedCount++
			skipped[e.RelPath] = "policy"
			continue
		}
		info, statErr := os.Stat(resolved)
		if statErr != nil {
			if os.IsPermission(statErr) {
				skippedCount++
				skipped[e.RelPath] = "permission"
				continue
			}
			return "", nil, ResolvedDirectory{}, fmt.Errorf("stat @%s/%s: %w", m.Path, e.RelPath, statErr)
		}
		if info.Size() > int64(maxRead*4) {
			skippedCount++
			skipped[e.RelPath] = "too-large"
			continue
		}

		content, readErr := os.ReadFile(resolved)
		if readErr != nil {
			if os.IsPermission(readErr) {
				skippedCount++
				skipped[e.RelPath] = "permission"
				continue
			}
			return "", nil, ResolvedDirectory{}, fmt.Errorf("read @%s/%s: %w", m.Path, e.RelPath, readErr)
		}
		if !utf8.Valid(content) {
			skippedCount++
			skipped[e.RelPath] = "binary"
			continue
		}

		fileTruncated := false
		if len(content) > maxRead {
			content = content[:maxRead]
			fileTruncated = true
			truncated = true
			if reason == "" {
				reason = "byte-cap"
			}
		}

		nextBytes := totalBytes + len(content)
		if maxDirBytes > 0 && int64(nextBytes) > maxDirBytes {
			truncated = true
			reason = dirwalk.ReasonByteCap
			break
		}
		if budget.MaxBytes > 0 && budget.Bytes+int64(len(content)) > budget.MaxBytes {
			truncated = true
			reason = "prompt-byte-cap"
			break
		}

		fileCount++
		totalBytes = nextBytes
		budget.Files++
		budget.Bytes += int64(len(content))
		if ctx.RecordFileSnapshot != nil {
			ctx.RecordFileSnapshot(resolved, content)
		}

		dirFiles = append(dirFiles, inlineFile{
			Path:      e.RelPath,
			Content:   content,
			Truncated: fileTruncated,
		})
		resolvedFiles = append(resolvedFiles, ResolvedFile{
			Path:      filepath.ToSlash(filepath.Join(m.Path, e.RelPath)),
			AbsPath:   resolved,
			Truncated: fileTruncated,
			SizeBytes: len(content),
		})
	}

	discoveredFiles := chooseDiscoveredFiles(walkStats)
	discoveredDirs := chooseDiscoveredDirs(walkStats)
	tree := renderTree(displayPath(m.Path, m.Abs, ctx), walked, skipped)
	block := renderDirectoryBlock(renderDirectoryArgs{
		Path:            m.Path,
		AbsPath:         m.Abs,
		Tree:            tree,
		Files:           dirFiles,
		FileCount:       fileCount,
		Bytes:           totalBytes,
		Truncated:       truncated,
		Reason:          reason,
		NoteCount:       noteCount,
		Mode:            string(effectiveMentionMode(m.Mode)),
		Source:          walkStats.Source,
		FilesDiscovered: discoveredFiles,
		DirsDiscovered:  discoveredDirs,
	}, ctx)

	omitted := map[string]int{}
	for _, sreason := range skipped {
		omitted[sreason]++
	}
	if walkStats.IgnoredByGit > 0 {
		omitted["gitignored"] += walkStats.IgnoredByGit
	}
	if truncated && reason != "" {
		omitted[reason]++
	}

	return block, resolvedFiles, ResolvedDirectory{
		Path:            m.Path,
		AbsPath:         m.Abs,
		Mode:            string(effectiveMentionMode(m.Mode)),
		Tree:            tree,
		FileCount:       fileCount,
		DirCount:        walkStats.DirCount,
		SkippedCount:    skippedCount,
		TotalBytes:      totalBytes,
		Truncated:       truncated,
		Reason:          reason,
		DiscoveredFiles: discoveredFiles,
		DiscoveredDirs:  discoveredDirs,
		IncludedFiles:   fileCount,
		IncludedDirs:    walkStats.DirCount,
		IgnoredByGit:    walkStats.IgnoredByGit,
		ExpansionSource: walkStats.Source,
		OmittedReasons:  omitted,
	}, nil
}

func renderDirectoryBlock(args renderDirectoryArgs, ctx tools.Context) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<directory path=\"%s\" files=\"%d\" bytes=\"%d\" files_discovered=\"%d\" dirs_discovered=\"%d\" files_included=\"%d\" content_bytes=\"%d\" truncated=\"%t\"",
		displayPath(args.Path, args.AbsPath, ctx),
		args.FileCount,
		args.Bytes,
		args.FilesDiscovered,
		args.DirsDiscovered,
		args.FileCount,
		args.Bytes,
		args.Truncated,
	)
	if args.Truncated && args.Reason != "" {
		fmt.Fprintf(&b, " reason=\"%s\"", args.Reason)
	}
	if args.NoteCount > 0 {
		fmt.Fprintf(&b, " note=\"dropped %d redundant file mention", args.NoteCount)
		if args.NoteCount != 1 {
			b.WriteString("s")
		}
		b.WriteString("\"")
	}
	if args.Mode != "" {
		fmt.Fprintf(&b, " mode=\"%s\"", args.Mode)
	}
	if args.Source != "" {
		fmt.Fprintf(&b, " source=\"%s\"", args.Source)
	}
	b.WriteString(">\n")
	b.WriteString("<tree>\n")
	b.WriteString(args.Tree)
	if args.Tree != "" && !strings.HasSuffix(args.Tree, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("</tree>\n")
	for _, f := range args.Files {
		fmt.Fprintf(&b, "<file path=\"%s\"", filepath.ToSlash(filepath.Join(displayPath(args.Path, args.AbsPath, ctx), f.Path)))
		if f.Truncated {
			b.WriteString(" truncated=\"true\"")
		}
		b.WriteString(">\n")
		b.Write(f.Content)
		if len(f.Content) > 0 && f.Content[len(f.Content)-1] != '\n' {
			b.WriteByte('\n')
		}
		b.WriteString("</file>\n")
	}
	b.WriteString("</directory>")
	return b.String()
}

func renderTree(root string, entries []dirwalk.Entry, extraSkipped map[string]string) string {
	var b strings.Builder
	root = filepath.ToSlash(root)
	root = strings.TrimSuffix(root, "/")
	if root == "" {
		root = "."
	}
	b.WriteString(root)
	b.WriteString("/")
	if len(entries) == 0 {
		return b.String()
	}
	b.WriteByte('\n')
	for _, e := range entries {
		if e.RelPath == "" {
			continue
		}
		line := e.RelPath
		if e.IsDir {
			line += "/"
		}
		reason := ""
		if e.Skipped {
			reason = e.SkipReason
		}
		if s, ok := extraSkipped[e.RelPath]; ok {
			reason = s
		}
		if reason != "" {
			line += " [skipped: " + reason + "]"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func displayPath(raw, abs string, ctx tools.Context) string {
	if rel, err := filepath.Rel(ctx.WorkingDir, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return "."
		}
		return rel
	}
	raw = filepath.ToSlash(raw)
	raw = strings.TrimSuffix(raw, "/")
	if raw == "" {
		return "."
	}
	return raw
}

func isWithinPath(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if root == path {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func extractMentionPaths(input string) []parsedMention {
	tokens := mentionTokens(input)
	if len(tokens) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(tokens))
	paths := make([]parsedMention, 0, len(tokens))
	for _, tok := range tokens {
		parsed := parseMention(tok.Raw)
		if strings.TrimSpace(parsed.ParseError) != "" {
			continue
		}
		path := NormalizeMentionPath(parsed.Path)
		if path == "" || path == ".." {
			continue
		}
		key := string(parsed.Mode) + ":" + path
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		paths = append(paths, parsedMention{
			RawPath: tok.Raw,
			Path:    path,
			Mode:    parsed.Mode,
		})
	}
	return paths
}

// TokenAtCursor returns the @-token currently under the cursor, if any.
// pos is a rune offset. Token.Active=false when not inside one.
func TokenAtCursor(line string, pos int) Token {
	runes := []rune(line)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	for _, tok := range mentionTokens(line) {
		if pos >= tok.Start && pos <= tok.End {
			return Token{
				Active: true,
				Start:  tok.Start,
				End:    tok.End,
				Raw:    tok.Raw,
			}
		}
	}
	return Token{}
}

// NormalizeMentionPath cleans a raw query into the form ExpandPrompt expects.
func NormalizeMentionPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	rootRef := trimmed == "." || trimmed == "./" || trimmed == ".\\"

	path := trimmed
	path = strings.TrimRight(path, ".,;:!)]}\"'`")
	path = strings.TrimPrefix(path, "@")
	path = strings.ReplaceAll(path, "\\", "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if !rootRef {
		path = strings.TrimPrefix(path, "./")
	}
	path = strings.TrimPrefix(path, ".\\")
	path = strings.TrimSuffix(path, "/")
	if path == "" && rootRef {
		return "."
	}
	return path
}

func parseMention(raw string) parsedMention {
	p := parsedMention{RawPath: raw, Path: raw, Mode: MentionModeAuto}
	if strings.Contains(raw, "#L") {
		if strings.Contains(raw, "?") {
			p.ParseError = "line ranges cannot be combined with mention mode suffixes; use only @file#L10-L20"
			return p
		}
		path, start, end, ok := ParseLineRangeToken(raw)
		if !ok {
			p.ParseError = "invalid mention line range; expected @file#L10-L20"
			return p
		}
		p.Path = path
		p.HasLineRange = true
		p.StartLine = start
		p.EndLine = end
		return p
	}
	idx := strings.LastIndex(raw, "?")
	if idx < 0 {
		return p
	}
	modeRaw := strings.ToLower(strings.TrimSpace(raw[idx+1:]))
	switch modeRaw {
	case "tree":
		p.Mode = MentionModeTree
	case "content":
		p.Mode = MentionModeContent
	case "all":
		p.Mode = MentionModeAll
	default:
		return p
	}
	p.Path = raw[:idx]
	return p
}

func hasAnyWord(words []string, set map[string]struct{}) bool {
	for _, w := range words {
		if _, ok := set[w]; ok {
			return true
		}
	}
	return false
}

func hasPhrase(words, phrase []string) bool {
	if len(phrase) == 0 || len(words) < len(phrase) {
		return false
	}
	for i := 0; i <= len(words)-len(phrase); i++ {
		match := true
		for j := range phrase {
			if words[i+j] != phrase[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func intentWords(input string) []string {
	lower := strings.ToLower(input)
	var b strings.Builder
	b.Grow(len(lower))
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Fields(b.String())
}

func effectiveMentionMode(mode MentionMode) MentionMode {
	switch mode {
	case MentionModeAuto:
		return MentionModeContent
	case MentionModeAll:
		return MentionModeAll
	default:
		return mode
	}
}

func chooseDiscoveredFiles(stats dirwalk.Stats) int {
	if stats.TotalFilesystemFiles > 0 {
		return stats.TotalFilesystemFiles
	}
	return stats.FileCount
}

func chooseDiscoveredDirs(stats dirwalk.Stats) int {
	if stats.TotalFilesystemDirs > 0 {
		return stats.TotalFilesystemDirs
	}
	return stats.DirCount
}

func mentionTokens(input string) []Token {
	runes := []rune(input)
	out := make([]Token, 0, 8)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '@' {
			continue
		}
		if i > 0 && !unicode.IsSpace(runes[i-1]) {
			continue
		}
		j := i + 1
		for j < len(runes) && !unicode.IsSpace(runes[j]) {
			j++
		}
		if j == i+1 {
			continue
		}
		out = append(out, Token{
			Active: false,
			Start:  i,
			End:    j,
			Raw:    string(runes[i+1 : j]),
		})
		i = j - 1
	}
	return out
}
