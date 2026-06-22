package skills

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type watcherState struct {
	w   *fsnotify.Watcher
	cls chan struct{}
	wg  sync.WaitGroup
}

func (w *watcherState) close() error {
	close(w.cls)
	w.wg.Wait()
	return w.w.Close()
}

func startWatcher(l *Loader) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	ws := &watcherState{w: w, cls: make(chan struct{})}
	l.watcher = ws

	for _, dir := range []string{l.userDir, l.projDir} {
		if dir == "" {
			continue
		}
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			_ = w.Add(dir)
		}
	}

	ws.wg.Add(1)
	go func() {
		defer ws.wg.Done()
		pending := map[string]fsnotify.Op{}
		deadline := map[string]time.Time{}
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ws.cls:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				path := filepath.Clean(ev.Name)
				if !strings.HasSuffix(strings.ToLower(path), ".md") {
					continue
				}
				pending[path] |= ev.Op
				deadline[path] = time.Now().Add(50 * time.Millisecond)
			case <-w.Errors:
			case <-ticker.C:
				now := time.Now()
				for path, due := range deadline {
					if now.Before(due) {
						continue
					}
					op := pending[path]
					delete(pending, path)
					delete(deadline, path)
					l.rescanPath(path, op)
				}
			}
		}
	}()
	return nil
}

func (l *Loader) rescanPath(path string, op fsnotify.Op) {
	switch {
	case op&(fsnotify.Create|fsnotify.Write) != 0:
		l.upsertPath(path)
	case op&(fsnotify.Remove|fsnotify.Rename) != 0:
		l.removePath(path)
	}
}

func (l *Loader) upsertPath(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		slog.Debug("skills: failed to read updated skill file", "path", path, "error", err)
		return
	}
	sf, body, err := parseFrontmatter(strings.NewReader(string(b)))
	if err != nil {
		slog.Warn("skills: skipping updated skill with invalid frontmatter", "path", path, "error", err)
		return
	}
	st, _ := os.Stat(path)
	sf.Path = path
	sf.Body = body
	if st != nil {
		sf.ModTime = st.ModTime()
	}
	if strings.HasPrefix(path, filepath.Clean(l.projDir)+string(os.PathSeparator)) {
		sf.Source = SourceProject
	} else {
		sf.Source = SourceUser
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.putLocked(sf)
	l.notifyLocked(sf.Name, sf.Source)
}

func (l *Loader) removePath(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	removed := false
	for name, variants := range l.byName {
		for src, sf := range variants {
			if sf.Path == path {
				delete(variants, src)
				if len(variants) == 0 {
					delete(l.byName, name)
				}
				l.notifyLocked(name, src)
				removed = true
			}
		}
	}
	if !removed {
		slog.Debug("skills: remove/rename event had no indexed skill match", "path", path)
	}
}
