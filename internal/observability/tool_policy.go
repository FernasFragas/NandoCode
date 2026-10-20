package observability

import "github.com/FernasFragas/Nandocode/internal/agent"

// SetParentPolicy preserves policy-aware tools (the Agent tool) through decoration.
func (t *observedTool) SetParentPolicy(p agent.ParentPolicy) {
	if aware, ok := t.next.(interface{ SetParentPolicy(agent.ParentPolicy) }); ok {
		aware.SetParentPolicy(p)
	}
}
