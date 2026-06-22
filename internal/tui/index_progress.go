package tui

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/semantic"
)

type indexProgressState struct {
	Active       bool
	Operation    string
	Stage        semantic.Stage
	StartedAt    time.Time
	UpdatedAt    time.Time
	FilesTotal   int
	FilesSeen    int
	FilesDone    int
	FilesIndexed int
	FilesSkipped int
	RecordsDone  int
	RecordsTotal int
	BatchesDone  int
	TotalBatches int
	LastMessage  string
}

func (m *Model) startIndexProgress(operation string) {
	now := time.Now()
	m.indexProgress = indexProgressState{
		Active:    true,
		Operation: normalizeIndexOperation(operation),
		Stage:     semantic.StageScanStart,
		StartedAt: now,
		UpdatedAt: now,
	}
}

func (m *Model) clearIndexProgress() {
	m.indexProgress = indexProgressState{}
}

func (m *Model) updateIndexProgress(evt semantic.Event) {
	now := time.Now()
	st := m.indexProgress
	if !st.Active {
		st.Active = true
		st.StartedAt = now
	}
	st.UpdatedAt = now
	if st.Operation == "" {
		st.Operation = inferIndexOperation(evt.Message)
	}
	st.Operation = normalizeIndexOperation(st.Operation)
	if evt.Stage != "" {
		st.Stage = evt.Stage
	}
	if msg := strings.TrimSpace(evt.Message); msg != "" {
		st.LastMessage = msg
	}

	// Keep counters monotonic while still supporting older event payloads.
	st.FilesSeen = maxProgressInt(st.FilesSeen, evt.FilesSeen)
	st.FilesDone = maxProgressInt(st.FilesDone, evt.FilesDone)
	st.RecordsDone = maxProgressInt(st.RecordsDone, evt.RecordsDone)
	st.BatchesDone = maxProgressInt(st.BatchesDone, evt.BatchesDone)
	st.TotalBatches = maxProgressInt(st.TotalBatches, evt.TotalBatches)
	filesTotal := semanticEventIntField(evt, "FilesTotal")
	if filesTotal > 0 {
		st.FilesTotal = maxProgressInt(st.FilesTotal, filesTotal)
	} else if evt.Stage == semantic.StageScanStart || evt.Stage == semantic.StageScanProgress {
		// Scan events may intentionally report an unknown total.
		st.FilesTotal = 0
	}
	st.FilesIndexed = maxProgressInt(st.FilesIndexed, semanticEventIntField(evt, "FilesIndexed"))
	st.FilesSkipped = maxProgressInt(st.FilesSkipped, semanticEventIntField(evt, "FilesSkipped"))
	st.RecordsTotal = maxProgressInt(st.RecordsTotal, semanticEventIntField(evt, "RecordsTotal"))

	// Backfill totals/indexed from legacy fields used by the current semantic service.
	if st.FilesIndexed == 0 && st.FilesDone > 0 {
		st.FilesIndexed = st.FilesDone
	}
	if st.Stage == semantic.StageExtractProgress && st.FilesTotal == 0 && evt.FilesSeen > 0 {
		st.FilesTotal = evt.FilesSeen
	}
	if st.RecordsTotal < st.RecordsDone {
		st.RecordsTotal = st.RecordsDone
	}
	if st.FilesDone < st.FilesIndexed {
		st.FilesDone = st.FilesIndexed
	}
	if st.FilesSkipped == 0 && st.FilesTotal >= st.FilesIndexed && st.FilesTotal > 0 {
		st.FilesSkipped = st.FilesTotal - st.FilesIndexed
	}

	m.indexProgress = st
}

func (m *Model) renderIndexProgressStatus() string {
	st := m.indexProgress
	if !st.Active {
		return ""
	}
	op := normalizeIndexOperation(st.Operation)
	body := "running index"
	switch st.Stage {
	case semantic.StageScanStart, semantic.StageScanProgress:
		seen := maxProgressInt(st.FilesSeen, st.FilesDone)
		if seen <= 0 {
			body = "scanning workspace"
			break
		}
		if st.FilesTotal > 0 {
			body = fmt.Sprintf("scanning files %d/%d", seen, st.FilesTotal)
		} else {
			body = fmt.Sprintf("scanning files %d", seen)
		}
	case semantic.StageExtractProgress:
		done := st.FilesDone
		if done == 0 {
			done = st.FilesIndexed
		}
		segment := "extracting files"
		if st.FilesTotal > 0 && done > 0 {
			segment = fmt.Sprintf("extracting files %d/%d", done, st.FilesTotal)
		} else if done > 0 {
			segment = fmt.Sprintf("extracting files %d", done)
		}
		body = fmt.Sprintf("%s | indexed %d | skipped %d | records %d", segment, st.FilesIndexed, st.FilesSkipped, st.RecordsDone)
	case semantic.StageEmbedProgress:
		if st.TotalBatches > 0 {
			body = fmt.Sprintf("embedding batch %d/%d", st.BatchesDone, st.TotalBatches)
		} else if st.BatchesDone > 0 {
			body = fmt.Sprintf("embedding batch %d", st.BatchesDone)
		} else {
			body = "embedding records"
		}
		if st.RecordsTotal > 0 {
			body += fmt.Sprintf(" | records %d/%d", st.RecordsDone, st.RecordsTotal)
		} else if st.RecordsDone > 0 {
			body += fmt.Sprintf(" | records %d", st.RecordsDone)
		}
	case semantic.StageWriteStart, semantic.StageWriteDone:
		body = "writing index"
	default:
		if strings.TrimSpace(st.LastMessage) != "" {
			body = st.LastMessage
		}
	}
	line := fmt.Sprintf("Index %s: %s", op, body)
	if !st.StartedAt.IsZero() && m.width >= 110 {
		line += " | " + formatElapsedCompact(time.Since(st.StartedAt))
	}
	return line
}

func normalizeIndexOperation(operation string) string {
	op := strings.ToLower(strings.TrimSpace(operation))
	switch op {
	case "refresh":
		return "refresh"
	case "build":
		return "build"
	default:
		return "build"
	}
}

func inferIndexOperation(message string) string {
	msg := strings.ToLower(strings.TrimSpace(message))
	if strings.Contains(msg, "refresh") {
		return "refresh"
	}
	return "build"
}

func maxProgressInt(a, b int) int {
	if b > a {
		return b
	}
	return a
}

func semanticEventIntField(evt semantic.Event, field string) int {
	v := reflect.ValueOf(evt)
	f := v.FieldByName(field)
	if !f.IsValid() || f.Kind() != reflect.Int {
		return 0
	}
	return int(f.Int())
}
