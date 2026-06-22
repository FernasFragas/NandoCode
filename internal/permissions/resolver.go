package permissions

import (
	"context"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

// HookDecisionFunc is called early in resolution to allow pre-checks to override decisions.
// It returns (Result, ok) where ok indicates whether the hook decided the outcome.
type HookDecisionFunc func(context.Context, Request) (Result, bool)

// PromptFunc is called when a mode produces Ask and prompting is needed.
// It returns the final (Decision, Reason, error).
// Phase 5 does not implement this; nil means prompting is unavailable.
type PromptFunc func(context.Context, Prompt) (Decision, string, error)

// ClassifierFunc is called when a mode produces Ask and auto-classification is needed.
// Phase 5 does not implement this; nil means auto-classification is unavailable.
type ClassifierFunc func(context.Context, Request, tools.PermissionResult) (Result, bool)

// ObserverFunc is called with the final resolution result.
type ObserverFunc func(context.Context, Request, Result)

// Prompt carries information needed for interactive permission prompts.
type Prompt struct {
	ToolName string
	Target   string
	Reason   string
}

// Request carries all information needed to resolve a permission decision.
type Request struct {
	// Mode is the permission mode that applies to this request.
	Mode Mode
	// Rules are the permission rules to check before mode defaults.
	Rules Rules
	// Tool is the tool being called.
	Tool tools.Tool
	// ToolName is the name of the tool being called.
	ToolName string
	// Input is the parsed tool input.
	Input any
	// ToolContext is the execution context passed to tool checkers.
	ToolContext tools.Context

	// HookDecision is an optional callback for pre-checks. May be nil.
	HookDecision HookDecisionFunc
	// Prompt is an optional callback for interactive prompts. May be nil.
	Prompt PromptFunc
	// Classifier is an optional callback for auto-classification. May be nil.
	Classifier ClassifierFunc
	// Observer receives the final decision. May be nil.
	Observer ObserverFunc
}

// Resolve evaluates a permission request through six stages:
// 1. Hook decision (if provided)
// 2. Rule matching
// 3. Tool classifier
// 4. Mode defaults
// 5. Prompt (if decision was Ask and provided)
// 6. Auto classifier (if decision was Ask and provided)
//
// Each stage can terminate resolution with a decision. The resolver is deterministic:
// with nil Prompt and nil Classifier, the same request always produces the same result.
func Resolve(ctx context.Context, req Request) Result {
	observe := func(res Result) Result {
		if req.Observer != nil {
			req.Observer(ctx, req, res)
		}
		return res
	}

	// Validate request.
	if req.Tool == nil || req.ToolName == "" {
		return observe(Result{
			Decision: DecisionDeny,
			Stage:    StageTool,
			Reason:   "invalid request: tool or tool name missing",
		})
	}

	// Normalize mode.
	req.Mode = req.Mode.Normalize()

	// Stage 1: Hook decision.
	if req.HookDecision != nil {
		if result, ok := req.HookDecision(ctx, req); ok {
			return observe(result)
		}
	}

	// Stage 2: Rule matching.
	target := ExtractTarget(req.Input)
	if rule, decision, ok := req.Rules.FirstMatchingRule(req.ToolName, target); ok {
		return observe(Result{
			Decision: decision,
			Stage:    StageRule,
			Reason:   "matched rule: " + rule.Pattern,
			Rule:     rule,
		})
	}

	// Stage 3: Tool classifier.
	classifierResult := req.Tool.CheckPermissions(neutralContext(req.ToolContext), req.Input)
	toolDecision := mapToolPermissionToDecision(classifierResult.Decision)
	if classifierResult.UpdatedInput != nil {
		// Tool classifier may have transformed the input.
		req.Input = classifierResult.UpdatedInput
	}

	// If tool says deny, deny overrides everything except rules (which already matched above).
	if toolDecision == DecisionDeny {
		return observe(Result{
			Decision:     DecisionDeny,
			Stage:        StageTool,
			Reason:       classifierResult.Reason,
			UpdatedInput: classifierResult.UpdatedInput,
		})
	}

	// Stage 4: Mode defaults.
	modeResult := applyMode(req.Mode, toolDecision, classifierResult.Reason)
	if modeResult.Decision != DecisionAsk {
		return observe(modeResult)
	}

	// At this point, modeResult.Decision == DecisionAsk.

	// Stage 5: Prompt (if Ask and callback provided).
	if req.Prompt != nil {
		decision, reason, err := req.Prompt(ctx, Prompt{
			ToolName: req.ToolName,
			Target:   target,
			Reason:   modeResult.Reason,
		})
		if err == nil {
			return observe(Result{
				Decision:     decision,
				Stage:        StagePrompt,
				Reason:       reason,
				UpdatedInput: classifierResult.UpdatedInput,
			})
		}
		// On error, fall through to return the Ask.
	}

	// Stage 6: Classifier (if Ask and callback provided).
	// Phase 5 does not call this. The hook slot exists for future phases.
	if req.Classifier != nil {
		if result, ok := req.Classifier(ctx, req, classifierResult); ok {
			result.UpdatedInput = classifierResult.UpdatedInput
			return observe(result)
		}
	}

	// No further resolution; return the mode Ask.
	modeResult.UpdatedInput = classifierResult.UpdatedInput
	return observe(modeResult)
}

// neutralContext creates a copy of the tool context with a neutral permission mode
// for tool classifier calls.
func neutralContext(ctx tools.Context) tools.Context {
	ctx.PermissionMode = tools.PermissionDefault
	return ctx
}

// mapToolDecision converts a tools.Permission decision to a permissions.Decision.
func mapToolPermissionToDecision(toolPermission tools.Permission) Decision {
	switch toolPermission {
	case tools.PermDeny:
		return DecisionDeny
	case tools.PermAllow:
		return DecisionAllow
	case tools.PermAsk:
		return DecisionAsk
	default:
		return DecisionDeny // Fail closed for unknown values.
	}
}

// applyMode applies the permission mode semantics to a tool classifier decision.
// It returns the mode's override of the classifier result, or preserves the
// classifier's Ask if the mode doesn't override it.
func applyMode(mode Mode, classifierDecision Decision, classifierReason string) Result {
	switch mode {
	case ModeBypass:
		// Allow all classifier allow; deny all classifier deny; allow (override) classifier ask.
		if classifierDecision == DecisionDeny {
			return Result{
				Decision: DecisionDeny,
				Stage:    StageTool,
				Reason:   classifierReason,
			}
		}
		return Result{
			Decision: DecisionAllow,
			Stage:    StageMode,
			Reason:   "bypass mode allows this operation",
		}

	case ModeDontAsk:
		// Allow classifier allow; deny classifier ask and deny.
		if classifierDecision == DecisionAllow {
			return Result{
				Decision: DecisionAllow,
				Stage:    StageMode,
				Reason:   "classifier allows and dontAsk mode permits it",
			}
		}
		return Result{
			Decision: DecisionDeny,
			Stage:    StageMode,
			Reason:   "dontAsk mode denies operations that would require a prompt",
		}

	case ModeAuto:
		// Phase 5 has no auto classifier, so this behaves like ModeDefault.
		// Allow classifier allow; ask classifier ask; deny classifier deny.
		if classifierDecision == DecisionAllow {
			return Result{
				Decision: DecisionAllow,
				Stage:    StageMode,
				Reason:   "classifier allows this operation",
			}
		}
		if classifierDecision == DecisionDeny {
			return Result{
				Decision: DecisionDeny,
				Stage:    StageTool,
				Reason:   classifierReason,
			}
		}
		// Ask for further resolution.
		return Result{
			Decision: DecisionAsk,
			Stage:    StageMode,
			Reason:   classifierReason,
		}

	case ModeAcceptEdits:
		// Allow classifier allow; allow classifier ask for file edits only (future); deny classifier deny.
		// Phase 5 doesn't distinguish file edits, so this behaves like ModeAuto for now.
		if classifierDecision == DecisionAllow {
			return Result{
				Decision: DecisionAllow,
				Stage:    StageMode,
				Reason:   "classifier allows this operation",
			}
		}
		if classifierDecision == DecisionDeny {
			return Result{
				Decision: DecisionDeny,
				Stage:    StageTool,
				Reason:   classifierReason,
			}
		}
		return Result{
			Decision: DecisionAsk,
			Stage:    StageMode,
			Reason:   classifierReason,
		}

	case ModeDefault:
		// Allow classifier allow; ask classifier ask; deny classifier deny.
		if classifierDecision == DecisionAllow {
			return Result{
				Decision: DecisionAllow,
				Stage:    StageMode,
				Reason:   "classifier allows this operation",
			}
		}
		if classifierDecision == DecisionDeny {
			return Result{
				Decision: DecisionDeny,
				Stage:    StageTool,
				Reason:   classifierReason,
			}
		}
		return Result{
			Decision: DecisionAsk,
			Stage:    StageMode,
			Reason:   classifierReason,
		}

	case ModePlan:
		// Allow classifier allow; deny classifier ask and deny.
		if classifierDecision == DecisionAllow {
			return Result{
				Decision: DecisionAllow,
				Stage:    StageMode,
				Reason:   "plan mode allows read-only operations",
			}
		}
		return Result{
			Decision: DecisionDeny,
			Stage:    StageMode,
			Reason:   "plan mode denies operations that would modify state",
		}

	case ModeBubble:
		// Ask for everything except classifier deny.
		if classifierDecision == DecisionDeny {
			return Result{
				Decision: DecisionDeny,
				Stage:    StageTool,
				Reason:   classifierReason,
			}
		}
		return Result{
			Decision: DecisionAsk,
			Stage:    StageMode,
			Reason:   "bubble mode asks all decisions, deferring to parent",
		}

	default:
		// Fallback to ModeDefault behavior.
		if classifierDecision == DecisionAllow {
			return Result{
				Decision: DecisionAllow,
				Stage:    StageMode,
				Reason:   "classifier allows this operation",
			}
		}
		if classifierDecision == DecisionDeny {
			return Result{
				Decision: DecisionDeny,
				Stage:    StageTool,
				Reason:   classifierReason,
			}
		}
		return Result{
			Decision: DecisionAsk,
			Stage:    StageMode,
			Reason:   classifierReason,
		}
	}
}
