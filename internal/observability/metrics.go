package observability

import (
	"fmt"
	"sync"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/permissions"
)

// Snapshot is an immutable view of the in-memory metrics.
type Snapshot struct {
	StartedAt time.Time

	LLMCalls             int64
	LLMErrors            int64
	LLMWatchdogTimeouts  int64
	LLMFirstTokenLatency time.Duration
	LLMTotalDuration     time.Duration

	ToolCalls       int64
	ToolErrors      int64
	ToolCallsByName map[string]int64

	AgentRuns          int64
	AgentRunErrors     int64
	AgentTotalDuration time.Duration
	Turns              int64
	PromptTokens       int64
	CompletionTokens   int64
	TotalTokens        int64
	LastDoneReason     string

	RetryCount          int64
	RetryCountByKind    map[string]int64
	LastRetryKind       string
	LastRetryReason     string
	LastRetryDoneReason string

	PermissionDecisions map[string]int64

	BatchCount           int64
	ConcurrentBatchCount int64
	SerialBatchCount     int64
	BatchCallsTotal      int64
	BatchTotalDuration   time.Duration

	LastRunTrace     RunTrace
	CurrentRunTrace  RunTrace
	CurrentRunActive bool
}

// RunTrace stores high-level timings and outcomes for the latest completed run.
type RunTrace struct {
	RunStartedAt           time.Time
	FirstEventLatency      time.Duration
	FirstAssistantLatency  time.Duration
	FirstThinkingLatency   time.Duration
	FirstToolStartLatency  time.Duration
	FirstRetryLatency      time.Duration
	CompactionStartLatency time.Duration
	CompactionEndLatency   time.Duration
	TerminalLatency        time.Duration
	TerminalReason         string
	DoneReason             string
	RetryCount             int64
	RetryKinds             map[string]int64
	StageLatencies         map[string]time.Duration
	MentionMode            string
	MentionDirs            int
	MentionFilesDiscovered int
	MentionFileBodies      int
	MentionListingIntent   bool
	ContextMode            string
	ToolMode               string
	RouteAction            string
	RouteReason            string
	RouteProfile           string
}

// Meter stores in-memory observability counters and timings.
// It is safe for concurrent use.
type Meter struct {
	mu               sync.RWMutex
	snap             Snapshot
	pendingRunStages map[string]time.Duration
	pendingExpansion runExpansion
}

type runExpansion struct {
	Mode            string
	Dirs            int
	FilesDiscovered int
	FileBodies      int
	ListingIntent   bool
	Set             bool
}

func NewMeter() *Meter {
	return &Meter{
		snap: Snapshot{
			StartedAt:           time.Now().UTC(),
			ToolCallsByName:     map[string]int64{},
			RetryCountByKind:    map[string]int64{},
			PermissionDecisions: map[string]int64{},
		},
		pendingRunStages: map[string]time.Duration{},
		pendingExpansion: runExpansion{},
	}
}

func (m *Meter) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := m.snap
	out.ToolCallsByName = copyInt64Map(m.snap.ToolCallsByName)
	out.RetryCountByKind = copyInt64Map(m.snap.RetryCountByKind)
	out.PermissionDecisions = copyInt64Map(m.snap.PermissionDecisions)
	out.LastRunTrace.RetryKinds = copyInt64Map(m.snap.LastRunTrace.RetryKinds)
	out.LastRunTrace.StageLatencies = copyDurationMap(m.snap.LastRunTrace.StageLatencies)
	out.CurrentRunTrace.RetryKinds = copyInt64Map(m.snap.CurrentRunTrace.RetryKinds)
	out.CurrentRunTrace.StageLatencies = copyDurationMap(m.snap.CurrentRunTrace.StageLatencies)
	return out
}

func (m *Meter) RecordLLMChat(firstTokenLatency, duration time.Duration, promptTokens, completionTokens int64, doneReason string, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	previousCalls := m.snap.LLMCalls
	m.snap.LLMCalls++
	m.snap.LLMTotalDuration += duration
	if firstTokenLatency > 0 {
		total := int64(m.snap.LLMFirstTokenLatency)*previousCalls + int64(firstTokenLatency)
		m.snap.LLMFirstTokenLatency = time.Duration(total / m.snap.LLMCalls)
	}
	if err != nil {
		m.snap.LLMErrors++
	}
	if doneReason == "watchdog_timeout" {
		m.snap.LLMWatchdogTimeouts++
	}
	if doneReason != "" {
		m.snap.LastDoneReason = doneReason
	}
	m.snap.PromptTokens += promptTokens
	m.snap.CompletionTokens += completionTokens
	m.snap.TotalTokens = m.snap.PromptTokens + m.snap.CompletionTokens
}

