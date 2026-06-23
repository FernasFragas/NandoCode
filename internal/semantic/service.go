package semantic

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type LocalService struct {
	store    Store
	embedder Embedder
	cacheMu  sync.RWMutex
	cache    map[string]loadedIndexCacheEntry
}

type loadedIndexCacheEntry struct {
	key     string
	records []Record
	vectors VectorSet
}

func NewLocalService(store Store, embedder Embedder) *LocalService {
	return &LocalService{
		store:    store,
		embedder: embedder,
		cache:    map[string]loadedIndexCacheEntry{},
	}
}

func (s *LocalService) Status(ctx context.Context, root string) (Status, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return Status{}, err
	}
	st := Status{
		Root:    canonical,
		Enabled: true,
	}
	manifest, err := s.store.LoadManifest(ctx, canonical)
	if err != nil {
		if errors.Is(err, ErrIndexMissing) {
			st.Exists = false
			st.Compatible = false
			return st, nil
		}
		return st, err
	}
	records, err := s.store.LoadRecords(ctx, manifest)
	if err != nil {
		return st, err
	}
	vectors, err := s.store.LoadVectors(ctx, manifest)
	if err != nil {
		return st, err
	}
	st.Exists = true
	st.Compatible = manifest.SchemaVersion == SchemaVersion
	st.Model = manifest.Model
	st.Dimensions = manifest.Dimensions
	st.SchemaVersion = manifest.SchemaVersion
	st.FileCount = manifest.FileCount
	st.RecordCount = len(records)
	st.VectorCount = len(vectors.Vectors)
	st.UpdatedAt = manifest.UpdatedAt
	if ls, ok := s.store.(*LocalStore); ok {
		st.IndexPath = ls.workspaceDir(manifest)
	}
	return st, nil
}

func (s *LocalService) Build(ctx context.Context, req BuildRequest) (BuildReport, error) {
	start := time.Now()
	cfg, err := s.resolveConfig(req.Config)
	if err != nil {
		return BuildReport{}, err
	}
	root, err := CanonicalRoot(req.Root)
	if err != nil {
		return BuildReport{}, err
	}
	if !cfg.Enabled {
		return BuildReport{}, ErrDisabled
	}
	if s.embedder == nil || s.store == nil {
		return BuildReport{}, fmt.Errorf("semantic service not initialized")
	}

	scanStarted := false
	scan, err := ScanWorkspace(ctx, ScanOptions{
		Root:            root,
		MaxFileBytes:    cfg.MaxFileBytes,
		MaxChunkBytes:   cfg.MaxChunkTokens * 4,
		ChunkOverlap:    maxInt(1, cfg.ChunkOverlapTokens/32),
		SecretScanBytes: 8 * 1024,
		DirwalkMaxFiles: cfg.MaxRecords,
		DirwalkMaxDepth: 16,
		OnProgress: func(progress ScanProgress) {
			if !scanStarted {
				scanStarted = true
				s.publish(req.EventSink, Event{
					Stage:      StageScanStart,
					Root:       root,
					FilesTotal: progress.FilesTotal,
					Message:    "scan start",
				})
			}
			if progress.FilesSeen == 0 {
				return
			}
			s.publish(req.EventSink, Event{
				Stage:        StageScanProgress,
				Root:         root,
				FilesTotal:   progress.FilesTotal,
				FilesSeen:    progress.FilesSeen,
				FilesIndexed: progress.FilesIndexed,
				FilesSkipped: progress.FilesSkipped,
				RecordsDone:  progress.RecordsDone,
				Message:      "scan progress",
			})
		},
	})
	if err != nil {
		return BuildReport{}, err
	}
	if !scanStarted {
		s.publish(req.EventSink, Event{
			Stage:      StageScanStart,
			Root:       root,
			FilesTotal: scan.FilesTotal,
			Message:    "scan start",
		})
	}
	s.publish(req.EventSink, Event{
		Stage:        StageExtractProgress,
		Root:         root,
		FilesTotal:   scan.FilesTotal,
		FilesSeen:    scan.FilesSeen,
		FilesDone:    scan.FilesIndexed,
		FilesIndexed: scan.FilesIndexed,
		FilesSkipped: len(scan.Skipped),
		RecordsDone:  len(scan.Records),
		RecordsTotal: len(scan.Records),
		Message:      "scan complete",
	})

	records := scan.Records
	if cfg.MaxRecords > 0 && len(records) > cfg.MaxRecords {
		records = records[:cfg.MaxRecords]
	}
	vectors, batches, err := s.embedRecords(ctx, cfg, records, req.EventSink, root, cfg.BuildKeepAlive)
	if err != nil {
		if looksLikeMissingModel(err) {
			return BuildReport{}, fmt.Errorf("%w: %v", ErrModelMissing, err)
		}
		return BuildReport{}, err
	}

	fileCount := uniqueFileCount(records)
	workspaceID, err := WorkspaceID(root, cfg.Model, vectors.Dimensions, SchemaVersion)
	if err != nil {
		return BuildReport{}, err
	}
	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		WorkspaceRoot: root,
		WorkspaceID:   workspaceID,
		Model:         cfg.Model,
		Dimensions:    vectors.Dimensions,
		FileCount:     fileCount,
		StorePreviews: cfg.StorePreviews,
	}
	s.publish(req.EventSink, Event{Stage: StageWriteStart, Root: root, Message: "write index"})
	if err := s.store.Replace(ctx, manifest, records, vectors); err != nil {
		return BuildReport{}, err
	}
	s.invalidateIndexCache(root)
	s.publish(req.EventSink, Event{Stage: StageWriteDone, Root: root, Message: "write complete"})

	report := BuildReport{
		Root:           root,
		Model:          cfg.Model,
		Dimensions:     vectors.Dimensions,
		FilesSeen:      scan.FilesSeen,
		FilesIndexed:   scan.FilesIndexed,
		FilesSkipped:   len(scan.Skipped),
		RecordsIndexed: len(records),
		EmbedBatches:   batches,
		Duration:       time.Since(start),
		Skipped:        scan.Skipped,
	}
	if ls, ok := s.store.(*LocalStore); ok {
		report.IndexPath = ls.workspaceDir(manifest)
	}
	return report, nil
}

