package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func snapshotText(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read snapshot %s: %v", name, err)
	}
	return strings.TrimSpace(string(b))
}

func TestStatusBarSnapshotsWidths(t *testing.T) {
	model := newTestModel(t)
	model.store.Set(func(app state.App) state.App {
		app.ActiveModel = "test-model"
		return app
	})
	assertStatusSnapshots(t, model, "status_idle")
}

func TestStatusBarSnapshotsPhases(t *testing.T) {
	tests := []struct {
		name  string
		base  string
		setup func(*Model)
	}{
		{
			name: "queued",
			base: "status_queued",
			setup: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveModel = "test-model"
					app.QueuedPrompts = []string{"one", "two"}
					return app
				})
			},
		},
		{
			name: "waiting",
			base: "status_waiting",
			setup: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveModel = "test-model"
					app.ActiveRun = true
					return app
				})
			},
		},
		{
			name: "streaming",
			base: "status_streaming",
			setup: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveModel = "test-model"
					app.ActiveRun = true
					return app
				})
				m.firstStreamAt = time.Now()
			},
		},
		{
			name: "thinking",
			base: "status_thinking",
			setup: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveModel = "test-model"
					app.ActiveRun = true
					return app
				})
				m.thinkingActive = true
			},
		},
		{
			name: "retrying",
			base: "status_retrying",
			setup: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveModel = "test-model"
					return app
				})
				m.retryActiveUntil = time.Now().Add(2 * time.Second)
			},
		},
		{
			name: "compacting",
			base: "status_compacting",
			setup: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveModel = "test-model"
					return app
				})
				m.compactingActive = true
			},
		},
		{
			name: "permission",
			base: "status_permission",
			setup: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveModel = "test-model"
					app.PermissionPrompt = &state.PermissionPrompt{ID: "p1", ToolName: "bash", Target: "pwd"}
					return app
				})
			},
		},
		{
			name: "running_tool",
			base: "status_running_tool",
			setup: func(m *Model) {
				m.store.Set(func(app state.App) state.App {
					app.ActiveModel = "test-model"
					app.ActiveTools["t1"] = state.ToolUse{
						ID:   "t1",
						Name: "Bash",
						Done: false,
					}
					return app
				})
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := newTestModel(t)
			tc.setup(model)
			assertStatusSnapshots(t, model, tc.base)
		})
	}
}

func assertStatusSnapshots(t *testing.T, model *Model, base string) {
	t.Helper()
	cases := []struct {
		name  string
		width int
		file  string
	}{
		{name: "w60", width: 60, file: base + "_60.txt"},
		{name: "w80", width: 80, file: base + "_80.txt"},
		{name: "w120", width: 120, file: base + "_120.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model.Update(tea.WindowSizeMsg{Width: tc.width, Height: 30})
			got := strings.TrimSpace(stripANSI(model.renderStatusBar(model.store.Get())))
			want := snapshotText(t, tc.file)
			if got != want {
				t.Fatalf("snapshot mismatch\nwant: %q\n got: %q", want, got)
			}
		})
	}
}
