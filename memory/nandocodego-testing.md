---
name: nandocodego-testing
description: Test layout standards, race detector rules, mock boundaries, time injection, and coverage expectations for nandocodego
type: feedback
---

## Test Layout

| Type | File pattern | Build tag | Tooling |
|---|---|---|---|
| Unit | `<file>_test.go` co-located | none | stdlib + `testify/require` |
| Integration | `<file>_integration_test.go` | `//go:build integration` | `testcontainers-go` for Ollama |
| E2E | `e2e/<scenario>_test.go` | `//go:build e2e` | `creack/pty` for PTY-driven REPL |
| Fuzz | `Fuzz<X>` in `_test.go` | none | `go test -fuzz=<X>` |
| Bench | `Benchmark<X>` in `_test.go` | none | `go test -bench=<X>` |

## Rules

- **Mock at outermost boundary** — for LLM, mock `llm.Client` interface; never mock `net/http` directly
- **Real filesystem** — `t.TempDir()` is hermetic and fast; use it for file tools
- **Time injection** — inject `Clock` interface (`Now() time.Time`, `After(d) <-chan time.Time`) wherever timing matters; never call `time.Now()` directly in tested code
- **No network in unit tests** — no real Ollama, no real HTTP
- **No real home dir** — `t.Setenv("HOME", t.TempDir())` for path tests
- **Race detector mandatory** — `go test -race ./...` in CI; `goleak.VerifyTestMain` in long-running suites
- **Fake llm.Client** — all test implementations must include `ShowModel(context.Context, string) (llm.ModelDetails, error)` returning `llm.ModelDetails{}, nil`

## Acceptance Gates per Phase

- `go test -race ./...` passes
- `golangci-lint run` clean
- No new dep without `tools/allowed-deps.txt` entry
- Public symbols documented with `// Foo does X.`
- New tools registered in `internal/tools/builtin/`

## Key Test Patterns

```go
// Fake LLM client skeleton
type fakeLLMClient struct{}
func (f *fakeLLMClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) { ... }
func (f *fakeLLMClient) ChatOnce(ctx context.Context, req *llm.ChatRequest) (*llm.StreamEvent, error) { ... }
func (f *fakeLLMClient) Embed(ctx context.Context, req llm.EmbedRequest) ([][]float32, error) { return nil, nil }
func (f *fakeLLMClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (f *fakeLLMClient) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error { return nil }
func (f *fakeLLMClient) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) { return llm.ModelDetails{}, nil }
func (f *fakeLLMClient) Close() error { return nil }
```
