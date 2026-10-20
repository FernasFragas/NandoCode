package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/state"
)

// Characterization (X1): the agent.Input fields the TUI derives from a prompt
// today (no semantic service wired).
func TestTUITurnPrepCharacterization(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("status\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ prompt, want string }{
		{
			"Respond with exactly: ok",
			"intent= attach= hist=default tool=default action=use_explicit_context_only reason=skip_explicit_context profile=semantic_off orig=\"Respond with exactly: ok\" msgs=1 pack=true",
		},
		{
			"what is the current status of @plan.md",
			"intent=file_status attach=content hist=latest_only tool=default action=use_explicit_context_only reason=skip_explicit_context profile=file_status orig=\"what is the current status of @plan.md\" msgs=1 pack=true",
		},
		{
			"list the files in @pkg/",
			"intent=directory_listing attach=listing_tree_only hist=latest_only tool=default action=use_local_search_only reason=skip_listing_intent profile=listing_intent orig=\"list the files in @pkg/\" msgs=1 pack=true",
		},
		{
			"fix authentication flow",
			"intent= attach= hist=default tool=default action=use_explicit_context_only reason=skip_explicit_context profile=semantic_off orig=\"fix authentication flow\" msgs=1 pack=true",
		},
		{
			"Find the server path that updates session models, verify whether cloud-listed models can actually be selected and summarize the behavioral currently",
			"intent= attach= hist=default tool=default action=use_explicit_context_only reason=skip_explicit_context profile=semantic_off orig=\"Find the server path that updates session models, verify whether cloud-listed models can actually be selected and summarize the behavioral currently\" msgs=1 pack=true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			initial := bootstrap.DefaultInitial(root)
			initial.DefaultModel = "test-model"
			store := state.NewStore(state.DefaultApp(bootstrap.New(initial).Snapshot()), nil)
			runner := &recordingRunner{}
			model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			cmd := model.submitPrompt(tc.prompt, tc.prompt, false)
			if cmd == nil {
				t.Fatal("expected an agent command")
			}
			cmd()
			in := runner.input
			got := fmt.Sprintf("intent=%s attach=%s hist=%s tool=%s action=%s reason=%s profile=%s orig=%q msgs=%d pack=%t",
				in.PromptIntent, in.AttachmentPolicy, in.HistoryPolicy, in.ToolMode, in.RouteAction, in.RouteReason,
				in.RouteProfile, in.OriginalUserText, len(in.Messages), in.EvidencePack != nil)
			if got != tc.want {
				t.Fatalf("got:\n%s", got)
			}
		})
	}
}
