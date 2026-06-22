package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/semantic"
	tea "github.com/charmbracelet/bubbletea"
)

type captureProgramSender struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (s *captureProgramSender) Send(msg tea.Msg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
}

func (s *captureProgramSender) drain() []tea.Msg {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]tea.Msg(nil), s.msgs...)
	s.msgs = nil
	return out
}

type indexProgressServiceStub struct {
	status        semantic.Status
	statusErr     error
	clearErr      error
	buildReport   semantic.BuildReport
	buildErr      error
	refreshReport semantic.BuildReport
	refreshErr    error

	buildEvents   []semantic.Event
	refreshEvents []semantic.Event

	buildReqs   []semantic.BuildRequest
	refreshReqs []semantic.RefreshRequest
}

func (s *indexProgressServiceStub) Status(context.Context, string) (semantic.Status, error) {
	return s.status, s.statusErr
}

func (s *indexProgressServiceStub) Build(ctx context.Context, req semantic.BuildRequest) (semantic.BuildReport, error) {
	s.buildReqs = append(s.buildReqs, req)
	for _, evt := range s.buildEvents {
		if req.EventSink != nil {
			req.EventSink.Publish(evt)
		}
	}
	return s.buildReport, s.buildErr
}

func (s *indexProgressServiceStub) Refresh(ctx context.Context, req semantic.RefreshRequest) (semantic.BuildReport, error) {
	s.refreshReqs = append(s.refreshReqs, req)
	for _, evt := range s.refreshEvents {
		if req.EventSink != nil {
			req.EventSink.Publish(evt)
		}
	}
	return s.refreshReport, s.refreshErr
}

func (s *indexProgressServiceStub) Clear(context.Context, string) error {
	return s.clearErr
}

func (s *indexProgressServiceStub) Retrieve(context.Context, semantic.RetrieveRequest) (semantic.RetrieveResult, error) {
	return semantic.RetrieveResult{}, semantic.ErrIndexMissing
}

func submitSlash(t *testing.T, m *Model, raw string) tea.Cmd {
	t.Helper()
	m.input.SetValue(raw)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected slash command %q to return async cmd", raw)
	}
	return cmd
}

func applyMsgs(m *Model, msgs []tea.Msg) {
	for _, msg := range msgs {
		m.Update(msg)
	}
}

func statusText(m *Model) string {
	return m.renderStatusBar(m.store.Get())
}

func countIndexErrorLines(items []TranscriptItem) int {
	count := 0
	for _, item := range items {
		if item.Kind != TranscriptSystem {
			continue
		}
		if strings.HasPrefix(item.Content, "[Index error]") {
			count++
		}
	}
	return count
}

func setOptionalEventInt(evt *semantic.Event, field string, value int) bool {
	rv := reflect.ValueOf(evt).Elem()
	fv := rv.FieldByName(field)
	if !fv.IsValid() || !fv.CanSet() || fv.Kind() != reflect.Int {
		return false
	}
	fv.SetInt(int64(value))
	return true
}

func eventHasField(field string) bool {
	var evt semantic.Event
	_, ok := reflect.TypeOf(evt).FieldByName(field)
	return ok
}

func TestIndexProgress_BuildPassesNonNilEventSink(t *testing.T) {
	model := newTestModel(t)
	sender := &captureProgramSender{}
	model.SetProgramSender(sender)
	svc := &indexProgressServiceStub{
		buildReport: semantic.BuildReport{
			FilesSeen:      4,
			FilesIndexed:   3,
			RecordsIndexed: 7,
			FilesSkipped:   1,
			Duration:       40 * time.Millisecond,
		},
	}
	model.SetSemanticService(svc, semantic.DefaultConfig())

	cmd := submitSlash(t, model, "/index build")
	done := cmd()

	if len(svc.buildReqs) != 1 {
		t.Fatalf("build requests = %d, want 1", len(svc.buildReqs))
	}
	if svc.buildReqs[0].EventSink == nil {
		t.Fatalf("/index build passed nil EventSink")
	}

	model.Update(done)
}

