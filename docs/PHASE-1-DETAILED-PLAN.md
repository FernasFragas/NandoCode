# Phase 1 Detailed Plan - Repo Scaffolding and Tooling

Date: 2026-05-02
Status: Implemented
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PHASE-3-DETAILED-PLAN.md`

## Goal

Phase 1 establishes the first runnable `nandocodego` binary and turns the Phase 0 security baseline into an enforceable Go project.

The goal is not to build LLM calls, tools, the agent loop, permissions, memory, MCP, hooks, or TUI. Phase 1 should leave a fresh clone able to build, test, lint, print a version, and run a network-free `doctor` command while preserving every Phase 0 guardrail.

## Current Repository Reality

Phase 1 has already been implemented and later phases have built on top of it. This document is therefore not a greenfield implementation plan. It is a final detailed plan plus an audit checklist showing:

- what Phase 1 did complete,
- what later phases back-filled,
- what differs from `.codex/go-ollama-plan-AGENTS.md`,
- what should be fixed before declaring Phase 1 fully aligned with the plan.

## Baseline Analysis From Phases 0-3

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md`
- `tools/allowed-deps.txt`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`
- `tools/verify-phase-0.sh`
- GitHub Actions CI
- Dependabot
- dependency review config
- security issue template
- initial phase log

Phase 1 implications:

- Do not weaken Phase 0 checks to make Go code pass.
- Do not add direct dependencies unless they are allowlisted or explicitly added with rationale.
- Do not add network behavior in Phase 1.
- `doctor` must remain network-free in Phase 1.
- Any CI skip added because `go.mod` was missing should be removed or narrowed once `go.mod` exists.

Current status:

- Security scripts exist.
- Dependency allowlist check passes.
- Network policy check was hardened during Phase 3 to avoid macOS `grep -P` failures.
- `tools/verify-phase-0.sh` runs Go checks when `go.mod` exists.

Remaining Phase 0/1 interface debt:

- CI still contains `TODO(Phase 1)` skip blocks for missing `go.mod`, although `go.mod` now exists.

### Phase 1 - Repo Scaffolding and Tooling

Implemented:

- `go.mod` with module path `github.com/FernasFragas/Nandocode`.
- `go 1.26.2`.
- Cobra CLI package.
- `cmd/nandocodego/main.go`.
- `internal/version`.
- `internal/paths`.
- `internal/logging`.
- `internal/cli/root.go`.
- `internal/cli/doctor.go`.
- `Makefile`.
- `.golangci.yml`.
- `README.md`.
- directory scaffold for future phases.

Back-filled later:

- `internal/cli/root_test.go`
- `internal/paths/paths_test.go`
- `internal/version/version_test.go`

Important deltas from the original Phase 1 plan:

- No `LICENSE` file is present, although Phase log says one was created.
- CLI does not expose planned `Run(ctx, args)` / `ExitCode(err)` API.
- `main.go` does not use `signal.NotifyContext`.
- No `version` subcommand exists.
- `internal/version` uses `Info()` / `FullInfo()` and `CommitSHA`, not `String()` and `Commit`.
- `internal/paths` lacks cache/state/session APIs and `NANDOCODEGO_*` overrides.
- `doctor` does not print cache dir, state dir, Ollama phase status, or security baseline status.
- `doctor` writes directly to stdout.
- Dedicated `doctor_test.go` and `logging_test.go` are missing.
- Makefile lacks explicit `fmt-check`, `vet`, `security`, `verify-phase-0`, and `tidy` targets.

### Phase 2 - LLM Client

Implemented:

- provider-neutral `llm` types,
- Ollama HTTP client,
- streaming chat,
- watchdog,
- retry helpers,
- capabilities,
- chat example,
- LLM tests.

Phase 1 implications from Phase 2:

- Phase 1 network-free `doctor` behavior is still important.
- Phase 2 did not back-fill `doctor --ollama`, so Phase 1 `doctor` should keep clearly saying Ollama is not checked unless a future explicit flag is added.
- Phase 2 debt is documented in `docs/PHASE-LOG.md`; it should not be confused with Phase 1 scope.

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- `internal/tools` API,
- Bash/FileRead/FileWrite,
- built-in tool registry,
- one-shot tool harness,
- Phase 1 smoke tests for CLI, paths, and version,
- network policy checker portability fix.

Phase 1 implications from Phase 3:

- Phase 1 now has some test coverage, but not all planned Phase 1 tests.
- Phase 3 added direct dependency `mvdan.cc/sh/v3`; this is correct for Phase 3, not Phase 1.
- Phase 1 hardening should avoid touching Phase 3 tool behavior unless tests reveal integration breakage.

## Evaluation of the Original Phase 1 Plan

The original Phase 1 plan is strong in intent:

- Preserve Phase 0 security guardrails.
- Build a runnable binary early.
- Keep `doctor` network-free.
- Add deterministic build/test/lint/security commands.
- Keep later-phase features out of Phase 1.
- Add tests for CLI, paths, logging, and version.

The current implementation only partially matches the detailed plan:

- It has a working binary and basic `doctor`.
- It has enough structure for later phases, and Phases 2/3 already compile against it.
- It does not have the planned CLI API shape.
- It does not have the full path API.
- It does not have the planned `doctor` security baseline output.
- It does not have every planned Makefile target.
- It does not have full Phase 1 tests.
- It has stale CI TODO skip blocks.

The best path is not to rewrite the repo from scratch. The right plan is a Phase 1 hardening pass that aligns the existing implementation with the original plan while preserving Phase 2 and Phase 3 behavior.

## Final Phase 1 Scope

In scope:

- `cmd/nandocodego/main.go`
- `internal/cli`
- `internal/version`
- `internal/paths`
- `internal/logging`
- `Makefile`
- `.github/workflows/ci.yml`
- `README.md`
- `docs/PHASE-LOG.md`
- Phase 1 tests
- `LICENSE`

Out of scope:

- LLM client changes except preserving compatibility.
- Tool system changes except preserving tests.
- Agent loop.
- Interactive REPL.
- TUI.
- Permissions system.
- Memory.
- MCP.
- Hooks.
- Network calls from default `doctor`.

## Detailed Target Design

### CLI API

Target API:

```go
func Run(ctx context.Context, args []string) error
func ExitCode(err error) int
```

Recommended command construction:

```go
func newRootCommand(ctx context.Context, out, errOut io.Writer) *cobra.Command
```

Rules:

- `main.go` owns OS signals and `os.Exit`.
- `internal/cli` owns user-facing errors.
- Tests should call `Run(ctx, args)` or command constructors without mutating global `os.Args`.
- Running with no args prints help and exits zero.
- Unknown commands return a non-zero exit code.

Current status:

- [x] Cobra root command exists.
- [x] `--version` works.
- [x] `doctor` command exists.
- [x] `Run(ctx, args)` exists.
- [x] `ExitCode(err)` exists.
- [x] `version` subcommand exists.
- [x] no-args help behavior is tested.
- [x] unknown-command exit behavior is tested.

### Main Entrypoint

Target:

- Use `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`.
- Call `cli.Run(ctx, os.Args[1:])`.
- Call `os.Exit(cli.ExitCode(err))` on error.

Current status:

- [x] `cmd/nandocodego/main.go` exists.
- [x] It delegates to CLI.
- [x] It wires signal context.
- [x] It delegates through `cli.Run`.
- [x] It keeps `os.Exit` only in `main`.

### Version Package

Target API:

```go
var Version = "0.0.0-dev"
var Commit = "unknown"
var BuildTime = "unknown"

