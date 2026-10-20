package filewrite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestLockPathHonoursContextAndShrinks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	unlock, err := LockPath(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := LockPath(ctx, p); err == nil {
		t.Fatal("second lock should time out")
	}
	if _, ok := TryLockPath(p); ok {
		t.Fatal("TryLockPath succeeded while held")
	}
	unlock()
	if n := lockCount(); n != 0 {
		t.Fatalf("pathLocks has %d entries after release", n)
	}
}
