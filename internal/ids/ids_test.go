package ids

import (
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/types"
)

func TestNewUnique(t *testing.T) {
	t.Parallel()
	seen := map[string]struct{}{}
	for i := 0; i < 10000; i++ {
		id := New(types.KindBash)
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewPrefixAndFormat(t *testing.T) {
	t.Parallel()
	id := New(types.KindBash)
	if !strings.HasPrefix(id, "b-") {
		t.Fatalf("expected b- prefix, got %q", id)
	}
	re := regexp.MustCompile(`^[a-z]-[0-9a-f]{12}$`)
	if !re.MatchString(id) {
		t.Fatalf("id format mismatch: %q", id)
	}
}

func TestNewConcurrentUnique(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	seen := map[string]struct{}{}
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				id := New(types.KindAgent)
				mu.Lock()
				if _, ok := seen[id]; ok {
					t.Errorf("duplicate id %s", id)
				}
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}

func BenchmarkIDGen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New(types.KindAgent)
	}
}
