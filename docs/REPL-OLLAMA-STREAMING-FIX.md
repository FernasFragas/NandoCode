# REPL Ollama Streaming Fix

Date: 2026-05-03

## Summary

This note records the fixes made after the REPL was able to call Ollama but did not show assistant output reliably.

The issue was not one single bug. It crossed three boundaries:

- `internal/tui` was submitting stale message history to the agent.
- Raw Ollama streaming output was being inspected as if it were plain JSON, even though `/api/chat` streams NDJSON over HTTP chunked transfer.
- The TUI transcript could hold valid assistant deltas without reliably placing them after the current user prompt or scrolling the viewport to the newest response.

## Symptoms

### `done_reason: "load"`

Ollama returned a successful response like:

```json
{"message":{"role":"assistant","content":""},"done":true,"done_reason":"load"}
```

For Ollama chat, this means the request loaded the model but did not ask it to generate an answer. In this repo, the first REPL prompt could do that because `internal/tui/app.go` read `appState := m.store.Get()` before appending the submitted user message, then used the stale `appState.Messages` slice to build `agent.Input`.

The fix builds `updatedMessages` first and passes that same slice to both app state and `agent.Input`.

### Raw chunked NDJSON looked malformed

The raw response included lines such as `8d`, `80`, `12f`, and a final `0`. Those are HTTP chunk sizes, not JSON. Go's `net/http` client removes that transfer framing before callers read `resp.Body`.

The correct parser is still:

```go
decoder := json.NewDecoder(resp.Body)
for {
	var event ollamaStreamEvent
	if err := decoder.Decode(&event); err != nil {
		// handle EOF or stream error
	}
}
```

If debugging the stream, inspect decoded `ollamaStreamEvent` or `llm.StreamEvent` values. Do not parse a copied raw HTTP transcript as JSON.

### Assistant text streamed but was not visible in the REPL

The agent emitted `AssistantTextDelta` for `event.Message.Content`, but the TUI had two display risks:

- `AppendAssistantDelta` searched backward for any previous assistant item. After a later user prompt, that could append the new response to an old assistant transcript item.
- The viewport was not explicitly refreshed and moved to the bottom after transcript changes.

The fix makes assistant and thinking deltas append only to the current last transcript item. If the last item is not the same kind, a new transcript item is created. The TUI now refreshes viewport content and follows the bottom after prompt submissions and agent events.

## Code Changes

- `internal/llm/ollama/ollama.go`
  - Replaced the top-level `map[string]any` Ollama request with typed `ollamaChatRequest` and `ollamaMessage` structs.
  - This preserves the JSON shape and makes debugger inspection less fragile with newer Go map internals.

- `internal/tui/app.go`
  - Builds `updatedMessages` from the current prompt before starting the agent.
  - Passes `updatedMessages` into `agent.Input`.
  - Refreshes and scrolls the viewport after transcript changes.

- `internal/tui/transcript.go`
  - Appends assistant/thinking deltas only to the last transcript item when it is the same kind.
  - Starts a new item after a user prompt, tool item, retry notice, or system item.

- Tests added or extended:
  - `internal/tui/app_test.go`
  - `internal/tui/slash_test.go`

## Verification

The targeted regression suite is:

```sh
go test ./internal/llm/... ./internal/tui
```

This verifies:

- LLM package behavior still compiles and tests pass.
- A submitted prompt is included in `agent.Input.Messages`.
- Assistant deltas appear in the rendered TUI view.
- New assistant and thinking output starts after the latest user prompt instead of mutating older transcript entries.

## Documentation Decision

Yes, documentation needed an update.

This bug was easy to misdiagnose because the symptoms looked like an Ollama/API issue, an HTTP parsing issue, and a TUI rendering issue at different points. A future engineer or agent needs the cross-layer explanation, not just code comments.

The documentation was updated in two places:

- This file, `docs/REPL-OLLAMA-STREAMING-FIX.md`, is the human-facing troubleshooting and change record.
- `.codex/agent-context/learnings-memory.md` now has an agent-facing lesson for future automated work.

The large `.codex/go-ollama-plan-AGENTS.md` and `.codex/go-ollama-plan-HUMANS.md` files were not edited because they are phase plans and target architecture references. This was a post-Phase-7 bugfix and operational learning, so a targeted note plus agent-context memory is the lower-drift documentation shape.
