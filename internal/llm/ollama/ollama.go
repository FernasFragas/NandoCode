// Package ollama implements the LLM client interface for Ollama.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

// Client implements the llm.Client interface for Ollama.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Options configures an Ollama client.
type Options struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a new Ollama client.
func NewClient(baseURL string) *Client {
	return NewClientWithOptions(Options{BaseURL: baseURL})
}

// NewClientWithOptions creates a new Ollama client with optional auth and HTTP transport overrides.
func NewClientWithOptions(opts Options) *Client {
	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 0, // No timeout at client level; use context
		}
	}
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     strings.TrimSpace(opts.APIKey),
		httpClient: httpClient,
	}
}

func (c *Client) applyHeaders(req *http.Request, hasJSONBody bool) {
	if hasJSONBody {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// Chat initiates a chat completion and returns a channel of streaming events.
func (c *Client) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	// Convert our request to Ollama API format
	ollamaReq := c.toOllamaRequest(req)

	// Marshal request
	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.applyHeaders(httpReq, true)

	// Execute request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	// Check for non-2xx status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Create channel and start streaming goroutine
	events := make(chan llm.StreamEvent, 10)

	go func() {
		defer close(events)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)

		for {
			var event ollamaStreamEvent
			if err := decoder.Decode(&event); err != nil {
				if err != io.EOF {
					// Send error event
					events <- llm.StreamEvent{
						Done:       true,
						DoneReason: fmt.Sprintf("stream_error: %v", err),
					}
				}
				return
			}

			// Convert to our StreamEvent format
			streamEvent := c.fromOllamaEvent(event)

			// Send event (with context cancellation check)
			select {
			case events <- streamEvent:
			case <-ctx.Done():
				return
			}

			// Stop if done
			if event.Done {
				return
			}
		}
	}()

	return events, nil
}

// Embed generates embeddings for the given input strings.
func (c *Client) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return c.EmbedWithOptions(ctx, model, input, nil)
}

// EmbedWithOptions generates embeddings with optional Ollama-specific controls.
func (c *Client) EmbedWithOptions(ctx context.Context, model string, input []string, opts *llm.EmbedOptions) ([][]float32, error) {
	if len(input) == 0 {
		return make([][]float32, 0), nil
	}

	req := ollamaEmbedRequest{
		Model: model,
		Input: input,
	}
	if opts != nil {
		req.Dimensions = opts.Dimensions
		req.Truncate = opts.Truncate
		req.KeepAlive = opts.KeepAlive
		if len(opts.Options) > 0 {
			req.Options = opts.Options
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embed request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create embed request: %w", err)
	}
	c.applyHeaders(httpReq, true)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed error (status %d): %s", resp.StatusCode, string(body))
	}

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode embed response: %w", err)
	}

	if len(result.Embeddings) == 0 && len(result.Embedding) > 0 {
		result.Embeddings = [][]float64{result.Embedding}
	}
	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama embed response missing embeddings")
	}

	embeddings := make([][]float32, len(result.Embeddings))
	for i, vector := range result.Embeddings {
		embeddings[i] = make([]float32, len(vector))
		for j, v := range vector {
			embeddings[i][j] = float32(v)
		}
	}
	return embeddings, nil
}

// ListModels returns a list of available models.
func (c *Client) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create list request: %w", err)
	}
	c.applyHeaders(httpReq, false)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute list request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama list error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Models []struct {
			Name       string    `json:"name"`
			Size       int64     `json:"size"`
			Digest     string    `json:"digest"`
			ModifiedAt time.Time `json:"modified_at"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode list response: %w", err)
	}

	models := make([]llm.ModelInfo, len(result.Models))
	for i, m := range result.Models {
		models[i] = llm.ModelInfo{
			Name:       m.Name,
			Size:       m.Size,
			Digest:     m.Digest,
			ModifiedAt: m.ModifiedAt,
		}
	}

	return models, nil
}

// ShowModel returns detailed metadata for a specific model by calling /api/show.
func (c *Client) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) {
	reqBody, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return llm.ModelDetails{}, fmt.Errorf("failed to marshal show request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/show", bytes.NewReader(reqBody))
	if err != nil {
		return llm.ModelDetails{}, fmt.Errorf("failed to create show request: %w", err)
	}
	c.applyHeaders(httpReq, true)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return llm.ModelDetails{}, fmt.Errorf("failed to execute show request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return llm.ModelDetails{}, fmt.Errorf("ollama show error (status %d): %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Parameters string `json:"parameters"`
		Details    struct {
			Family            string `json:"family"`
			ParameterSize     string `json:"parameter_size"`
			QuantizationLevel string `json:"quantization_level"`
		} `json:"details"`
		ModelInfo map[string]any `json:"model_info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return llm.ModelDetails{}, fmt.Errorf("failed to decode show response: %w", err)
	}

	details := llm.ModelDetails{
		Name:              name,
		Family:            raw.Details.Family,
		ParameterSize:     raw.Details.ParameterSize,
		QuantizationLevel: raw.Details.QuantizationLevel,
		Parameters:        parseOllamaParameters(raw.Parameters),
	}

	// Extract context length from model_info (key ends with ".context_length")
	for k, v := range raw.ModelInfo {
		if strings.HasSuffix(k, ".context_length") {
			switch n := v.(type) {
			case float64:
				details.ContextLength = int64(n)
			case int64:
				details.ContextLength = n
			}
			break
		}
	}
	// Fallback: parse num_ctx from parameters string
	if details.ContextLength == 0 {
		if v, ok := details.Parameters["num_ctx"]; ok {
			switch n := v.(type) {
			case float64:
				details.ContextLength = int64(n)
			case int64:
				details.ContextLength = n
			}
		}
	}

	return details, nil
}

