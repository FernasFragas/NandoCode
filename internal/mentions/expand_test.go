package mentions

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

func TestExpandPromptNoMentions(t *testing.T) {
	t.Parallel()
	ctx := tools.DefaultContext(context.Background(), t.TempDir())
	got, files, dirs, err := ExpandPrompt("plain prompt", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "plain prompt" {
		t.Fatalf("got %q", got)
	}
	if len(files) != 0 || len(dirs) != 0 {
		t.Fatalf("files=%d dirs=%d", len(files), len(dirs))
	}
}

func TestExpandPromptAppendsMentionedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "alpha\n")
	mustWriteFile(t, filepath.Join(dir, "b.txt"), "beta\n")
	ctx := tools.DefaultContext(context.Background(), dir)

	got, files, dirs, err := ExpandPrompt("compare @a.txt with @b.txt", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || len(dirs) != 0 {
		t.Fatalf("files=%d dirs=%d", len(files), len(dirs))
	}
	if !strings.Contains(got, "<file path=\"a.txt\">") || !strings.Contains(got, "alpha\n") {
		t.Fatalf("expanded prompt missing a.txt contents:\n%s", got)
	}
	if !strings.Contains(got, "<file path=\"b.txt\">") || !strings.Contains(got, "beta\n") {
		t.Fatalf("expanded prompt missing b.txt contents:\n%s", got)
	}
}

func TestExpandPromptDeduplicatesMentions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "alpha\n")
	ctx := tools.DefaultContext(context.Background(), dir)

	got, files, dirs, err := ExpandPrompt("check @a.txt and again @a.txt", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || len(dirs) != 0 {
		t.Fatalf("files=%d dirs=%d", len(files), len(dirs))
	}
	if strings.Count(got, "<file path=\"a.txt\">") != 1 {
		t.Fatalf("expected one appended file block:\n%s", got)
	}
}

func TestExpandPromptRespectsMaxReadChars(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), "abcdef")
	ctx := tools.DefaultContext(context.Background(), dir)
	ctx.MaxReadChars = 3

	got, files, _, err := ExpandPrompt("read @a.txt", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !files[0].Truncated {
		t.Fatalf("files=%#v", files)
	}
	if !strings.Contains(got, "truncated=\"true\"") || !strings.Contains(got, "\nabc\n") {
		t.Fatalf("expanded prompt missing truncation marker/content:\n%s", got)
	}
}

func TestExpandPromptRejectsMissingFiles(t *testing.T) {
	t.Parallel()
	ctx := tools.DefaultContext(context.Background(), t.TempDir())
	_, _, _, err := ExpandPrompt("read @missing.txt", ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExpandPromptDirectoryInlinesFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	mustWriteFile(t, filepath.Join(root, "docs", "b.txt"), "b\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, err := ExpandPrompt("summarize @docs/", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) != 2 {
		t.Fatalf("files=%d", len(files))
	}
	if !strings.Contains(got, "<directory path=\"docs\"") {
		t.Fatalf("missing directory block:\n%s", got)
	}
	if !strings.Contains(got, "files_discovered=\"2\"") || !strings.Contains(got, "files_included=\"2\"") {
		t.Fatalf("missing discovered/included metadata:\n%s", got)
	}
	if !strings.Contains(got, "content_bytes=\"4\"") {
		t.Fatalf("missing content byte metadata:\n%s", got)
	}
	if !strings.Contains(got, "<file path=\"docs/a.txt\">") || !strings.Contains(got, "<file path=\"docs/b.txt\">") {
		t.Fatalf("missing inlined file blocks:\n%s", got)
	}
}

func TestExpandPromptDirectoryTreeMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, err := ExpandPrompt("list files in @docs?tree", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) != 0 {
		t.Fatalf("expected tree mode to inline no files, got %d", len(files))
	}
	if dirs[0].Mode != "tree" {
		t.Fatalf("expected tree mode metadata, got %q", dirs[0].Mode)
	}
	if !strings.Contains(got, "User request:") || !strings.Contains(got, "Directory tree data:") {
		t.Fatalf("expected listing envelope in prompt:\n%s", got)
	}
	if strings.Contains(got, "<file path=") || strings.Contains(got, "<directory path=") {
		t.Fatalf("listing envelope should not include xml attachment blocks:\n%s", got)
	}
}

func TestExpandPromptListingIntentUsesTreeMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, err := ExpandPrompt("name the all the files and folders in @docs", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) != 0 {
		t.Fatalf("expected tree mode to inline no files, got %d", len(files))
	}
	if dirs[0].Mode != "tree" {
		t.Fatalf("expected tree mode metadata, got %q", dirs[0].Mode)
	}
	if !strings.Contains(got, "User request:") || !strings.Contains(got, "Directory tree data:") {
		t.Fatalf("expected listing envelope in prompt:\n%s", got)
	}
}

func TestExpandPromptListingIntentListAllFilesUsesTreeMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, err := ExpandPrompt("list all the files in @docs/", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) != 0 {
		t.Fatalf("expected tree mode to inline no files, got %d", len(files))
	}
	if dirs[0].Mode != "tree" {
		t.Fatalf("expected tree mode metadata, got %q", dirs[0].Mode)
	}
	if !strings.Contains(got, "Directory tree data:") {
		t.Fatalf("expected listing envelope in prompt:\n%s", got)
	}
}

func TestExpandPromptListingIntentListEveryFileUsesTreeMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, err := ExpandPrompt("list every file in @docs", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) != 0 {
		t.Fatalf("expected tree mode to inline no files, got %d", len(files))
	}
	if dirs[0].Mode != "tree" {
		t.Fatalf("expected tree mode metadata, got %q", dirs[0].Mode)
	}
	if !strings.Contains(got, "Directory tree data:") {
		t.Fatalf("expected listing envelope in prompt:\n%s", got)
	}
}

func TestExpandPromptListingIntentShowFoldersUsesTreeMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, err := ExpandPrompt("show folders in @docs", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) != 0 {
		t.Fatalf("expected tree mode to inline no files, got %d", len(files))
	}
	if dirs[0].Mode != "tree" {
		t.Fatalf("expected tree mode metadata, got %q", dirs[0].Mode)
	}
	if !strings.Contains(got, "Directory tree data:") {
		t.Fatalf("expected listing envelope in prompt:\n%s", got)
	}
}

func TestExpandPromptListingIntentBlockedForReview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, err := ExpandPrompt("review and list all files in @docs", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) == 0 {
		t.Fatal("expected review intent to keep content mode")
	}
	if strings.Contains(got, "mode=\"tree\"") {
		t.Fatalf("expected content mode, got tree:\n%s", got)
	}
}

func TestExpandPromptReviewDocsStaysContentModeAndNoListingConstraint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, report, err := ExpandPromptDetailed("review @docs/", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) == 0 {
		t.Fatal("expected content mode for review prompt")
	}
	if report.ListingIntent {
		t.Fatal("did not expect listing intent for review prompt")
	}
	if strings.Contains(got, "Listing response constraint:") {
		t.Fatalf("did not expect listing constraint for review prompt:\n%s", got)
	}
}

func TestExpandPromptSummarizeDocsStaysContentModeAndNoListingConstraint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, report, err := ExpandPromptDetailed("summarize @docs/", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) == 0 {
		t.Fatal("expected content mode for summarize prompt")
	}
	if report.ListingIntent {
		t.Fatal("did not expect listing intent for summarize prompt")
	}
	if report.Intent.Kind != IntentReview && report.Intent.Kind != IntentAnalysis {
		t.Fatalf("expected non-listing intent class, got %q", report.Intent.Kind)
	}
	if strings.Contains(got, "Listing response constraint:") {
		t.Fatalf("did not expect listing constraint for summarize prompt:\n%s", got)
	}
}

