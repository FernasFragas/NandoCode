package tui

import (
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/turnprep"
)

func TestTUIRunCarriesGroundingSystemPrompt(t *testing.T) {
	initial := bootstrap.DefaultInitial(t.TempDir())
	initial.DefaultModel = "test-model"
	store := state.NewStore(state.DefaultApp(bootstrap.New(initial).Snapshot()), nil)
	runner := &recordingRunner{}
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	prompt := "Find the server path that updates session models"
	cmd := model.submitPrompt(prompt, prompt, false)
	if cmd == nil {
		t.Fatal("expected an agent command")
	}
	cmd()
	if !strings.Contains(runner.input.SystemPrompt, turnprep.GroundingMarker) {
		t.Fatalf("system prompt lacks grounding: %q", runner.input.SystemPrompt)
	}
}