// parseOllamaParameters parses the Ollama parameters string into a key-value map.
// Each line is "key value", e.g. "num_ctx 32768\nstop \"<|im_end|>\"".
func parseOllamaParameters(raw string) map[string]any {
	result := make(map[string]any)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Try numeric first
		if n, err := strconv.ParseFloat(val, 64); err == nil {
			result[key] = n
		} else {
			result[key] = strings.Trim(val, `"`)
		}
	}
	return result
}

// PullModel downloads a model and reports progress via the channel.
func (c *Client) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	req := map[string]string{"name": name, "stream": "true"}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal pull request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create pull request: %w", err)
	}
	c.applyHeaders(httpReq, true)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to execute pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama pull error (status %d): %s", resp.StatusCode, string(body))
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		var event llm.PullProgress
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("failed to decode pull progress: %w", err)
		}

		select {
		case progress <- event:
		case <-ctx.Done():
			return ctx.Err()
		}

		// Check if done
		if event.Status == "success" {
			return nil
		}
	}
}

type ollamaChatRequest struct {
	Model     string          `json:"model"`
	Messages  []ollamaMessage `json:"messages"`
	Tools     []llm.ToolDef   `json:"tools,omitempty"`
	Format    any             `json:"format,omitempty"`
	Stream    bool            `json:"stream"`
	Think     any             `json:"think,omitempty"`
	KeepAlive string          `json:"keep_alive,omitempty"`
	Options   map[string]any  `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Thinking  string         `json:"thinking,omitempty"`
	ToolCalls []llm.ToolCall `json:"tool_calls,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	Images    []string       `json:"images,omitempty"`
}

type ollamaEmbedRequest struct {
	Model      string         `json:"model"`
	Input      []string       `json:"input"`
	Dimensions *int           `json:"dimensions,omitempty"`
	Truncate   *bool          `json:"truncate,omitempty"`
	KeepAlive  string         `json:"keep_alive,omitempty"`
	Options    map[string]any `json:"options,omitempty"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Embedding  []float64   `json:"embedding"`
}

// toOllamaRequest converts our ChatRequest to Ollama API format.
func (c *Client) toOllamaRequest(req *llm.ChatRequest) ollamaChatRequest {
	ollamaReq := ollamaChatRequest{
		Model:  req.Model,
		Stream: req.Stream,
	}

	// Convert messages
	messages := make([]ollamaMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = ollamaMessage{
			Role:      string(msg.Role),
			Content:   msg.Content,
			Thinking:  msg.Thinking,
			ToolCalls: msg.ToolCalls,
			ToolName:  msg.ToolName,
			Images:    msg.Images,
		}
	}
	ollamaReq.Messages = messages

	// Optional fields
	if len(req.Tools) > 0 {
		ollamaReq.Tools = req.Tools
	}
	if req.Format != nil {
		ollamaReq.Format = req.Format
	}
	if req.Think != nil {
		ollamaReq.Think = req.Think
	}
	if req.KeepAlive != "" {
		ollamaReq.KeepAlive = req.KeepAlive
	}
	if len(req.Options) > 0 {
		ollamaReq.Options = req.Options
	}

	return ollamaReq
}

// ollamaStreamEvent matches the Ollama API streaming response format.
type ollamaStreamEvent struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Message   struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		Thinking  string `json:"thinking,omitempty"`
		ToolCalls []struct {
			Function struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls,omitempty"`
	} `json:"message"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason,omitempty"`
	PromptEvalCount int64  `json:"prompt_eval_count,omitempty"`
	EvalCount       int64  `json:"eval_count,omitempty"`
	TotalDuration   int64  `json:"total_duration,omitempty"`
}

// fromOllamaEvent converts Ollama API event to our StreamEvent format.
func (c *Client) fromOllamaEvent(event ollamaStreamEvent) llm.StreamEvent {
	createdAt, _ := time.Parse(time.RFC3339, event.CreatedAt)

	// Convert tool calls
	toolCalls := make([]llm.ToolCall, len(event.Message.ToolCalls))
	for i, tc := range event.Message.ToolCalls {
		toolCalls[i] = llm.ToolCall{
			Function: struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}

	return llm.StreamEvent{
		Model:     event.Model,
		CreatedAt: createdAt,
		Message: llm.Message{
			Role:      llm.Role(event.Message.Role),
			Content:   event.Message.Content,
			Thinking:  event.Message.Thinking,
			ToolCalls: toolCalls,
		},
		Done:            event.Done,
		DoneReason:      event.DoneReason,
		PromptEvalCount: event.PromptEvalCount,
		EvalCount:       event.EvalCount,
		TotalDuration:   event.TotalDuration,
	}
}
