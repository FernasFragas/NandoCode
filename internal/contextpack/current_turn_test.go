package contextpack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

func TestPackCurrentTurnPromptSmallFilePassThrough(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	packed, err := PackCurrentTurnPrompt("summarize @note.txt", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 50_000})
	if err != nil {
		t.Fatal(err)
	}
	if packed.PackReport.Packed {
		t.Fatalf("did not expect packed=true for small file: %+v", packed.PackReport)
	}
	if !strings.Contains(packed.Prompt, "<file path=\"note.txt\">") {
		t.Fatalf("expected raw file block, got:\n%s", packed.Prompt)
	}
}

func TestPackCurrentTurnPromptLargeFileAddsEnvelopeAndAnchor(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte(strings.Repeat("x", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	packed, err := PackCurrentTurnPrompt("report status for @note.txt", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !packed.PackReport.Packed {
		t.Fatalf("expected packed=true: %+v", packed.PackReport)
	}
	if !packed.PackReport.AnchorAdded {
		t.Fatalf("expected anchor=true: %+v", packed.PackReport)
	}
	if !strings.Contains(packed.Prompt, "Original user request:") || !strings.Contains(packed.Prompt, "Reminder: answer this original request exactly:") {
		t.Fatalf("expected envelope+anchor, got:\n%s", packed.Prompt)
	}
	if !strings.Contains(packed.Prompt, "Referenced path manifest:") {
		t.Fatalf("expected manifest in packed prompt, got:\n%s", packed.Prompt)
	}
}

func TestPackCurrentTurnPromptTooLargeWhenAllFileBodiesOmitted(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("very-large-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	ctx.MaxPromptBytes = 1
	_, err := PackCurrentTurnPrompt("report status for @note.txt", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 50_000})
	if err == nil {
		t.Fatal("expected too-large error")
	}
	if _, ok := err.(ErrEvidenceTooLarge); !ok {
		t.Fatalf("expected ErrEvidenceTooLarge, got %T: %v", err, err)
	}
}

func TestPackCurrentTurnPromptDirectoryLowConfidenceNotice(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "a.md"), []byte("markdown\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	packed, err := PackCurrentTurnPrompt("review @sub", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 40})
	if err != nil {
		t.Fatal(err)
	}
	if !packed.PackReport.Packed {
		t.Fatalf("expected packed for directory budgeted request")
	}
	if !strings.Contains(packed.Prompt, "<referenced_directory_tree path=\"sub\">") {
		t.Fatalf("expected directory tree in packed prompt:\n%s", packed.Prompt)
	}
	if !strings.Contains(packed.Prompt, "reason=\"low_confidence\"") {
		t.Fatalf("expected low-confidence omission notice:\n%s", packed.Prompt)
	}
	if strings.Contains(packed.Prompt, "<referenced_file_raw path=\"sub/a.md\">") {
		t.Fatalf("did not expect arbitrary file body on low-confidence directory selection:\n%s", packed.Prompt)
	}
}

func TestPackCurrentTurnPromptDirectoryExplicitModeAndLexicalSelection(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "parser.md"), []byte("parser details\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "other.md"), []byte("other details\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	packed, err := PackCurrentTurnPrompt("review parser in @docs?content", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 500})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packed.Prompt, "- docs (directory)") {
		t.Fatalf("expected normalized directory mention in manifest:\n%s", packed.Prompt)
	}
	if !strings.Contains(packed.Prompt, "<referenced_directory_tree path=\"docs\">") {
		t.Fatalf("expected tree-first directory evidence:\n%s", packed.Prompt)
	}
	if !strings.Contains(packed.Prompt, "<referenced_file_raw path=\"docs/parser.md\">") {
		t.Fatalf("expected lexical match file body:\n%s", packed.Prompt)
	}
}

func TestPackCurrentTurnPromptRawBytesOmittedNotDoubleCounted(t *testing.T) {
	root := t.TempDir()
	body := strings.Repeat("x", 1000)
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	packed, err := PackCurrentTurnPrompt("report status for @note.txt", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 50})
	if err != nil {
		t.Fatal(err)
	}
	if packed.PackReport.RawBytesOmitted != 900 {
		t.Fatalf("raw bytes omitted=%d want 900; report=%+v", packed.PackReport.RawBytesOmitted, packed.PackReport)
	}
}