func (s *LocalService) Refresh(ctx context.Context, req RefreshRequest) (BuildReport, error) {
	start := time.Now()
	cfg, err := s.resolveConfig(req.Config)
	if err != nil {
		return BuildReport{}, err
	}
	root, err := CanonicalRoot(req.Root)
	if err != nil {
		return BuildReport{}, err
	}
	if !cfg.Enabled {
		return BuildReport{}, ErrDisabled
	}
	manifest, err := s.store.LoadManifest(ctx, root)
	if err != nil {
		if errors.Is(err, ErrIndexMissing) {
			s.publish(req.EventSink, Event{
				Stage:   StageScanStart,
				Root:    root,
				Message: "refresh falling back to full build",
			})
			return s.Build(ctx, BuildRequest{Root: root, Config: cfg, EventSink: req.EventSink})
		}
		return BuildReport{}, err
	}
	oldRecords, err := s.store.LoadRecords(ctx, manifest)
	if err != nil {
		return BuildReport{}, err
	}
	oldVectors, err := s.store.LoadVectors(ctx, manifest)
	if err != nil {
		return BuildReport{}, err
	}

	scanStarted := false
	scan, err := ScanWorkspace(ctx, ScanOptions{
		Root:            root,
		MaxFileBytes:    cfg.MaxFileBytes,
		MaxChunkBytes:   cfg.MaxChunkTokens * 4,
		ChunkOverlap:    maxInt(1, cfg.ChunkOverlapTokens/32),
		SecretScanBytes: 8 * 1024,
		DirwalkMaxFiles: cfg.MaxRecords,
		DirwalkMaxDepth: 16,
		OnProgress: func(progress ScanProgress) {
			if !scanStarted {
				scanStarted = true
				s.publish(req.EventSink, Event{
					Stage:      StageScanStart,
					Root:       root,
					FilesTotal: progress.FilesTotal,
					Message:    "scan start",
				})
			}
			if progress.FilesSeen == 0 {
				return
			}
			s.publish(req.EventSink, Event{
				Stage:        StageScanProgress,
				Root:         root,
				FilesTotal:   progress.FilesTotal,
				FilesSeen:    progress.FilesSeen,
				FilesIndexed: progress.FilesIndexed,
				FilesSkipped: progress.FilesSkipped,
				RecordsDone:  progress.RecordsDone,
				Message:      "scan progress",
			})
		},
	})
	if err != nil {
		return BuildReport{}, err
	}
	if !scanStarted {
		s.publish(req.EventSink, Event{
			Stage:      StageScanStart,
			Root:       root,
			FilesTotal: scan.FilesTotal,
			Message:    "scan start",
		})
	}
	s.publish(req.EventSink, Event{
		Stage:        StageExtractProgress,
		Root:         root,
		FilesTotal:   scan.FilesTotal,
		FilesSeen:    scan.FilesSeen,
		FilesDone:    scan.FilesIndexed,
		FilesIndexed: scan.FilesIndexed,
		FilesSkipped: len(scan.Skipped),
		RecordsDone:  len(scan.Records),
		RecordsTotal: len(scan.Records),
		Message:      "scan complete",
	})

	// Identify changed/new file paths by comparing file-record content hashes.
	oldFileHash := fileHashesByPath(oldRecords)
	newFileHash := fileHashesByPath(scan.Records)
	changed := map[string]struct{}{}
	for p, h := range newFileHash {
		if old, ok := oldFileHash[p]; !ok || old != h {
			changed[p] = struct{}{}
		}
	}
	for p := range oldFileHash {
		if _, ok := newFileHash[p]; !ok {
			changed[p] = struct{}{}
		}
	}

	keepVecByID := map[string][]float32{}
	keepRecByID := map[string]Record{}
	for i := range oldRecords {
		rec := oldRecords[i]
		if _, isChanged := changed[rec.Path]; isChanged {
			continue
		}
		if i >= len(oldVectors.Vectors) {
			continue
		}
		keepRecByID[rec.ID] = rec
		keepVecByID[rec.ID] = oldVectors.Vectors[i]
	}

	newRecords := make([]Record, 0, len(scan.Records))
	for _, rec := range scan.Records {
		if _, isChanged := changed[rec.Path]; isChanged {
			newRecords = append(newRecords, rec)
		}
	}
	newVectors, batches, err := s.embedRecords(ctx, cfg, newRecords, req.EventSink, root, cfg.BuildKeepAlive)
	if err != nil {
		if looksLikeMissingModel(err) {
			return BuildReport{}, fmt.Errorf("%w: %v", ErrModelMissing, err)
		}
		return BuildReport{}, err
	}
	if len(newRecords) > 0 && newVectors.Dimensions != manifest.Dimensions {
		s.publish(req.EventSink, Event{
			Stage:   StageScanStart,
			Root:    root,
			Message: "refresh falling back to full build",
		})
		return s.Build(ctx, BuildRequest{Root: root, Config: cfg, EventSink: req.EventSink})
	}

	combined := make([]Record, 0, len(keepRecByID)+len(newRecords))
	for _, rec := range keepRecByID {
		combined = append(combined, rec)
	}
	combined = append(combined, newRecords...)
	sortRecords(combined)

	vecByID := map[string][]float32{}
	for id, vec := range keepVecByID {
		vecByID[id] = vec
	}
	for i := range newRecords {
		if i < len(newVectors.Vectors) {
			vecByID[newRecords[i].ID] = newVectors.Vectors[i]
		}
	}
	orderedVectors := make([][]float32, 0, len(combined))
	filtered := make([]Record, 0, len(combined))
	for _, rec := range combined {
		vec, ok := vecByID[rec.ID]
		if !ok {
			continue
		}
		filtered = append(filtered, rec)
		orderedVectors = append(orderedVectors, vec)
	}
	combined = filtered

	manifest.Model = cfg.Model
	manifest.Dimensions = orderedDimensions(manifest.Dimensions, orderedVectors)
	manifest.StorePreviews = cfg.StorePreviews
	manifest.RecordCount = len(combined)
	manifest.FileCount = uniqueFileCount(combined)
	s.publish(req.EventSink, Event{Stage: StageWriteStart, Root: root, Message: "write index"})
	if err := s.store.Replace(ctx, manifest, combined, VectorSet{
		Dimensions: manifest.Dimensions,
		Vectors:    orderedVectors,
	}); err != nil {
		return BuildReport{}, err
	}
	s.invalidateIndexCache(root)
	s.publish(req.EventSink, Event{Stage: StageWriteDone, Root: root, Message: "write complete"})

	report := BuildReport{
		Root:           root,
		Model:          cfg.Model,
		Dimensions:     manifest.Dimensions,
		FilesSeen:      scan.FilesSeen,
		FilesIndexed:   scan.FilesIndexed,
		FilesSkipped:   len(scan.Skipped),
		RecordsIndexed: len(combined),
		EmbedBatches:   batches,
		Duration:       time.Since(start),
		Skipped:        scan.Skipped,
	}
	if ls, ok := s.store.(*LocalStore); ok {
		report.IndexPath = ls.workspaceDir(manifest)
	}
	return report, nil
}