func (m *Meter) RecordLLMCallError() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.snap.LLMCalls++
	m.snap.LLMErrors++
	m.mu.Unlock()
}

func (m *Meter) RecordToolCall(name string, duration time.Duration, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.snap.ToolCalls++
	if err != nil {
		m.snap.ToolErrors++
	}
	if name == "" {
		name = "<unknown>"
	}
	m.snap.ToolCallsByName[name]++
	_ = duration
}

func (m *Meter) RecordAgentRun(usage agent.Usage, duration time.Duration, reason agent.TerminalReason) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.snap.AgentRuns++
	if reason != agent.TerminalCompleted {
		m.snap.AgentRunErrors++
	}
	m.snap.Turns += int64(usage.Turns)
	m.snap.PromptTokens += usage.PromptEvalCount
	m.snap.CompletionTokens += usage.EvalCount
	m.snap.TotalTokens = m.snap.PromptTokens + m.snap.CompletionTokens
	m.snap.AgentTotalDuration += duration
	if usage.DoneReason != "" {
		m.snap.LastDoneReason = usage.DoneReason
	}
}

func (m *Meter) RecordAgentRetry(retry agent.RetryNotice) {
	if m == nil {
		return
	}
	kind := retry.Kind
	if kind == "" {
		kind = "unknown"
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.snap.RetryCount++
	m.snap.RetryCountByKind[kind]++
	m.snap.LastRetryKind = kind
	m.snap.LastRetryReason = retry.Cause
	m.snap.LastRetryDoneReason = retry.DoneReason
}

func (m *Meter) RecordPermissionDecision(mode permissions.Mode, stage permissions.Stage, toolName string, decision permissions.Decision) {
	if m == nil {
		return
	}
	key := fmt.Sprintf("mode=%s stage=%s tool=%s decision=%s", mode, stage, toolName, decision)
	m.mu.Lock()
	m.snap.PermissionDecisions[key]++
	m.mu.Unlock()
}

func (m *Meter) RecordToolBatch(batchSize int, safe bool, duration time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.snap.BatchCount++
	if safe && batchSize > 1 {
		m.snap.ConcurrentBatchCount++
	} else {
		m.snap.SerialBatchCount++
	}
	if batchSize > 0 {
		m.snap.BatchCallsTotal += int64(batchSize)
	}
	m.snap.BatchTotalDuration += duration
}

func (m *Meter) RecordRunTrace(trace RunTrace) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	trace.RetryKinds = copyInt64Map(trace.RetryKinds)
	trace.StageLatencies = copyDurationMap(trace.StageLatencies)
	m.snap.LastRunTrace = trace
	m.snap.CurrentRunTrace = RunTrace{}
	m.snap.CurrentRunActive = false
}

func (m *Meter) RecordCurrentRunTrace(trace RunTrace) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	trace.RetryKinds = copyInt64Map(trace.RetryKinds)
	trace.StageLatencies = copyDurationMap(trace.StageLatencies)
	m.snap.CurrentRunTrace = trace
	m.snap.CurrentRunActive = true
}

func (m *Meter) ClearCurrentRunTrace() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snap.CurrentRunTrace = RunTrace{}
	m.snap.CurrentRunActive = false
}

func (m *Meter) NotePendingRunStage(stage string, d time.Duration) {
	if m == nil || stage == "" || d <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pendingRunStages == nil {
		m.pendingRunStages = map[string]time.Duration{}
	}
	m.pendingRunStages[stage] = d
}

func (m *Meter) NotePendingRunExpansion(mode string, dirs, discovered, bodies int, listingIntent bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pendingExpansion = runExpansion{
		Mode:            mode,
		Dirs:            dirs,
		FilesDiscovered: discovered,
		FileBodies:      bodies,
		ListingIntent:   listingIntent,
		Set:             true,
	}
}

func (m *Meter) ConsumePendingRunExpansion() (mode string, dirs, discovered, bodies int, listingIntent, ok bool) {
	if m == nil {
		return "", 0, 0, 0, false, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.pendingExpansion.Set {
		return "", 0, 0, 0, false, false
	}
	exp := m.pendingExpansion
	m.pendingExpansion = runExpansion{}
	return exp.Mode, exp.Dirs, exp.FilesDiscovered, exp.FileBodies, exp.ListingIntent, true
}

func (m *Meter) ConsumePendingRunStages() map[string]time.Duration {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := copyDurationMap(m.pendingRunStages)
	m.pendingRunStages = map[string]time.Duration{}
	return out
}

func copyInt64Map(in map[string]int64) map[string]int64 {
	if len(in) == 0 {
		return map[string]int64{}
	}
	out := make(map[string]int64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyDurationMap(in map[string]time.Duration) map[string]time.Duration {
	if len(in) == 0 {
		return map[string]time.Duration{}
	}
	out := make(map[string]time.Duration, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