func TestExpandPromptAllModeRendersModeAndSource(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, err := ExpandPrompt("inspect @docs?all", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) == 0 {
		t.Fatal("expected content inclusion in all mode")
	}
	if !strings.Contains(got, "mode=\"all\"") {
		t.Fatalf("expected mode=all:\n%s", got)
	}
	if !strings.Contains(got, "source=\"filesystem\"") {
		t.Fatalf("expected source=filesystem:\n%s", got)
	}
}

func TestExpandPromptContentModeOverridesListingIntent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, _, err := ExpandPrompt("name all files in @docs?content", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected content mode to inline files")
	}
	if !strings.Contains(got, "<file path=\"docs/a.txt\">") {
		t.Fatalf("expected inlined file body:\n%s", got)
	}
}

func TestExpandPromptDetailedListingIntentReport(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	_, files, dirs, report, err := ExpandPromptDetailed("list all the files in @docs/", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ListingIntent {
		t.Fatal("expected listing intent to be detected")
	}
	if report.Intent.Kind != IntentDirectoryListing {
		t.Fatalf("intent=%q", report.Intent.Kind)
	}
	if report.Intent.AttachmentPolicy != AttachListingTreeOnly {
		t.Fatalf("policy=%q", report.Intent.AttachmentPolicy)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) != 0 {
		t.Fatalf("expected no file bodies in tree mode, got %d", len(files))
	}
	if report.IncludedFileBodies != 0 {
		t.Fatalf("included_file_bodies=%d", report.IncludedFileBodies)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", report.Warnings)
	}
}

func TestExpandPromptDetailedListingDoesNotAddAnswerConstraint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, report, err := ExpandPromptDetailed("list all the files in @docs/", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || len(files) != 0 {
		t.Fatalf("dirs=%d files=%d", len(dirs), len(files))
	}
	if !report.ListingIntent {
		t.Fatal("expected listing intent")
	}
	if strings.Contains(got, "Listing response constraint:") {
		t.Fatalf("did not expect listing answer constraint in expanded prompt:\n%s", got)
	}
	if !strings.Contains(got, "User request:") || !strings.Contains(got, "Directory tree data:") {
		t.Fatalf("expected listing envelope:\n%s", got)
	}
}

func TestExpandPromptDetailedListingIntentWarningOnExplicitContent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	_, files, _, report, err := ExpandPromptDetailed("list all the files in @docs?content", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ListingIntent {
		t.Fatal("expected listing intent to be detected")
	}
	if report.Intent.Kind != IntentDirectoryListingWithContent {
		t.Fatalf("intent=%q", report.Intent.Kind)
	}
	if report.Intent.AttachmentPolicy != AttachContent {
		t.Fatalf("policy=%q", report.Intent.AttachmentPolicy)
	}
	if len(files) == 0 {
		t.Fatal("expected explicit content mode to include file bodies")
	}
	found := false
	for _, warn := range report.Warnings {
		if warn == "listing-intent-with-file-bodies" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected listing-intent-with-file-bodies warning, got %#v", report.Warnings)
	}
}

func TestExpandPromptDetailedListingConstraintNotAddedForContentMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, _, report, err := ExpandPromptDetailed("list all the files in @docs?content", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ListingIntent {
		t.Fatal("expected listing intent")
	}
	if len(files) == 0 {
		t.Fatal("expected content mode to include file bodies")
	}
	if strings.Contains(got, "Listing response constraint:") {
		t.Fatalf("did not expect listing constraint with content mode:\n%s", got)
	}
	if strings.Contains(got, "Directory tree data:") {
		t.Fatalf("did not expect listing envelope when explicit content mode used:\n%s", got)
	}
}

func TestExpandPromptDirectorySkipsBinary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "bin.dat"), []byte{0xff, 0x00, 0x10}, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)

	got, _, dirs, err := ExpandPrompt("summarize @docs", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if dirs[0].SkippedCount == 0 {
		t.Fatalf("expected skipped files, got %+v", dirs[0])
	}
	if !strings.Contains(got, "[skipped: binary]") {
		t.Fatalf("missing binary skip marker:\n%s", got)
	}
}

