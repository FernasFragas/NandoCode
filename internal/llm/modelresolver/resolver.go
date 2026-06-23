package modelresolver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

var (
	ErrModelNotFound = errors.New("model not found")
	ErrCloudDisabled = errors.New("ollama cloud disabled")
)

// Resolver resolves model origin between local Ollama and direct Ollama Cloud.
type Resolver struct {
	LocalClient  llm.Client
	CloudClient  llm.Client
	CloudEnabled bool

	SuccessTTL time.Duration
	ErrorTTL   time.Duration
	Now        func() time.Time

	mu            sync.Mutex
	cachedCloud   []llm.ModelInfo
	cachedCloudAt time.Time
	cacheErr      error
	cacheErrAt    time.Time
}

// Resolve returns a local/cloud model decision for a requested model name.
func (r *Resolver) Resolve(ctx context.Context, requested string) (llm.ResolvedModel, error) {
	name := strings.TrimSpace(requested)
	if name == "" {
		return llm.ResolvedModel{}, fmt.Errorf("%w: empty model name", ErrModelNotFound)
	}
	if trimmed, ok := trimColonCloudSuffix(name); ok {
		return r.resolveCloudOnly(ctx, name, trimmed, "trimmed final :cloud suffix")
	}
	localModels, err := r.LocalClient.ListModels(ctx)
	if err != nil {
		return llm.ResolvedModel{}, fmt.Errorf("failed to list local models: %w", err)
	}
	if hasModel(localModels, name) {
		return llm.ResolvedModel{
			RequestedName: name,
			Model:         name,
			Origin:        llm.ModelOriginLocal,
			Provider:      llm.ProviderOllamaLocal,
		}, nil
	}
	if !r.CloudEnabled {
		return llm.ResolvedModel{}, fmt.Errorf("%w: model %s not found locally", ErrCloudDisabled, name)
	}
	cloudModels, err := r.ListCloud(ctx)
	if err != nil {
		return llm.ResolvedModel{}, err
	}
	if hasModel(cloudModels, name) {
		return llm.ResolvedModel{
			RequestedName: name,
			Model:         name,
			Origin:        llm.ModelOriginOllamaCloudAPI,
			Provider:      llm.ProviderOllamaCloudAPI,
			BaseURL:       llm.OllamaCloudBaseURL,
		}, nil
	}
	if trimmed, ok := trimCloudSuffix(name); ok && hasModel(cloudModels, trimmed) {
		return llm.ResolvedModel{
			RequestedName: name,
			Model:         trimmed,
			Origin:        llm.ModelOriginOllamaCloudAPI,
			Provider:      llm.ProviderOllamaCloudAPI,
			BaseURL:       llm.OllamaCloudBaseURL,
			AliasUsed:     true,
			AliasReason:   "trimmed final -cloud suffix",
		}, nil
	}
	return llm.ResolvedModel{}, fmt.Errorf("%w: %s", ErrModelNotFound, name)
}

func (r *Resolver) resolveCloudOnly(ctx context.Context, requested, canonical, aliasReason string) (llm.ResolvedModel, error) {
	if !r.CloudEnabled {
		return llm.ResolvedModel{}, fmt.Errorf("%w: model %s requested cloud explicitly", ErrCloudDisabled, requested)
	}
	cloudModels, err := r.ListCloud(ctx)
	if err != nil {
		return llm.ResolvedModel{}, err
	}
	if hasModel(cloudModels, canonical) {
		return llm.ResolvedModel{
			RequestedName: requested,
			Model:         canonical,
			Origin:        llm.ModelOriginOllamaCloudAPI,
			Provider:      llm.ProviderOllamaCloudAPI,
			BaseURL:       llm.OllamaCloudBaseURL,
			AliasUsed:     true,
			AliasReason:   aliasReason,
		}, nil
	}
	if hasModel(cloudModels, requested) {
		return llm.ResolvedModel{
			RequestedName: requested,
			Model:         requested,
			Origin:        llm.ModelOriginOllamaCloudAPI,
			Provider:      llm.ProviderOllamaCloudAPI,
			BaseURL:       llm.OllamaCloudBaseURL,
		}, nil
	}
	return llm.ResolvedModel{}, fmt.Errorf("%w: %s", ErrModelNotFound, requested)
}

// ListCloud returns direct Ollama Cloud API model catalog entries with a short TTL cache.
func (r *Resolver) ListCloud(ctx context.Context) ([]llm.ModelInfo, error) {
	now := r.now()
	successTTL := r.successTTL()
	errorTTL := r.errorTTL()

	r.mu.Lock()
	if len(r.cachedCloud) > 0 && now.Sub(r.cachedCloudAt) < successTTL {
		out := cloneModels(r.cachedCloud)
		r.mu.Unlock()
		return out, nil
	}
	if r.cacheErr != nil && now.Sub(r.cacheErrAt) < errorTTL {
		err := r.cacheErr
		r.mu.Unlock()
		return nil, err
	}
	r.mu.Unlock()

	models, err := r.CloudClient.ListModels(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.cacheErr = fmt.Errorf("failed to list ollama cloud models: %w", err)
		r.cacheErrAt = now
		return nil, r.cacheErr
	}
	r.cacheErr = nil
	r.cachedCloud = cloneModels(models)
	r.cachedCloudAt = now
	return cloneModels(r.cachedCloud), nil
}

func (r *Resolver) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Resolver) successTTL() time.Duration {
	if r.SuccessTTL > 0 {
		return r.SuccessTTL
	}
	return 5 * time.Minute
}

func (r *Resolver) errorTTL() time.Duration {
	if r.ErrorTTL > 0 {
		return r.ErrorTTL
	}
	return 15 * time.Second
}

func hasModel(models []llm.ModelInfo, name string) bool {
	for _, m := range models {
		if m.Name == name {
			return true
		}
	}
	return false
}

func trimCloudSuffix(name string) (string, bool) {
	if !strings.HasSuffix(name, "-cloud") {
		return "", false
	}
	trimmed := strings.TrimSuffix(name, "-cloud")
	if strings.TrimSpace(trimmed) == "" {
		return "", false
	}
	return trimmed, true
}

func trimColonCloudSuffix(name string) (string, bool) {
	if !strings.HasSuffix(name, ":cloud") {
		return "", false
	}
	trimmed := strings.TrimSuffix(name, ":cloud")
	if strings.TrimSpace(trimmed) == "" {
		return "", false
	}
	return trimmed, true
}

func cloneModels(in []llm.ModelInfo) []llm.ModelInfo {
	out := make([]llm.ModelInfo, len(in))
	copy(out, in)
	return out
}