func TestPackCurrentTurnPromptLargeFileIncludesTailAndMatchRanges(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 4200; i++ {
		line := "noise line"
		if i == 2100 {
			line = "context packing blocked pending validation marker"
		}
		if i == 4199 {
			line = "LATEST_STATUS_MARKER implemented context packing"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(root, "phase-log.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	packed, err := PackCurrentTurnPrompt("review what is implemented in @phase-log.md", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 16_000})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packed.Prompt, "<file_range path=\"phase-log.md\"") {
		t.Fatalf("expected range evidence blocks:\n%s", packed.Prompt)
	}
	if !strings.Contains(packed.Prompt, "LATEST_STATUS_MARKER implemented context packing") {
		t.Fatalf("expected tail marker in packed prompt:\n%s", packed.Prompt)
	}
	if !strings.Contains(packed.Prompt, "matched:") {
		t.Fatalf("expected lexical match evidence:\n%s", packed.Prompt)
	}
	if packed.PackReport.RawBytesOmitted > len(b.String()) {
		t.Fatalf("raw bytes omitted=%d exceeds file bytes=%d", packed.PackReport.RawBytesOmitted, len(b.String()))
	}
	if len(packed.PackReport.IncludedRanges) == 0 {
		t.Fatalf("expected range report entries: %+v", packed.PackReport)
	}
}

func TestPackCurrentTurnPromptTinyBudgetPrioritizesLatestTailLines(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 4200; i++ {
		line := "noise"
		if i == 4199 {
			line = "LATEST_STATUS_MARKER implemented context packing"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(root, "phase-log.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	packed, err := PackCurrentTurnPrompt("review what is implemented in @phase-log.md", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 120})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packed.Prompt, "LATEST_STATUS_MARKER implemented context packing") {
		t.Fatalf("expected tiny packet to include newest tail marker:\n%s", packed.Prompt)
	}
}

func TestPackCurrentTurnPromptExplicitLineRangeOverridesAutoSelection(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 120; i++ {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", 2))
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(root, "phase-log.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	packed, err := PackCurrentTurnPrompt("review @phase-log.md#L10-L20", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packed.Prompt, "kind=\"explicit\"") {
		t.Fatalf("expected explicit range kind:\n%s", packed.Prompt)
	}
	if strings.Contains(packed.Prompt, "kind=\"tail\"") || strings.Contains(packed.Prompt, "kind=\"head\"") {
		t.Fatalf("expected explicit range to override automatic selection:\n%s", packed.Prompt)
	}
}

func TestPackCurrentTurnPromptRejectsInvalidMentionRange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	_, err := PackCurrentTurnPrompt("review @a.txt#L20-L10", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 2000})
	if err == nil {
		t.Fatal("expected invalid mention range error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid mention line range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPackCurrentTurnPromptRejectsMentionModeLineRangeCombination(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	_, err := PackCurrentTurnPrompt("review @a.txt?content#L10-L20", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 2000})
	if err == nil {
		t.Fatal("expected unsupported syntax error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "line ranges cannot be combined with mention modes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPackCurrentTurnPromptRenderedEvidenceStaysWithinBudgetAllowance(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	b.WriteString("# Phase Log\n")
	for i := 1; i <= 4500; i++ {
		line := "noise line"
		if i == 4488 {
			line = "LATEST_STATUS_MARKER implemented context packing"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(root, "phase-log.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	budget := agent.AssemblyBudget{AvailableEvidenceTokens: 300}
	packed, err := PackCurrentTurnPrompt("review @phase-log.md", ctx, budget)
	if err != nil {
		t.Fatal(err)
	}
	got := estimateRenderedEvidenceTokens(packed.Prompt)
	maxAllowed := budget.AvailableEvidenceTokens + renderedEvidenceOverheadTokenAllowance
	if got > maxAllowed {
		t.Fatalf("rendered evidence tokens=%d exceeds budget+allowance=%d", got, maxAllowed)
	}
}

func TestPackCurrentTurnPromptExplicitOversizedRangeTruncatesToUsefulPartialPacket(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 600; i++ {
		if i == 580 {
			b.WriteString("LATEST_STATUS_MARKER implemented context packing\n")
			continue
		}
		b.WriteString("filler filler filler filler filler filler filler filler filler filler\n")
	}
	if err := os.WriteFile(filepath.Join(root, "phase-log.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	packed, err := PackCurrentTurnPrompt("review @phase-log.md#L1-L600", ctx, agent.AssemblyBudget{AvailableEvidenceTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packed.Prompt, "<file_range path=\"phase-log.md\" kind=\"explicit\"") {
		t.Fatalf("expected explicit file_range block:\n%s", packed.Prompt)
	}
	if !strings.Contains(packed.Prompt, "<partial_file_notice path=\"phase-log.md\">") {
		t.Fatalf("expected partial file notice for oversized explicit range:\n%s", packed.Prompt)
	}
	if !strings.Contains(packed.Prompt, "597  filler") {
		t.Fatalf("expected partial range content to be included:\n%s", packed.Prompt)
	}
}

func TestBuildEvidencePartsSimpleFileReadsPreserveRefOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "c.txt"), []byte("C"), 0o644); err != nil {
		t.Fatal(err)
	}
	refs := []mentionRef{
		{Path: "b.txt", Abs: filepath.Join(root, "b.txt")},
		{Path: "a.txt", Abs: filepath.Join(root, "a.txt")},
		{Path: "c.txt", Abs: filepath.Join(root, "c.txt")},
	}
	parts, omitted := buildEvidenceParts(
		"review",
		refs,
		nil,
		agent.AssemblyBudget{AvailableEvidenceTokens: 20_000},
		tools.DefaultContext(context.Background(), root),
	)
	if len(omitted) != 0 {
		t.Fatalf("unexpected omitted entries: %+v", omitted)
	}
	if len(parts) != len(refs) {
		t.Fatalf("parts len=%d want %d", len(parts), len(refs))
	}
	for i, want := range []string{"b.txt", "a.txt", "c.txt"} {
		if parts[i].Path != want {
			t.Fatalf("part[%d] path=%q want %q", i, parts[i].Path, want)
		}
	}
}

func TestSelectDirectoryEvidenceParallelReadsDeterministicOrder(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{"zeta.md", "alpha.md", "beta.md"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(docs, name), []byte("content "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ref := mentionRef{Path: "docs", Abs: docs, IsDir: true}
	const input = "review alpha beta zeta in @docs?content"
	const wantOrder = "docs/alpha.md,docs/beta.md,docs/zeta.md"
	for i := 0; i < 20; i++ {
		parts, omitted := selectDirectoryEvidence(context.Background(), ref, input)
		if len(omitted) != 0 {
			t.Fatalf("unexpected omitted entries: %+v", omitted)
		}
		if len(parts) != 3 {
			t.Fatalf("parts len=%d want 3", len(parts))
		}
		gotOrder := strings.Join([]string{parts[0].Path, parts[1].Path, parts[2].Path}, ",")
		if gotOrder != wantOrder {
			t.Fatalf("run %d order mismatch: got %q want %q", i, gotOrder, wantOrder)
		}
	}
}

func TestBuildEvidencePartsRespectsCanceledContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	refs := []mentionRef{
		{Path: "a.txt", Abs: filepath.Join(root, "a.txt")},
		{Path: "b.txt", Abs: filepath.Join(root, "b.txt")},
	}
	parts, omitted := buildEvidenceParts(
		"review",
		refs,
		nil,
		agent.AssemblyBudget{AvailableEvidenceTokens: 20_000},
		tools.DefaultContext(ctx, root),
	)
	if len(parts) != 0 {
		t.Fatalf("expected no file parts after cancellation, got %+v", parts)
	}
	if len(omitted) != 2 {
		t.Fatalf("omitted len=%d want 2 (%+v)", len(omitted), omitted)
	}
	for i, want := range []string{"a.txt", "b.txt"} {
		if omitted[i].Path != want || omitted[i].Reason != "read_error" {
			t.Fatalf("omitted[%d]=%+v want path=%q reason=read_error", i, omitted[i], want)
		}
	}
}
