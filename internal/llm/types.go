// Package llm provides the LLM client interface and types for interacting with Ollama.
package llm

import (
	"context"
	"time"
)

// Role represents the role of a message in a conversation.
type Role string

const (
	// RoleSystem is the system role for context and instructions.
	RoleSystem Role = "system"
	// RoleUser is the user role for user input.
	RoleUser Role = "user"
	// RoleAssistant is the assistant role for model responses.
	RoleAssistant Role = "assistant"
	// RoleTool is the tool role for tool execution results.
	RoleTool Role = "tool"
)

// Message represents a single message in the conversation.
type Message struct {
	Role      Role       `json:"role"`
	Content   string     `json:"content,omitempty"`
	Thinking  string     `json:"thinking,omitempty"` // For thinking models
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"` // When Role==RoleTool
	Images    []string   `json:"images,omitempty"`    // Base64-encoded images
}

// ToolCall represents a tool invocation request from the model.
type ToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

// ToolDef represents a tool definition for the model.
type ToolDef struct {
	Type     string `json:"type"` // Always "function"
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"` // JSON Schema
	} `json:"function"`
}

// ChatRequest represents a request to the chat API.
type ChatRequest struct {
	Model     string         `json:"model"`
	Messages  []Message      `json:"messages"`
	Tools     []ToolDef      `json:"tools,omitempty"`
	Format    any            `json:"format,omitempty"` // "json" or JSON Schema map
	Stream    bool           `json:"stream"`
	Think     any            `json:"think,omitempty"` // bool or "low"|"medium"|"high"
	KeepAlive string         `json:"keep_alive,omitempty"`
	Options   map[string]any `json:"options,omitempty"` // num_ctx, temperature, etc.
}

// StreamEvent represents a single event in the streaming response.
type StreamEvent struct {
	Model           string    `json:"model"`
	CreatedAt       time.Time `json:"created_at"`
	Message         Message   `json:"message"`
	Done            bool      `json:"done"`
	DoneReason      string    `json:"done_reason,omitempty"`
	PromptEvalCount int64     `json:"prompt_eval_count,omitempty"`
	EvalCount       int64     `json:"eval_count,omitempty"`
	TotalDuration   int64     `json:"total_duration,omitempty"` // Nanoseconds
}

// ModelInfo represents information about a model.
type ModelInfo struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	ModifiedAt time.Time `json:"modified_at"`
}

// ModelDetails holds detailed metadata about a specific model returned by ShowModel.
type ModelDetails struct {
	Name              string         `json:"name"`
	Family            string         `json:"family,omitempty"`
	ParameterSize     string         `json:"parameter_size,omitempty"`
	QuantizationLevel string         `json:"quantization_level,omitempty"`
	ContextLength     int64          `json:"context_length,omitempty"`
	Parameters        map[string]any `json:"parameters,omitempty"`
}

// PullProgress represents progress information when pulling a model.
type PullProgress struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
}

// Client defines the interface for interacting with an LLM provider.
type Client interface {
	// Chat initiates a chat completion and returns a channel of streaming events.
	// The channel will be closed when the stream completes or an error occurs.
	// Context cancellation will abort the stream.
	Chat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)

	// Embed generates embeddings for the given input strings.
	Embed(ctx context.Context, model string, input []string) ([][]float32, error)

	// ListModels returns a list of available models.
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// ShowModel returns detailed metadata for a specific model (context length, family, params).
	ShowModel(ctx context.Context, name string) (ModelDetails, error)

	// PullModel downloads a model and reports progress via the channel.
	// The channel will be closed when the pull completes or fails.
	PullModel(ctx context.Context, name string, progress chan<- PullProgress) error
}

// EmbedOptions configures optional embedding request behavior.
// Implementations may ignore unsupported fields.
type EmbedOptions struct {
	Dimensions *int           `json:"dimensions,omitempty"`
	Truncate   *bool          `json:"truncate,omitempty"`
	KeepAlive  string         `json:"keep_alive,omitempty"`
	Options    map[string]any `json:"options,omitempty"`
}

// EmbedderWithOptions extends embedding support with optional request controls.
// This is additive and does not change the base Client interface.
type EmbedderWithOptions interface {
	EmbedWithOptions(ctx context.Context, model string, input []string, opts *EmbedOptions) ([][]float32, error)
}
