package modelruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/credentials"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/llm/modelresolver"
	"github.com/FernasFragas/nandocodego/internal/llm/ollama"
)

var (
	ErrModelNotFound       = modelresolver.ErrModelNotFound
	ErrCloudDisabled       = modelresolver.ErrCloudDisabled
	ErrCredentialRequired  = credentials.ErrCredentialRequired
	ErrCredentialCanceled  = credentials.ErrCredentialCanceled
	ErrUnauthorized        = errors.New("ollama cloud unauthorized")
	ErrForbidden           = errors.New("ollama cloud forbidden")
	ErrRateLimited         = errors.New("ollama cloud rate limited")
	ErrProviderUnavailable = errors.New("ollama cloud unavailable")
)

// Service orchestrates model resolution, credentials, and runtime switching.
type Service struct {
	LocalClient    llm.Client
	LocalBaseURL   string
	Runtime        *llm.RuntimeClient
	Resolver       *modelresolver.Resolver
	Creds          *credentials.Resolver
	NewCloudClient func(baseURL, apiKey string) llm.Client
}

// SwitchOptions controls model switch behavior.
type SwitchOptions struct {
	RequestedModel string
	AllowPrompt    bool
	Prompt         credentials.PromptFunc
}

// SwitchResult describes a model switch outcome.
type SwitchResult struct {
	Resolved      llm.ResolvedModel
	CredentialSrc credentials.KeySource
	UsedCloud     bool
	Message       string
}

// Resolve resolves the requested model to a provider without switching.
func (s *Service) Resolve(ctx context.Context, requested string) (llm.ResolvedModel, error) {
	if s == nil || s.Resolver == nil {
		return llm.ResolvedModel{}, errors.New("model runtime resolver unavailable")
	}
	return s.Resolver.Resolve(ctx, requested)
}

// Switch resolves and activates the requested model.
func (s *Service) Switch(ctx context.Context, opts SwitchOptions) (SwitchResult, error) {
	if s == nil || s.Runtime == nil || s.Resolver == nil || s.LocalClient == nil {
		return SwitchResult{}, errors.New("model runtime service unavailable")
	}
	resolved, err := s.Resolve(ctx, opts.RequestedModel)
	if err != nil {
		return SwitchResult{}, err
	}
	if resolved.Provider == llm.ProviderOllamaLocal {
		s.Runtime.Switch(s.LocalClient, llm.ProviderOllamaLocal, s.LocalBaseURL)
		return SwitchResult{
			Resolved:      resolved,
			CredentialSrc: credentials.KeySourceNone,
			UsedCloud:     false,
			Message:       "[Switched to local model: " + resolved.Model + "]",
		}, nil
	}

	if s.Creds == nil {
		return SwitchResult{}, ErrCredentialRequired
	}
	key, src, err := s.Creds.Resolve(ctx, credentials.ResolveOptions{
		AllowPrompt: opts.AllowPrompt,
		Prompt:      opts.Prompt,
		PromptData: credentials.Prompt{
			Provider: string(llm.ProviderOllamaCloudAPI),
			Model:    resolved.Model,
		},
	})
	if err != nil {
		return SwitchResult{}, err
	}
	newCloudClient := s.NewCloudClient
	if newCloudClient == nil {
		newCloudClient = func(baseURL, apiKey string) llm.Client {
			return ollama.NewClientWithOptions(ollama.Options{
				BaseURL: baseURL,
				APIKey:  apiKey,
			})
		}
	}
	cloudClient := newCloudClient(llm.OllamaCloudBaseURL, key)
	if err := validateCloudCredential(ctx, cloudClient); err != nil {
		return SwitchResult{}, err
	}
	s.Runtime.Switch(cloudClient, llm.ProviderOllamaCloudAPI, llm.OllamaCloudBaseURL)
	return SwitchResult{
		Resolved:      resolved,
		CredentialSrc: src,
		UsedCloud:     true,
		Message:       "[Switched to Ollama Cloud model: " + resolved.Model + "]",
	}, nil
}

func validateCloudCredential(ctx context.Context, client llm.Client) error {
	_, err := client.ListModels(ctx)
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "status 401"):
		return fmt.Errorf("%w: invalid or missing OLLAMA_API_KEY", ErrUnauthorized)
	case strings.Contains(msg, "status 403"):
		return fmt.Errorf("%w: key cannot access requested Ollama Cloud model", ErrForbidden)
	case strings.Contains(msg, "status 429"):
		return fmt.Errorf("%w: Ollama Cloud rejected request", ErrRateLimited)
	case strings.Contains(msg, "status 5"):
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	default:
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
}

// ListLocal lists local daemon models.
func (s *Service) ListLocal(ctx context.Context) ([]llm.ModelInfo, error) {
	if s == nil || s.LocalClient == nil {
		return nil, errors.New("local model client unavailable")
	}
	return s.LocalClient.ListModels(ctx)
}

// ListCloud lists direct Ollama Cloud models.
func (s *Service) ListCloud(ctx context.Context) ([]llm.ModelInfo, error) {
	if s == nil || s.Resolver == nil {
		return nil, errors.New("cloud resolver unavailable")
	}
	if !s.Resolver.CloudEnabled {
		return nil, ErrCloudDisabled
	}
	return s.Resolver.ListCloud(ctx)
}

// PullLocal pulls a model using the local daemon client.
func (s *Service) PullLocal(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	if s == nil || s.LocalClient == nil {
		return errors.New("local model client unavailable")
	}
	return s.LocalClient.PullModel(ctx, name, progress)
}
