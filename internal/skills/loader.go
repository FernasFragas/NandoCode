package skills

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Loader struct {
	mu      sync.RWMutex
	byName  map[string]map[Source]SkillFile
	userDir string
	projDir string
	embedFS fs.FS

	watcher  *watcherState
	onChange []func(name string, src Source)
}

func NewLoader(userDir, projDir string, embedFS fs.FS) (*Loader, error) {
	l := &Loader{
		byName:  map[string]map[Source]SkillFile{},
		userDir: userDir,
		projDir: projDir,
		embedFS: embedFS,
	}
	l.scan()
	if err := startWatcher(l); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Loader) scan() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.byName = map[string]map[Source]SkillFile{}

	if l.embedFS != nil {
		if bundled, err := loadBundledSkills(l.embedFS); err == nil {
			for _, s := range bundled {
				l.putLocked(s)
			}
		}
	}
	l.scanDirLocked(l.userDir, SourceUser)
	l.scanDirLocked(l.projDir, SourceProject)
}

func (l *Loader) scanDirLocked(dir string, src Source) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	seenByName := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		b, readErr := os.ReadFile(full)
		if readErr != nil {
			slog.Debug("skills: failed to read skill file", "path", full, "error", readErr)
			continue
		}
		sf, body, parseErr := parseFrontmatter(strings.NewReader(string(b)))
		if parseErr != nil {
			slog.Warn("skills: skipping invalid skill file", "path", full, "source", src.String(), "error", parseErr)
			continue
		}
		if prevPath, ok := seenByName[sf.Name]; ok {
			slog.Warn(
				"skills: duplicate skill name in source tier; lexicographically later filename wins",
				"name", sf.Name,
				"source", src.String(),
				"kept", full,
				"shadowed", prevPath,
			)
		}
		seenByName[sf.Name] = full
		st, _ := os.Stat(full)
		sf.Source = src
		sf.Path = full
		sf.Body = body
		if st != nil {
			sf.ModTime = st.ModTime()
		}
		l.putLocked(sf)
	}
}

func (l *Loader) putLocked(sf SkillFile) {
	if _, ok := l.byName[sf.Name]; !ok {
		l.byName[sf.Name] = map[Source]SkillFile{}
	}
	l.byName[sf.Name][sf.Source] = sf
}

func (l *Loader) List() []SkillFile {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]SkillFile, 0, len(l.byName))
	for _, variants := range l.byName {
		out = append(out, highestPriority(variants))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (l *Loader) Lookup(name string) (SkillFile, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	v, ok := l.byName[name]
	if !ok || len(v) == 0 {
		return SkillFile{}, false
	}
	return highestPriority(v), true
}

func (l *Loader) ReadBody(sf SkillFile) (string, error) {
	if sf.IsEmbedded() {
		b, err := fs.ReadFile(l.embedFS, sf.EmbedPath)
		if err != nil {
			return "", err
		}
		_, body, err := parseFrontmatter(strings.NewReader(string(b)))
		return body, err
	}
	b, err := os.ReadFile(sf.Path)
	if err != nil {
		return "", err
	}
	_, body, err := parseFrontmatter(strings.NewReader(string(b)))
	return body, err
}

func (l *Loader) AddMCPSkill(sf SkillFile) {
	l.mu.Lock()
	defer l.mu.Unlock()
	sf.Source = SourceMCP
	l.putLocked(sf)
	l.notifyLocked(sf.Name, sf.Source)
}

func (l *Loader) OnChange(fn func(name string, src Source)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onChange = append(l.onChange, fn)
}

func (l *Loader) notifyLocked(name string, src Source) {
	for _, fn := range l.onChange {
		fn(name, src)
	}
}

func (l *Loader) Close() error {
	if l.watcher == nil {
		return nil
	}
	return l.watcher.close()
}

func highestPriority(variants map[Source]SkillFile) SkillFile {
	order := []Source{SourceMCP, SourceProject, SourceUser, SourceBundled}
	for _, src := range order {
		if sf, ok := variants[src]; ok {
			return sf
		}
	}
	return SkillFile{}
}
