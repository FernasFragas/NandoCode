package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type benchmarkDatasetSize struct {
	name    string
	records int
}

type benchmarkDimensionProfile struct {
	name       string
	dimensions int
	sizes      []benchmarkDatasetSize
}

type benchmarkCase struct {
	name       string
	dimensions int
	records    int
}

var benchmarkDimensionProfiles = []benchmarkDimensionProfile{
	{
		name:       "dim256",
		dimensions: 256,
		sizes: []benchmarkDatasetSize{
			{name: "small_300", records: 300},
			{name: "medium_1200", records: 1200},
			{name: "large_4800", records: 4800},
		},
	},
	{
		name:       "dim1024",
		dimensions: 1024,
		sizes: []benchmarkDatasetSize{
			{name: "small_300", records: 300},
			{name: "medium_1200", records: 1200},
			{name: "large_2400", records: 2400},
		},
	},
	{
		name:       "dim4096",
		dimensions: 4096,
		sizes: []benchmarkDatasetSize{
			// 4096-dimension vectors multiply I/O and scoring costs substantially, so
			// keep record counts conservative while still covering higher-dimensional shapes.
			{name: "small_150", records: 150},
			{name: "medium_600", records: 600},
			{name: "large_1200", records: 1200},
		},
	},
}

var (
	benchRecordsSink  []Record
	benchVectorsSink  VectorSet
	benchHitsSink     []SearchHit
	benchRetrieveSink RetrieveResult
)

type benchmarkStoreFixture struct {
	root     string
	manifest Manifest
	store    *LocalStore
	records  []Record
	vectors  VectorSet
}

type benchmarkEmbedder struct{}

func (benchmarkEmbedder) Embed(_ context.Context, req EmbedRequest) (EmbedResult, error) {
	dims := req.Dimensions
	if dims <= 0 {
		dims = DefaultDimensions
	}
	out := make([][]float32, len(req.Input))
	for i, text := range req.Input {
		vec := deterministicBenchVector(hashSeed(text, i+1), dims)
		if !NormalizeVector(vec) {
			return EmbedResult{}, fmt.Errorf("generated zero-norm vector for benchmark input")
		}
		out[i] = vec
	}
	return EmbedResult{Vectors: out, Dimensions: dims}, nil
}

func BenchmarkLocalStoreLoadRecordsScaling(b *testing.B) {
	ctx := context.Background()
	forEachBenchmarkCase(b, func(b *testing.B, tc benchmarkCase) {
		b.Run(tc.name, func(b *testing.B) {
			fixture := seedBenchmarkStore(b, tc.records, tc.dimensions, false)
			b.ReportAllocs()
			b.SetBytes(int64(tc.records))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				records, err := fixture.store.LoadRecords(ctx, fixture.manifest)
				if err != nil {
					b.Fatal(err)
				}
				benchRecordsSink = records
			}
		})
	})
}

func BenchmarkLocalStoreLoadVectorsScaling(b *testing.B) {
	ctx := context.Background()
	forEachBenchmarkCase(b, func(b *testing.B, tc benchmarkCase) {
		b.Run(tc.name, func(b *testing.B) {
			fixture := seedBenchmarkStore(b, tc.records, tc.dimensions, false)
			b.ReportAllocs()
			b.SetBytes(int64(tc.records * tc.dimensions * 4))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				vectors, err := fixture.store.LoadVectors(ctx, fixture.manifest)
				if err != nil {
					b.Fatal(err)
				}
				benchVectorsSink = vectors
			}
		})
	})
}

func BenchmarkScoreRecordsScaling(b *testing.B) {
	query := "fix authentication token validation middleware bug"
	cfg := DefaultConfig()
	forEachBenchmarkCase(b, func(b *testing.B, tc benchmarkCase) {
		b.Run(tc.name, func(b *testing.B) {
			records, vectors := generateBenchmarkRecordsVectors(tc.records, tc.dimensions, nil)
			queryVec := deterministicBenchVector(999_983, tc.dimensions)
			NormalizeVector(queryVec)
			explicitPaths := []string{records[0].Path, records[len(records)-1].Path}
			b.ReportAllocs()
			b.SetBytes(int64(tc.records * tc.dimensions * 4))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				hits, err := scoreRecords(query, queryVec, records, vectors, cfg, explicitPaths, nil, false)
				if err != nil {
					b.Fatal(err)
				}
				benchHitsSink = hits
			}
		})
	})
}