func String() string
func Info() string
func FullInfo() string
```

Compatibility recommendation:

- Add `Commit` while preserving `CommitSHA` as an alias or compatibility variable if existing code uses it.
- Add `String()` and keep `Info()` delegating to `String()`.

Current status:

- [x] `Version` exists.
- [x] `CommitSHA` exists.
- [x] `BuildTime` exists.
- [x] `Info()` exists.
- [x] `FullInfo()` exists.
- [x] `Commit` exists.
- [x] `String()` exists.

### Paths Package

Target API:

```go
func ConfigDir() string
func DataDir() string
func CacheDir() string
func StateDir() string
func MemoryDir(gitRoot string) string
func SessionsDir() string
func SessionDir(sessionID string) string
func SkillsDir() string
func ProjectSkillsDir() string
func SanitizePathForDir(path string) string
```

Target env override behavior:

- `NANDOCODEGO_CONFIG_HOME`
- `NANDOCODEGO_DATA_HOME`
- `NANDOCODEGO_CACHE_HOME`
- `NANDOCODEGO_STATE_HOME`
- XDG fallbacks.

Current status:

- [x] `ConfigDir()` exists.
- [x] `DataDir()` exists.
- [x] `MemoryDir()` exists.
- [x] `SessionsDir()` exists.
- [x] `SkillsDir()` exists.
- [x] `ProjectSkillsDir()` exists.
- [x] `CacheDir()` exists.
- [x] `StateDir()` exists.
- [x] `SessionDir(sessionID)` exists.
- [x] exported `SanitizePathForDir()` exists.
- [x] `NANDOCODEGO_*` overrides are implemented.

### Logging Package

Target:

```go
func New(level slog.Level, format string, out io.Writer) *slog.Logger
```

Compatibility recommendation:

- Preserve existing `logging.New(level, format)` if used.
- Add `NewWithWriter` or adjust existing `NewWithWriter` to satisfy the target behavior.
- Avoid global mutation in the package itself.

Current status:

- [x] logging package exists.
- [x] text/json behavior exists.
- [x] writer injection exists via `NewWithWriter`.
- [x] dedicated logging tests exist.
- [x] CLI avoids global logger mutation.

### Doctor Command

Target default behavior:

- No network calls.
- Print version.
- Print Go version.
- Print OS/arch.
- Print config, data, cache, and state dirs.
- Print `ollama: not checked in phase 1` or equivalent.
- Check Phase 0 baseline files.
- Exit non-zero if required Phase 0 baseline files are missing.

Current status:

- [x] `doctor` exists.
- [x] version/Go/OS/arch/config/data output exists.
- [x] default doctor makes no Ollama call.
- [x] cache dir output exists.
- [x] state dir output exists.
- [x] Ollama phase status output exists.
- [x] security baseline file check exists.
- [x] missing baseline file exits non-zero.
- [x] dedicated doctor tests exist.

### Makefile

Target required targets:

- `build`
- `test`
- `test-race`
- `test-integration`
- `test-e2e`
- `lint`
- `fmt`
- `fmt-check`
- `vet`
- `security`
- `verify-phase-0`
- `clean`
- `install`
- `vendor`
- `tidy`

Current status:

- [x] `build`
- [x] `test`
- [x] `test-race`
- [x] `test-integration`
- [x] `test-e2e`
- [x] `lint`
- [x] `fmt`
- [x] `install`
- [x] `clean`
- [x] `vendor`
- [x] `check`
- [x] `fmt-check`
- [x] `vet`
- [x] `security`
- [x] `verify-phase-0`
- [x] `tidy`

### CI

Target:

- Remove Phase 0-era `go.mod` skip blocks once `go.mod` exists.
- Keep security baseline check.
- Run Go build/vet/race on Linux, macOS, Windows.
- Run lint.
- Run security scanners.
- Keep dependency review.

Current status:

- [x] CI exists.
- [x] Go matrix exists.
- [x] security baseline exists.
- [x] lint job exists.
- [x] security scan job exists.
- [x] dependency review exists.
- [x] stale `TODO(Phase 1)` skip blocks removed.

### Tests

Target Phase 1 tests:

- `internal/cli/root_test.go`
- `internal/cli/doctor_test.go`
- `internal/paths/paths_test.go`
- `internal/logging/logging_test.go`
- `internal/version/version_test.go`

Current status:

- [x] `internal/cli/root_test.go`
- [x] `internal/paths/paths_test.go`
- [x] `internal/version/version_test.go`
- [x] `internal/cli/doctor_test.go`
- [x] `internal/logging/logging_test.go`

## Concrete Todos

### A. Confirm Baseline

- [x] Run `tools/check-allowed-deps.sh`.
- [x] Run `tools/check-network-policy.sh`.
- [x] Run `env GOCACHE=/private/tmp/nandocodego-gocache go test ./...`.
- [x] Run `env GOCACHE=/private/tmp/nandocodego-gocache go vet ./...`.
- [x] Run `tools/verify-phase-0.sh`.

### B. Fix Phase 1 Documentation Drift

- [x] Add a Post-Phase-3 audit addendum to `docs/PHASE-LOG.md`.
- [x] Decide whether to keep Phase 1 marked completed with known deltas or change wording to "completed with alignment debt".
- [x] Update Phase 1 log if the missing `LICENSE` is restored.
- [x] Update Phase 1 log after hardening todos are completed.

### C. Restore/Add LICENSE

- [x] Add `LICENSE` if the project still intends to use MIT.
- [x] Use `nandocodego contributors` if an exact owner name is not desired.
- [x] Update `docs/PHASE-LOG.md` to make the current file state accurate.

### D. Align CLI API

- [x] Add `Run(ctx context.Context, args []string) error`.
- [x] Add `ExitCode(err error) int`.
- [x] Add injectable command construction with output and error writers.
- [x] Add `version` subcommand.
- [x] Ensure no-args prints help and exits zero.
- [x] Ensure unknown commands return non-zero without calling `os.Exit` in `internal/cli`.
- [x] Update `cmd/nandocodego/main.go` to use signal context and `cli.Run`.
- [x] Update existing CLI tests.

### E. Align Version Package

- [x] Add `Commit` variable or alias.
- [x] Add `String()`.
- [x] Preserve `CommitSHA`, `Info()`, and `FullInfo()` for existing callers.
- [x] Update ldflags in `Makefile` if needed.
- [x] Extend version tests.

### F. Complete Paths Package

- [x] Add `CacheDir()`.
- [x] Add `StateDir()`.
- [x] Add `SessionDir(sessionID string)`.
- [x] Export `SanitizePathForDir`.
- [x] Implement `NANDOCODEGO_CONFIG_HOME`.
- [x] Implement `NANDOCODEGO_DATA_HOME`.
- [x] Implement `NANDOCODEGO_CACHE_HOME`.
- [x] Implement `NANDOCODEGO_STATE_HOME`.
- [x] Keep existing `SessionsDir()` compatibility.
- [x] Extend path tests for overrides, XDG fallback, sanitization, and empty path behavior.

### G. Complete Doctor

- [x] Print cache dir.
- [x] Print state dir.
- [x] Print Ollama status as not checked unless future explicit flag is used.
- [x] Check Phase 0 files:
  - `SECURITY.md`
  - `tools/allowed-deps.txt`
  - `tools/check-allowed-deps.sh`
  - `tools/check-network-policy.sh`
- [x] Exit non-zero when required Phase 0 files are missing.
- [x] Add dedicated `internal/cli/doctor_test.go`.
- [x] Ensure default doctor remains network-free.

### H. Complete Logging Tests and API Alignment

- [x] Add `internal/logging/logging_test.go`.
- [x] Test text logger writes text.
- [x] Test JSON logger writes JSON.
- [x] Test unknown format falls back to text.
- [x] Confirm package does not mutate global logger.
- [x] Decide whether CLI global logger mutation should remain or be deferred.

### I. Complete Makefile Targets

- [x] Add `fmt-check`.
- [x] Add `vet`.
- [x] Add `security`.
- [x] Add `verify-phase-0`.
- [x] Add `tidy`.
- [x] Update `.PHONY`.
- [x] Update `help` output.
- [x] Keep Docker targets separate and non-blocking.

### J. Clean CI Phase 1 Skips

- [x] Remove `Check for go.mod` skip steps from Go job.
- [x] Remove `if: steps.check_gomod.outputs.exists == 'true'` from Go job steps.
- [x] Remove `Check for go.mod` skip from lint job.
- [x] Remove `Check for go.mod` skip from security-scan job.
- [x] Keep `go-version-file: go.mod`.
- [x] Confirm CI still runs security baseline.

### K. Verify Commands

- [x] `go mod tidy`
- [x] `make fmt`
- [x] `make fmt-check`
- [x] `make test`
- [x] `make test-race`
- [x] `make vet`
- [x] `make build`
- [x] `./bin/nandocodego --version`
- [x] `./bin/nandocodego version`
- [x] `./bin/nandocodego doctor`
- [x] `tools/check-allowed-deps.sh`
- [x] `tools/check-network-policy.sh`
- [x] `tools/verify-phase-0.sh`
- [x] `make lint` if `golangci-lint` is installed; local tool was not installed.
- [x] `make security` if `gosec` and `govulncheck` are installed; local tools were not installed.

## Acceptance Criteria

Phase 1 is fully aligned when:

- [x] Phase 0 verification passes before and after Phase 1 hardening.
- [x] `go.mod` exists with module path `github.com/FernasFragas/Nandocode` and `go 1.26.2`.
- [x] `go.sum` exists.
- [x] `LICENSE` exists or Phase log is corrected to say it does not.
- [x] `cmd/nandocodego/main.go` delegates to `cli.Run` and handles OS interrupt/SIGTERM through context.
- [x] `internal/cli.Run` and `internal/cli.ExitCode` exist.
- [x] `./bin/nandocodego --version` prints a version line.
- [x] `./bin/nandocodego version` prints the same version line.
- [x] `./bin/nandocodego` with no args prints help and exits zero.
- [x] `./bin/nandocodego doctor` prints version, Go version, OS/arch, config dir, data dir, cache dir, state dir, Ollama phase status, and security baseline status.
- [x] `internal/paths` honors `NANDOCODEGO_*` overrides and XDG fallbacks.
- [x] `internal/logging` has text/JSON tests and does not mutate global state itself.
- [x] `README.md` exists.
- [x] `.golangci.yml` exists.
- [x] Makefile includes all required Phase 1 targets.
- [x] Required Phase 1 tests exist and pass.
- [x] `go test ./...` passes.
- [x] `go vet ./...` passes.
- [x] `make lint` passes or missing local tool is documented.
- [x] `make security` passes or missing local tool is documented.
- [x] `tools/check-allowed-deps.sh` passes.
- [x] `tools/check-network-policy.sh` passes.
- [x] `docs/PHASE-LOG.md` has a Phase 1 entry and audit addendum.

## Recommended Execution Order

1. Add/restore `LICENSE`.
2. Add `Run`, `ExitCode`, writer injection, and `version` subcommand.
3. Update `main.go` signal handling.
4. Add version compatibility API.
5. Expand paths API and tests.
6. Expand doctor output and tests.
7. Add logging tests.
8. Add missing Makefile targets.
9. Remove stale CI skip blocks.
10. Run verification.
11. Update Phase log with completed hardening results.

## Risks

- Changing CLI API could break later phase tests. Mitigation: keep `NewRootCmd()` and `Execute()` wrappers while adding `Run`.
- Changing paths defaults could move user data unexpectedly. Mitigation: preserve current defaults where possible and add new explicit overrides without removing existing behavior.
- Doctor security-baseline checks may be sensitive to working directory. Mitigation: make repository-root detection explicit or document that Phase 1 doctor checks the current repo.
- Removing CI skips could break unusual pre-module branches. Mitigation: this repo now has `go.mod`; current mainline should prefer real checks.
- Makefile security targets can fail locally when tools are missing. Mitigation: keep local messages actionable and document installation.
