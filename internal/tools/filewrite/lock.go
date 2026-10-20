package filewrite

import (
	"context"
	"path/filepath"
	"sync"
	"time"
)

type pathLockEntry struct {
	ch   chan struct{}
	refs int
}

var (
	pathLocksMu sync.Mutex
	pathLocks   = map[string]*pathLockEntry{}
)

func pathKey(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path
}

func acquireEntry(key string) *pathLockEntry {
	pathLocksMu.Lock()
	defer pathLocksMu.Unlock()
	e := pathLocks[key]
	if e == nil {
		e = &pathLockEntry{ch: make(chan struct{}, 1)}
		pathLocks[key] = e
	}
	e.refs++
	return e
}

func releaseEntry(key string, e *pathLockEntry) {
	pathLocksMu.Lock()
	e.refs--
	if e.refs == 0 {
		delete(pathLocks, key)
	}
	pathLocksMu.Unlock()
}

// LockPath reserves a path while an agent write is in progress. It gives up
// with the context's error when ctx ends first, so a stuck holder cannot block
// the caller forever.
func LockPath(ctx context.Context, path string) (func(), error) {
	key := pathKey(path)
	e := acquireEntry(key)
	select {
	case e.ch <- struct{}{}:
		return func() { <-e.ch; releaseEntry(key, e) }, nil
	case <-ctx.Done():
		releaseEntry(key, e)
		return nil, ctx.Err()
	}
}

// TryLockPath reserves a path for a browser save without waiting for a writer.
func TryLockPath(path string) (func(), bool) {
	key := pathKey(path)
	e := acquireEntry(key)
	select {
	case e.ch <- struct{}{}:
		return func() { <-e.ch; releaseEntry(key, e) }, true
	default:
		releaseEntry(key, e)
		return nil, false
	}
}

func lockCount() int {
	pathLocksMu.Lock()
	defer pathLocksMu.Unlock()
	return len(pathLocks)
}

// PathLockTimeout bounds how long an agent write waits for a browser save.
const PathLockTimeout = 30 * time.Second
