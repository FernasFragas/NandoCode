package server

import (
	"fmt"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/memory"
	"github.com/FernasFragas/Nandocode/internal/observability"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// sessionRuntimeFactory builds the llm client and the agent runner stack that
// sit on top of one RuntimeClient. Each browser session gets its own, so a
// model switch in one session cannot reroute another (review item P0-3).
type sessionRuntimeFactory func(rc *llm.RuntimeClient) (llm.Client, AgentRunner, error)

type sessionRuntimeDeps struct {
	meter        *observability.Meter
	bridge       observability.Bridge
	registry     *tools.Registry
	agentCfg     agent.Config
	memoryCfg    memory.Config
	hookSnapshot hooks.Snapshot
	hookCfg      hooks.Config
}

// newSessionRuntimeFactory returns the production factory: observability
// wrapper, agent loop, memory runner and hook runner, all bound to rc.
func newSessionRuntimeFactory(d sessionRuntimeDeps) sessionRuntimeFactory {
	return func(rc *llm.RuntimeClient) (llm.Client, AgentRunner, error) {
		client := observability.WrapLLMClient(rc, d.meter, d.bridge)
		raw, err := agent.New(client, d.registry, agent.WithConfig(d.agentCfg))
		if err != nil {
			return nil, nil, fmt.Errorf("build session agent: %w", err)
		}
		mem := memory.NewRunner(raw, client, d.memoryCfg)
		dispatcher := hooks.NewDispatcher(d.hookSnapshot, client, d.hookCfg)
		return client, hooks.NewRunner(mem, dispatcher), nil
	}
}

// sessionRuntime is what one session runs on.
type sessionRuntime struct {
	rc     *llm.RuntimeClient // nil when the server has no model runtime service
	client llm.Client
	runner AgentRunner
	models *modelruntime.Service
}

// newSessionRuntime gives a new session its own RuntimeClient (starting on the
// local daemon), its own switching service and its own runner stack. Without a
// model runtime service (some tests) the session shares the server's client
// and runner.
func (s *Server) newSessionRuntime() (sessionRuntime, error) {
	if s.modelRuntime == nil || s.modelRuntime.LocalClient == nil {
		return sessionRuntime{client: s.client, runner: s.runner, models: s.modelRuntime}, nil
	}
	rc := llm.NewRuntimeClient(s.modelRuntime.LocalClient, llm.ProviderOllamaLocal, s.modelRuntime.LocalBaseURL)
	rt := sessionRuntime{rc: rc, client: rc, runner: s.runner, models: s.modelRuntime.WithRuntime(rc)}
	if s.sessionRuntimes != nil {
		client, runner, err := s.sessionRuntimes(rc)
		if err != nil {
			return sessionRuntime{}, err
		}
		rt.client, rt.runner = client, runner
	}
	return rt, nil
}
