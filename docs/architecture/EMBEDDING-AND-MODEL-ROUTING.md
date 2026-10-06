# Embedding And Model Routing Rules

This document describes the current Go/Ollama implementation. Last checked against code 2026-10-06; see "Known Gaps" at the end for places where code does not yet meet the rules below.

## Short Version

`nandocodego` is local-first. Normal chat, tool use, and embeddings use the
configured local Ollama daemon unless the user explicitly selects a cloud-only
Ollama model and provides credentials.

Current model paths:

- Local chat and tools: configured local Ollama base URL, default
  `http://localhost:11434`.
- Direct Ollama Cloud chat: `https://ollama.com`, only after model resolution
  selects cloud and `OLLAMA_API_KEY` or a keychain credential is available.
- Semantic embeddings: intended to be local Ollama embedding calls through
  `POST /api/embed` using `semantic_index.model`, default `qwen3-embedding:8b`
  (see Known Gaps: today they follow the active runtime client).

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
- `internal/credentials`: session/env/keychain credential resolution for
  Ollama Cloud API keys (interactive entry: TUI today, browser in roadmap step B2).

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
- tool schemas are omitted (`ToolModeNone`) when `internal/retrievalroute` classifies
  the prompt as general chat; this only happens when semantic retrieval is enabled
  and in `auto` mode, and the classifier is a keyword list (see Known Gaps),
- output budget defaults are larger, while length-retry behavior is preserved.

When tools are needed, the agent loop still routes model tool calls through the
permission system, hooks, tool execution, and follow-up model turns.

## Semantic Index And Embeddings

Phase 28 made embeddings a first-class local retrieval feature. Phase 29 added
TUI progress visibility for long index operations.

Current surfaces:

- CLI: `nandocodego index build|refresh|status|clear`
- TUI only: `/semantic on|off|auto|explicit|status|deep` and `/index build|refresh|status|clear`
- Browser: semantic retrieval runs automatically per prompt (the server
  emits semantic events); there are no index or semantic controls yet, and
  `/semantic deep` has no browser equivalent. Index build/status endpoints are
  roadmap step B2.

The semantic index stores local cache data under the app cache directory. It
records manifest, record, and vector files keyed by workspace/model/schema/
embedding-dimension metadata. Index build/refresh scans workspace files, extracts records, embeds
batched text, and writes cache files atomically.

Retrieval behavior:

- `semantic_index.mode = "auto"` lets route policy decide when semantic
  evidence should be attached.
- `semantic_index.mode = "explicit"` limits retrieval to explicit controls, but
  prompts with explicit mentions and related-code wording still get light
  semantic retrieval.
- `/semantic deep` applies broader retrieval to the next prompt only.
- Light-mode retrieval narrows candidates when the route asks for current-path
  weighting (related-context prompts).
- Missing, disabled, incompatible (schema version), or model-missing indexes
  degrade with visible fallback messages instead of blocking normal prompts.
  The `stale` and `deadline` route reasons exist but are not produced yet.

## User-Facing Privacy Boundary

Local model and embedding traffic stays on the configured local Ollama endpoint.
If a user chooses a direct Ollama Cloud model, chat prompts, attached file
context, tool results, memory snippets, semantic evidence, and project metadata
needed for that run can be sent to Ollama Cloud after credential consent.

API keys are resolved in this order:

1. session memory,
2. `OLLAMA_API_KEY`,
3. OS keychain (`service: nandocodego`, `account: ollama.com`),
4. interactive entry when prompting is allowed: the TUI masked prompt today;
   browser key entry is roadmap step B2.

Keys are redacted from logs, telemetry, transcripts, prompt dumps, and config
files.

## Known Gaps (verified 2026-10-06)

From the code review in `docs/reports/investigations/CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md`. These are code bugs against the rules above, not doc choices.

- **Embeddings follow the active model.** `semantic.LLMEmbedder` wraps the `llm.RuntimeClient` (`internal/server/server.go:229`, `internal/cli/repl.go:526`, `internal/cli/index.go:145`), and `RuntimeClient.Embed` delegates to the current client (`internal/llm/router.go:63`). After switching to a cloud model, semantic embedding calls go to Ollama Cloud, contradicting the local-embedding rule above (memory side-queries also use the active model, which matches the privacy boundary). Fix: bind the embedder to the local client.
- **Tool-mode routing is a substring keyword list.** `isWorkspaceDiscoveryPrompt` (`internal/retrievalroute/route.go:244`) misses investigation wording ("where is the endpoint…", "explain the agent event loop"), so such prompts lose tools and the model can hallucinate; it also matches inside words ("capital" → "api"). See `BACKLOG.md` §5 (P0).
- **Index refresh after an embedding-model change** relabels old vectors with the new model instead of re-embedding (`internal/semantic/service.go`).
- **Light/full/deep limits** are defined twice with different defaults (`internal/semantic/config.go`, `internal/retrievalroute/route.go`).
