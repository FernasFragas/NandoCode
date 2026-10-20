package agent

import (
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// ParentPolicy is the permission policy of the agent run that is about to call
// a tool: its mode, rules and hooks. A tool that spawns a sub-agent (the Agent
// tool) uses it so the child can never be looser than the parent.
type ParentPolicy struct {
	Mode             permissions.Mode
	Rules            permissions.Rules
	HookDecision     permissions.HookDecisionFunc
	PostToolUse      ToolHookFunc
	PermissionDenied ToolHookFunc
}

// parentPolicyAware is implemented by tools that need the calling run's policy.
type parentPolicyAware interface {
	SetParentPolicy(ParentPolicy)
}

// setParentPolicy hands the calling run's policy to tool when it asks for it.
func setParentPolicy(tool tools.Tool, p ParentPolicy) {
	if aware, ok := tool.(parentPolicyAware); ok {
		aware.SetParentPolicy(p)
	}
}
