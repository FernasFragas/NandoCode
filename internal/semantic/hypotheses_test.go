package semantic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/observability"
)

func TestRetrieveUsesDefaultConfigForKeepAliveAndScoring(t *testing.T) {
	t.Parallel()
	root, records := writeRetrieveFixtureWorkspace(t)

	store := &stubRetrieveStore{
		manifest: Manifest{
			SchemaVersion: SchemaVersion,
			WorkspaceRoot: root,
			Model:         DefaultModel,
			Dimensions:    2,
		},
		records: records,
		vectors: VectorSet{
			Dimensions: 2,
			Vectors: [][]float32{
				{1.0, 0.0},  // weaker lexical match; wins only if lexical weight is zero
				{0.93, 0.0}, // stronger lexical match; wins with default lexical weight
			},
		},
	}
	embedder := &captureEmbedder{
		queryVector: []float32{1.0, 0.0},
	}
	svc := NewLocalService(store, embedder)

	got, err := svc.Retrieve(context.Background(), RetrieveRequest{
		Root:  root,
		Query: "authentication bug",
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if !got.Used {
		t.Fatalf("Retrieve() Used=false, fallback=%q", got.FallbackReason)
	}
	if len(got.Records) == 0 {
		t.Fatalf("Retrieve() returned no records")
	}
	if got.Records[0].Record.Path != "src/authentication_handler.go" {
		t.Fatalf("top record path=%q want %q", got.Records[0].Record.Path, "src/authentication_handler.go")
	}
	if got.Records[0].Reason != "vector+lexical" {
		t.Fatalf("top record reason=%q want %q", got.Records[0].Reason, "vector+lexical")
	}
	if embedder.lastReq.KeepAlive != DefaultQueryKeepAlive {
		t.Fatalf("embed keep_alive=%q want query default %q", embedder.lastReq.KeepAlive, DefaultQueryKeepAlive)
	}
}

func TestRetrieveCurrentTurnPathsDoesNotAffectOutput(t *testing.T) {
	t.Parallel()
	root, records := writeRetrieveFixtureWorkspace(t)

	store := &stubRetrieveStore{
		manifest: Manifest{
			SchemaVersion: SchemaVersion,
			WorkspaceRoot: root,
			Model:         DefaultModel,
			Dimensions:    2,
		},
		records: records,
		vectors: VectorSet{
			Dimensions: 2,
			Vectors: [][]float32{
				{1.0, 0.0},
				{0.93, 0.0},
			},
		},
	}
	embedder := &captureEmbedder{
		queryVector: []float32{1.0, 0.0},
	}
	svc := NewLocalService(store, embedder)

	base, err := svc.Retrieve(context.Background(), RetrieveRequest{
		Root:  root,
		Query: "authentication bug",
	})
	if err != nil {
		t.Fatalf("Retrieve() baseline error = %v", err)
	}
	withCurrentTurn, err := svc.Retrieve(context.Background(), RetrieveRequest{
		Root:             root,
		Query:            "authentication bug",
		CurrentTurnPaths: []string{"src/unrelated.go", "src/authentication_handler.go"},
	})
	if err != nil {
		t.Fatalf("Retrieve() current-turn error = %v", err)
	}

	if fmt.Sprintf("%v", base.Records) != fmt.Sprintf("%v", withCurrentTurn.Records) {
		t.Fatalf("records differ when only CurrentTurnPaths changed")
	}
	if fmt.Sprintf("%v", base.Files) != fmt.Sprintf("%v", withCurrentTurn.Files) {
		t.Fatalf("files differ when only CurrentTurnPaths changed")
	}
	if base.RenderedContext != withCurrentTurn.RenderedContext {
		t.Fatalf("rendered context differs when only CurrentTurnPaths changed")
	}
}

func TestRetrieveReturnsErrIndexMissingWhenManifestMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := &stubRetrieveStore{
		loadManifestErr: ErrIndexMissing,
	}
	embedder := &captureEmbedder{
		queryVector: []float32{1.0, 0.0},
	}
	svc := NewLocalService(store, embedder)

	_, err := svc.Retrieve(context.Background(), RetrieveRequest{
		Root:  root,
		Query: "authentication bug",
	})
	if !errors.Is(err, ErrIndexMissing) {
		t.Fatalf("Retrieve() err=%v want ErrIndexMissing", err)
	}
	if embedder.calls != 0 {
		t.Fatalf("embedder calls=%d want 0 when index is missing", embedder.calls)
	}
}

func TestRetrievePathWhenIndexExistsReturnsSemanticHits(t *testing.T) {
	t.Parallel()
	root, records := writeRetrieveFixtureWorkspace(t)

	store := &stubRetrieveStore{
		manifest: Manifest{
			SchemaVersion: SchemaVersion,
			WorkspaceRoot: root,
			Model:         DefaultModel,
			Dimensions:    2,
		},
		records: records,
		vectors: VectorSet{
			Dimensions: 2,
			Vectors: [][]float32{
				{1.0, 0.0},
				{0.93, 0.0},
			},
		},
	}
	embedder := &captureEmbedder{
		queryVector: []float32{1.0, 0.0},
	}
	svc := NewLocalService(store, embedder)

	got, err := svc.Retrieve(context.Background(), RetrieveRequest{
		Root:  root,
		Query: "authentication bug",
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if !got.Used {
		t.Fatalf("Retrieve() Used=false, fallback=%q", got.FallbackReason)
	}
	if len(got.Records) == 0 {
		t.Fatalf("Retrieve() returned zero hits with existing index")
	}
	if embedder.calls != 1 {
		t.Fatalf("embedder calls=%d want 1 with existing index", embedder.calls)
	}
}

type stubRetrieveStore struct {
	manifest        Manifest
	records         []Record
	vectors         VectorSet
	loadManifestErr error
	loadRecordsErr  error
	loadVectorsErr  error
}

func (s *stubRetrieveStore) LoadManifest(_ context.Context, _ string) (Manifest, error) {
	if s.loadManifestErr != nil {
		return Manifest{}, s.loadManifestErr
	}
	return s.manifest, nil
}

func (s *stubRetrieveStore) LoadRecords(_ context.Context, _ Manifest) ([]Record, error) {
	if s.loadRecordsErr != nil {
		return nil, s.loadRecordsErr
	}
	return s.records, nil
}

func (s *stubRetrieveStore) LoadVectors(_ context.Context, _ Manifest) (VectorSet, error) {
	if s.loadVectorsErr != nil {
		return VectorSet{}, s.loadVectorsErr
	}
	return s.vectors, nil
}

func (s *stubRetrieveStore) Replace(_ context.Context, _ Manifest, _ []Record, _ VectorSet) error {
	return nil
}

func (s *stubRetrieveStore) Clear(_ context.Context, _ string) error {
	return nil
}

type captureEmbedder struct {
	queryVector []float32
	lastReq     EmbedRequest
	calls       int
}

func (e *captureEmbedder) Embed(_ context.Context, req EmbedRequest) (EmbedResult, error) {
	e.calls++
	e.lastReq = req
	if len(req.Input) != 1 {
		return EmbedResult{}, fmt.Errorf("expected single query input got %d", len(req.Input))
	}
	out := make([]float32, len(e.queryVector))
	copy(out, e.queryVector)
	return EmbedResult{
		Dimensions: len(out),
		Vectors:    [][]float32{out},
	}, nil
}

func writeRetrieveFixtureWorkspace(t *testing.T) (string, []Record) {
	t.Helper()
	root := t.TempDir()
	fileA := filepath.Join(root, "src", "unrelated.go")
	fileB := filepath.Join(root, "src", "authentication_handler.go")
	if err := os.MkdirAll(filepath.Dir(fileA), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	contentA := "package main\nfunc unrelated() {}\n"
	contentB := "package main\nfunc authenticationHandler() { /* bug */ }\n"
	if err := os.WriteFile(fileA, []byte(contentA), 0o644); err != nil {
		t.Fatalf("write fileA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte(contentB), 0o644); err != nil {
		t.Fatalf("write fileB: %v", err)
	}
	hashA, err := HashFile(fileA)
	if err != nil {
		t.Fatalf("hash fileA: %v", err)
	}
	hashB, err := HashFile(fileB)
	if err != nil {
		t.Fatalf("hash fileB: %v", err)
	}
	return root, []Record{
		{
			ID:          "a",
			Kind:        RecordKindSymbol,
			Path:        "src/unrelated.go",
			Name:        "core",
			StartLine:   1,
			EndLine:     2,
			ContentHash: hashA,
			TextPreview: "general utilities",
		},
		{
			ID:          "b",
			Kind:        RecordKindSymbol,
			Path:        "src/authentication_handler.go",
			Name:        "authentication",
			StartLine:   1,
			EndLine:     2,
			ContentHash: hashB,
			TextPreview: "authentication bug flow",
		},
	}
}

type captureOptsClient struct {
	lastOpts *llm.EmbedOptions
}

func (c *captureOptsClient) Chat(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, nil
}
func (c *captureOptsClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return [][]float32{{0.1, 0.2}}, nil
}
func (c *captureOptsClient) EmbedWithOptions(_ context.Context, _ string, _ []string, opts *llm.EmbedOptions) ([][]float32, error) {
	c.lastOpts = opts
	return [][]float32{{0.1, 0.2}}, nil
}
func (c *captureOptsClient) ListModels(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (c *captureOptsClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (c *captureOptsClient) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

func TestLLMEmbedderPropagatesOptionsThroughRuntimeAndObservabilityWrappers(t *testing.T) {
	t.Parallel()
	base := &captureOptsClient{}
	runtime := llm.NewRuntimeClient(base, llm.ProviderOllamaLocal, "http://localhost:11434")
	observed := observability.WrapLLMClient(runtime, observability.NewMeter(), nil)

	embedder := LLMEmbedder{Client: observed}
	truncate := true
	out, err := embedder.Embed(context.Background(), EmbedRequest{
		Model:      "qwen3-embedding:8b",
		Input:      []string{"authentication bug"},
		Dimensions: 1024,
		Truncate:   &truncate,
		KeepAlive:  "30m",
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if out.Dimensions != 1024 {
		t.Fatalf("dimensions=%d want 1024", out.Dimensions)
	}
	if base.lastOpts == nil {
		t.Fatal("lastOpts=nil")
	}
	if base.lastOpts.Dimensions == nil || *base.lastOpts.Dimensions != 1024 {
		t.Fatalf("lastOpts.Dimensions=%v", base.lastOpts.Dimensions)
	}
	if base.lastOpts.Truncate == nil || !*base.lastOpts.Truncate {
		t.Fatalf("lastOpts.Truncate=%v", base.lastOpts.Truncate)
	}
	if base.lastOpts.KeepAlive != "30m" {
		t.Fatalf("lastOpts.KeepAlive=%q want 30m", base.lastOpts.KeepAlive)
	}
}