func BenchmarkScoreRecordsLightModeScaling(b *testing.B) {
	query := "status ping"
	cfg := DefaultConfig()
	forEachBenchmarkCase(b, func(b *testing.B, tc benchmarkCase) {
		b.Run(tc.name, func(b *testing.B) {
			records, vectors := generateBenchmarkRecordsVectorsMultiDir(tc.records, tc.dimensions, 32)
			queryVec := deterministicBenchVector(999_983, tc.dimensions)
			NormalizeVector(queryVec)
			currentTurnPaths := []string{"pkg/group_000/file_00000.go"}
			explicitPaths := []string{"pkg/group_001/file_00001.go"}
			b.ReportAllocs()
			b.SetBytes(int64(tc.records * tc.dimensions * 4))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				hits, err := scoreRecords(query, queryVec, records, vectors, cfg, explicitPaths, currentTurnPaths, true)
				if err != nil {
					b.Fatal(err)
				}
				benchHitsSink = hits
			}
		})
	})
}

func BenchmarkLocalServiceRetrieveScaling(b *testing.B) {
	ctx := context.Background()
	forEachBenchmarkCase(b, func(b *testing.B, tc benchmarkCase) {
		b.Run(tc.name, func(b *testing.B) {
			fixture := seedBenchmarkStore(b, tc.records, tc.dimensions, true)
			svc := NewLocalService(fixture.store, benchmarkEmbedder{})
			req := RetrieveRequest{
				Root:            fixture.root,
				Query:           "fix authentication token validation middleware bug",
				MaxRecords:      40,
				MaxFiles:        12,
				MaxContextBytes: 128 * 1024,
				ExplicitPaths: []string{
					fixture.records[0].Path,
					fixture.records[len(fixture.records)-1].Path,
				},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				res, err := svc.Retrieve(ctx, req)
				if err != nil {
					b.Fatal(err)
				}
				benchRetrieveSink = res
			}
		})
	})
}

func forEachBenchmarkCase(b *testing.B, fn func(*testing.B, benchmarkCase)) {
	b.Helper()
	for _, profile := range benchmarkDimensionProfiles {
		profile := profile
		for _, size := range profile.sizes {
			size := size
			fn(b, benchmarkCase{
				name:       fmt.Sprintf("%s/%s", profile.name, size.name),
				dimensions: profile.dimensions,
				records:    size.records,
			})
		}
	}
}

func seedBenchmarkStore(b *testing.B, recordCount, dimensions int, writeWorkspaceFiles bool) benchmarkStoreFixture {
	b.Helper()
	td := b.TempDir()
	root := filepath.Join(td, "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		b.Fatal(err)
	}
	fileContent := map[string]string(nil)
	if writeWorkspaceFiles {
		fileContent = createBenchmarkWorkspaceFiles(b, root, benchFileCount(recordCount))
	}
	records, vectors := generateBenchmarkRecordsVectors(recordCount, dimensions, fileContent)
	workspaceID, err := WorkspaceID(root, DefaultModel, dimensions, SchemaVersion)
	if err != nil {
		b.Fatal(err)
	}
	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		WorkspaceRoot: root,
		WorkspaceID:   workspaceID,
		Model:         DefaultModel,
		Dimensions:    dimensions,
		StorePreviews: true,
	}
	store := NewLocalStore(td)
	if err := store.Replace(context.Background(), manifest, records, vectors); err != nil {
		b.Fatal(err)
	}
	loadedManifest, err := store.LoadManifest(context.Background(), root)
	if err != nil {
		b.Fatal(err)
	}
	return benchmarkStoreFixture{
		root:     root,
		manifest: loadedManifest,
		store:    store,
		records:  records,
		vectors:  vectors,
	}
}