func (s *LocalService) Clear(ctx context.Context, root string) error {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return err
	}
	if err := s.store.Clear(ctx, canonical); err != nil {
		return err
	}
	s.invalidateIndexCache(canonical)
	return nil
}

func (s *LocalService) Retrieve(ctx context.Context, req RetrieveRequest) (RetrieveResult, error) {
	retrieveCtx := ctx
	cancel := func() {}
	if req.Deadline > 0 {
		retrieveCtx, cancel = context.WithTimeout(ctx, req.Deadline)
	}
	defer cancel()

	totalStart := time.Now()
	defer func() {
		req.Observer.Observe(RetrieveStageEvent{
			Stage:    RetrieveStageTotal,
			Duration: time.Since(totalStart),
		})
	}()

	root, err := CanonicalRoot(req.Root)
	if err != nil {
		return RetrieveResult{}, err
	}
	if s.store == nil || s.embedder == nil {
		return RetrieveResult{}, fmt.Errorf("semantic service not initialized")
	}
	cfg := DefaultConfig()
	if req.Config != (Config{}) {
		cfg, _ = NormalizeConfig(req.Config)
	}

	manifestStart := time.Now()
	manifest, err := s.store.LoadManifest(retrieveCtx, root)
	req.Observer.Observe(RetrieveStageEvent{
		Stage:    RetrieveStageManifest,
		Duration: time.Since(manifestStart),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(retrieveCtx.Err(), context.DeadlineExceeded) {
			return RetrieveResult{}, ErrDeadline
		}
		if errors.Is(err, ErrIndexMissing) {
			return RetrieveResult{}, ErrIndexMissing
		}
		return RetrieveResult{}, err
	}
	if manifest.SchemaVersion != SchemaVersion {
		return RetrieveResult{}, ErrSchemaMismatch
	}

	records, vectors, _, err := s.loadIndexData(retrieveCtx, root, manifest, req.Observer)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(retrieveCtx.Err(), context.DeadlineExceeded) {
			return RetrieveResult{}, ErrDeadline
		}
		return RetrieveResult{}, err
	}
	if len(records) == 0 || len(vectors.Vectors) == 0 {
		return RetrieveResult{}, ErrIndexMissing
	}
	queryKeepAlive := strings.TrimSpace(cfg.QueryKeepAlive)
	if queryKeepAlive == "" {
		queryKeepAlive = strings.TrimSpace(cfg.KeepAlive)
	}
	if queryKeepAlive == "" {
		queryKeepAlive = DefaultQueryKeepAlive
	}
	embedStart := time.Now()
	embedRes, err := s.embedder.Embed(retrieveCtx, EmbedRequest{
		Model:      manifest.Model,
		Input:      []string{strings.TrimSpace(req.Query)},
		Dimensions: manifest.Dimensions,
		KeepAlive:  queryKeepAlive,
	})
	req.Observer.Observe(RetrieveStageEvent{
		Stage:    RetrieveStageEmbed,
		Duration: time.Since(embedStart),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(retrieveCtx.Err(), context.DeadlineExceeded) {
			return RetrieveResult{}, ErrDeadline
		}
		if looksLikeMissingModel(err) {
			return RetrieveResult{}, fmt.Errorf("%w: %v", ErrModelMissing, err)
		}
		return RetrieveResult{}, err
	}
	if len(embedRes.Vectors) == 0 {
		return RetrieveResult{}, ErrIndexMissing
	}
	queryVec := embedRes.Vectors[0]
	if len(queryVec) != manifest.Dimensions {
		return RetrieveResult{}, fmt.Errorf("%w: got %d want %d", ErrDimensionsMismatch, len(queryVec), manifest.Dimensions)
	}
	if !NormalizeVector(queryVec) {
		return RetrieveResult{}, fmt.Errorf("semantic query embedding had zero norm")
	}

	scoreStart := time.Now()
	hits, err := scoreRecords(req.Query, queryVec, records, vectors, cfg, req.ExplicitPaths, req.CurrentTurnPaths, req.UseCurrentPathWeight)
	req.Observer.Observe(RetrieveStageEvent{
		Stage:    RetrieveStageScore,
		Duration: time.Since(scoreStart),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(retrieveCtx.Err(), context.DeadlineExceeded) {
			return RetrieveResult{}, ErrDeadline
		}
		return RetrieveResult{}, err
	}
	maxRecords := req.MaxRecords
	if maxRecords <= 0 {
		maxRecords = cfg.TopKRecords
	}
	maxFiles := req.MaxFiles
	if maxFiles <= 0 {
		maxFiles = cfg.TopKFiles
	}
	top := diversifyHits(hits, maxFiles, 4, maxRecords)
	maxContextBytes := req.MaxContextBytes
	if maxContextBytes <= 0 {
		maxContextBytes = cfg.MaxContextBytes
	}
	renderStart := time.Now()
	rendered, ctxBytes, staleDropped, keptHits, warnings := renderRetrievedContext(root, top, maxContextBytes)
	req.Observer.Observe(RetrieveStageEvent{
		Stage:    RetrieveStageRender,
		Duration: time.Since(renderStart),
	})
	grouped := groupByFile(keptHits)

	if len(keptHits) == 0 {
		return RetrieveResult{
			Used:           false,
			FallbackReason: "no semantic hits after stale/read filtering",
			Warnings:       warnings,
			StaleDropped:   staleDropped,
		}, nil
	}

	return RetrieveResult{
		Used:            true,
		Records:         keptHits,
		Files:           grouped,
		RenderedContext: rendered,
		ContextBytes:    ctxBytes,
		StaleDropped:    staleDropped,
		Warnings:        warnings,
	}, nil
}

func (s *LocalService) loadIndexData(ctx context.Context, root string, manifest Manifest, observer RetrieveObserverFunc) ([]Record, VectorSet, bool, error) {
	cacheKey := manifestCacheKey(manifest)
	if records, vectors, ok := s.cachedIndex(root, cacheKey); ok {
		observer.Observe(RetrieveStageEvent{Stage: RetrieveStageRecords, CacheHit: true})
		observer.Observe(RetrieveStageEvent{Stage: RetrieveStageVectors, CacheHit: true})
		return records, vectors, true, nil
	}

	recordsStart := time.Now()
	records, err := s.store.LoadRecords(ctx, manifest)
	observer.Observe(RetrieveStageEvent{
		Stage:    RetrieveStageRecords,
		Duration: time.Since(recordsStart),
		CacheHit: false,
	})
	if err != nil {
		return nil, VectorSet{}, false, err
	}

	vectorsStart := time.Now()
	vectors, err := s.store.LoadVectors(ctx, manifest)
	observer.Observe(RetrieveStageEvent{
		Stage:    RetrieveStageVectors,
		Duration: time.Since(vectorsStart),
		CacheHit: false,
	})
	if err != nil {
		return nil, VectorSet{}, false, err
	}

	s.putCachedIndex(root, cacheKey, records, vectors)
	return records, vectors, false, nil
}

func (s *LocalService) resolveConfig(cfg Config) (Config, error) {
	out, _ := NormalizeConfig(cfg)
	if err := ValidateConfig(out); err != nil {
		return Config{}, err
	}
	return out, nil
}

func (s *LocalService) embedRecords(ctx context.Context, cfg Config, records []Record, sink EventSink, root, keepAlive string) (VectorSet, int, error) {
	inputs := make([]string, 0, len(records))
	for _, rec := range records {
		in := strings.TrimSpace(rec.EmbedText)
		if in == "" {
			in = strings.TrimSpace(rec.TextPreview)
		}
		if in == "" {
			in = strings.TrimSpace(rec.Path)
		}
		inputs = append(inputs, in)
	}
	if len(inputs) == 0 {
		return VectorSet{Dimensions: cfg.Dimensions, Vectors: [][]float32{}}, 0, nil
	}
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	embedKeepAlive := strings.TrimSpace(keepAlive)
	if embedKeepAlive == "" {
		embedKeepAlive = strings.TrimSpace(cfg.BuildKeepAlive)
	}
	if embedKeepAlive == "" {
		embedKeepAlive = strings.TrimSpace(cfg.KeepAlive)
	}
	if embedKeepAlive == "" {
		embedKeepAlive = DefaultBuildKeepAlive
	}
	vectors := make([][]float32, 0, len(inputs))
	batches := 0
	actualDimensions := 0
	truncate := false
	for start := 0; start < len(inputs); start += batchSize {
		end := start + batchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		out, err := s.embedder.Embed(ctx, EmbedRequest{
			Model:      cfg.Model,
			Input:      inputs[start:end],
			Dimensions: cfg.Dimensions,
			Truncate:   &truncate,
			KeepAlive:  embedKeepAlive,
		})
		if err != nil {
			return VectorSet{}, batches, err
		}
		for _, vec := range out.Vectors {
			if actualDimensions == 0 {
				actualDimensions = len(vec)
			}
			if len(vec) != actualDimensions {
				return VectorSet{}, batches, fmt.Errorf("semantic embed dimensions changed within build got %d want %d", len(vec), actualDimensions)
			}
			NormalizeVector(vec)
			vectors = append(vectors, vec)
		}
		batches++
		s.publish(sink, Event{
			Stage:        StageEmbedProgress,
			Root:         root,
			BatchesDone:  batches,
			TotalBatches: (len(inputs) + batchSize - 1) / batchSize,
			RecordsDone:  len(vectors),
			RecordsTotal: len(inputs),
		})
	}
	if actualDimensions == 0 {
		actualDimensions = cfg.Dimensions
	}
	return VectorSet{Dimensions: actualDimensions, Vectors: vectors}, batches, nil
}

func (s *LocalService) publish(sink EventSink, evt Event) {
	if sink == nil {
		return
	}
	sink.Publish(evt)
}

func (s *LocalService) cachedIndex(root, key string) ([]Record, VectorSet, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	entry, ok := s.cache[root]
	if !ok || entry.key != key {
		return nil, VectorSet{}, false
	}
	return entry.records, entry.vectors, true
}

func (s *LocalService) putCachedIndex(root, key string, records []Record, vectors VectorSet) {
	entry := loadedIndexCacheEntry{
		key:     key,
		records: append([]Record(nil), records...),
		vectors: VectorSet{
			Dimensions: vectors.Dimensions,
			Vectors:    append([][]float32(nil), vectors.Vectors...),
		},
	}
	s.cacheMu.Lock()
	s.cache[root] = entry
	s.cacheMu.Unlock()
}

func (s *LocalService) invalidateIndexCache(root string) {
	s.cacheMu.Lock()
	delete(s.cache, root)
	s.cacheMu.Unlock()
}

func manifestCacheKey(manifest Manifest) string {
	return fmt.Sprintf(
		"%d|%s|%s|%d|%d|%d|%d",
		manifest.SchemaVersion,
		manifest.WorkspaceID,
		manifest.Model,
		manifest.Dimensions,
		manifest.RecordCount,
		manifest.FileCount,
		manifest.UpdatedAt.UnixNano(),
	)
}

func orderedDimensions(fallback int, vectors [][]float32) int {
	for _, vec := range vectors {
		if len(vec) > 0 {
			return len(vec)
		}
	}
	return fallback
}

func uniqueFileCount(records []Record) int {
	set := map[string]struct{}{}
	for _, rec := range records {
		if rec.Kind == RecordKindFile || rec.Kind == RecordKindSymbol || rec.Kind == RecordKindDocSection || rec.Kind == RecordKindChunk {
			set[rec.Path] = struct{}{}
		}
	}
	return len(set)
}

func fileHashesByPath(records []Record) map[string]string {
	out := map[string]string{}
	for _, rec := range records {
		if rec.Kind != RecordKindFile {
			continue
		}
		if rec.Path == "" || rec.ContentHash == "" {
			continue
		}
		out[rec.Path] = rec.ContentHash
	}
	return out
}

func groupByFile(hits []SearchHit) []RetrievedFile {
	if len(hits) == 0 {
		return nil
	}
	m := map[string]*RetrievedFile{}
	order := make([]string, 0, len(hits))
	for _, h := range hits {
		rf, ok := m[h.Record.Path]
		if !ok {
			order = append(order, h.Record.Path)
			rf = &RetrievedFile{Path: h.Record.Path, Score: h.Score}
			m[h.Record.Path] = rf
		}
		if h.Score > rf.Score {
			rf.Score = h.Score
		}
		rf.Records = append(rf.Records, h)
	}
	out := make([]RetrievedFile, 0, len(order))
	for _, path := range order {
		out = append(out, *m[path])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func looksLikeMissingModel(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "model") && (strings.Contains(s, "not found") || strings.Contains(s, "pull"))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
