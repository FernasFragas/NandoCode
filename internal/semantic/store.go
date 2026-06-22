package semantic

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type LocalStore struct {
	baseCacheDir string
}

func NewLocalStore(baseCacheDir string) *LocalStore {
	if strings.TrimSpace(baseCacheDir) == "" {
		if cache, err := os.UserCacheDir(); err == nil {
			baseCacheDir = cache
		} else {
			baseCacheDir = os.TempDir()
		}
	}
	return &LocalStore{baseCacheDir: baseCacheDir}
}

func (s *LocalStore) LoadManifest(ctx context.Context, root string) (Manifest, error) {
	_ = ctx
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return Manifest{}, err
	}

	indexDir := filepath.Join(s.baseCacheDir, "semantic")
	entries, err := os.ReadDir(indexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, ErrIndexMissing
		}
		return Manifest{}, err
	}

	var matches []Manifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		mfPath := filepath.Join(indexDir, entry.Name(), "manifest.json")
		b, err := os.ReadFile(mfPath)
		if err != nil {
			continue
		}
		var m Manifest
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		if m.WorkspaceRoot == canonical {
			matches = append(matches, m)
		}
	}
	if len(matches) == 0 {
		return Manifest{}, ErrIndexMissing
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].UpdatedAt.After(matches[j].UpdatedAt)
	})
	manifest := matches[0]
	if manifest.SchemaVersion != SchemaVersion {
		return Manifest{}, fmt.Errorf("%w: got %d want %d", ErrSchemaMismatch, manifest.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(manifest.WorkspaceID) == "" {
		return Manifest{}, fmt.Errorf("%w: missing workspace_id", ErrCorruptIndex)
	}
	return manifest, nil
}

func (s *LocalStore) LoadRecords(ctx context.Context, manifest Manifest) ([]Record, error) {
	_ = ctx
	path := filepath.Join(s.workspaceDir(manifest), "records.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: missing records file", ErrCorruptIndex)
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := make([]Record, 0, manifest.RecordCount)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("%w: invalid record json: %v", ErrCorruptIndex, err)
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if manifest.RecordCount > 0 && len(out) != manifest.RecordCount {
		return nil, fmt.Errorf("%w: record count mismatch got %d want %d", ErrCorruptIndex, len(out), manifest.RecordCount)
	}
	return out, nil
}

func (s *LocalStore) LoadVectors(ctx context.Context, manifest Manifest) (VectorSet, error) {
	_ = ctx
	if manifest.Dimensions <= 0 {
		return VectorSet{}, fmt.Errorf("%w: invalid dimensions %d", ErrCorruptIndex, manifest.Dimensions)
	}
	path := filepath.Join(s.workspaceDir(manifest), "vectors.f32")
	vectors, err := LoadF32File(path, manifest.Dimensions, manifest.RecordCount)
	if err != nil {
		return VectorSet{}, err
	}
	return VectorSet{
		Dimensions: manifest.Dimensions,
		Vectors:    vectors,
	}, nil
}

func (s *LocalStore) Replace(ctx context.Context, manifest Manifest, records []Record, vectors VectorSet) error {
	_ = ctx
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = SchemaVersion
	}
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: got %d want %d", ErrSchemaMismatch, manifest.SchemaVersion, SchemaVersion)
	}
	canonicalRoot, err := CanonicalRoot(manifest.WorkspaceRoot)
	if err != nil {
		return err
	}
	manifest.WorkspaceRoot = canonicalRoot
	if strings.TrimSpace(manifest.Model) == "" {
		return fmt.Errorf("manifest model is required")
	}
	if manifest.Dimensions <= 0 {
		return fmt.Errorf("manifest dimensions must be > 0")
	}
	if strings.TrimSpace(manifest.WorkspaceID) == "" {
		workspaceID, err := WorkspaceID(manifest.WorkspaceRoot, manifest.Model, manifest.Dimensions, manifest.SchemaVersion)
		if err != nil {
			return err
		}
		manifest.WorkspaceID = workspaceID
	}
	if err := ValidateVectorSet(vectors, manifest.Dimensions, len(records)); err != nil {
		return err
	}

	now := time.Now().UTC()
	dir := s.workspaceDir(manifest)
	if prior, err := s.loadManifestByPath(filepath.Join(dir, "manifest.json")); err == nil && !prior.CreatedAt.IsZero() {
		manifest.CreatedAt = prior.CreatedAt
	}
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = now
	}
	manifest.UpdatedAt = now
	manifest.RecordCount = len(records)

	baseSemantic := filepath.Join(s.baseCacheDir, "semantic")
	if err := os.MkdirAll(baseSemantic, 0o755); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(baseSemantic, "replace-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := writeManifest(filepath.Join(tmpDir, "manifest.json"), manifest); err != nil {
		return err
	}
	if err := writeRecords(filepath.Join(tmpDir, "records.jsonl"), records, manifest.StorePreviews); err != nil {
		return err
	}
	if err := WriteF32File(filepath.Join(tmpDir, "vectors.f32"), vectors.Vectors, vectors.Dimensions); err != nil {
		return err
	}

	targetParent := filepath.Dir(dir)
	if err := os.MkdirAll(targetParent, 0o755); err != nil {
		return err
	}
	oldDir := dir + ".old"
	_ = os.RemoveAll(oldDir)
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, oldDir); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpDir, dir); err != nil {
		if _, statErr := os.Stat(oldDir); statErr == nil {
			_ = os.Rename(oldDir, dir)
		}
		return err
	}
	_ = os.RemoveAll(oldDir)
	return nil
}

func (s *LocalStore) Clear(ctx context.Context, root string) error {
	_ = ctx
	manifest, err := s.LoadManifest(context.Background(), root)
	if err != nil {
		if errors.Is(err, ErrIndexMissing) {
			return nil
		}
		return err
	}
	return os.RemoveAll(s.workspaceDir(manifest))
}

func (s *LocalStore) workspaceDir(manifest Manifest) string {
	return filepath.Join(s.baseCacheDir, "semantic", manifest.WorkspaceID)
}

func (s *LocalStore) loadManifestByPath(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func writeManifest(path string, manifest Manifest) error {
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func writeRecords(path string, records []Record, storePreviews bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := range records {
		rec := records[i]
		if !storePreviews {
			rec.TextPreview = ""
			rec.EmbedText = ""
		}
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}
	return nil
}
