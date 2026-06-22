package agent

import (
	"os"
	"strconv"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

const (
	defaultCoordinatorMaxWorkers = 3
	maxCoordinatorMaxWorkers     = 5
)

var CoordinatorInternalTools = map[string]bool{
	"SendMessage": true,
	"Agent":       true,
	"TaskCreate":  true,
	"TaskGet":     true,
	"TaskStop":    true,
	"TaskList":    true,
	"TaskOutput":  true,
}

type CoordinatorConfig struct {
	Enabled    bool
	Dream      bool
	MaxWorkers int
}

func ReadCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		Enabled:    IsCoordinatorMode(),
		Dream:      IsDreamEnabled(),
		MaxWorkers: coordinatorMaxWorkers(),
	}
}

func IsCoordinatorMode() bool {
	return envTruthy(os.Getenv("NANDOCODEGO_COORDINATOR"))
}

func IsDreamEnabled() bool {
	return envTruthy(os.Getenv("NANDOCODEGO_DREAM"))
}

func BuildCoordinatorSystemPrompt(workerToolNames []string, scratchpadDir string) string {
	toolsList := "none listed"
	if len(workerToolNames) > 0 {
		toolsList = strings.Join(workerToolNames, ", ")
	}
	scratch := "disabled"
	if strings.TrimSpace(scratchpadDir) != "" {
		scratch = strings.TrimSpace(scratchpadDir)
	}
	return strings.TrimSpace(`
You are a coordinator agent.
You must plan and delegate, then synthesize.

Never delegate understanding.
You must own synthesis quality before sending implementation tasks.

Use this phase loop:
1. Research: spawn workers to collect facts in parallel.
2. Synthesize: combine findings yourself and resolve conflicts.
3. Implement: spawn precise workers with exact file/task boundaries.
4. Verify: run targeted checks and summarize confidence.

Continue vs spawn guidance:
- Continue with SendMessage when follow-up stays in the same scope.
- Spawn a fresh worker when scope shifts or context is noisy.
- Stop and reassign if a worker stalls or drifts.

Worker tools available: ` + toolsList + `
Scratchpad directory: ` + scratch + `
`)
}

func BuildCoordinatorRegistry(agentTool tools.Tool, sendMessageTool tools.Tool, taskStopTool tools.Tool) *tools.Registry {
	r := tools.NewRegistry()
	if agentTool != nil {
		_ = r.Register(agentTool)
	}
	if sendMessageTool != nil {
		_ = r.Register(sendMessageTool)
	}
	if taskStopTool != nil {
		_ = r.Register(taskStopTool)
	}
	return r
}

func BuildWorkerRegistry(full *tools.Registry) *tools.Registry {
	r := tools.NewRegistry()
	if full == nil {
		return r
	}
	for _, t := range full.All() {
		if CoordinatorInternalTools[t.Name()] {
			continue
		}
		_ = r.Register(t)
	}
	return r
}

func coordinatorMaxWorkers() int {
	raw := strings.TrimSpace(os.Getenv("NANDOCODEGO_COORDINATOR_MAX_WORKERS"))
	if raw == "" {
		return defaultCoordinatorMaxWorkers
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultCoordinatorMaxWorkers
	}
	if n > maxCoordinatorMaxWorkers {
		return maxCoordinatorMaxWorkers
	}
	return n
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
