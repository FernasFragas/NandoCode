// Package filewrite implements the FileWrite tool.
package filewrite

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

// Input is the FileWrite tool input.
type Input struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// PermissionTarget returns the file path for permission target matching.
func (i Input) PermissionTarget() string {
	return i.Path
}

// Output is the FileWrite tool output.
type Output struct {
	Path         string `json:"path"`
	BytesWritten int    `json:"bytes_written"`
	Created      bool   `json:"created"`
}

// NewFileWriteTool creates a FileWrite tool.
func NewFileWriteTool() tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:        "FileWrite",
		Description: "Write a text file atomically inside an allowed working directory.",
		Aliases:     []string{"Write"},
		Schema:      schema(),
		Unmarshal:   unmarshalInput,
		IsReadOnlyFunc: func(input any) bool {
			return false
		},
		IsConcurrentFunc: func(input any) bool {
			return false
		},
		IsDestructiveFunc: func(input any) bool {
			_, ok := input.(Input)
			return ok
		},
		CheckPermFunc: checkPermissions,
		CallFunc:      call,
		RenderFunc: func(input any, result tools.Result) tools.RenderHints {
			in, _ := input.(Input)
			return tools.RenderHints{Title: "FileWrite", Summary: in.Path}
		},
	})
}

func schema() map[string]any {
	return tools.ObjectSchema(map[string]any{
		"path":    tools.StringProperty("Path to write."),
		"content": tools.StringProperty("Complete file contents."),
	}, []string{"path", "content"})
}

func unmarshalInput(raw json.RawMessage) (any, error) {
	var input Input
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Path) == "" {
		return nil, errors.New("path is required")
	}
	return input, nil
}

func checkPermissions(ctx tools.Context, input any) tools.PermissionResult {
	if _, ok := input.(Input); !ok {
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "invalid FileWrite input"}
	}
	switch ctx.PermissionMode {
	case tools.PermissionBypassPermissions:
		return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
	case tools.PermissionPlan, tools.PermissionDontAsk:
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "FileWrite modifies files"}
	default:
		return tools.PermissionResult{Decision: tools.PermAsk, Reason: "FileWrite modifies files", UpdatedInput: input}
	}
}

func call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	in, ok := input.(Input)
	if !ok {
		return tools.Result{}, errors.New("invalid FileWrite input")
	}
	path, err := tools.ResolvePath(ctx, in.Path, tools.PathWrite)
	if err != nil {
		return tools.Result{}, err
	}
	parent := filepath.Dir(path)
	if stat, err := os.Stat(parent); err != nil {
		return tools.Result{}, err
	} else if !stat.IsDir() {
		return tools.Result{}, fmt.Errorf("parent path is not a directory: %s", parent)
	}

	prior, readErr := os.ReadFile(path)
	created := errors.Is(readErr, os.ErrNotExist)
	if readErr != nil && !created {
		return tools.Result{}, readErr
	}
	if ctx.RecordFileSnapshot != nil && !created {
		ctx.RecordFileSnapshot(path, append([]byte(nil), prior...))
	}

	if err := AtomicWrite(path, []byte(in.Content), 0o644); err != nil {
		return tools.Result{}, err
	}

	out := Output{Path: path, BytesWritten: len(in.Content), Created: created}
	display := fmt.Sprintf("Wrote %d bytes to %s", out.BytesWritten, out.Path)
	if out.Created {
		display = fmt.Sprintf("Created %s with %d bytes", out.Path, out.BytesWritten)
	}
	return tools.Result{Data: out, Display: display}, nil
}
