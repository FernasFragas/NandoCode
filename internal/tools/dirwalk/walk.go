package dirwalk

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ReasonFileCap  = "file-cap"
	ReasonByteCap  = "byte-cap"
	ReasonDepthCap = "depth-cap"
)

var defaultExcludes = []string{
	".git",
	".svn",
	"node_modules",
	"vendor",
	"dist",
	".gocache",
	".tmp-config",
	".next",
	"target",
	"build",
	"out",
	"coverage",
}

type Options struct {
	Excludes       []string
	MaxFiles       int
	MaxBytes       int64
	MaxDepth       int
	FollowSymlinks bool
	Source         SourceMode
	DetectIgnored  bool
}

type SourceMode string

const (
	SourceAuto       SourceMode = "auto"
	SourceGit        SourceMode = "git"
	SourceFilesystem SourceMode = "filesystem"
)

type Entry struct {
	RelPath    string
	AbsPath    string
	IsDir      bool
	Skipped    bool
	SkipReason string
	SizeBytes  int64
}

type Stats struct {
	FileCount             int
	DirCount              int
	ByteCount             int64
	Truncated             bool
	Reason                string
	Source                string
	IgnoredByGit          int
	TotalFilesystemFiles  int
	TotalFilesystemDirs   int
}

func DefaultExcludes() []string {
	out := make([]string, len(defaultExcludes))
	copy(out, defaultExcludes)
	return out
}

func Walk(ctx context.Context, root string, opts Options) ([]Entry, Stats, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, Stats{}, err
	}
	absRoot = filepath.Clean(absRoot)

	excludes := opts.Excludes
	if len(excludes) == 0 {
		excludes = DefaultExcludes()
	}
	excluded := make(map[string]struct{}, len(excludes))
	for _, name := range excludes {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		excluded[name] = struct{}{}
	}

	w := &walker{
		root:           absRoot,
		opts:           opts,
		excludedDir:    excluded,
		entriesByRel:   make(map[string]Entry, 4096),
		files:          0,
		bytes:          0,
		truncated:      false,
		truncatedCause: "",
	}

	source := opts.Source
	if source == "" {
		source = SourceAuto
	}
	switch source {
	case SourceFilesystem:
		if err := w.populateViaWalk(); err != nil {
			return nil, Stats{}, err
		}
		w.source = "filesystem"
	default:
		if err := w.populateViaGit(ctx); err != nil {
			if walkErr := w.populateViaWalk(); walkErr != nil {
				return nil, Stats{}, walkErr
			}
			w.source = "filesystem"
		} else {
			w.source = "git"
		}
	}
	if w.source == "git" && opts.DetectIgnored {
		fsFiles, fsDirs, _ := w.scanFilesystemCounts()
		w.totalFilesystemFiles = fsFiles
		w.totalFilesystemDirs = fsDirs
		if fsFiles > w.files {
			w.ignoredByGit = fsFiles - w.files
		}
	}
	return w.entries(), Stats{
		FileCount:            w.files,
		DirCount:             w.countDirs(),
		ByteCount:            w.bytes,
		Truncated:            w.truncated,
		Reason:               w.truncatedCause,
		Source:               w.source,
		IgnoredByGit:         w.ignoredByGit,
		TotalFilesystemFiles: w.totalFilesystemFiles,
		TotalFilesystemDirs:  w.totalFilesystemDirs,
	}, nil
}

type walker struct {
	root           string
	opts           Options
	excludedDir    map[string]struct{}
	entriesByRel   map[string]Entry
	files          int
	bytes          int64
	truncated      bool
	truncatedCause string
	source         string
	ignoredByGit   int
	totalFilesystemFiles int
	totalFilesystemDirs  int
}

func (w *walker) markTruncated(reason string) {
	if w.truncated {
		return
	}
	w.truncated = true
	w.truncatedCause = reason
}

