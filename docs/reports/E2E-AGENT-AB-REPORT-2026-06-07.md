# E2E Agent AB Report - 2026-06-07

## Scope

- Lane: `A` and `B`
- Owner agent: `Coordinator`
- Functional areas:
  - Lane A: CLI bootstrap, config handling, `doctor`, `init`, `version`, `--print`, semantic index CLI
  - Lane B: runtime and model behavior for local and cloud model selection
- Source commit: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- Start/end time:
  - Start: `2026-06-07T10:50:00+01:00`
  - Last updated: `2026-06-07T11:50:00+01:00`

## Environment

- OS: `Darwin 25.5.0 arm64`
- Go version: `go version go1.26.2 darwin/arm64`
- Ollama status:
  - installed client `0.30.5`
  - local daemon reachable only with unsandboxed loopback access from this execution environment
- Model/provider:
  - local: `qwen3.6:35b`
  - embedding: `qwen3-embedding:8b`
  - installed cloud-backed model: `kimi-k2.6:cloud`
- Isolated config/data/cache/state paths:
  - `NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-config.VE8BGL`
  - `NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-data.xcAJfP`
  - `NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-cache.2Ti8Oj`
  - `NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-state.zExYvb`

## Scenario Results

| Scenario | Priority | Automation | Evidence | Status | Bug/Block | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| `A-001` `version` prints build metadata | `p0` | `automated` | `E0` | `pass` |  | `nandocodego 0.0.0-dev (unknown)` |
| `A-002` `doctor` verifies local environment and records current MCP-status behavior | `p0` | `automated` | `E1` | `pass` |  | `doctor` reports roots, writable state under temp dirs, and `MCP Status: Servers: 0` |
| `A-003` `doctor` reports path roots from overridden env vars | `p0` | `automated` | `E1` | `pass` |  | Overridden `NANDOCODEGO_*_HOME` paths surfaced exactly |
| `A-004` `init` creates default config once and handles rerun cleanly | `p0` | `automated` | `E1` | `pass` |  | First run created config; second run reported existing path cleanly |
| `A-005` `--help` and root help text are accurate | `p1` | `automated` | `E0` | `pass` |  | Root help lists `doctor`, `version`, `init`, `index`, `server`; no `connect` |
| `A-006` `--print` runs without TUI | `p0` | `automated` | `E1` | `pass` |  | Direct stdout `ok`; no TUI interaction required |
| `A-007` `--print --json` returns machine-readable output | `p0` | `automated` | `E1` | `pass` |  | JSON contained `content`, `tool_uses`, and `usage` |
| `A-008` config precedence works across defaults, file, and flags | `p1` | `automated` | `E1` | `pass` |  | Invalid default model in config was overridden successfully by `--model qwen3.6:35b` |
| `A-009` invalid config fails with actionable error text | `p1` | `automated` | `E1` | `fail` | [BUG-20260607-invalid-config-warning-does-not-fail-print](./bugs/BUG-20260607-invalid-config-warning-does-not-fail-print.md) | Malformed config only emitted warning and command still completed |
| `A-010` `index build`, `refresh`, `status`, and `clear` work on sample workspace | `p0` | `hybrid` | `E2` | `pass` |  | `status`, `build`, `refresh`, and `clear` all validated on `tiny-go`; `build`/`refresh` required unsandboxed loopback access |
| `B-001` model list succeeds with Ollama available | `p0` | `automated` | `E1` | `pass` |  | `ollama list` and server `/v1/models` both reported installed models |
| `B-002` model list failure is clear when Ollama is unavailable | `p0` | `automated` | `E1` | `pass` |  | `--ollama-url http://127.0.0.1:1` failed with explicit connection-refused error |
| `B-003` `ShowModel`-derived limits affect runtime state | `p1` | `manual` | `E0` | `not_tested` |  | Direct runtime-limit evidence not captured in this run |
| `B-004` small prompt on local model streams successfully | `p0` | `automated` | `E1` | `pass` |  | `--print 'Respond with exactly: ok'` succeeded on `qwen3.6:35b` |
| `B-005` long prompt triggers retry or length handling as expected | `p1` | `manual` | `E0` | `not_tested` |  | Not exercised in this run |
| `B-006` watchdog timeout is surfaced clearly | `p1` | `manual` | `E0` | `not_tested` |  | Not exercised in this run |
| `B-007` cloud-only model selection requests credentials before sending context | `p1` | `manual` | `E0` | `blocked` | [BLOCK-20260607-b007-cloud-credential-gate-not-observable-with-preconfigured-cloud-access](./blocks/BLOCK-20260607-b007-cloud-credential-gate-not-observable-with-preconfigured-cloud-access.md) | Installed cloud access already works; negative credential gate not safely observable |
| `B-008` `--print` with unavailable credential fails non-interactively and clearly | `p1` | `manual` | `E0` | `blocked` | [BLOCK-20260607-b008-unavailable-cloud-credential-path-not-safely-reproducible](./blocks/BLOCK-20260607-b008-unavailable-cloud-credential-path-not-safely-reproducible.md) | Current environment already has working cloud access |
| `B-009` server-mode credential requirement is explicit and non-blocking | `p1` | `automated` | `E1` | `fail` | [BUG-20260607-server-model-endpoint-rejects-listed-cloud-model](./bugs/BUG-20260607-server-model-endpoint-rejects-listed-cloud-model.md) | Server lists `kimi-k2.6:cloud` but rejects switch to it with `400 model not found` |
| `B-010` model switch back to local model clears cloud-only dependency path | `p1` | `automated` | `E1` | `fail` | [BUG-20260607-server-model-endpoint-rejects-listed-cloud-model](./bugs/BUG-20260607-server-model-endpoint-rejects-listed-cloud-model.md) | Cannot validate cloud-to-local switchback because cloud activation via server API fails |

