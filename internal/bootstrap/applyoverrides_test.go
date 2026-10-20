package bootstrap

import (
	"reflect"
	"testing"
)

func TestApplyOverridesWinOverConfig(t *testing.T) {
	initial := DefaultInitial(t.TempDir())
	initial.DefaultModel = "from-config"
	initial.OllamaBaseURL = "http://localhost:11434"
	ApplyOverrides(&initial, Overrides{Model: "flag-model", OllamaURL: "https://ollama.com", NumCtx: 4096})
	if initial.DefaultModel != "flag-model" || initial.OllamaBaseURL != "https://ollama.com" || initial.LLMBaseURL != "https://ollama.com" || initial.NumCtx != 4096 {
		t.Fatalf("overrides not applied: %+v", initial)
	}
	again := initial
	ApplyOverrides(&again, Overrides{})
	if !reflect.DeepEqual(initial, again) {
		t.Fatal("empty overrides modified Initial")
	}
}
