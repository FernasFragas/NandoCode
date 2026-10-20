package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// Characterization (X1): the agent.Input fields that --print derives from a
// prompt today. The same table lives in the server and TUI packages; the
// differences between surfaces (semantic wiring) are intentional and frozen.
func TestPrintTurnPrepCharacterization(t *testing.T) {
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
			"intent= attach= hist=default tool=none action=skip_all_retrieval reason=skip_general_prompt profile=general_prompt orig=\"Respond with exactly: ok\" msgs=1 pack=true",
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
			"intent= attach= hist=default tool=default action=use_semantic_full reason=run_workspace_discovery profile=workspace_discovery orig=\"fix authentication flow\" msgs=1 pack=true",
		},
		{
			"Find the server path that updates session models, verify whether cloud-listed models can actually be selected and summarize the behavioral currently",
			"intent= attach= hist=default tool=default action=use_semantic_full reason=run_workspace_discovery profile=workspace_discovery orig=\"Find the server path that updates session models, verify whether cloud-listed models can actually be selected and summarize the behavioral currently\" msgs=1 pack=true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			toolCtx := tools.DefaultContext(context.Background(), root)
			in, err := buildPrintInput(tc.prompt, toolCtx,
				agent.Config{ContextMode: "auto", MaxOutputTokens: 4096, NumCtx: 32768, ContextReserve: 4096},
				"test-model", "auto", 4096)
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("intent=%s attach=%s hist=%s tool=%s action=%s reason=%s profile=%s orig=%q msgs=%d pack=%t",
				in.PromptIntent, in.AttachmentPolicy, in.HistoryPolicy, in.ToolMode, in.RouteAction, in.RouteReason,
				in.RouteProfile, in.OriginalUserText, len(in.Messages), in.EvidencePack != nil)
			if got != tc.want {
				t.Fatalf("got:\n%s", got)
			}
		})
	}
}
