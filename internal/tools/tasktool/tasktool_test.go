package tasktool

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tasks"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/types"
)

func newSup(t *testing.T) *tasks.Supervisor {
	t.Helper()
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	return tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), st)
}

func getTools(t *testing.T, all []tools.Tool) (*CreateTool, *ListTool, *GetTool, *OutputTool, *StopTool) {
	t.Helper()
	var create *CreateTool
	var list *ListTool
	var get *GetTool
	var output *OutputTool
	var stop *StopTool
	for _, tt := range all {
		switch v := tt.(type) {
		case *CreateTool:
			create = v
		case *ListTool:
			list = v
		case *GetTool:
			get = v
		case *OutputTool:
			output = v
		case *StopTool:
			stop = v
		}
	}
	if create == nil || list == nil || get == nil || output == nil || stop == nil {
		t.Fatal("missing one or more task tools")
	}
	return create, list, get, output, stop
}

func TestCreateListGetOutputStop(t *testing.T) {
	sup := newSup(t)
	create, list, get, output, stop := getTools(t, NewAll(sup))

	res, err := create.Call(tools.Context{Context: context.Background(), WorkingDir: "."}, CreateInput{
		Kind: "bash", Description: "sleep", Command: "sleep 2",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := res.Data.(map[string]any)
	id := data["task_id"].(string)
	if id == "" {
		t.Fatal("expected task id")
	}
	if _, ok := data["output_file"].(string); !ok {
		t.Fatal("expected output_file in create response")
	}

	listRes, err := list.Call(tools.Context{}, ListInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listRes.Data.([]types.TaskSummary)) == 0 {
		t.Fatal("expected list to contain tasks")
	}

	getRes, err := get.Call(tools.Context{}, IDInput{TaskID: id}, nil)
	if err != nil {
		t.Fatal(err)
	}
	getData := getRes.Data.(map[string]any)
	if getData["task_id"] != id {
		t.Fatalf("expected task id %s in get output", id)
	}

	if _, err := output.Call(tools.Context{}, OutputInput{TaskID: id, MaxLines: 10}, nil); err != nil {
		t.Fatal(err)
	}

	stopRes, err := stop.Call(tools.Context{}, IDInput{TaskID: id}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := stopRes.Data.(types.TaskSummary)
	if sum.Status != types.StatusKilled && sum.Status != types.StatusCompleted && sum.Status != types.StatusFailed {
		t.Fatalf("unexpected stop status %q", sum.Status)
	}
}

func TestCreateReservedAndUnknownKindsReturnErrors(t *testing.T) {
	t.Parallel()
	sup := newSup(t)
	create, _, _, _, _ := getTools(t, NewAll(sup))
	ctx := tools.Context{Context: context.Background(), WorkingDir: "."}
	if _, err := create.Call(ctx, CreateInput{Kind: "mcp", Description: "x"}, nil); err == nil {
		t.Fatal("expected mcp kind error")
	}
	if _, err := create.Call(ctx, CreateInput{Kind: "remote", Description: "x"}, nil); err == nil {
		t.Fatal("expected remote kind error")
	}
	if _, err := create.Call(ctx, CreateInput{Kind: "unknown", Description: "x"}, nil); err == nil {
		t.Fatal("expected unknown kind error")
	}
}

func TestTaskListSupportsKindFilter(t *testing.T) {
	sup := newSup(t)
	_, list, _, _, _ := getTools(t, NewAll(sup))
	if _, err := sup.Start(context.Background(), types.KindBash, "bash", func(ctx context.Context, out *tasks.OutputWriter) (int, error) {
		time.Sleep(20 * time.Millisecond)
		return 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sup.Start(context.Background(), types.KindAgent, "agent", func(ctx context.Context, out *tasks.OutputWriter) (int, error) {
		time.Sleep(20 * time.Millisecond)
		return 0, nil
	}); err != nil {
		t.Fatal(err)
	}

	agentRes, err := list.Call(tools.Context{}, ListInput{Kind: "agent"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range agentRes.Data.([]types.TaskSummary) {
		if s.Kind != types.KindAgent {
			t.Fatalf("expected only agent tasks, got %q", s.Kind)
		}
	}
}

func TestGetUnknownTaskReturnsError(t *testing.T) {
	t.Parallel()
	sup := newSup(t)
	_, _, get, _, _ := getTools(t, NewAll(sup))
	if _, err := get.Call(tools.Context{}, IDInput{TaskID: "missing"}, nil); err == nil {
		t.Fatal("expected unknown task error")
	}
}

func TestToolMetadataAndEnabled(t *testing.T) {
	t.Parallel()
	sup := newSup(t)
	for _, tt := range NewAll(sup) {
		if tt.Name() == "" {
			t.Fatalf("empty name for %T", tt)
		}
		if tt.Description() == "" {
			t.Fatalf("empty description for %T", tt)
		}
		if tt.JSONSchema() == nil {
			t.Fatalf("nil schema for %T", tt)
		}
		if !tt.IsEnabled(tools.Context{}) {
			t.Fatalf("tool should be enabled %T", tt)
		}
	}
}
