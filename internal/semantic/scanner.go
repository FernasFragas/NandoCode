package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FernasFragas/nandocodego/internal/tools/dirwalk"
)

type ScanOptions struct {
	Root             string
	MaxFileBytes     int64
	MaxChunkBytes    int
	ChunkOverlap     int
	SecretScanBytes  int
	DirwalkMaxFiles  int
	DirwalkMaxBytes  int64
	DirwalkMaxDepth  int
	FollowSymlinks   bool
	IncludeGitignore bool
	OnProgress       func(ScanProgress)
	ProgressEvery    int
	ProgressInterval time.Duration
}

type ScanResult struct {
	Root         string
	Records      []Record
	Skipped      []SkippedFile
	FilesTotal   int
	FilesSeen    int
	FilesIndexed int
}

type ScanProgress struct {
	Root         string
	FilesTotal   int
	FilesSeen    int
	FilesIndexed int
	FilesSkipped int
	RecordsDone  int
}

func ScanWorkspace(ctx context.Context, opts ScanOptions) (ScanResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(opts.Root) == "" {
		return ScanResult{}, fmt.Errorf("scan root is required")
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return ScanResult{}, err
	}
	root = filepath.Clean(root)

	walked, _, err := dirwalk.Walk(ctx, root, dirwalk.Options{
		Excludes:       semanticWalkExcludes(),
		MaxFiles:       opts.DirwalkMaxFiles,
		MaxBytes:       opts.DirwalkMaxBytes,
		MaxDepth:       opts.DirwalkMaxDepth,
		FollowSymlinks: opts.FollowSymlinks,
		Source:         dirwalk.SourceAuto,
		DetectIgnored:  opts.IncludeGitignore,
	})
	if err != nil {
		return ScanResult{}, err
	}

	filter := NewFileFilter(FilterOptions{
		MaxFileBytes:    opts.MaxFileBytes,
		SecretScanBytes: opts.SecretScanBytes,
	})

	skipped := make([]SkippedFile, 0, 64)
	records := make([]Record, 0, 1024)
	includedPaths := make([]string, 0, len(walked))
	type fileTask struct {
		idx     int
		absPath string
		rel     string
	}
	type fileOutcome struct {
		idx      int
		rel      string
		records  []Record
		included bool
		skipped  *SkippedFile
	}
	tasks := make([]fileTask, 0, len(walked))
	filesTotal := 0
	filesSeen := 0
	filesIndexed := 0
	filesSkipped := 0
	for _, e := range walked {
		if e.IsDir {
			continue
		}
		filesTotal++
	}
	progressEvery := opts.ProgressEvery
	if progressEvery <= 0 {
		progressEvery = 64
	}
	progressInterval := opts.ProgressInterval
	if progressInterval <= 0 {
		progressInterval = 250 * time.Millisecond
	}
	lastProgressAt := time.Now()
	lastProgressSeen := -1
	lastProgressIndexed := -1
	lastProgressSkipped := -1
	lastProgressRecords := -1
	emitProgress := func(force bool) {
		if opts.OnProgress == nil {
			return
		}
		if !force {
			if filesSeen == lastProgressSeen &&
				filesIndexed == lastProgressIndexed &&
				filesSkipped == lastProgressSkipped &&
				len(records) == lastProgressRecords {
				return
			}
			if filesSeen%progressEvery != 0 && time.Since(lastProgressAt) < progressInterval {
				return
			}
		}
		opts.OnProgress(ScanProgress{
			Root:         root,
			FilesTotal:   filesTotal,
			FilesSeen:    filesSeen,
			FilesIndexed: filesIndexed,
			FilesSkipped: filesSkipped,
			RecordsDone:  len(records),
		})
		lastProgressSeen = filesSeen
		lastProgressIndexed = filesIndexed
		lastProgressSkipped = filesSkipped
		lastProgressRecords = len(records)
		lastProgressAt = time.Now()
	}

	emitProgress(true)

	for _, e := range walked {
		select {
		case <-ctx.Done():
			return ScanResult{}, ctx.Err()
		default:
		}
		if e.IsDir {
			continue
		}
		filesSeen++
		rel := normalizeRelPath(e.RelPath)
		if e.Skipped {
			skipped = append(skipped, SkippedFile{
				Path:   rel,
				Reason: e.SkipReason,
			})
			filesSkipped++
			emitProgress(false)
			continue
		}

		if d := filter.ShouldSkipPath(rel, e.SizeBytes); d.Skip {
			skipped = append(skipped, SkippedFile{
				Path:   rel,
				Reason: string(d.Reason),
			})
			filesSkipped++
			emitProgress(false)
			continue
		}

		tasks = append(tasks, fileTask{
			idx:     len(tasks),
			absPath: e.AbsPath,
			rel:     rel,
		})
		emitProgress(false)
	}

	if len(tasks) > 0 {
		workerCount := runtime.GOMAXPROCS(0)
		if workerCount <= 0 {
			workerCount = 1
		}
		if workerCount > len(tasks) {
			workerCount = len(tasks)
		}
		if workerCount > 16 {
			workerCount = 16
		}

		jobs := make(chan fileTask, workerCount)
		outcomes := make(chan fileOutcome, workerCount)
		var workers sync.WaitGroup

		processTask := func(task fileTask) fileOutcome {
			body, readErr := os.ReadFile(task.absPath)
			if readErr != nil {
				return fileOutcome{
					idx: task.idx,
					skipped: &SkippedFile{
						Path:   task.rel,
						Reason: string(SkipReasonReadError),
					},
				}
			}
			if d := filter.ShouldSkipContent(task.rel, body); d.Skip {
				return fileOutcome{
					idx: task.idx,
					skipped: &SkippedFile{
						Path:   task.rel,
						Reason: string(d.Reason),
					},
				}
			}

			contentHash := hashBytes(body)
			lang := languageForPath(task.rel)
			lineCount := strings.Count(string(body), "\n") + 1
			fileText := fmt.Sprintf("file %s (%s)\n%s", task.rel, lang, snippetForRange(string(body), 1, lineCount))
			fileRecord := makeRecord(
				RecordKindFile,
				task.rel,
				lang,
				filepath.Base(task.rel),
				filepath.Dir(task.rel),
				1,
				lineCount,
				contentHash,
				fileText,
			)

			extracted := make([]Record, 0, 16)
			switch lang {
			case "go":
				symbols, _ := extractGoSymbolRecords(task.rel, body, contentHash)
				extracted = append(extracted, symbols...)
				if len(extracted) == 0 {
					extracted = append(extracted, chunkFallbackRecords(task.rel, lang, body, contentHash, opts.MaxChunkBytes, opts.ChunkOverlap)...)
				}
			case "markdown":
				docs := extractMarkdownSectionRecords(task.rel, body, contentHash)
				extracted = append(extracted, docs...)
				if len(extracted) == 0 {
					extracted = append(extracted, chunkFallbackRecords(task.rel, lang, body, contentHash, opts.MaxChunkBytes, opts.ChunkOverlap)...)
				}
			default:
				extracted = append(extracted, chunkFallbackRecords(task.rel, lang, body, contentHash, opts.MaxChunkBytes, opts.ChunkOverlap)...)
			}

			rec := make([]Record, 0, 1+len(extracted))
			rec = append(rec, fileRecord)
			rec = append(rec, extracted...)
			return fileOutcome{
				idx:      task.idx,
				rel:      task.rel,
				records:  rec,
				included: true,
			}
		}

		for i := 0; i < workerCount; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for task := range jobs {
					select {
					case <-ctx.Done():
						return
					default:
					}
					outcome := processTask(task)
					select {
					case outcomes <- outcome:
					case <-ctx.Done():
						return
					}
				}
			}()
		}

		go func() {
			for _, task := range tasks {
				select {
				case <-ctx.Done():
					break
				case jobs <- task:
				}
			}
			close(jobs)
		}()

		results := make([]fileOutcome, len(tasks))
		received := 0
		for received < len(tasks) {
			select {
			case <-ctx.Done():
				return ScanResult{}, ctx.Err()
			case out := <-outcomes:
				results[out.idx] = out
				if out.skipped != nil {
					filesSkipped++
				}
				if out.included {
					filesIndexed++
					records = append(records, out.records...)
				}
				received++
				emitProgress(false)
			}
		}
		workers.Wait()

		for _, out := range results {
			if out.included {
				includedPaths = append(includedPaths, out.rel)
				continue
			}
			if out.skipped != nil {
				skipped = append(skipped, *out.skipped)
			}
		}
	}

	records = append(records, folderRecordsForPaths(includedPaths)...)
	emitProgress(true)
	sortRecords(records)
	sort.Slice(skipped, func(i, j int) bool {
		if skipped[i].Path != skipped[j].Path {
			return skipped[i].Path < skipped[j].Path
		}
		return skipped[i].Reason < skipped[j].Reason
	})

	return ScanResult{
		Root:         root,
		Records:      records,
		Skipped:      skipped,
		FilesTotal:   filesTotal,
		FilesSeen:    filesSeen,
		FilesIndexed: filesIndexed,
	}, nil
}

