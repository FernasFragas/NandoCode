package permissions

// Decision represents the outcome of a permission check.
type Decision string

const (
	// DecisionAllow permits the tool call to execute.
	DecisionAllow Decision = "allow"
	// DecisionDeny prevents the tool call from executing.
	DecisionDeny Decision = "deny"
	// DecisionAsk indicates that permission should be requested from the user
	// or forwarded to an interactive layer.
	DecisionAsk Decision = "ask"
)

// String returns the string representation of the decision.
func (d Decision) String() string {
	return string(d)
}

// Stage indicates which resolver stage produced the decision.
type Stage string

const (
	// StageHook means a pre-check hook produced the decision.
	StageHook Stage = "hook"
	// StageRule means a matching rule produced the decision.
	StageRule Stage = "rule"
	// StageTool means the tool's per-call classifier produced the decision.
	StageTool Stage = "tool"
	// StageMode means the selected permission mode produced the decision.
	StageMode Stage = "mode"
	// StagePrompt means an interactive prompt produced the decision.
	StagePrompt Stage = "prompt"
	// StageClassifier means an LLM or auto classifier produced the decision.
	StageClassifier Stage = "classifier"
)

// String returns the string representation of the stage.
func (s Stage) String() string {
	return string(s)
}

// Result carries the outcome of a permission resolution.
type Result struct {
	// Decision is the resolved permission decision.
	Decision Decision
	// Stage indicates which stage in the resolver produced this decision.
	Stage Stage
	// Reason is a brief, model-readable explanation suitable for tool-result
	// messages. It should not include stack traces or sensitive command output.
	Reason string
	// UpdatedInput holds a modified version of the input if the tool classifier
	// or another stage transformed it. Nil means no transformation occurred.
	UpdatedInput any
	// Rule holds a reference to the matching rule that produced this decision,
	// if applicable. Nil if the decision came from another stage.
	Rule *Rule
}
