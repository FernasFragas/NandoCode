package analysis

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuildResumePrompt(t *testing.T) {
	p := BuildResumePrompt(Checkpoint{
		LastUserPrompt:      "analyze this repo and report missing items",
		LastAssistantOutput: "Now I have the full picture. Let me write the report:",
	}, "continue")
	if !strings.Contains(p, "provide the missing final answer directly") {
		t.Fatalf("missing recovery directive: %q", p)
	}
	if !strings.Contains(p, "analyze this repo") {
		t.Fatalf("missing original prompt: %q", p)
	}
}

func TestLooksLikeIncompleteFinalAnswer(t *testing.T) {
	if !LooksLikeIncompleteFinalAnswer("Now I have the full picture. Let me write the report:") {
		t.Fatal("expected incomplete promise-like output")
	}
	if LooksLikeIncompleteFinalAnswer("Here is the full report with tasks:\n1. ...\n2. ...") {
		t.Fatal("full answer should not be flagged as incomplete")
	}
}

func TestCheckpointSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	in := Checkpoint{
		Model:               "test-model",
		WorkingDir:          "/tmp/work",
		LastUserPrompt:      "analyze",
		LastAssistantOutput: "partial",
		TerminalReason:      "completed",
		PendingFinalAnswer:  true,
		InspectedFiles:      []string{" internal/tui/app.go ", "internal/tui/app.go", "docs/PHASE-LOG.md"},
		SummariesUsed:       []string{"chunk:internal/tui/app.go:0-4000"},
		UnresolvedTasks:     []string{"complete final answer"},
		FinalObligations:    []string{"include actionable tasks"},
		SynthesisStage:      "gather",
	}
	if err := SaveCheckpoint(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := LoadCheckpoint()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.Model != in.Model || out.LastUserPrompt != in.LastUserPrompt || out.PendingFinalAnswer != in.PendingFinalAnswer {
		t.Fatalf("round trip mismatch: in=%#v out=%#v", in, out)
	}
	if out.CreatedAt.IsZero() || out.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: %#v", out)
	}
	if len(out.InspectedFiles) != 2 {
		t.Fatalf("expected de-duplicated inspected files, got %#v", out.InspectedFiles)
	}
	if out.SynthesisStage != "gather" {
		t.Fatalf("unexpected synthesis stage: %q", out.SynthesisStage)
	}
}

func TestCheckpointRepeatedWritesRemainValidJSON(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	for i := 0; i < 200; i++ {
		in := Checkpoint{
			Model:               "m" + fmt.Sprintf("%03d", i%7),
			WorkingDir:          "/tmp/work",
			LastUserPrompt:      "prompt-" + fmt.Sprintf("%03d", i),
			LastAssistantOutput: "output-" + fmt.Sprintf("%03d", i),
			TerminalReason:      "completed",
			PendingFinalAnswer:  i%2 == 0,
		}
		if err := SaveCheckpoint(in); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
		if _, err := LoadCheckpoint(); err != nil {
			t.Fatalf("load %d: %v", i, err)
		}
	}
}

func TestLoadCheckpointMissingFile(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	_, err := LoadCheckpoint()
	if err == nil {
		t.Fatal("expected error for missing checkpoint")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestLoadPendingCheckpoint(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	if err := SaveCheckpoint(Checkpoint{
		LastUserPrompt:     "analyze",
		PendingFinalAnswer: true,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := LoadPendingCheckpoint(24 * time.Hour); err != nil {
		t.Fatalf("load pending: %v", err)
	}
}

func TestLoadPendingCheckpointNonPendingTreatsAsMissing(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	if err := SaveCheckpoint(Checkpoint{
		LastUserPrompt:     "analyze",
		PendingFinalAnswer: false,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	_, err := LoadPendingCheckpoint(24 * time.Hour)
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("expected not-exist for non-pending checkpoint, got %v", err)
	}
}

func TestLoadPendingCheckpointStale(t *testing.T) {
	c := Checkpoint{
		UpdatedAt: time.Now().UTC().Add(-4 * time.Hour),
	}
	if !IsStale(c, 30*time.Minute, time.Now().UTC()) {
		t.Fatal("expected stale checkpoint")
	}
}

func TestDeleteCheckpoint(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	if err := SaveCheckpoint(Checkpoint{
		LastUserPrompt:     "analyze",
		PendingFinalAnswer: true,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := DeleteCheckpoint(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := LoadCheckpoint(); err == nil || !os.IsNotExist(err) {
		t.Fatalf("expected missing after delete, got %v", err)
	}
}