func (w *walker) populateViaGit(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "-C", w.root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	paths := strings.Split(string(out), "\x00")
	for _, rel := range paths {
		rel = cleanRel(rel)
		if rel == "" {
			continue
		}
		if w.isExcludedDescendant(rel) {
			continue
		}
		if w.opts.MaxDepth > 0 && depthOf(rel) > w.opts.MaxDepth {
			w.markTruncated(ReasonDepthCap)
			continue
		}
		abs := filepath.Join(w.root, filepath.FromSlash(rel))
		info, statErr := os.Lstat(abs)
		if statErr != nil {
			if os.IsPermission(statErr) {
				w.addSkipped(rel, false, "permission")
				continue
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 && !w.opts.FollowSymlinks {
			w.addSkipped(rel, false, "symlink")
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if !w.canAddFile(info.Size()) {
			continue
		}
		if err := w.addFile(rel, abs, info.Size()); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) populateViaWalk() error {
	return filepath.WalkDir(w.root, func(path string, d os.DirEntry, err error) error {
		relRaw, relErr := filepath.Rel(w.root, path)
		if relErr != nil {
			return nil
		}
		relRaw = filepath.ToSlash(relRaw)
		if relRaw == "." {
			return nil
		}

		if err != nil {
			if os.IsPermission(err) {
				w.addSkipped(cleanRel(relRaw), d != nil && d.IsDir(), "permission")
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return err
		}

		rel := cleanRel(relRaw)
		if rel == "" {
			return nil
		}
		name := d.Name()
		if d.IsDir() && w.isExcludedName(name) {
			return filepath.SkipDir
		}

		if w.opts.MaxDepth > 0 && depthOf(rel) > w.opts.MaxDepth {
			w.markTruncated(ReasonDepthCap)
			w.addSkipped(rel, d.IsDir(), "depth-cap")
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 && !w.opts.FollowSymlinks {
			w.addSkipped(rel, d.IsDir(), "symlink")
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			w.addDir(rel, path)
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			if os.IsPermission(statErr) {
				w.addSkipped(rel, false, "permission")
				return nil
			}
			return statErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if !w.canAddFile(info.Size()) {
			return filepath.SkipDir
		}
		return w.addFile(rel, path, info.Size())
	})
}

func (w *walker) canAddFile(size int64) bool {
	if w.opts.MaxFiles > 0 && w.files >= w.opts.MaxFiles {
		w.markTruncated(ReasonFileCap)
		return false
	}
	if w.opts.MaxBytes > 0 && w.bytes+size > w.opts.MaxBytes {
		w.markTruncated(ReasonByteCap)
		return false
	}
	return true
}

func (w *walker) addFile(rel, abs string, size int64) error {
	w.files++
	w.bytes += size
	if err := w.addDirPrefixes(rel); err != nil {
		return err
	}
	w.addEntry(Entry{
		RelPath:   rel,
		AbsPath:   abs,
		IsDir:     false,
		SizeBytes: size,
	})
	return nil
}

func (w *walker) addDir(rel, abs string) {
	w.addEntry(Entry{
		RelPath: rel,
		AbsPath: abs,
		IsDir:   true,
	})
}

func (w *walker) addDirPrefixes(rel string) error {
	parts := strings.Split(rel, "/")
	if len(parts) < 2 {
		return nil
	}
	for idx := 1; idx < len(parts); idx++ {
		dir := strings.Join(parts[:idx], "/")
		if w.opts.MaxDepth > 0 && depthOf(dir) > w.opts.MaxDepth {
			w.markTruncated(ReasonDepthCap)
			return nil
		}
		w.addEntry(Entry{
			RelPath: dir,
			AbsPath: filepath.Join(w.root, filepath.FromSlash(dir)),
			IsDir:   true,
		})
	}
	return nil
}

func (w *walker) addSkipped(rel string, isDir bool, reason string) {
	w.addEntry(Entry{
		RelPath:    rel,
		AbsPath:    filepath.Join(w.root, filepath.FromSlash(rel)),
		IsDir:      isDir,
		Skipped:    true,
		SkipReason: reason,
	})
}

func (w *walker) addEntry(e Entry) {
	prev, ok := w.entriesByRel[e.RelPath]
	if !ok {
		w.entriesByRel[e.RelPath] = e
		return
	}
	// Prefer non-skipped over skipped.
	if prev.Skipped && !e.Skipped {
		w.entriesByRel[e.RelPath] = e
		return
	}
	// Prefer files over dirs when both exist at same path.
	if prev.IsDir && !e.IsDir {
		w.entriesByRel[e.RelPath] = e
	}
}

func (w *walker) entries() []Entry {
	out := make([]Entry, 0, len(w.entriesByRel))
	for _, e := range w.entriesByRel {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].RelPath < out[j].RelPath
	})
	return out
}

func (w *walker) countDirs() int {
	n := 0
	for _, e := range w.entriesByRel {
		if e.IsDir && !e.Skipped {
			n++
		}
	}
	return n
}

func (w *walker) scanFilesystemCounts() (int, int, error) {
	files := 0
	dirs := 0
	err := filepath.WalkDir(w.root, func(path string, d os.DirEntry, err error) error {
		relRaw, relErr := filepath.Rel(w.root, path)
		if relErr != nil {
			return nil
		}
		relRaw = filepath.ToSlash(relRaw)
		if relRaw == "." {
			return nil
		}
		if err != nil {
			if os.IsPermission(err) && d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel := cleanRel(relRaw)
		if rel == "" {
			return nil
		}
		if d.IsDir() {
			if w.isExcludedName(d.Name()) {
				return filepath.SkipDir
			}
			if w.opts.MaxDepth > 0 && depthOf(rel) > w.opts.MaxDepth {
				return filepath.SkipDir
			}
			dirs++
			return nil
		}
		if w.isExcludedDescendant(rel) {
			return nil
		}
		if w.opts.MaxDepth > 0 && depthOf(rel) > w.opts.MaxDepth {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		files++
		return nil
	})
	return files, dirs, err
}

func (w *walker) isExcludedName(name string) bool {
	_, ok := w.excludedDir[name]
	return ok
}

func (w *walker) isExcludedDescendant(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if w.isExcludedName(part) {
			return true
		}
	}
	return false
}

func cleanRel(rel string) string {
	rel = strings.TrimSpace(rel)
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimSuffix(rel, "/")
	return rel
}

func depthOf(rel string) int {
	rel = cleanRel(rel)
	if rel == "" {
		return 0
	}
	return strings.Count(rel, "/") + 1
}
