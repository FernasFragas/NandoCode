package semantic

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// opsCountingStore counts the heavy loads so a test can prove Status reads only
// the manifest.
type opsCountingStore struct {
	Store
	mu      sync.Mutex
	records int
	vectors int
}

func (c *opsCountingStore) LoadRecords(ctx context.Context, m Manifest) ([]Record, error) {
	c.mu.Lock()
	c.records++
	c.mu.Unlock()
	return c.Store.LoadRecords(ctx, m)
}

func (c *opsCountingStore) LoadVectors(ctx context.Context, m Manifest) (VectorSet, error) {
	c.mu.Lock()
	c.vectors++
	c.mu.Unlock()
	return c.Store.LoadVectors(ctx, m)
}

// modelRecordingEmbedder records the model of every embed request.
type modelRecordingEmbedder struct {
	fixedDimensionEmbedder
	mu     sync.Mutex
	models []string
	texts  int
}

func (e *modelRecordingEmbedder) Embed(ctx context.Context, req EmbedRequest) (EmbedResult, error) {
	e.mu.Lock()
	e.models = append(e.models, req.Model)
	e.texts += len(req.Input)
	e.mu.Unlock()
	return e.fixedDimensionEmbedder.Embed(ctx, req)
}

func (e *modelRecordingEmbedder) reset() {
	e.mu.Lock()
	e.models, e.texts = nil, 0
	e.mu.Unlock()
}

// P1-8: Status must answer from the manifest without loading records or
// vectors (it runs on every browser prompt).
func TestStatusReadsOnlyTheManifest(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	store := &opsCountingStore{Store: NewLocalStore(t.TempDir())}
	svc := NewLocalService(store, fixedDimensionEmbedder{dimensions: 16})
	cfg := DefaultConfig()
	report, err := svc.Build(context.Background(), BuildRequest{Root: root, Config: cfg})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	store.mu.Lock()
	store.records, store.vectors = 0, 0
	store.mu.Unlock()

	st, err := svc.Status(context.Background(), root)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.records != 0 || store.vectors != 0 {
		t.Fatalf("Status loaded records=%d vectors=%d, want 0 and 0", store.records, store.vectors)
	}
	if !st.Exists || !st.Compatible {
		t.Fatalf("status exists=%v compatible=%v, want both true", st.Exists, st.Compatible)
	}
	if st.RecordCount != report.RecordsIndexed || st.RecordCount == 0 {
		t.Fatalf("status records=%d want %d", st.RecordCount, report.RecordsIndexed)
	}
	if st.VectorCount != st.RecordCount {
		t.Fatalf("status vectors=%d want %d", st.VectorCount, st.RecordCount)
	}
}

// P1-9: after an embedding-model change Refresh must re-embed every record
// with the new model instead of relabelling the old vectors.
func TestRefreshReembedsAfterEmbeddingModelChange(t *testing.T) {
	t.Parallel()
	root := buildFixtureWorkspace(t)
	emb := &modelRecordingEmbedder{fixedDimensionEmbedder: fixedDimensionEmbedder{dimensions: 16}}
	svc := NewLocalService(NewLocalStore(t.TempDir()), emb)
	cfg := DefaultConfig()
	cfg.Model = "embed-old"
	built, err := svc.Build(context.Background(), BuildRequest{Root: root, Config: cfg})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	emb.reset()

	cfg.Model = "embed-new"
	report, err := svc.Refresh(context.Background(), RefreshRequest{Root: root, Config: cfg})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	emb.mu.Lock()
	defer emb.mu.Unlock()
	if emb.texts != built.RecordsIndexed {
		t.Fatalf("refresh embedded %d records, want all %d re-embedded", emb.texts, built.RecordsIndexed)
	}
	for _, m := range emb.models {
		if m != "embed-new" {
			t.Fatalf("embed request used model %q, want embed-new", m)
		}
	}
	if report.Model != "embed-new" {
		t.Fatalf("report model=%q want embed-new", report.Model)
	}
	st, err := svc.Status(context.Background(), root)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if st.Model != "embed-new" || !strings.HasPrefix(st.Model, "embed-") {
		t.Fatalf("status model=%q want embed-new", st.Model)
	}
}
