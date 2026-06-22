package credentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const (
	// DefaultService is the keyring service name for app-level credentials.
	DefaultService = "nandocodego"
	// DefaultAccount is the keyring account key for Ollama Cloud.
	DefaultAccount = "ollama.com"
)

// KeySource identifies where a resolved key came from.
type KeySource string

const (
	KeySourceNone     KeySource = "none"
	KeySourceSession  KeySource = "session"
	KeySourceEnv      KeySource = "env"
	KeySourceKeychain KeySource = "keychain"
)

var (
	ErrCredentialRequired = errors.New("ollama cloud api key required")
	ErrCredentialCanceled = errors.New("ollama cloud credential prompt canceled")
)

// Store abstracts key storage so tests avoid the real OS keychain.
type Store interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

// Prompt describes a credential request prompt.
type Prompt struct {
	Provider string
	Model    string
}

// PromptResult holds prompt output.
type PromptResult struct {
	Key      string
	Save     bool
	Canceled bool
}

// PromptFunc prompts for a cloud credential.
type PromptFunc func(context.Context, Prompt) (PromptResult, error)

// ResolveOptions controls optional interactive resolution.
type ResolveOptions struct {
	AllowPrompt bool
	Prompt      PromptFunc
	PromptData  Prompt
}

// Resolver resolves Ollama cloud credentials from session/env/keychain/prompt.
type Resolver struct {
	Store   Store
	Service string
	Account string

	mu         sync.RWMutex
	sessionKey string
}

type keyringStore struct{}

func (keyringStore) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}
func (keyringStore) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}
func (keyringStore) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

// NewResolver creates a credential resolver with defaults.
func NewResolver() *Resolver {
	return &Resolver{
		Store:   keyringStore{},
		Service: DefaultService,
		Account: DefaultAccount,
	}
}

func (r *Resolver) normalize() {
	if strings.TrimSpace(r.Service) == "" {
		r.Service = DefaultService
	}
	if strings.TrimSpace(r.Account) == "" {
		r.Account = DefaultAccount
	}
	if r.Store == nil {
		r.Store = keyringStore{}
	}
}

// SetSessionKey stores an in-memory key for this process only.
func (r *Resolver) SetSessionKey(key string) {
	r.mu.Lock()
	r.sessionKey = strings.TrimSpace(key)
	r.mu.Unlock()
}

// Resolve resolves a credential by precedence: session, env, keychain, optional prompt.
func (r *Resolver) Resolve(ctx context.Context, opts ResolveOptions) (string, KeySource, error) {
	r.normalize()
	if key := r.getSessionKey(); key != "" {
		return key, KeySourceSession, nil
	}
	if key := strings.TrimSpace(os.Getenv("OLLAMA_API_KEY")); key != "" {
		return key, KeySourceEnv, nil
	}
	if key, err := r.getKeychain(); err == nil && key != "" {
		return key, KeySourceKeychain, nil
	}
	if !opts.AllowPrompt || opts.Prompt == nil {
		return "", KeySourceNone, ErrCredentialRequired
	}
	promptRes, err := opts.Prompt(ctx, opts.PromptData)
	if err != nil {
		return "", KeySourceNone, err
	}
	if promptRes.Canceled {
		return "", KeySourceNone, ErrCredentialCanceled
	}
	key := strings.TrimSpace(promptRes.Key)
	if key == "" {
		return "", KeySourceNone, ErrCredentialRequired
	}
	if promptRes.Save {
		if err := r.Store.Set(r.Service, r.Account, key); err != nil {
			return "", KeySourceNone, fmt.Errorf("failed to save ollama cloud api key to keychain: %w", err)
		}
	}
	r.SetSessionKey(key)
	return key, KeySourceSession, nil
}

func (r *Resolver) getSessionKey() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return strings.TrimSpace(r.sessionKey)
}

func (r *Resolver) getKeychain() (string, error) {
	key, err := r.Store.Get(r.Service, r.Account)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(key), nil
}

// DeleteKeychain removes any persisted keychain key.
func (r *Resolver) DeleteKeychain() error {
	r.normalize()
	err := r.Store.Delete(r.Service, r.Account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
