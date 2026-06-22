# Embedding And Model Routing Rules

This document describes the current Go/Ollama implementation.

## Short Version

`nandocodego` is local-first. Normal chat, tool use, and embeddings use the
configured local Ollama daemon unless the user explicitly selects a cloud-only
Ollama model and provides credentials.

Current model paths:

- Local chat and tools: configured local Ollama base URL, default
  `http://localhost:11434`.
- Direct Ollama Cloud chat: `https://ollama.com`, only after model resolution
  selects cloud and `OLLAMA_API_KEY` or a keychain credential is available.
- Semantic embeddings: local Ollama embedding calls through `POST /api/embed`
  using `semantic_index.model`, default `qwen3-embedding:8b`.

Generic OpenAI-compatible providers are not active in the v0.1 roadmap.

## Provider Routing

The provider-neutral contract is `llm.Client`. Runtime routing is handled by
`llm.RuntimeClient`, which delegates to the currently active client.

Important packages:

- `internal/llm`: shared model, stream, watchdog, retry, provider, and runtime
  router types.
- `internal/llm/ollama`: Ollama local/direct-cloud HTTP client.
- `internal/llm/modelresolver`: local-first model origin resolution.
- `internal/llm/modelruntime`: credential-gated model switching.
- `internal/credentials`: session/env/keychain/TUI credential resolution for
  Ollama Cloud API keys.

Model resolution rules:

1. Local Ollama models win by default.
2. Cloud-only models resolve against the direct Ollama Cloud catalog.
3. `:cloud` and `-cloud` suffixes can force cloud intent and normalize to the
   canonical cloud model name.
4. Cloud switching requires a credential before any project context is packed
   or sent to the cloud model.
5. Canceling or failing the credential flow leaves the previous model/provider
   active.
6. `/pull` always targets the local Ollama daemon.

Non-interactive paths do not prompt. `--print` exits with a credential-required
error for cloud-only models without credentials, and server mode returns a
structured `requires_credential` response.

## Chat Request Shape

The response-time refactor keeps common prompts cheap:

- general prompts can use a chat-only fast path,
- semantic retrieval can be bypassed when route policy says it is unnecessary,
- tool schemas are omitted when tools are not needed,
- output budget defaults are larger, while length-retry behavior is preserved.

When tools are needed, the agent loop still routes model tool calls through the
permission system, hooks, tool execution, and follow-up model turns.

## Semantic Index And Embeddings

Phase 28 made embeddings a first-class local retrieval feature. Phase 29 added
TUI progress visibility for long index operations.

Current surfaces:

- `nandocodego index build`
- `nandocodego index refresh`
- `nandocodego index status`
- `nandocodego index clear`
- `/semantic on|off|auto|explicit|status|deep`
- `/index build|refresh|status|clear`

The semantic index stores local cache data under the app cache directory. It
records manifest, record, and vector files keyed by workspace/model/schema
metadata. Index build/refresh scans workspace files, extracts records, embeds
batched text, and writes cache files atomically.

Retrieval behavior:

- `semantic_index.mode = "auto"` lets route policy decide when semantic
  evidence should be attached.
- `semantic_index.mode = "explicit"` limits retrieval to explicit controls.
- `/semantic deep` applies broader retrieval to the next prompt only.
- Light-mode retrieval narrows candidates for latency-sensitive prompts.
- Missing, stale, disabled, incompatible, or model-missing indexes degrade with
  visible fallback messages instead of blocking normal prompts.

## User-Facing Privacy Boundary

Local model and embedding traffic stays on the configured local Ollama endpoint.
If a user chooses a direct Ollama Cloud model, chat prompts, attached file
context, tool results, memory snippets, semantic evidence, and project metadata
needed for that run can be sent to Ollama Cloud after credential consent.

API keys are resolved in this order:

1. session memory,
2. `OLLAMA_API_KEY`,
3. OS keychain (`service: nandocodego`, `account: ollama.com`),
4. TUI masked prompt when interactive prompting is allowed.

Keys are redacted from logs, telemetry, transcripts, prompt dumps, and config
files.