## Coverage Notes

- Functional paths covered:
  - root help/version/doctor/init
  - non-interactive print path for local and installed cloud model
  - config precedence and malformed-config runtime handling
  - semantic index lifecycle on a real fixture
  - installed model inventory and unavailable-Ollama failure path
- Positive paths covered:
  - local `--print`
  - JSON `--print`
  - config override via flag
  - semantic index build/refresh/status/clear
  - installed cloud model in direct print mode
- Negative/error paths covered:
  - malformed config warning-only behavior
  - unreachable Ollama URL connection refusal
  - server cloud model switch inconsistency
- Performance or reliability evidence captured:
  - local print round-trip completed in roughly 3-6 seconds
  - semantic build duration `2.986s`
  - semantic refresh duration `3.283s`
- Known coverage gaps:
  - no direct evidence yet for model-limit propagation, watchdog timeout, or long-prompt retry behavior

## Findings

| Finding | Severity | Disposition | Impacted Scenarios | Evidence |
| --- | --- | --- | --- | --- |
| Malformed config only warns and does not fail the print path | `sev4_low` | `documentation_drift` | `A-009` | malformed config plus successful `--print` |
| Server model endpoint rejects a cloud model that `/v1/models` advertises | `sev2_high` | `confirmed` | `B-009`, `B-010` | direct loopback API repro |

## Bugs

- [BUG-20260607-invalid-config-warning-does-not-fail-print](./bugs/BUG-20260607-invalid-config-warning-does-not-fail-print.md)
- [BUG-20260607-server-model-endpoint-rejects-listed-cloud-model](./bugs/BUG-20260607-server-model-endpoint-rejects-listed-cloud-model.md)

## Blocks

- [BLOCK-20260607-b007-cloud-credential-gate-not-observable-with-preconfigured-cloud-access](./blocks/BLOCK-20260607-b007-cloud-credential-gate-not-observable-with-preconfigured-cloud-access.md)
- [BLOCK-20260607-b008-unavailable-cloud-credential-path-not-safely-reproducible](./blocks/BLOCK-20260607-b008-unavailable-cloud-credential-path-not-safely-reproducible.md)

## Risk Assessment

- Top user-facing risks:
  - server clients can be shown a cloud model they cannot actually select through the session API
  - malformed config may be easier to miss because warning-only behavior allows commands to continue
- Top release risks:
  - server cloud-model workflows are inconsistent across listing and mutation
  - several runtime stress/error branches remain unproven
- Top test-confidence risks:
  - credential-negative cloud branches were not safely reproducible in this environment

## Rerun Recommendation

- Scenarios to rerun immediately:
  - `B-009` and `B-010` after fixing the server model-switch path
- Scenarios to rerun after fixes:
  - `A-009` after deciding whether malformed config should be fatal
  - `B-007` and `B-008` in a credential-clean environment
- Scenarios that need a different environment:
  - negative cloud-credential tests need isolated auth state

## Lane Recommendation

- `blocked`

Rationale: core CLI/bootstrap and direct runtime paths are mostly proven, but one high-severity server/runtime inconsistency is confirmed and key negative cloud-auth branches remain blocked by environment state.
