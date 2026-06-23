package analysis

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/paths"
)

// Checkpoint stores a minimal durable run checkpoint for analysis recovery.
type Checkpoint struct {
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	Model               string    `json:"model"`
	WorkingDir          string    `json:"working_dir"`
	LastUserPrompt      string    `json:"last_user_prompt"`
	LastAssistantOutput string    `json:"last_assistant_output"`
	TerminalReason      string    `json:"terminal_reason"`
	TerminalDetail      string    `json:"terminal_detail"`
	PendingFinalAnswer  bool      `json:"pending_final_answer"`
	InspectedFiles      []string  `json:"inspected_files,omitempty"`
	SummariesUsed       []string  `json:"summaries_used,omitempty"`
	UnresolvedTasks     []string  `json:"unresolved_tasks,omitempty"`
	FinalObligations    []string  `json:"final_obligations,omitempty"`
	SynthesisStage      string    `json:"synthesis_stage,omitempty"`
}

var ErrCheckpointStale = errors.New("analysis checkpoint is stale")

func checkpointPath() string {
	return filepath.Join(paths.StateDir(), "analysis", "latest-checkpoint.json")
}

func SaveCheckpoint(c Checkpoint) error {
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	c.InspectedFiles = canonicalizeList(c.InspectedFiles)
	c.SummariesUsed = canonicalizeList(c.SummariesUsed)
	c.UnresolvedTasks = canonicalizeList(c.UnresolvedTasks)
	c.FinalObligations = canonicalizeList(c.FinalObligations)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := checkpointPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func LoadCheckpoint() (Checkpoint, error) {
	path := checkpointPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return Checkpoint{}, err
	}
	var c Checkpoint
	if err := json.Unmarshal(data, &c); err != nil {
		return Checkpoint{}, err
	}
	return c, nil
}

func DeleteCheckpoint() error {
	path := checkpointPath()
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func IsStale(c Checkpoint, maxAge time.Duration, now time.Time) bool {
	if maxAge <= 0 {
		return false
	}
	ts := c.UpdatedAt
	if ts.IsZero() {
		ts = c.CreatedAt
	}
	if ts.IsZero() {
		return true
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.Sub(ts) > maxAge
}

func LoadPendingCheckpoint(maxAge time.Duration) (Checkpoint, error) {
	c, err := LoadCheckpoint()
	if err != nil {
		return Checkpoint{}, err
	}
	if !c.PendingFinalAnswer {
		return Checkpoint{}, os.ErrNotExist
	}
	if IsStale(c, maxAge, time.Now().UTC()) {
		return Checkpoint{}, ErrCheckpointStale
	}
	return c, nil
}

func BuildResumePrompt(c Checkpoint, userPrompt string) string {
	ask := strings.TrimSpace(userPrompt)
	if ask == "" {
		ask = "continue"
	}
	var b strings.Builder
	b.WriteString("Continue the previous task from checkpoint and provide the missing final answer directly.\n")
	b.WriteString("Do not restate that you will write it. Write it now.\n\n")
	b.WriteString("Original task:\n")
	b.WriteString(c.LastUserPrompt)
	b.WriteString("\n\n")
	if trimmed := strings.TrimSpace(c.LastAssistantOutput); trimmed != "" {
		b.WriteString("Partial assistant output before interruption/incomplete stop:\n")
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}
	b.WriteString("User follow-up:\n")
	b.WriteString(ask)
	return b.String()
}

func LooksLikeIncompleteFinalAnswer(text string) bool {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return false
	}
	if len(s) > 1500 {
		return false
	}
	cues := []string{
		"let me write",
		"i will write",
		"i'll write",
		"i will now write",
		"now i have the full picture",
		"here is the comprehensive",
		"here is the missing",
		"as follows",
	}
	for _, cue := range cues {
		if strings.Contains(s, cue) && strings.HasSuffix(s, ":") {
			return true
		}
	}
	return false
}

func canonicalizeList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}