func TestExpandPromptDirectoryByteCap(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), strings.Repeat("a", 20))
	mustWriteFile(t, filepath.Join(root, "docs", "b.txt"), strings.Repeat("b", 20))
	ctx := tools.DefaultContext(context.Background(), root)
	ctx.MaxDirBytes = 25

	got, _, dirs, err := ExpandPrompt("summarize @docs", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if !dirs[0].Truncated {
		t.Fatalf("expected truncated dir: %+v", dirs[0])
	}
	if dirs[0].OmittedReasons["byte-cap"] == 0 {
		t.Fatalf("expected byte-cap omitted reason: %+v", dirs[0].OmittedReasons)
	}
	if !strings.Contains(got, "truncated=\"true\"") || !strings.Contains(got, "reason=\"byte-cap\"") {
		t.Fatalf("missing byte-cap marker:\n%s", got)
	}
}

func TestExpandPromptMultiDirectorySharesPromptBudget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a", "x.txt"), strings.Repeat("x", 20))
	mustWriteFile(t, filepath.Join(root, "b", "y.txt"), strings.Repeat("y", 20))
	ctx := tools.DefaultContext(context.Background(), root)
	ctx.MaxPromptBytes = 25

	got, _, dirs, err := ExpandPrompt("check @a @b", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if !dirs[1].Truncated {
		t.Fatalf("expected second directory truncated due prompt budget: %+v", dirs[1])
	}
	if !strings.Contains(got, "reason=\"prompt-byte-cap\"") {
		t.Fatalf("missing prompt-byte-cap marker:\n%s", got)
	}
}

func TestExpandPromptDropsRedundantFileMentionInsideDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "hello\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, files, dirs, err := ExpandPrompt("check @docs @docs/a.txt", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) != 1 {
		t.Fatalf("expected single inlined file via directory expansion, got %d", len(files))
	}
	if strings.Count(got, "<file path=\"docs/a.txt\">") != 1 {
		t.Fatalf("expected one file block:\n%s", got)
	}
	if !strings.Contains(got, "note=\"dropped 1 redundant file mention\"") {
		t.Fatalf("expected overlap note:\n%s", got)
	}
}

func TestExpandPromptDropsRedundantChildDirectoryMention(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a", "b", "x.txt"), "x\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, _, dirs, err := ExpandPrompt("check @a/b @a", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("expected only parent directory, got %d", len(dirs))
	}
	if strings.Count(got, "<directory path=\"a\"") != 1 {
		t.Fatalf("expected only parent directory block:\n%s", got)
	}
}

func TestExpandPromptDirectoryOutsideAllowedRootsErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := tools.DefaultContext(context.Background(), root)
	_, _, _, err := ExpandPrompt("read @/tmp", ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExpandPromptHiddenDirectoryExplicitMention(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".codex", "config.txt"), "ok\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, _, dirs, err := ExpandPrompt("inspect @.codex", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if !strings.Contains(got, "<directory path=\".codex\"") {
		t.Fatalf("missing hidden dir block:\n%s", got)
	}
}

func TestExpandPromptRecordsSnapshotsForDirectoryFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "a.txt"), "a\n")
	mustWriteFile(t, filepath.Join(root, "docs", "b.txt"), "b\n")
	ctx := tools.DefaultContext(context.Background(), root)
	calls := make([]string, 0, 2)
	ctx.RecordFileSnapshot = func(path string, _ []byte) {
		calls = append(calls, filepath.Base(path))
	}

	_, _, dirs, err := ExpandPrompt("check @docs", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(calls))
	}
}