func TestIndexProgress_RefreshPassesNonNilEventSink(t *testing.T) {
	model := newTestModel(t)
	sender := &captureProgramSender{}
	model.SetProgramSender(sender)
	svc := &indexProgressServiceStub{
		refreshReport: semantic.BuildReport{
			FilesSeen:      2,
			FilesIndexed:   2,
			RecordsIndexed: 5,
			Duration:       20 * time.Millisecond,
		},
	}
	model.SetSemanticService(svc, semantic.DefaultConfig())

	cmd := submitSlash(t, model, "/index refresh")
	done := cmd()

	if len(svc.refreshReqs) != 1 {
		t.Fatalf("refresh requests = %d, want 1", len(svc.refreshReqs))
	}
	if svc.refreshReqs[0].EventSink == nil {
		t.Fatalf("/index refresh passed nil EventSink")
	}

	model.Update(done)
}

func TestIndexProgress_ProgressEventActivatesAndDoneClears(t *testing.T) {
	model := newTestModel(t)
	sender := &captureProgramSender{}
	model.SetProgramSender(sender)

	progress := semantic.Event{
		Stage:       semantic.StageScanProgress,
		FilesSeen:   3,
		Message:     "scan progress",
		FilesDone:   0,
		Duration:    10 * time.Millisecond,
		RecordsDone: 0,
	}
	_ = setOptionalEventInt(&progress, "FilesTotal", 10)

	svc := &indexProgressServiceStub{
		buildEvents: []semantic.Event{progress},
		buildReport: semantic.BuildReport{
			FilesSeen:      4,
			FilesIndexed:   3,
			RecordsIndexed: 7,
			Duration:       50 * time.Millisecond,
		},
	}
	model.SetSemanticService(svc, semantic.DefaultConfig())

	cmd := submitSlash(t, model, "/index build")
	done := cmd()
	if len(svc.buildReqs) != 1 {
		t.Fatalf("build requests = %d, want 1", len(svc.buildReqs))
	}
	if svc.buildReqs[0].EventSink == nil {
		t.Fatalf("/index build passed nil EventSink")
	}

	progressMsgs := sender.drain()
	if len(progressMsgs) == 0 {
		t.Fatalf("expected progress messages to be sent to program")
	}
	applyMsgs(model, progressMsgs)

	activeStatus := statusText(model)
	if !strings.Contains(strings.ToLower(activeStatus), "index") {
		t.Fatalf("expected active index progress in status, got: %q", activeStatus)
	}

	model.Update(done)
	finalStatus := statusText(model)
	if strings.Contains(strings.ToLower(finalStatus), "index build:") ||
		strings.Contains(strings.ToLower(finalStatus), "index refresh:") {
		t.Fatalf("expected index progress to clear on completion, got: %q", finalStatus)
	}
}

func TestIndexProgress_ErrorClearsAndAppendsSingleErrorLine(t *testing.T) {
	model := newTestModel(t)
	sender := &captureProgramSender{}
	model.SetProgramSender(sender)

	progress := semantic.Event{
		Stage:     semantic.StageScanProgress,
		FilesSeen: 1,
		Message:   "scan progress",
	}
	svc := &indexProgressServiceStub{
		buildEvents: []semantic.Event{progress},
		buildErr:    errors.New("semantic embed failed"),
	}
	model.SetSemanticService(svc, semantic.DefaultConfig())

	cmd := submitSlash(t, model, "/index build")
	done := cmd()
	if len(svc.buildReqs) != 1 {
		t.Fatalf("build requests = %d, want 1", len(svc.buildReqs))
	}
	if svc.buildReqs[0].EventSink == nil {
		t.Fatalf("/index build passed nil EventSink")
	}

	progressMsgs := sender.drain()
	applyMsgs(model, progressMsgs)

	beforeErrors := countIndexErrorLines(model.transcript)
	model.Update(done)
	afterErrors := countIndexErrorLines(model.transcript)
	if delta := afterErrors - beforeErrors; delta != 1 {
		t.Fatalf("expected one new index error transcript line, delta=%d", delta)
	}
	last := model.transcript[len(model.transcript)-1]
	if !strings.Contains(last.Content, "[Index error] semantic embed failed") {
		t.Fatalf("unexpected final transcript line: %q", last.Content)
	}

	finalStatus := statusText(model)
	if strings.Contains(strings.ToLower(finalStatus), "index build:") ||
		strings.Contains(strings.ToLower(finalStatus), "index refresh:") {
		t.Fatalf("expected index progress to clear after error, got: %q", finalStatus)
	}
}

