package logging

import "testing"

func TestRedactSkToken(t *testing.T) {
	got := Redact("key=sk-abc123xyz")
	if got != "key=sk-***" {
		t.Fatalf("Redact() = %q", got)
	}
}

func TestRedactBearerToken(t *testing.T) {
	got := Redact("Authorization: Bearer abcdefghijklmnop")
	if got != "Authorization: Bearer ***" {
		t.Fatalf("Redact() = %q", got)
	}
}

func TestRedactAssignment(t *testing.T) {
	got := Redact("TOKEN=mysecretvalue path=/tmp")
	if got != "TOKEN=*** path=/tmp" {
		t.Fatalf("Redact() = %q", got)
	}
}

func TestRedactOllamaAPIKey(t *testing.T) {
	got := Redact("OLLAMA_API_KEY=supersecret")
	if got != "OLLAMA_API_KEY=***" {
		t.Fatalf("Redact() = %q", got)
	}
}

func TestRedactGenericAPIKey(t *testing.T) {
	got := Redact("api_key: abcdefg123")
	if got != "api_key=***" {
		t.Fatalf("Redact() = %q", got)
	}
}