func semanticWalkExcludes() []string {
	base := dirwalk.DefaultExcludes()
	keepForClassification := map[string]struct{}{
		"vendor":       {},
		"node_modules": {},
		"dist":         {},
		"build":        {},
		"out":          {},
		"target":       {},
		"coverage":     {},
		".next":        {},
	}
	out := make([]string, 0, len(base))
	for _, name := range base {
		if _, ok := keepForClassification[name]; ok {
			continue
		}
		out = append(out, name)
	}
	return out
}

func languageForPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".md", ".markdown", ".mdx":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".txt":
		return "text"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".js":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	case ".sh":
		return "shell"
	case ".sql":
		return "sql"
	default:
		return "text"
	}
}

func folderRecordsForPaths(paths []string) []Record {
	if len(paths) == 0 {
		return nil
	}
	children := map[string][]string{}
	for _, rel := range paths {
		rel = normalizeRelPath(rel)
		if rel == "" {
			continue
		}
		parts := strings.Split(rel, "/")
		for i := 0; i < len(parts)-1; i++ {
			parent := strings.Join(parts[:i+1], "/")
			child := parts[i+1]
			children[parent] = append(children[parent], child)
		}
	}

	out := make([]Record, 0, len(children))
	for folder, kids := range children {
		sort.Strings(kids)
		uniq := make([]string, 0, len(kids))
		last := ""
		for _, k := range kids {
			if k == last {
				continue
			}
			last = k
			uniq = append(uniq, k)
		}
		previewKids := uniq
		if len(previewKids) > 20 {
			previewKids = previewKids[:20]
		}
		body := fmt.Sprintf("folder %s contains: %s", folder, strings.Join(previewKids, ", "))
		out = append(out, makeRecord(
			RecordKindFolder,
			folder,
			"",
			filepath.Base(folder),
			filepath.Dir(folder),
			0,
			0,
			hashText(strings.Join(uniq, "\n")),
			body,
		))
	}
	return out
}
