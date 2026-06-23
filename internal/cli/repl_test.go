package cli

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/mcp"
	"github.com/FernasFragas/Nandocode/internal/skills"
)

func TestPrepareStartupParallelRunsIndependentStepsConcurrently(t *testing.T) {
	t.Parallel()

	var started atomic.Int32
	release := make(chan struct{})
	stageMu := sync.Mutex{}
	stageDurations := map[string]time.Duration{}

	done := make(chan struct {
		result startupParallelResult
		err    error
	}, 1)
	go func() {
		result, err := prepareStartupParallel(context.Background(), startupParallelDeps{
			showModel: func(context.Context) (llm.ModelDetails, error) {
				started.Add(1)
				<-release
				return llm.ModelDetails{
					ContextLength: 8192,
					Parameters: map[string]any{
						"num_predict": float64(2048),
					},
				}, nil
			},
			newSkillLoader: func() (*skills.Loader, error) {
				started.Add(1)
				<-release
				return &skills.Loader{}, nil
			},
			loadMCPConfig: func() (mcp.Config, []string) {
				started.Add(1)
				<-release
				return mcp.Config{}, []string{"config-warning"}
			},
			startMCP: func(context.Context, mcp.Config) (*mcp.Manager, []string) {
				return &mcp.Manager{}, []string{"startup-warning"}
			},
			noteStage: func(stage string, d time.Duration) {
				stageMu.Lock()
				stageDurations[stage] = d
				stageMu.Unlock()
			},
		}, 1024, 0)
		done <- struct {
			result startupParallelResult
			err    error
		}{result: result, err: err}
	}()

	deadline := time.Now().Add(250 * time.Millisecond)
	for started.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if started.Load() < 3 {
		t.Fatalf("expected all startup workers to start, got %d", started.Load())
	}
	close(release)

	out := <-done
	if out.err != nil {
		t.Fatalf("prepareStartupParallel returned error: %v", out.err)
	}
	if out.result.modelLimits == nil {
		t.Fatal("expected model limits to be populated")
	}
	if out.result.startupMaxOutputTokens != 1024 {
		t.Fatalf("expected startup max output tokens 1024, got %d", out.result.startupMaxOutputTokens)
	}
	if out.result.startupNumCtx != 8192 {
		t.Fatalf("expected startup num_ctx 8192, got %d", out.result.startupNumCtx)
	}
	if got := len(out.result.mcpConfigWarnings); got != 1 || out.result.mcpConfigWarnings[0] != "config-warning" {
		t.Fatalf("unexpected mcp config warnings: %#v", out.result.mcpConfigWarnings)
	}
	if got := len(out.result.mcpStartupWarnings); got != 1 || out.result.mcpStartupWarnings[0] != "startup-warning" {
		t.Fatalf("unexpected mcp startup warnings: %#v", out.result.mcpStartupWarnings)
	}

	stageMu.Lock()
	defer stageMu.Unlock()
	for _, stage := range []string{"startup_model_limits", "startup_skills_loader", "startup_mcp_bootstrap"} {
		if _, ok := stageDurations[stage]; !ok {
			t.Fatalf("expected stage duration for %s", stage)
		}
	}
}

func TestPrepareStartupParallelKeepsConfiguredNumCtx(t *testing.T) {
	t.Parallel()

	result, err := prepareStartupParallel(context.Background(), startupParallelDeps{
		showModel: func(context.Context) (llm.ModelDetails, error) {
			return llm.ModelDetails{
				ContextLength: 8192,
				Parameters: map[string]any{
					"num_predict": float64(2048),
				},
			}, nil
		},
		newSkillLoader: func() (*skills.Loader, error) {
			return &skills.Loader{}, nil
		},
		loadMCPConfig: func() (mcp.Config, []string) {
			return mcp.Config{}, nil
		},
		startMCP: func(context.Context, mcp.Config) (*mcp.Manager, []string) {
			return &mcp.Manager{}, nil
		},
	}, 1024, 4096)
	if err != nil {
		t.Fatalf("prepareStartupParallel returned error: %v", err)
	}
	if result.startupNumCtx != 4096 {
		t.Fatalf("expected explicit num_ctx to be preserved, got %d", result.startupNumCtx)
	}
	if result.startupMaxOutputTokens != 1024 {
		t.Fatalf("expected startup max output tokens to keep configured default when model allows more, got %d", result.startupMaxOutputTokens)
	}
}

