package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/state"
)

func TestSessionViewExposesWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	app := state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(root)).Snapshot())
	app.ToolSettings.WorkingDir = root
	s := newSession(context.Background(), "root-test", app, fakeRunner{})
	t.Cleanup(s.stop)
	v := s.view()
	if v.WorkspaceRoot != root {
		t.Fatalf("workspace root=%q want %q", v.WorkspaceRoot, root)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["workspace_root"] != root {
		t.Fatalf("json workspace_root=%v want %q", m["workspace_root"], root)
	}
}
