package agent

import (
	"context"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

func TestInputDefaultsSubagentFalse(t *testing.T) {
	t.Parallel()
	var in Input
	if in.IsSubagent {
		t.Fatal("expected zero-value IsSubagent to be false")
	}
}

func TestValidateInputAllowsParentAbortAndOutputSink(t *testing.T) {
	t.Parallel()
	abortCh := make(chan struct{})
	in := Input{
		Model:       "test-model",
		ParentAbort: abortCh,
		OutputSink:  &testWriter{},
		ToolContext: tools.Context{
			Context: context.Background(),
		},
	}
	if err := validateInput(context.Background(), &in); err != nil {
		t.Fatalf("validateInput returned error: %v", err)
	}
}

type testWriter struct{}

func (w *testWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
