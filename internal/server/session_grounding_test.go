package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/turnprep"
)

func TestServerRunCarriesGroundingSystemPrompt(t *testing.T) {
	root := t.TempDir()
	app := state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(root)).Snapshot())
	inputs := make(chan agent.Input, 1)
	s := newSession(context.Background(), "ground", app, capturingRunner{inputs: inputs})
	if err := s.StartRun(MessageRequest{Prompt: "Find the server path that updates session models"}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	select {
	case in := <-inputs:
		if !strings.Contains(in.SystemPrompt, turnprep.GroundingMarker) {
			t.Fatalf("system prompt lacks grounding: %q", in.SystemPrompt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for captured input")
	}
}