func createBenchmarkWorkspaceFiles(b *testing.B, root string, fileCount int) map[string]string {
	b.Helper()
	contentByPath := make(map[string]string, fileCount)
	for i := 0; i < fileCount; i++ {
		rel := fmt.Sprintf("pkg/file_%05d.go", i)
		var sb strings.Builder
		sb.WriteString("package pkg\n\n")
		sb.WriteString(fmt.Sprintf("// file_%05d benchmark fixture\n", i))
		for line := 0; line < 80; line++ {
			sb.WriteString(fmt.Sprintf("func Handler%05dLine%02d() string { return \"auth token middleware %d\" }\n", i, line, line))
		}
		content := sb.String()
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
		contentByPath[rel] = content
	}
	return contentByPath
}

func generateBenchmarkRecordsVectors(recordCount, dimensions int, fileContent map[string]string) ([]Record, VectorSet) {
	files := benchFileCount(recordCount)
	records := make([]Record, 0, recordCount)
	vectors := make([][]float32, 0, recordCount)
	for i := 0; i < recordCount; i++ {
		path := fmt.Sprintf("pkg/file_%05d.go", i%files)
		contentHash := HashText(path)
		if fileContent != nil {
			if content, ok := fileContent[path]; ok {
				contentHash = HashText(content)
			}
		}
		start := 5 + (i % 30)
		end := start + 8
		rec := Record{
			ID:          fmt.Sprintf("rec-%06d", i),
			Kind:        RecordKindSymbol,
			Path:        path,
			Language:    "go",
			Name:        fmt.Sprintf("AuthHandler%06d", i),
			StartLine:   start,
			EndLine:     end,
			ContentHash: contentHash,
			TextHash:    HashText(fmt.Sprintf("text-%06d", i)),
			TextPreview: "authentication token middleware validation flow",
			EmbedText:   fmt.Sprintf("go authentication token validation middleware handler %d", i),
		}
		records = append(records, rec)
		vec := deterministicBenchVector(i+17, dimensions)
		NormalizeVector(vec)
		vectors = append(vectors, vec)
	}
	return records, VectorSet{Dimensions: dimensions, Vectors: vectors}
}

func generateBenchmarkRecordsVectorsMultiDir(recordCount, dimensions, dirGroups int) ([]Record, VectorSet) {
	if dirGroups <= 0 {
		dirGroups = 1
	}
	files := benchFileCount(recordCount)
	records := make([]Record, 0, recordCount)
	vectors := make([][]float32, 0, recordCount)
	for i := 0; i < recordCount; i++ {
		group := i % dirGroups
		path := fmt.Sprintf("pkg/group_%03d/file_%05d.go", group, i%files)
		start := 5 + (i % 30)
		end := start + 8
		rec := Record{
			ID:          fmt.Sprintf("rec-%06d", i),
			Kind:        RecordKindSymbol,
			Path:        path,
			Language:    "go",
			Name:        fmt.Sprintf("AuthHandler%06d", i),
			StartLine:   start,
			EndLine:     end,
			ContentHash: HashText(path),
			TextHash:    HashText(fmt.Sprintf("text-%06d", i)),
			TextPreview: "authentication token middleware validation flow",
			EmbedText:   fmt.Sprintf("go authentication token validation middleware handler %d", i),
		}
		records = append(records, rec)
		vec := deterministicBenchVector(i+17, dimensions)
		NormalizeVector(vec)
		vectors = append(vectors, vec)
	}
	return records, VectorSet{Dimensions: dimensions, Vectors: vectors}
}

func deterministicBenchVector(seed, dimensions int) []float32 {
	if dimensions <= 0 {
		dimensions = 1
	}
	vec := make([]float32, dimensions)
	state := uint64(seed)*6364136223846793005 + 1442695040888963407
	for i := 0; i < dimensions; i++ {
		state = state*2862933555777941757 + 3037000493
		v := float32((state>>33)%2000) / 1000.0
		vec[i] = v + 0.001
	}
	return vec
}

func hashSeed(text string, salt int) int {
	h := uint32(2166136261)
	for i := 0; i < len(text); i++ {
		h ^= uint32(text[i])
		h *= 16777619
	}
	return int(h) + salt*97
}

func benchFileCount(recordCount int) int {
	if recordCount <= 0 {
		return 1
	}
	fileCount := recordCount / 4
	if fileCount < 1 {
		fileCount = 1
	}
	return fileCount
}
