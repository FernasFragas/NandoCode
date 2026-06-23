package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

func newBenchmarkModel(b *testing.B) *Model {
	b.Helper()
	initial := bootstrap.DefaultInitial("")
	appState := state.DefaultApp(bootstrap.New(initial).Snapshot())
	store := state.NewStore(appState, nil)
	model, err := New(store, &recordingRunner{}, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return model
}

func populateBenchmarkTranscript(model *Model, turns int) {
	for i := 0; i < turns; i++ {
		model.transcript = append(model.transcript, CreateUserItem(fmt.Sprintf(
			"Prompt %d\n\n- inspect logs\n- summarize the issue\n- keep the response concise",
			i,
		)))
		model.transcript = append(model.transcript, TranscriptItem{
			Kind: TranscriptAssistant,
			Content: fmt.Sprintf(
				"## Turn %d\n\n%s\n\n```go\nfmt.Println(%d)\n```\n",
				i,
				strings.Repeat("Historical markdown content with lists, emphasis, and wrapped text. ", 6),
				i,
			),
		})
		if i%8 == 0 {
			model.transcript = append(model.transcript, CreateSystemItem(fmt.Sprintf("checkpoint %d saved", i)))
		}
		if i%10 == 0 {
			model.transcript = append(model.transcript, TranscriptItem{
				Kind:     TranscriptTool,
				ToolID:   fmt.Sprintf("tool-%03d", i),
				ToolName: "FileRead",
				Content:  "[completed] scanned internal/tui/app.go",
			})
		}
	}
}

func BenchmarkRenderTranscript_LongTranscript_WarmCache(b *testing.B) {
	model := newBenchmarkModel(b)
	populateBenchmarkTranscript(model, 300)
	_ = model.renderTranscript()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.renderTranscript()
	}
}

func BenchmarkRenderTranscript_LongTranscript_Cold(b *testing.B) {
	model := newBenchmarkModel(b)
	populateBenchmarkTranscript(model, 300)
	_ = model.renderTranscript()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		model.invalidateTranscriptRenderCache()
		model.invalidateAssistantMarkdownCache()
		b.StartTimer()
		_ = model.renderTranscript()
	}
}

func BenchmarkRenderTranscript_LongTranscript_StreamingTail(b *testing.B) {
	model := newBenchmarkModel(b)
	populateBenchmarkTranscript(model, 300)
	_ = model.renderTranscript()
	model.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	model.transcript = append(model.transcript, CreateUserItem("Continue the analysis with the latest diff context."))
	model.transcript = append(model.transcript, TranscriptItem{
		Kind:    TranscriptAssistant,
		Content: "seed",
	})
	last := len(model.transcript) - 1
	if model.heightCache != nil {
		model.heightCache[last] = estimateTranscriptItemLines(model.transcript[last])
	}
	tailA := strings.Repeat("stream chunk with active tail markdown **alpha** ", 12)
	tailB := strings.Repeat("stream chunk with active tail markdown **beta** ", 12)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			model.transcript[last].Content = tailA
		} else {
			model.transcript[last].Content = tailB
		}
		model.transcript[last].Rendered = ""
		if model.heightCache != nil {
			model.heightCache[last] = estimateTranscriptItemLines(model.transcript[last])
		}
		_ = model.renderTranscript()
	}
}

func BenchmarkView_LongTranscript_AtBottom(b *testing.B) {
	model := newBenchmarkModel(b)
	populateBenchmarkTranscript(model, 300)
	model.viewport.Height = 12
	model.viewport.SetContent(model.renderTranscript())
	model.viewport.GotoBottom()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.View()
	}
}
