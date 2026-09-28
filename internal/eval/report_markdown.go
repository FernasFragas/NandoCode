package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/tools/filewrite"
)

func WriteMarkdownReport(path string, report RunReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# NandoCode Evaluation Report\n\n")
	b.WriteString("| Metric | Value |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| Fixtures | %d |\n", report.Counts.Fixtures)
	fmt.Fprintf(&b, "| Passed | %d |\n", report.Counts.Passed)
	fmt.Fprintf(&b, "| Failed | %d |\n", report.Counts.Failed)
	fmt.Fprintf(&b, "| Errors | %d |\n", report.Counts.Errors)
	fmt.Fprintf(&b, "| Aggregate score | %.2f |\n\n", report.AggregateScore)

	b.WriteString("| Fixture | Status | Score | Tests | Files | Tools | Approvals | Runtime |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, result := range report.Results {
		fmt.Fprintf(&b, "| %s | %s | %.2f | %d/%d | %d | %d | %d | %dms |\n",
			displayFixtureName(result), result.Status, result.Score, result.TestsPassed, result.TestsPassed+result.TestsFailed,
			len(result.FilesChanged), result.ToolCalls, result.ApprovalsRequired, result.RuntimeMS)
	}
	b.WriteString("\n")
	for _, result := range report.Results {
		fmt.Fprintf(&b, "## %s\n\n", displayFixtureName(result))
		fmt.Fprintf(&b, "- Status: `%s`\n", result.Status)
		fmt.Fprintf(&b, "- Score: `%.2f`\n", result.Score)
		fmt.Fprintf(&b, "- Task completion: `%s`\n", result.TaskCompletionStatus)
		fmt.Fprintf(&b, "- Tests passed/failed: `%d/%d`\n", result.TestsPassed, result.TestsFailed)
		fmt.Fprintf(&b, "- Files changed: `%d`\n", len(result.FilesChanged))
		fmt.Fprintf(&b, "- Unexpected files changed: `%s`\n", strings.Join(result.UnexpectedFilesChanged, ", "))
		fmt.Fprintf(&b, "- Tool calls: `%d`\n", result.ToolCalls)
		fmt.Fprintf(&b, "- Approvals required: `%d`\n", result.ApprovalsRequired)
		fmt.Fprintf(&b, "- Runtime: `%dms`\n", result.RuntimeMS)
		fmt.Fprintf(&b, "- Model used: `%s`\n", result.ModelUsed)
		fmt.Fprintf(&b, "- Retries: `%d`\n", result.Retries)
		fmt.Fprintf(&b, "- Final diff size: `%+v`\n", result.FinalDiffSize)
		fmt.Fprintf(&b, "- Failure reason: `%s`\n\n", result.FailureReason)
	}
	return filewrite.AtomicWrite(path, []byte(b.String()), 0o644)
}

func displayFixtureName(result FixtureResult) string {
	if result.Repetition <= 1 {
		return result.FixtureID
	}
	return fmt.Sprintf("%s#%d", result.FixtureID, result.Repetition)
}
