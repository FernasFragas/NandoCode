package semantic_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/semantic"
	"github.com/FernasFragas/nandocodego/internal/semantic/testutil"
)

func TestLocalStoreReplaceAndLoad(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	store := semantic.NewLocalStore(td)
	root := filepath.Join(td, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := semantic.WorkspaceID(root, semantic.DefaultModel, semantic.DefaultDimensions, semantic.SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testutil.FixtureManifest(root, semantic.DefaultModel, semantic.DefaultDimensions, id)
	records := testutil.FixtureRecords()
	vectors := semantic.VectorSet{
		Dimensions: semantic.DefaultDimensions,
		Vectors: [][]float32{
			testutil.DeterministicVector(records[0].EmbedText, semantic.DefaultDimensions),
			testutil.DeterministicVector(records[1].EmbedText, semantic.DefaultDimensions),
		},
	}
	for i := range vectors.Vectors {
		semantic.NormalizeVector(vectors.Vectors[i])
	}
	if err := store.Replace(context.Background(), manifest, records, vectors); err != nil {
		t.Fatal(err)
	}

	gotManifest, err := store.LoadManifest(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if gotManifest.RecordCount != len(records) {
		t.Fatalf("record_count=%d", gotManifest.RecordCount)
	}

	gotRecords, err := store.LoadRecords(context.Background(), gotManifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotRecords) != len(records) {
		t.Fatalf("records=%d", len(gotRecords))
	}

	gotVectors, err := store.LoadVectors(context.Background(), gotManifest)
	if err != nil {
		t.Fatal(err)
	}
	if gotVectors.Dimensions != semantic.DefaultDimensions || len(gotVectors.Vectors) != len(records) {
		t.Fatalf("unexpected vectors shape: dims=%d count=%d", gotVectors.Dimensions, len(gotVectors.Vectors))
	}
}

func TestLocalStoreClear(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	store := semantic.NewLocalStore(td)
	root := filepath.Join(td, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := semantic.WorkspaceID(root, semantic.DefaultModel, semantic.DefaultDimensions, semantic.SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testutil.FixtureManifest(root, semantic.DefaultModel, semantic.DefaultDimensions, id)
	records := testutil.FixtureRecords()
	vectors := semantic.VectorSet{Dimensions: semantic.DefaultDimensions, Vectors: make([][]float32, len(records))}
	for i := range records {
		vec := testutil.DeterministicVector(records[i].ID, semantic.DefaultDimensions)
		semantic.NormalizeVector(vec)
		vectors.Vectors[i] = vec
	}
	if err := store.Replace(context.Background(), manifest, records, vectors); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	_, err = store.LoadManifest(context.Background(), root)
	if !errors.Is(err, semantic.ErrIndexMissing) {
		t.Fatalf("expected ErrIndexMissing after clear, got %v", err)
	}
}

func TestLocalStoreReplaceFailurePreservesPreviousIndex(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	store := semantic.NewLocalStore(td)
	root := filepath.Join(td, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := semantic.WorkspaceID(root, semantic.DefaultModel, semantic.DefaultDimensions, semantic.SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testutil.FixtureManifest(root, semantic.DefaultModel, semantic.DefaultDimensions, id)
	records := testutil.FixtureRecords()
	valid := semantic.VectorSet{Dimensions: semantic.DefaultDimensions, Vectors: make([][]float32, len(records))}
	for i := range valid.Vectors {
		vec := testutil.DeterministicVector(records[i].ID+"-ok", semantic.DefaultDimensions)
		semantic.NormalizeVector(vec)
		valid.Vectors[i] = vec
	}
	if err := store.Replace(context.Background(), manifest, records, valid); err != nil {
		t.Fatal(err)
	}

	invalid := semantic.VectorSet{
		Dimensions: semantic.DefaultDimensions,
		Vectors:    valid.Vectors[:1],
	}
	if err := store.Replace(context.Background(), manifest, records, invalid); err == nil {
		t.Fatalf("expected replace failure for invalid vectors")
	}

	gotManifest, err := store.LoadManifest(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if gotManifest.RecordCount != len(records) {
		t.Fatalf("record_count changed after failed replace: %d", gotManifest.RecordCount)
	}
}

func TestLocalStoreSchemaMismatch(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	store := semantic.NewLocalStore(td)
	root := filepath.Join(td, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := semantic.Manifest{
		SchemaVersion: semantic.SchemaVersion + 1,
		WorkspaceRoot: root,
		Model:         semantic.DefaultModel,
		Dimensions:    semantic.DefaultDimensions,
	}
	err := store.Replace(context.Background(), manifest, nil, semantic.VectorSet{Dimensions: semantic.DefaultDimensions})
	if !errors.Is(err, semantic.ErrSchemaMismatch) {
		t.Fatalf("expected schema mismatch, got %v", err)
	}
}