func TestIndexProgress_RenderScanWithAndWithoutTotals(t *testing.T) {
	if !eventHasField("FilesTotal") {
		t.Skip("semantic.Event.FilesTotal not present; scan total rendering is unavailable")
	}

	t.Run("with total", func(t *testing.T) {
		model := newTestModel(t)
		sender := &captureProgramSender{}
		model.SetProgramSender(sender)

		withTotal := semantic.Event{
			Stage:     semantic.StageScanProgress,
			FilesSeen: 3,
			Message:   "scan with total",
		}
		if !setOptionalEventInt(&withTotal, "FilesTotal", 10) {
			t.Skip("unable to set semantic.Event.FilesTotal")
		}

		svc := &indexProgressServiceStub{
			buildEvents: []semantic.Event{withTotal},
			buildReport: semantic.BuildReport{Duration: 30 * time.Millisecond},
		}
		model.SetSemanticService(svc, semantic.DefaultConfig())

		cmd := submitSlash(t, model, "/index build")
		done := cmd()
		if len(svc.buildReqs) != 1 {
			t.Fatalf("build requests = %d, want 1", len(svc.buildReqs))
		}
		if svc.buildReqs[0].EventSink == nil {
			t.Fatalf("/index build passed nil EventSink")
		}
		progressMsgs := sender.drain()
		if len(progressMsgs) == 0 {
			t.Fatalf("expected at least 1 progress message")
		}
		model.Update(progressMsgs[0])

		statusWithTotal := strings.ToLower(statusText(model))
		if !strings.Contains(statusWithTotal, "3/10") {
			t.Fatalf("expected scan status with total 3/10, got: %q", statusWithTotal)
		}
		model.Update(done)
	})

	t.Run("without total", func(t *testing.T) {
		model := newTestModel(t)
		sender := &captureProgramSender{}
		model.SetProgramSender(sender)

		withoutTotal := semantic.Event{
			Stage:     semantic.StageScanProgress,
			FilesSeen: 4,
			Message:   "scan without total",
		}
		_ = setOptionalEventInt(&withoutTotal, "FilesTotal", 0)

		svc := &indexProgressServiceStub{
			buildEvents: []semantic.Event{withoutTotal},
			buildReport: semantic.BuildReport{Duration: 30 * time.Millisecond},
		}
		model.SetSemanticService(svc, semantic.DefaultConfig())

		cmd := submitSlash(t, model, "/index build")
		done := cmd()
		if len(svc.buildReqs) != 1 {
			t.Fatalf("build requests = %d, want 1", len(svc.buildReqs))
		}
		if svc.buildReqs[0].EventSink == nil {
			t.Fatalf("/index build passed nil EventSink")
		}
		progressMsgs := sender.drain()
		if len(progressMsgs) == 0 {
			t.Fatalf("expected at least 1 progress message")
		}
		model.Update(progressMsgs[0])

		statusWithoutTotal := strings.ToLower(statusText(model))
		if !strings.Contains(statusWithoutTotal, "4") {
			t.Fatalf("expected scan status to include files seen count, got: %q", statusWithoutTotal)
		}
		if strings.Contains(statusWithoutTotal, "4/") {
			t.Fatalf("expected scan status without total denominator, got: %q", statusWithoutTotal)
		}
		model.Update(done)
	})
}
