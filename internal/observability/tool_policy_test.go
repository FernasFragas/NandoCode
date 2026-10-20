package observability

import (
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

type policyRecordingTool struct {
	tools.Tool
	got *agent.ParentPolicy
}

func (p *policyRecordingTool) Name() string { return "Agent" }
func (p *policyRecordingTool) SetParentPolicy(pp agent.ParentPolicy) {
	p.got = &pp
}

func TestWrappedToolStillReceivesParentPolicy(t *testing.T) {
	inner := &policyRecordingTool{}
	wrapped := WrapTool(inner, nil, nil)
	aware, ok := wrapped.(interface{ SetParentPolicy(agent.ParentPolicy) })
	if !ok {
		t.Fatal("observed tool does not forward SetParentPolicy")
	}
	aware.SetParentPolicy(agent.ParentPolicy{Mode: permissions.ModePlan})
	if inner.got == nil || inner.got.Mode != permissions.ModePlan {
		t.Fatalf("inner tool did not receive the policy: %+v", inner.got)
	}
}
