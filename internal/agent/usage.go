package agent

// Usage tracks aggregate statistics for an agent run.
type Usage struct {
	PromptEvalCount int64
	EvalCount       int64
	TotalDuration   int64
	Turns           int
	ToolCalls       int
	DoneReason      string
}
