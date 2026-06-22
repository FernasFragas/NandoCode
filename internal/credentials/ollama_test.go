package credentials

import (
	"context"
	"errors"
	"testing"
)

type fakeStore struct {
	getValue string
	getErr   error
	setErr   error
	setCalls int
}

func (f *fakeStore) Get(_, _ string) (string, error) { return f.getValue, f.getErr }
func (f *fakeStore) Set(_, _ string, _ string) error {
	f.setCalls++
	return f.setErr
}
func (f *fakeStore) Delete(_, _ string) error { return nil }

func TestResolveSessionBeforeEnv(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "env-key")
	r := &Resolver{Store: &fakeStore{}}
	r.SetSessionKey("session-key")
	got, src, err := r.Resolve(context.Background(), ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "session-key" || src != KeySourceSession {
		t.Fatalf("got=%q src=%q", got, src)
	}
}

func TestResolveEnvBeforeKeychain(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "env-key")
	r := &Resolver{Store: &fakeStore{getValue: "keychain-key"}}
	got, src, err := r.Resolve(context.Background(), ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "env-key" || src != KeySourceEnv {
		t.Fatalf("got=%q src=%q", got, src)
	}
}

func TestResolveKeychainWhenNoSessionOrEnv(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	r := &Resolver{Store: &fakeStore{getValue: "keychain-key"}}
	got, src, err := r.Resolve(context.Background(), ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "keychain-key" || src != KeySourceKeychain {
		t.Fatalf("got=%q src=%q", got, src)
	}
}

func TestResolvePromptOnlyWhenAllowed(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	promptCalls := 0
	prompt := func(context.Context, Prompt) (PromptResult, error) {
		promptCalls++
		return PromptResult{Key: "k"}, nil
	}
	r := &Resolver{Store: &fakeStore{}}
	_, _, err := r.Resolve(context.Background(), ResolveOptions{AllowPrompt: false, Prompt: prompt})
	if !errors.Is(err, ErrCredentialRequired) {
		t.Fatalf("expected credential required, got %v", err)
	}
	if promptCalls != 0 {
		t.Fatalf("promptCalls=%d", promptCalls)
	}
}

func TestResolvePromptCancel(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	r := &Resolver{Store: &fakeStore{}}
	_, _, err := r.Resolve(context.Background(), ResolveOptions{
		AllowPrompt: true,
		Prompt: func(context.Context, Prompt) (PromptResult, error) {
			return PromptResult{Canceled: true}, nil
		},
	})
	if !errors.Is(err, ErrCredentialCanceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
}

func TestResolveUseOnceDoesNotWriteKeychain(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	fs := &fakeStore{}
	r := &Resolver{Store: fs}
	got, src, err := r.Resolve(context.Background(), ResolveOptions{
		AllowPrompt: true,
		Prompt: func(context.Context, Prompt) (PromptResult, error) {
			return PromptResult{Key: "once-key", Save: false}, nil
		},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "once-key" || src != KeySourceSession {
		t.Fatalf("got=%q src=%q", got, src)
	}
	if fs.setCalls != 0 {
		t.Fatalf("setCalls=%d", fs.setCalls)
	}
}

func TestResolveSaveWritesKeychainAndCachesSession(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	fs := &fakeStore{}
	r := &Resolver{Store: fs}
	if _, _, err := r.Resolve(context.Background(), ResolveOptions{
		AllowPrompt: true,
		Prompt: func(context.Context, Prompt) (PromptResult, error) {
			return PromptResult{Key: "save-key", Save: true}, nil
		},
	}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if fs.setCalls != 1 {
		t.Fatalf("setCalls=%d", fs.setCalls)
	}
	got, src, err := r.Resolve(context.Background(), ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() second error = %v", err)
	}
	if got != "save-key" || src != KeySourceSession {
		t.Fatalf("got=%q src=%q", got, src)
	}
}

func TestResolveKeychainUnavailableStillAllowsUseOnce(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	fs := &fakeStore{getErr: errors.New("keychain unavailable")}
	r := &Resolver{Store: fs}
	got, src, err := r.Resolve(context.Background(), ResolveOptions{
		AllowPrompt: true,
		Prompt: func(context.Context, Prompt) (PromptResult, error) {
			return PromptResult{Key: "once-key", Save: false}, nil
		},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "once-key" || src != KeySourceSession {
		t.Fatalf("got=%q src=%q", got, src)
	}
}