func TestExpandPromptOneDirectoryBlockPerMention(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a", "x.txt"), "x\n")
	mustWriteFile(t, filepath.Join(root, "b", "y.txt"), "y\n")
	ctx := tools.DefaultContext(context.Background(), root)

	got, _, dirs, err := ExpandPrompt("check @a @b", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if strings.Count(got, "<directory path=") != 2 {
		t.Fatalf("expected 2 directory blocks:\n%s", got)
	}
}

func TestExpandPromptLargeDirectoryBudget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	largeDir := filepath.Join(root, "big")
	if err := os.MkdirAll(largeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		name := filepath.Join(largeDir, "f"+strings.Repeat("0", 4-len(strconv.Itoa(i)))+strconv.Itoa(i)+".txt")
		mustWriteFile(t, name, strings.Repeat("x", 200))
	}
	if err := os.WriteFile(filepath.Join(largeDir, "bin.dat"), []byte{0xff, 0x00, 0x10}, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), root)
	ctx.MaxDirFiles = 200
	ctx.MaxDirBytes = 20_000
	ctx.MaxPromptBytes = 25_000

	got, files, dirs, err := ExpandPrompt("inspect @big", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d", len(dirs))
	}
	if len(files) == 0 {
		t.Fatal("expected some expanded files")
	}
	if len(files) > ctx.MaxDirFiles {
		t.Fatalf("expanded %d files, max=%d", len(files), ctx.MaxDirFiles)
	}
	if !dirs[0].Truncated {
		t.Fatalf("expected truncation for large directory: %+v", dirs[0])
	}
	if dirs[0].TotalBytes > int(ctx.MaxPromptBytes) {
		t.Fatalf("total bytes %d exceed max prompt bytes %d", dirs[0].TotalBytes, ctx.MaxPromptBytes)
	}
	if !strings.Contains(got, "<directory path=\"big\"") {
		t.Fatalf("missing directory block:\n%s", got)
	}
}

func TestTokenAtCursorMultipleMentions(t *testing.T) {
	t.Parallel()
	line := "inspect @a.txt and @b.txt now"
	tok := TokenAtCursor(line, strings.Index(line, "@b.txt")+2)
	if !tok.Active {
		t.Fatal("expected active token")
	}
	if tok.Raw != "b.txt" {
		t.Fatalf("raw=%q", tok.Raw)
	}
	if tok.Start >= tok.End {
		t.Fatalf("bad bounds: %d..%d", tok.Start, tok.End)
	}
}

func TestTokenAtCursorInactiveOutsideMention(t *testing.T) {
	t.Parallel()
	line := "inspect @a.txt now"
	tok := TokenAtCursor(line, 1)
	if tok.Active {
		t.Fatalf("expected inactive token, got %#v", tok)
	}
}

func TestNormalizeMentionPath(t *testing.T) {
	t.Parallel()
	if got := NormalizeMentionPath("./internal//tui/app.go."); got != "internal/tui/app.go" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeMentionPath("./"); got != "." {
		t.Fatalf("expected root path, got %q", got)
	}
	if got := NormalizeMentionPath("docs/"); got != "docs" {
		t.Fatalf("expected docs, got %q", got)
	}
	if got := NormalizeMentionPath("docs`"); got != "docs" {
		t.Fatalf("expected docs without trailing markdown backtick, got %q", got)
	}
}

func TestExtractMentionPathsParsesExplicitLineRangePath(t *testing.T) {
	t.Parallel()
	paths := extractMentionPaths("review @docs/phase-log.md#L10-L20")
	if len(paths) != 1 {
		t.Fatalf("paths=%d", len(paths))
	}
	if paths[0].Path != "docs/phase-log.md" {
		t.Fatalf("path=%q", paths[0].Path)
	}
}

func TestParseMentionRejectsModeRangeCombination(t *testing.T) {
	t.Parallel()
	p := parseMention("docs/phase-log.md?content#L10-L20")
	if p.ParseError == "" {
		t.Fatal("expected parse error")
	}
}

func TestParseMentionRejectsInvalidLineRange(t *testing.T) {
	t.Parallel()
	p := parseMention("docs/phase-log.md#L20-L10")
	if p.ParseError == "" {
		t.Fatal("expected parse error")
	}
}

func TestParseLineRangeToken(t *testing.T) {
	t.Parallel()
	path, start, end, ok := ParseLineRangeToken("docs/phase-log.md#L10-L20")
	if !ok {
		t.Fatal("expected valid range parse")
	}
	if path != "docs/phase-log.md" || start != 10 || end != 20 {
		t.Fatalf("got path=%q start=%d end=%d", path, start, end)
	}
}

func mustWriteFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