func TestPrepareStartupParallelCapsOutputBudgetByModelLimit(t *testing.T) {
	t.Parallel()

	result, err := prepareStartupParallel(context.Background(), startupParallelDeps{
		showModel: func(context.Context) (llm.ModelDetails, error) {
			return llm.ModelDetails{
				ContextLength: 8192,
				Parameters: map[string]any{
					"num_predict": float64(512),
				},
			}, nil
		},
		newSkillLoader: func() (*skills.Loader, error) {
			return &skills.Loader{}, nil
		},
		loadMCPConfig: func() (mcp.Config, []string) {
			return mcp.Config{}, nil
		},
		startMCP: func(context.Context, mcp.Config) (*mcp.Manager, []string) {
			return &mcp.Manager{}, nil
		},
	}, 1024, 0)
	if err != nil {
		t.Fatalf("prepareStartupParallel returned error: %v", err)
	}
	if result.startupMaxOutputTokens != 512 {
		t.Fatalf("expected startup max output tokens capped to 512, got %d", result.startupMaxOutputTokens)
	}
}

func TestPrepareStartupParallelModelFailureKeepsDefaultsAndContinues(t *testing.T) {
	t.Parallel()

	result, err := prepareStartupParallel(context.Background(), startupParallelDeps{
		showModel: func(context.Context) (llm.ModelDetails, error) {
			return llm.ModelDetails{}, errors.New("ollama unavailable")
		},
		newSkillLoader: func() (*skills.Loader, error) {
			return &skills.Loader{}, nil
		},
		loadMCPConfig: func() (mcp.Config, []string) {
			return mcp.Config{}, []string{"config-warning"}
		},
		startMCP: func(context.Context, mcp.Config) (*mcp.Manager, []string) {
			return &mcp.Manager{}, []string{"startup-warning"}
		},
	}, 1024, 4096)
	if err != nil {
		t.Fatalf("prepareStartupParallel returned error: %v", err)
	}
	if result.modelLimits != nil {
		t.Fatalf("expected no model limits on failure, got %#v", result.modelLimits)
	}
	if result.modelWarning == "" || result.modelWarning != "Warning: could not fetch model limits from Ollama (ollama unavailable), using defaults" {
		t.Fatalf("unexpected model warning: %q", result.modelWarning)
	}
	if result.startupMaxOutputTokens != 1024 {
		t.Fatalf("expected startup max output tokens to keep default, got %d", result.startupMaxOutputTokens)
	}
	if result.startupNumCtx != 4096 {
		t.Fatalf("expected startup num_ctx to keep default, got %d", result.startupNumCtx)
	}
	if result.skillLoader == nil {
		t.Fatal("expected skills loader to still be created")
	}
	if result.mcpMgr == nil {
		t.Fatal("expected mcp manager to still be created")
	}
	if got := len(result.mcpConfigWarnings); got != 1 || result.mcpConfigWarnings[0] != "config-warning" {
		t.Fatalf("unexpected mcp config warnings: %#v", result.mcpConfigWarnings)
	}
	if got := len(result.mcpStartupWarnings); got != 1 || result.mcpStartupWarnings[0] != "startup-warning" {
		t.Fatalf("unexpected mcp startup warnings: %#v", result.mcpStartupWarnings)
	}
}

func TestPrepareStartupParallelPropagatesSkillLoaderError(t *testing.T) {
	t.Parallel()

	_, err := prepareStartupParallel(context.Background(), startupParallelDeps{
		showModel: func(context.Context) (llm.ModelDetails, error) {
			return llm.ModelDetails{}, nil
		},
		newSkillLoader: func() (*skills.Loader, error) {
			return nil, errors.New("boom")
		},
		loadMCPConfig: func() (mcp.Config, []string) {
			return mcp.Config{}, nil
		},
		startMCP: func(context.Context, mcp.Config) (*mcp.Manager, []string) {
			return &mcp.Manager{}, nil
		},
	}, 1024, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "failed to create skills loader: boom" {
		t.Fatalf("unexpected error: %s", got)
	}
}
