package credentials

import (
	"context"
	"testing"
)

func TestRememberReplacesEnvAndOnlyPersistsOnRequest(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "env-key")
	store := &fakeStore{}
	r := &Resolver{Store: store}
	if err := r.Remember("  new-key ", false); err != nil {
		t.Fatal(err)
	}
	if store.setCalls != 0 {
		t.Fatalf("persisted without save: %d", store.setCalls)
	}
	key, src, err := r.Resolve(context.Background(), ResolveOptions{})
	if err != nil || key != "new-key" || src != KeySourceSession {
		t.Fatalf("got %q %v %v", key, src, err)
	}
	if err := r.Remember("saved-key", true); err != nil || store.setCalls != 1 {
		t.Fatalf("err=%v sets=%d", err, store.setCalls)
	}
	if err := r.Remember("  ", true); err == nil {
		t.Fatal("blank key accepted")
	}
}
