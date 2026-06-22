package semantic

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type fixedDimensionEmbedder struct {
	dimensions int
}

func (e fixedDimensionEmbedder) Embed(_ context.Context, req EmbedRequest) (EmbedResult, error) {
	out := make([][]float32, len(req.Input))
	for i, text := range req.Input {
		vec := make([]float32, e.dimensions)
		seed := len(text) + 1
		for j := range vec {
			vec[j] = float32((seed + j%17) + 1)
		}
		NormalizeVector(vec)
		out[i] = vec
	}
	return EmbedResult{Vectors: out, Dimensions: e.dimensions}, nil
}

func TestBuildAdoptsActualEmbeddingDimensions(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := NewLocalStore(t.TempDir())
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 4096})
	cfg := DefaultConfig()
	cfg.Dimensions = 1024
	cfg.BatchSize = 2

	report, err := svc.Build(context.Background(), BuildRequest{
		Root:   root,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if report.Dimensions != 4096 {
		t.Fatalf("report dimensions=%d want 4096", report.Dimensions)
	}

	status, err := svc.Status(context.Background(), root)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Dimensions != 4096 {
		t.Fatalf("status dimensions=%d want 4096", status.Dimensions)
	}
}

func TestBuildPublishesProgressStagesInOrder(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := NewLocalStore(t.TempDir())
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 64})
	cfg := DefaultConfig()
	cfg.BatchSize = 2

	sink := &collectingEventSink{}
	_, err := svc.Build(context.Background(), BuildRequest{
		Root:      root,
		Config:    cfg,
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	assertStageOrder(t, sink.events, []Stage{
		StageScanStart,
		StageScanProgress,
		StageExtractProgress,
		StageEmbedProgress,
		StageWriteStart,
		StageWriteDone,
	})
	extract, ok := firstEventByStage(sink.events, StageExtractProgress)
	if !ok {
		t.Fatalf("missing %s event", StageExtractProgress)
	}
	if extract.FilesTotal <= 0 {
		t.Fatalf("extract files_total=%d want >0", extract.FilesTotal)
	}
	if extract.RecordsTotal <= 0 {
		t.Fatalf("extract records_total=%d want >0", extract.RecordsTotal)
	}
	if extract.RecordsDone != extract.RecordsTotal {
		t.Fatalf("extract records_done=%d records_total=%d want equal", extract.RecordsDone, extract.RecordsTotal)
	}
}

func TestBuildProgressCountersAreMonotonic(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := NewLocalStore(t.TempDir())
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 64})
	cfg := DefaultConfig()
	cfg.BatchSize = 2

	sink := &collectingEventSink{}
	_, err := svc.Build(context.Background(), BuildRequest{
		Root:      root,
		Config:    cfg,
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	seenScanProgress := 0
	lastTotal := 0
	lastSeen := 0
	lastIndexed := 0
	lastSkipped := 0
	lastRecords := 0
	for _, evt := range sink.events {
		if evt.Stage != StageScanProgress {
			continue
		}
		seenScanProgress++
		if evt.FilesTotal <= 0 {
			t.Fatalf("scan progress files_total=%d want >0", evt.FilesTotal)
		}
		if lastTotal == 0 {
			lastTotal = evt.FilesTotal
		}
		if evt.FilesTotal != lastTotal {
			t.Fatalf("scan progress files_total changed from %d to %d", lastTotal, evt.FilesTotal)
		}
		if evt.FilesSeen < lastSeen {
			t.Fatalf("files_seen regressed from %d to %d", lastSeen, evt.FilesSeen)
		}
		if evt.FilesIndexed < lastIndexed {
			t.Fatalf("files_indexed regressed from %d to %d", lastIndexed, evt.FilesIndexed)
		}
		if evt.FilesSkipped < lastSkipped {
			t.Fatalf("files_skipped regressed from %d to %d", lastSkipped, evt.FilesSkipped)
		}
		if evt.RecordsDone < lastRecords {
			t.Fatalf("records_done regressed from %d to %d", lastRecords, evt.RecordsDone)
		}
		if evt.FilesSeen > evt.FilesTotal {
			t.Fatalf("files_seen=%d files_total=%d want files_seen<=files_total", evt.FilesSeen, evt.FilesTotal)
		}
		lastSeen = evt.FilesSeen
		lastIndexed = evt.FilesIndexed
		lastSkipped = evt.FilesSkipped
		lastRecords = evt.RecordsDone
	}
	if seenScanProgress == 0 {
		t.Fatalf("expected at least one %s event", StageScanProgress)
	}
}

func TestBuildEmbedProgressIncludesBatchAndRecordTotals(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := NewLocalStore(t.TempDir())
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 32})
	cfg := DefaultConfig()
	cfg.BatchSize = 1

	sink := &collectingEventSink{}
	report, err := svc.Build(context.Background(), BuildRequest{
		Root:      root,
		Config:    cfg,
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	embedEvents := eventsByStage(sink.events, StageEmbedProgress)
	if len(embedEvents) == 0 {
		t.Fatalf("expected %s events", StageEmbedProgress)
	}
	totalBatches := embedEvents[0].TotalBatches
	recordsTotal := embedEvents[0].RecordsTotal
	lastBatch := 0
	lastRecords := 0
	for _, evt := range embedEvents {
		if evt.TotalBatches != totalBatches {
			t.Fatalf("total_batches changed from %d to %d", totalBatches, evt.TotalBatches)
		}
		if evt.RecordsTotal != recordsTotal {
			t.Fatalf("records_total changed from %d to %d", recordsTotal, evt.RecordsTotal)
		}
		if evt.BatchesDone < lastBatch {
			t.Fatalf("batches_done regressed from %d to %d", lastBatch, evt.BatchesDone)
		}
		if evt.RecordsDone < lastRecords {
			t.Fatalf("records_done regressed from %d to %d", lastRecords, evt.RecordsDone)
		}
		lastBatch = evt.BatchesDone
		lastRecords = evt.RecordsDone
	}
	if lastBatch != totalBatches {
		t.Fatalf("last batches_done=%d total_batches=%d", lastBatch, totalBatches)
	}
	if lastRecords != report.RecordsIndexed {
		t.Fatalf("last records_done=%d report records_indexed=%d", lastRecords, report.RecordsIndexed)
	}
	if recordsTotal != report.RecordsIndexed {
		t.Fatalf("records_total=%d report records_indexed=%d", recordsTotal, report.RecordsIndexed)
	}
}

func TestRefreshPublishesProgressWhenReusingVectors(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := NewLocalStore(t.TempDir())
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 48})
	cfg := DefaultConfig()
	cfg.BatchSize = 2

	_, err := svc.Build(context.Background(), BuildRequest{
		Root:   root,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sink := &collectingEventSink{}
	report, err := svc.Refresh(context.Background(), RefreshRequest{
		Root:      root,
		Config:    cfg,
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if report.EmbedBatches != 0 {
		t.Fatalf("refresh embed batches=%d want 0 for unchanged workspace", report.EmbedBatches)
	}
	assertStageOrder(t, sink.events, []Stage{
		StageScanStart,
		StageScanProgress,
		StageExtractProgress,
		StageWriteStart,
		StageWriteDone,
	})
}

func TestRefreshFallbackToBuildPublishesMessage(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := NewLocalStore(t.TempDir())
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 48})
	cfg := DefaultConfig()
	cfg.BatchSize = 2

	sink := &collectingEventSink{}
	_, err := svc.Refresh(context.Background(), RefreshRequest{
		Root:      root,
		Config:    cfg,
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	found := false
	for _, evt := range sink.events {
		if strings.Contains(strings.ToLower(evt.Message), "refresh falling back to full build") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fallback message in refresh events")
	}
}

func TestProgressEventMessagesDoNotLeakRawContent(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := NewLocalStore(t.TempDir())
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 64})
	cfg := DefaultConfig()
	cfg.BatchSize = 2

	sink := &collectingEventSink{}
	_, err := svc.Build(context.Background(), BuildRequest{
		Root:      root,
		Config:    cfg,
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, evt := range sink.events {
		msg := strings.ToLower(evt.Message)
		for _, leaked := range []string{
			"return user !=",
			"auth details.",
			"this is a generic text file",
			"token notes.",
		} {
			if strings.Contains(msg, strings.ToLower(leaked)) {
				t.Fatalf("event message leaked raw content %q: %q", leaked, evt.Message)
			}
		}
	}
}

type collectingEventSink struct {
	events []Event
}

func (s *collectingEventSink) Publish(evt Event) {
	s.events = append(s.events, evt)
}

func assertStageOrder(t *testing.T, events []Event, stages []Stage) {
	t.Helper()
	prev := -1
	for _, stage := range stages {
		idx := firstStageIndex(events, stage)
		if idx < 0 {
			t.Fatalf("missing stage %s", stage)
		}
		if idx <= prev {
			t.Fatalf("stage %s order invalid index=%d prev=%d", stage, idx, prev)
		}
		prev = idx
	}
}

func firstStageIndex(events []Event, stage Stage) int {
	for i, evt := range events {
		if evt.Stage == stage {
			return i
		}
	}
	return -1
}

func firstEventByStage(events []Event, stage Stage) (Event, bool) {
	for _, evt := range events {
		if evt.Stage == stage {
			return evt, true
		}
	}
	return Event{}, false
}

func eventsByStage(events []Event, stage Stage) []Event {
	out := make([]Event, 0)
	for _, evt := range events {
		if evt.Stage == stage {
			out = append(out, evt)
		}
	}
	return out
}

func TestRetrieveUsesCacheAndInvalidatesOnBuild(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := &countingStore{Store: NewLocalStore(t.TempDir())}
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 64})
	cfg := DefaultConfig()
	cfg.BatchSize = 2

	if _, err := svc.Build(context.Background(), BuildRequest{
		Root:   root,
		Config: cfg,
	}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	store.resetLoads()
	if _, err := svc.Retrieve(context.Background(), RetrieveRequest{
		Root:  root,
		Query: "fix authentication bug",
	}); err != nil {
		t.Fatalf("Retrieve() first call error = %v", err)
	}
	loads := store.loadCounts()
	if loads.records != 1 || loads.vectors != 1 {
		t.Fatalf("first retrieve loads records=%d vectors=%d want 1/1", loads.records, loads.vectors)
	}

	if _, err := svc.Retrieve(context.Background(), RetrieveRequest{
		Root:  root,
		Query: "fix authentication bug",
	}); err != nil {
		t.Fatalf("Retrieve() second call error = %v", err)
	}
	loads = store.loadCounts()
	if loads.records != 1 || loads.vectors != 1 {
		t.Fatalf("second retrieve should hit cache, loads records=%d vectors=%d want unchanged 1/1", loads.records, loads.vectors)
	}

	if _, err := svc.Build(context.Background(), BuildRequest{
		Root:   root,
		Config: cfg,
	}); err != nil {
		t.Fatalf("Build() second build error = %v", err)
	}

	store.resetLoads()
	if _, err := svc.Retrieve(context.Background(), RetrieveRequest{
		Root:  root,
		Query: "fix authentication bug",
	}); err != nil {
		t.Fatalf("Retrieve() after rebuild error = %v", err)
	}
	loads = store.loadCounts()
	if loads.records != 1 || loads.vectors != 1 {
		t.Fatalf("retrieve after rebuild should reload index, loads records=%d vectors=%d want 1/1", loads.records, loads.vectors)
	}
}

func TestRetrieveObserverReportsSubstagesAndCacheHit(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := NewLocalStore(t.TempDir())
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 64})
	cfg := DefaultConfig()
	cfg.BatchSize = 2
	if _, err := svc.Build(context.Background(), BuildRequest{
		Root:   root,
		Config: cfg,
	}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	first := make([]RetrieveStageEvent, 0, 8)
	_, err := svc.Retrieve(context.Background(), RetrieveRequest{
		Root:  root,
		Query: "fix authentication bug",
		Observer: func(evt RetrieveStageEvent) {
			first = append(first, evt)
		},
	})
	if err != nil {
		t.Fatalf("Retrieve() first call error = %v", err)
	}
	assertRetrieveStageSeen(t, first, RetrieveStageManifest)
	assertRetrieveStageSeen(t, first, RetrieveStageRecords)
	assertRetrieveStageSeen(t, first, RetrieveStageVectors)
	assertRetrieveStageSeen(t, first, RetrieveStageEmbed)
	assertRetrieveStageSeen(t, first, RetrieveStageScore)
	assertRetrieveStageSeen(t, first, RetrieveStageRender)
	assertRetrieveStageSeen(t, first, RetrieveStageTotal)
	if evt, ok := firstRetrieveStage(first, RetrieveStageRecords); !ok || evt.CacheHit {
		t.Fatalf("first retrieve records cache_hit=%v want false", ok && evt.CacheHit)
	}
	if evt, ok := firstRetrieveStage(first, RetrieveStageVectors); !ok || evt.CacheHit {
		t.Fatalf("first retrieve vectors cache_hit=%v want false", ok && evt.CacheHit)
	}

	second := make([]RetrieveStageEvent, 0, 8)
	_, err = svc.Retrieve(context.Background(), RetrieveRequest{
		Root:  root,
		Query: "fix authentication bug",
		Observer: func(evt RetrieveStageEvent) {
			second = append(second, evt)
		},
	})
	if err != nil {
		t.Fatalf("Retrieve() second call error = %v", err)
	}
	if evt, ok := firstRetrieveStage(second, RetrieveStageRecords); !ok || !evt.CacheHit {
		t.Fatalf("second retrieve records cache_hit=%v want true", ok && evt.CacheHit)
	}
	if evt, ok := firstRetrieveStage(second, RetrieveStageVectors); !ok || !evt.CacheHit {
		t.Fatalf("second retrieve vectors cache_hit=%v want true", ok && evt.CacheHit)
	}
	assertRetrieveStageSeen(t, second, RetrieveStageTotal)
}

type countingStore struct {
	Store
	mu               sync.Mutex
	loadRecordsCalls int
	loadVectorsCalls int
}

func (s *countingStore) LoadRecords(ctx context.Context, manifest Manifest) ([]Record, error) {
	s.mu.Lock()
	s.loadRecordsCalls++
	s.mu.Unlock()
	return s.Store.LoadRecords(ctx, manifest)
}

func (s *countingStore) LoadVectors(ctx context.Context, manifest Manifest) (VectorSet, error) {
	s.mu.Lock()
	s.loadVectorsCalls++
	s.mu.Unlock()
	return s.Store.LoadVectors(ctx, manifest)
}

func (s *countingStore) resetLoads() {
	s.mu.Lock()
	s.loadRecordsCalls = 0
	s.loadVectorsCalls = 0
	s.mu.Unlock()
}

type loadCountSnapshot struct {
	records int
	vectors int
}

func (s *countingStore) loadCounts() loadCountSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return loadCountSnapshot{
		records: s.loadRecordsCalls,
		vectors: s.loadVectorsCalls,
	}
}

func assertRetrieveStageSeen(t *testing.T, events []RetrieveStageEvent, stage RetrieveStage) {
	t.Helper()
	if _, ok := firstRetrieveStage(events, stage); !ok {
		t.Fatalf("missing retrieve stage %q", stage)
	}
}

func firstRetrieveStage(events []RetrieveStageEvent, stage RetrieveStage) (RetrieveStageEvent, bool) {
	for _, evt := range events {
		if evt.Stage == stage {
			return evt, true
		}
	}
	return RetrieveStageEvent{}, false
}
