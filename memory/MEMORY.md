# nandocodego Project Status

`nandocodego` has successfully completed its core agent runtime and interface through Phase 16, alongside later runtime improvements, tool ecosystem expansion, content compaction, inline completion, and multi-agent coordination. The codebase is functionally complete, passing all automated tests and security policy checks, with active work focused on closing live acceptance gates for earlier memory, hooks, and MCP phases, and finalizing manual TUI evidence. The immediate roadmap ahead requires implementing Phase 25 (Remote/Bridge Mode) to support detached and remote sessions, followed by Phase 17 (Distribution and Install) for packaging, and concluding with Phase 18 (Hardening, Evals, and Docs) to deliver the v0.1.0 release candidate.

## Memory Index

- [Architecture](nandocodego-architecture.md) — six abstractions, Ollama constraints, keep_alive, NDJSON streaming, 32 tool cap, layered diagram
- [Constraints](nandocodego-constraints.md) — what must never be done, anti-patterns list, concurrency hygiene rules
- [Conventions](nandocodego-conventions.md) — naming rules, package layout, tooling choices, ID format, source tree
- [Phases](nandocodego-phases.md) — delivery phase status (0-21), which are done, active work areas, exit gates
- [Tool interface](nandocodego-tool-interface.md) — Tool contract, tools.Context, permission resolution, built-in tools, llm.Client interface
- [Testing](nandocodego-testing.md) — test layout, race detector rules, mock boundaries, fake client skeleton, time injection
- [Error & config](nandocodego-error-config.md) — error handling, config source priority, path resolution, logging policy, security constraints
