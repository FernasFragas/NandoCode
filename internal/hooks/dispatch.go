package hooks

import (
	"context"
	"strings"
	"sync"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"golang.org/x/sync/errgroup"
)

type Dispatcher struct {
	mu       sync.RWMutex
	Snapshot Snapshot
	Client   llm.Client
	Config   Config
}

func NewDispatcher(snapshot Snapshot, client llm.Client, cfg Config) *Dispatcher {
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = DefaultConfig().DefaultTimeout
	}
	return &Dispatcher{Snapshot: snapshot, Client: client, Config: cfg}
}

const parallelHookLimit = 4

func (d *Dispatcher) Dispatch(ctx context.Context, env Envelope) Result {
	d.mu.RLock()
	snap := d.Snapshot
	d.mu.RUnlock()
	hooks := matchingHooks(snap, env)
	if len(hooks) == 0 {
		return Result{}
	}
	results := make([]Result, len(hooks))
	parallelIdx := make([]int, 0, len(hooks))
	for i, h := range hooks {
		if h.ParallelSafe {
			parallelIdx = append(parallelIdx, i)
			continue
		}
		results[i] = d.runHook(ctx, h, env)
	}
	if len(parallelIdx) == 0 {
		return aggregate(results)
	}
	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(parallelHookLimit)
	for _, idx := range parallelIdx {
		i := idx
		h := hooks[i]
		g.Go(func() error {
			results[i] = d.runHook(groupCtx, h, env)
			return nil
		})
	}
	_ = g.Wait()
	return aggregate(results)
}

func (d *Dispatcher) runHook(ctx context.Context, h Hook, env Envelope) Result {
	switch h.Kind {
	case KindCommand:
		return runCommandHook(ctx, h, env, d.Config)
	case KindPrompt:
		return runPromptHook(ctx, d.Client, h, env, d.Config)
	case KindAgent:
		return runAgentHook(ctx, d.Client, h, env, d.Config)
	case KindHTTP:
		return runHTTPHook(ctx, h, env, d.Config)
	default:
		return Result{Warning: "unknown hook kind: " + string(h.Kind)}
	}
}

func (d *Dispatcher) SetSnapshot(snapshot Snapshot) {
	d.mu.Lock()
	d.Snapshot = snapshot
	d.mu.Unlock()
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	const max = 2000
	if len(s) > max {
		s = s[:max] + "\n<truncated>"
	}
	replacers := []string{"TOKEN=", "KEY=", "SECRET=", "PASSWORD="}
	for _, marker := range replacers {
		idx := strings.Index(strings.ToUpper(s), marker)
		if idx >= 0 {
			end := strings.IndexAny(s[idx+len(marker):], " \n\t")
			if end < 0 {
				s = s[:idx+len(marker)] + "<redacted>"
			} else {
				end += idx + len(marker)
				s = s[:idx+len(marker)] + "<redacted>" + s[end:]
			}
		}
	}
	return s
}
