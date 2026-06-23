// Package tools defines the common interface used by executable model tools.
package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

const (
	defaultMaxResultChars = 30_000
	defaultMaxReadChars   = 100_000
	defaultMaxDirFiles    = 200
	defaultMaxPromptFiles = 400
	defaultMaxDirBytes    = 512 * 1024
	defaultMaxPromptBytes = 2 * 1024 * 1024
	defaultMaxDirDepth    = 8
)

// Permission is a tool-specific permission decision.
type Permission int

const (
	// PermAllow allows execution.
	PermAllow Permission = iota
	// PermDeny denies execution.
	PermDeny
	// PermAsk requires a caller-level prompt before execution.
	PermAsk
)

// String returns a stable permission label.
func (p Permission) String() string {
	switch p {
	case PermAllow:
		return "allow"
	case PermDeny:
		return "deny"
	case PermAsk:
		return "ask"
	default:
		return "unknown"
	}
}

// PermissionResult describes a tool permission decision.
type PermissionResult struct {
	Decision     Permission
	Reason       string
	UpdatedInput any
}

// ProgressEvent is emitted by tools while running.
type ProgressEvent struct {
	Tool    string
	Stream  string
	Message string
	Data    any
}

// RenderHints contains compact UI-facing text for a tool call.
type RenderHints struct {
	Title   string
	Summary string
}

// Result is the common result returned by every tool.
type Result struct {
	Data           any
	Display        string
	NewMessages    []llm.Message
	MaxResultChars int
}

// Tool is the common interface for model-callable tools.
type Tool interface {
	Name() string
	Description() string
	Aliases() []string
	JSONSchema() map[string]any
	UnmarshalInput(raw json.RawMessage) (any, error)

	IsEnabled(ctx Context) bool
	IsReadOnly(input any) bool
	IsConcurrencySafe(input any) bool
	IsDestructive(input any) bool

	CheckPermissions(ctx Context, input any) PermissionResult
	Call(ctx Context, input any, progress chan<- ProgressEvent) (Result, error)

	Render(input any, result Result) RenderHints
}

// Spec defines a tool and lets BuildTool supply fail-closed defaults.
type Spec struct {
	Name              string
	Description       string
	Aliases           []string
	Schema            map[string]any
	Unmarshal         func(json.RawMessage) (any, error)
	CallFunc          func(Context, any, chan<- ProgressEvent) (Result, error)
	IsEnabledFunc     func(Context) bool
	IsReadOnlyFunc    func(any) bool
	IsConcurrentFunc  func(any) bool
	IsDestructiveFunc func(any) bool
	CheckPermFunc     func(Context, any) PermissionResult
	RenderFunc        func(any, Result) RenderHints
}

type builtTool struct {
	spec Spec
}

// BuildTool creates a Tool with conservative defaults for omitted behavior.
func BuildTool(spec Spec) Tool {
	return &builtTool{spec: spec}
}

func (t *builtTool) Name() string { return t.spec.Name }

func (t *builtTool) Description() string { return t.spec.Description }

func (t *builtTool) Aliases() []string { return append([]string(nil), t.spec.Aliases...) }

func (t *builtTool) JSONSchema() map[string]any { return t.spec.Schema }

func (t *builtTool) UnmarshalInput(raw json.RawMessage) (any, error) {
	if t.spec.Unmarshal == nil {
		return nil, errors.New("tool has no input parser")
	}
	return t.spec.Unmarshal(raw)
}

func (t *builtTool) IsEnabled(ctx Context) bool {
	if t.spec.IsEnabledFunc == nil {
		return true
	}
	return t.spec.IsEnabledFunc(ctx)
}

func (t *builtTool) IsReadOnly(input any) bool {
	if t.spec.IsReadOnlyFunc == nil {
		return false
	}
	return t.spec.IsReadOnlyFunc(input)
}

func (t *builtTool) IsConcurrencySafe(input any) bool {
	if t.spec.IsConcurrentFunc == nil {
		return false
	}
	return t.spec.IsConcurrentFunc(input)
}

func (t *builtTool) IsDestructive(input any) bool {
	if t.spec.IsDestructiveFunc == nil {
		return true
	}
	return t.spec.IsDestructiveFunc(input)
}

func (t *builtTool) CheckPermissions(ctx Context, input any) PermissionResult {
	if t.spec.CheckPermFunc != nil {
		return t.spec.CheckPermFunc(ctx, input)
	}
	if t.IsReadOnly(input) {
		return PermissionResult{Decision: PermAllow, UpdatedInput: input}
	}
	switch ctx.PermissionMode {
	case PermissionBypassPermissions:
		return PermissionResult{Decision: PermAllow, UpdatedInput: input}
	case PermissionPlan:
		return PermissionResult{Decision: PermDeny, Reason: "plan mode allows only read-only tools"}
	case PermissionDontAsk:
		return PermissionResult{Decision: PermDeny, Reason: "tool requires permission prompt"}
	default:
		return PermissionResult{Decision: PermAsk, Reason: "tool is not read-only", UpdatedInput: input}
	}
}

func (t *builtTool) Call(ctx Context, input any, progress chan<- ProgressEvent) (Result, error) {
	if t.spec.CallFunc == nil {
		return Result{}, errors.New("tool has no call function")
	}
	return t.spec.CallFunc(ctx, input, progress)
}

func (t *builtTool) Render(input any, result Result) RenderHints {
	if t.spec.RenderFunc != nil {
		return t.spec.RenderFunc(input, result)
	}
	return RenderHints{Title: t.Name(), Summary: t.Name()}
}

// ValidateTool checks invariants required before a tool enters a registry.
func ValidateTool(t Tool) error {
	if t == nil {
		return errors.New("tool is nil")
	}
	if strings.TrimSpace(t.Name()) == "" {
		return errors.New("tool name is empty")
	}
	desc := t.Description()
	if len(desc) > 1024 {
		return fmt.Errorf("tool %q description exceeds 1024 characters", t.Name())
	}
	first := desc
	if idx := strings.Index(desc, "."); idx >= 0 {
		first = desc[:idx+1]
	}
	if len(first) > 100 {
		return fmt.Errorf("tool %q description first sentence exceeds 100 characters", t.Name())
	}
	if t.JSONSchema() == nil {
		return fmt.Errorf("tool %q JSON schema is nil", t.Name())
	}
	return nil
}

// ToLLMToolDef converts a Tool into the Ollama-compatible LLM tool definition.
func ToLLMToolDef(t Tool) (llm.ToolDef, error) {
	if err := ValidateTool(t); err != nil {
		return llm.ToolDef{}, err
	}

	def := llm.ToolDef{Type: "function"}
	def.Function.Name = t.Name()
	def.Function.Description = t.Description()
	def.Function.Parameters = t.JSONSchema()
	return def, nil
}

// TruncateDisplay bounds model-facing text.
func TruncateDisplay(s string, limit int) (string, bool) {
	if limit <= 0 {
		limit = defaultMaxResultChars
	}
	if len(s) <= limit {
		return s, false
	}
	const suffix = "\n\n<truncated>"
	if limit <= len(suffix) {
		return s[:limit], true
	}
	return s[:limit-len(suffix)] + suffix, true
}
