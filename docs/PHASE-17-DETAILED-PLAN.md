# Phase 17 Detailed Plan - Distribution and Install

Date: 2026-05-07
Status: Pre-implementation plan; penultimate implementation phase
Source plans and references:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`

## Roadmap Placement

Phase 17 is intentionally one of the last two implementation phases. Do not start Phase 17 until all product, runtime, reliability, context-management, and agentic workflow phases that affect the shipped binary are implemented. Ollama Cloud API key support and Phase 24 multi-agent coordination are complete; Phase 25 remote/bridge mode remains required v0.1 work before Phase 17 begins.

The final implementation order is:

1. Complete all feature and runtime reliability phases before release packaging, including Phase 25.
2. Implement Phase 17: distribution, install, release workflow, and release-facing doctor checks.
3. Implement Phase 18 last: final hardening, evals, docs, security review, and v0.1.0 release approval.

Phase 17 must not become a place to hide unfinished product work. It packages and validates the product after the feature surface is stable. If implementation discovers missing core behavior, that behavior should be moved back to the appropriate feature/reliability phase or explicitly listed as a Phase 18 hardening gate.

## Goal

Phase 17 packages `nandocodego` for production distribution across five platforms using `goreleaser`. The mandatory user-visible goal is that any developer can install the tool with a single direct `install.sh` invocation or by downloading a verified GitHub release archive and have a working, correctly-versioned binary on their PATH.

Homebrew and Scoop are preferred package-manager channels, but they are only mandatory for the first Phase 17 implementation if the tap/bucket repositories and release tokens are already configured. If they are not ready, Phase 17 must document the setup and keep GitHub release artifacts plus `install.sh` working.

Deliverables:

- `.goreleaser.yml` producing five-platform builds with embedded version metadata.
- `install.sh` direct-download installer that verifies SHA256 before touching the filesystem.
- `CHANGELOG.md` in Keep a Changelog format covering phases 0–17.
- `.github/workflows/release.yml` CI release workflow triggered by `v*` tags.
- Enhanced `nandocodego doctor` reporting Ollama health, installed models, config paths, MCP server reachability, observability state, and memory stats.
- Homebrew tap formula template or documented deferred publisher setup for `github.com/FernasFragas/homebrew-nandocodego`.
- Scoop bucket manifest template or documented deferred publisher setup.
- Phase log update after implementation.

## Definition Of Success

The Phase 17 exit gate is a five-binary build and install smoke test:

1. Run `goreleaser build --snapshot --clean` locally.
2. Each of the five produced binaries runs `--version` and prints the correct version string.
3. A `checksums.txt` file is generated with SHA256 entries for every archive.
4. `install.sh` downloads a binary, verifies the checksum, and places it in the target directory without running any code before verification.
5. `nandocodego doctor` outputs all documented fields and exits zero by default without network probes.
6. `nandocodego doctor --ollama` exits non-zero when Ollama is unreachable.
7. `nandocodego doctor --mcp` exits non-zero when any enabled and trusted configured MCP server is unreachable.
8. CI release workflow syntax is valid and all pre-release test jobs complete before goreleaser runs.

## Implementation Review Decisions

The following decisions supersede earlier ambiguous wording in this document:

- **Phase ordering:** Phase 17 and Phase 18 are the last phases to implement. Phase 17 is penultimate; Phase 18 is final.
- **Default doctor behavior:** `nandocodego doctor` must not perform network probes by default. It should report Ollama and MCP as `[not checked]` unless the user passes `--ollama` or `--mcp`.
- **MCP doctor fix:** the current `internal/cli/doctor.go` starts MCP connectivity checks unconditionally. Phase 17 must change that behavior so MCP reachability is opt-in behind `--mcp`.
- **Release workflow:** the existing `.github/workflows/ci.yml` is not reusable unless it gains `workflow_call`. Either add `workflow_call` to CI or duplicate the required pre-release checks in `release.yml`. Do not use `uses: ./.github/workflows/ci.yml` unless CI is first made reusable.
- **Package-manager publishing:** Homebrew and Scoop publishing are optional for the first Phase 17 implementation unless the required tap/bucket repositories and tokens already exist. GitHub release artifacts and direct installer support are mandatory. Homebrew/Scoop blocks must either be disabled/omitted until secrets are configured or guarded so missing tokens do not fail the release.
- **Checksum scope:** `checksums.txt` must cover every uploaded release artifact. Do not require raw-binary checksum entries unless raw binaries are also uploaded as release artifacts.
- **Release hardening:** any issue that affects release trust, security, or final docs and cannot be closed in Phase 17 becomes a Phase 18 blocker, not a post-v0.1 TODO.

## Baseline Analysis From Implemented Phases

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md`
- dependency allowlist at `tools/allowed-deps.txt`
- network policy checker at `tools/check-network-policy.sh`
- CI baseline workflow at `.github/workflows/ci.yml`
- dependency review config
- no-secrets policy for logs, memory, telemetry, and test fixtures

Phase 17 implications:

- Release artifacts must not embed secrets. The goreleaser config must not reference credentials other than the `GITHUB_TOKEN` set by the runner environment.
- Signing with cosign is optional for the v0.1 release but must be documented with a clear setup path. If signing is skipped, it must be explicitly documented and not silently omitted.
- The install script must never execute downloaded code before checksum verification. This is a direct policy requirement from Phase 0 security posture.
- Any new direct dependency must be allowlisted before landing. The goreleaser toolchain runs only in CI and does not need to be listed as a Go dependency, but any new Go runtime import does.
- SHA256 checksums are the required format; MD5 and SHA1 are not acceptable under the dependency review config security posture.

### Phase 1 - CLI, Paths, Logging, Scaffold

Implemented:

- `cmd/nandocodego`
- Cobra root command with `--version`, `doctor`, `version`
- `internal/paths` with `ConfigDir()`, `DataDir()`, `CacheDir()`, `StateDir()`, `MemoryDir()`
- `internal/version` with `Version`, `Commit`, `BuildTime` variables injectable via ldflags
- `Makefile` with `build`, `clean`, `install`, `cross-compile` targets

Phase 17 implications:

- goreleaser ldflags must use the same variable names already defined in `internal/version`: `version.Version`, `version.Commit`, `version.BuildTime`.
- goreleaser templates use `.Version`, `.Commit`, and `.Date`, which map correctly to those variables.
- `doctor` already prints config dir, data dir, cache dir, and state dir. Phase 17 extends it without breaking existing output.
- `CacheDir()` and `StateDir()` are available for the enhanced doctor to report alongside the existing config and data directories.
- The `Makefile` `build` target already injects version metadata. goreleaser must produce identical metadata injection; the two paths must not diverge in behavior.

### Phase 2 - LLM Client

Implemented:

- `internal/llm/ollama` Ollama HTTP client
- `ListModels()` returning model names and sizes
- ping/health capability through the `/api/tags` endpoint

Phase 17 implications:

- `doctor` health reporting uses `ListModels()` to detect Ollama reachability and enumerate installed models. This is an existing method and adds no new dependency.
- The doctor call must be gated behind an `--ollama` flag or produce a clear "not checked" annotation when Ollama is not running, so the command does not fail in airgapped environments.
- The existing Ollama URL configuration must flow into the doctor health check from bootstrap state. No hardcoded localhost URL is acceptable in the health probe.

### Phase 3 - Tool Interface And Starter Tools

Implemented:

- `Bash`, `FileRead`, `FileWrite` tools
- Path safety helpers

Phase 17 implications:

- No direct implications for the release packaging path.
- `install.sh` must not use Bash constructs that require non-POSIX extensions. The script targets systems where `/bin/sh` is the only guaranteed shell.
- The script must handle Darwin, Linux (glibc and musl), and produce a clear error message on Windows rather than silently failing.

### Phase 4 - Agent Loop

Implemented:

- `agent.Run(ctx, input) <-chan agent.Event`
- Turn budget, context overflow, retry, and terminal events

Phase 17 implications:

- No direct implications for distribution packaging.
- `doctor` must not invoke the agent loop or require a running model to produce output. All doctor checks are local or gated behind explicit flags.

### Phase 5 - Permission System

Implemented:

- Canonical permission modes
- Source-tagged rules
- Central resolver

Phase 17 implications:

- No direct implications for release packaging.
- `doctor` may report active permission mode from bootstrap but must not enumerate permission rules by default, as they may contain sensitive glob patterns from project configs.

### Phase 6 - State Layer

Implemented:

- `bootstrap.State`
- `state.Store[state.App]`
- XDG path resolution with env overrides

Phase 17 implications:

- `doctor` reads from `bootstrap.Global()` to get all path values for reporting. No new path resolution is needed.
- The enhanced doctor output should show all four XDG directories from `internal/paths` in a single table.
- `NANDOCODEGO_CONFIG_HOME`, `NANDOCODEGO_DATA_HOME`, `NANDOCODEGO_CACHE_HOME`, `NANDOCODEGO_STATE_HOME` overrides are already in place and should be reported in doctor output if set.

### Phase 7 - Bubble Tea TUI and REPL

Implemented:

- Full REPL
- Minimal slash commands
- Permission modal
- Transcript rendering

Phase 17 implications:

- No direct implications for distribution packaging.
- `nandocodego doctor` is a CLI command that runs outside the TUI. Phase 17 must not change the TUI entrypoint.

### Phase 8 - Memory

Implemented:

- `internal/memory` package
- Per-project memory files under `paths.MemoryDir(scopeRoot)`
- Memory runner decorator

Phase 17 implications:

- `doctor` should report the memory directory path and entry count.
- Reading the memory directory count is a local filesystem operation. It must not trigger memory recall or LLM calls.
- If the memory directory does not exist, `doctor` should show "not initialized" rather than an error.

### Phase 9 - Hooks

Implemented:

- Snapshot-based hook system
- User-level command and prompt hooks
- `internal/hooks` package

Phase 17 implications:

- `doctor` should report how many hooks are configured (user + project) but must not execute hooks during the check.
- Hook configuration is loaded from `~/.nandocodego/hooks.json`. If missing, doctor shows "not configured".

### Phase 10 - MCP

Implemented:

- MCP server config loading
- OAuth support
- Trust model

Phase 17 implications:

- `doctor` should report MCP server count and reachability. Reachability probes are network calls and must be gated behind the `--mcp` flag or shown as "not checked" by default.
- MCP server configuration sources from the config file loaded by Phase 13.

### Phase 11 - Sub-agents and Fork

Implemented:

- Sub-agent delegation
- Agent fork / bounded sub-agent runtime

Phase 17 implications:

- No direct implications for distribution packaging.

### Phase 12 - Skills

Implemented:

- Skill frontmatter parsing
- Slash command-registered skills
- Skill sources (user, project, built-in)

Phase 17 implications:

- `doctor` may optionally report the skill count from user and project skill directories but this is low priority for Phase 17.

### Phase 13 - Slash Commands and Config UX

Implemented:

- Full slash command registry
- `~/.nandocodego/config.toml` via koanf
- `/models`, `/pull`, `/memory`, `/hooks`, `/permissions`, `/skills`, `/agents`, `/cost`, `/init`

Phase 17 implications:

- `doctor` should report the config file path and whether it is present.
- Config loading through koanf is already in place. doctor reads from bootstrap, which reflects the loaded config.

### Phase 14 - Tasks

Implemented:

- `TaskCreate`, `TaskList`, `TaskGet`, `TaskOutput`, `TaskStop`
- Task supervisor

Phase 17 implications:

- No direct implications for distribution packaging.

### Phase 15 - Concurrency

Implemented:

- Concurrent and speculative tool execution
- Partition algorithm

Phase 17 implications:

- No direct implications for distribution packaging.

### Phase 16 - Observability and Metrics

Implemented:

- Full observability decorators
- OTEL opt-in instrumentation
- Redaction helpers
- Telemetry endpoint configuration

Phase 17 implications:

- `doctor` should report observability state: whether telemetry is enabled, and if so, the configured endpoint (with the endpoint URL shown but not the auth token).
- Redaction helpers from Phase 16 should be used when printing any config-derived value in doctor output to prevent accidental credential exposure.

## Deep Analysis Of Distribution Requirements

### Why goreleaser

goreleaser is the standard Go multi-platform release tool. The key design properties that make it suitable for this project are:

- Single `.goreleaser.yml` config file that declaratively specifies all targets, archive formats, checksum algorithms, and release note sources.
- Native GitHub Releases integration: on tag push, goreleaser creates the release, uploads all archives, and updates the release notes automatically.
- Homebrew formula generation: goreleaser can generate and push a Homebrew formula to a separate tap repository as part of the release.
- Scoop manifest generation: goreleaser can generate a Scoop bucket JSON file.
- Build matrix: cross-compilation is handled natively using the GOOS/GOARCH environment variables that Go already understands. No Docker cross-compilation is required.
- `goreleaser build --snapshot --clean` produces local binaries for all targets without requiring a tag or GitHub credentials, enabling local testing.

### Why the install script matters

Despite packaging for Homebrew and Scoop, a direct `install.sh` is essential because:

- Users on CI/CD machines or restricted environments may not have Homebrew installed.
- Docker/container users building custom images prefer a single-command binary install.
- Windows users who cannot use Scoop need a PowerShell path (separate `install.ps1` is in scope for future phases; this phase covers POSIX only).
- The install script is the trust boundary for security: it must download, verify, then install. The canonical anti-pattern is `curl | bash`. This script downloads the binary and checksum separately, verifies the checksum, then copies the binary. The script itself can be curl'd and inspected before execution. Execution of the script does not pipe any code into the shell interpreter before the verification step completes.

### Homebrew tap structure

Homebrew expects a Git repository named `homebrew-<name>` under the same GitHub organization. For this project:

- Tap repository: `github.com/FernasFragas/homebrew-nandocodego`
- Installation command: `brew install FernasFragas/nandocodego/nandocodego`
- Formula path inside the tap: `Formula/nandocodego.rb`
- goreleaser generates this formula using its built-in `brews` configuration block, which fills in the correct SHA256, URL, and version automatically on release.
- The formula's `test` block should run `nandocodego --version` to validate the installed binary.

### Version string design

The version string injected at build time must be consistent:

- `version.Version` receives `{{.Version}}`, which goreleaser resolves from the Git tag (e.g., `v0.1.0`).
- `version.Commit` receives `{{.Commit}}`, the short commit SHA.
- `version.BuildTime` receives `{{.Date}}`, the ISO 8601 build timestamp.
- `nandocodego --version` output format: `nandocodego v0.1.0 (abc1234)`.
- `nandocodego version` output: same plus Go version and OS/arch.

### Enhanced doctor design

The enhanced doctor consolidates all system readiness information into a single table that a user can copy-paste into a bug report. The existing doctor prints config dir, data dir, Go version, and OS/arch. Phase 17 extends it with:

- Version line with binary name, version, and commit SHA.
- Go runtime version and OS/arch (already present).
- Config dir, data dir, cache dir, state dir — all four XDG paths with writability status.
- Config file path with `[found]` or `[not found]` annotation.
- Ollama endpoint with reachability and installed model list (gated behind `--ollama` or shown as `[not checked]`).
- MCP server count and reachability (gated behind `--mcp` or shown as `[not checked]`).
- Observability state: `telemetry disabled` or `telemetry enabled → <endpoint>`.
- Memory directory and entry count: `~/.nandocodego/projects/... [12 entries]`.
- Security baseline files: `all baseline files present` or listing missing files.

The `--ollama` flag behavior:

- Without `--ollama`: print `[not checked]` next to the Ollama line.
- With `--ollama`: probe the configured endpoint, print `[reachable, N models]` or `[unreachable]`, exit non-zero if unreachable.

This preserves the existing behavior where `doctor` is always safe to run in airgapped environments.

## Evaluation Of The Original Distribution Plan

The original plan is correct at the product level:

- goreleaser with five targets
- CGO disabled
- trimpath ldflags
- tar.gz for Linux/macOS, zip for Windows
- SHA256 checksums
- CHANGELOG.md
- Homebrew tap
- Scoop bucket
- install.sh with checksum verification

It needs more implementation detail for this repo:

- It does not specify which goreleaser version to pin in CI. Use the `goreleaser-action` at a pinned major version.
- It does not specify how the release CI job is kept separate from the main CI test jobs. The release workflow should be a separate file that requires all test jobs to pass before goreleaser runs.
- It does not specify how to handle a missing cosign configuration gracefully.
- It does not define the exact install.sh detection logic for musl versus glibc Linux.
- It does not specify the exact doctor output format as a testable contract.
- It does not cover the `nandocodego doctor --ollama` flag design.
- It does not define how the Homebrew tap repo is created or initially configured.

## Final Phase 17 Scope

In scope:

- `.goreleaser.yml` with five targets and all required build flags.
- `CHANGELOG.md` covering phases 0–17 in Keep a Changelog format.
- `install.sh` POSIX installer with OS/arch detection and SHA256 verification.
- `.github/workflows/release.yml` triggered by `v*` tags.
- Enhanced `nandocodego doctor` with full system readiness table.
- Homebrew tap formula template if the tap repository and token are ready; otherwise document it as a manual Phase 17 follow-up and keep the release from failing.
- Scoop bucket manifest template if the bucket repository and token are ready; otherwise document it as a manual Phase 17 follow-up and keep the release from failing.
- `docs/PHASE-LOG.md` update.

Out of scope:

- `install.ps1` PowerShell installer for Windows.
- Homebrew tap repository creation (document the steps; a human must create the repo and grant goreleaser push access).
- Scoop bucket repository creation.
- Release signing with cosign (document the setup steps; mark as optional).
- Docs site (Phase 18).
- Eval suite (Phase 18).
- Any new feature work in the agent, memory, hooks, MCP, or skills layers.
- Release automation for pre-release (alpha/beta) tags.

## Architecture

### Package Layout Changes

```text
.goreleaser.yml              # goreleaser configuration
install.sh                   # POSIX direct-download installer
CHANGELOG.md                 # Keep a Changelog

.github/
  workflows/
    ci.yml                   # existing, reference release workflow
    release.yml              # new release workflow

internal/
  cli/
    doctor.go                # enhanced with full table output
    doctor_test.go           # extended tests for new fields
```

No new Go packages. All changes are in configuration files, scripts, and the existing `doctor.go` command.

### goreleaser Configuration Design

```yaml
# .goreleaser.yml

version: 2

project_name: nandocodego

before:
  hooks:
    - go mod tidy
    - go generate ./...

builds:
  - id: nandocodego
    main: ./cmd/nandocodego
    binary: nandocodego
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64
    ldflags:
      - -trimpath
      - -s -w
      - -X github.com/FernasFragas/Nandocode/internal/version.Version={{.Version}}
      - -X github.com/FernasFragas/Nandocode/internal/version.Commit={{.Commit}}
      - -X github.com/FernasFragas/Nandocode/internal/version.BuildTime={{.Date}}

archives:
  - id: nandocodego
    builds:
      - nandocodego
    format_overrides:
      - goos: windows
        format: zip
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

changelog:
  sort: asc
  use: github
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"
      - Merge pull request
      - Merge branch

release:
  github:
    owner: FernasFragas
    name: nandocodego
  draft: false
  prerelease: auto
  name_template: "{{ .ProjectName }} {{ .Tag }}"
  footer: |
    **Full Changelog**: https://github.com/FernasFragas/Nandocode/blob/main/CHANGELOG.md

brews:
  - name: nandocodego
    repository:
      owner: FernasFragas
      name: homebrew-nandocodego
      branch: main
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    commit_author:
      name: goreleaserbot
      email: bot@goreleaser.com
    homepage: "https://github.com/FernasFragas/Nandocode"
    description: "A local-first AI coding agent powered by Ollama"
    license: "MIT"
    test: |
      system "#{bin}/nandocodego --version"
    install: |
      bin.install "nandocodego"

scoops:
  - repository:
      owner: FernasFragas
      name: scoop-nandocodego
    commit_author:
      name: goreleaserbot
      email: bot@goreleaser.com
    homepage: "https://github.com/FernasFragas/Nandocode"
    description: "A local-first AI coding agent powered by Ollama"
    license: "MIT"
```

### Install Script Design

```sh
#!/bin/sh
# install.sh — nandocodego installer
# Usage: sh install.sh [--install-dir DIR] [--version VERSION]
# Downloads from GitHub releases, verifies SHA256, then installs.
# The verification step always runs BEFORE any binary execution.
```

Key behaviors:

- Detect OS with `uname -s` (Linux, Darwin).
- Detect architecture with `uname -m` (x86_64 → amd64, aarch64/arm64 → arm64).
- Default install dir: `/usr/local/bin` if writable; otherwise `~/.local/bin`.
- Accept `--install-dir` and `--version` overrides.
- Download the archive and `checksums.txt` to a temporary directory.
- Verify SHA256 using `sha256sum` or `shasum -a 256`.
- Exit non-zero if verification fails; never copy binary on failure.
- Extract and copy binary.
- Print success with the installed path and version.
- Clean up the temporary directory on exit (trap EXIT).

Anti-patterns deliberately avoided:

- `curl -fsSL https://...install.sh | sh` — the script itself can be downloaded and inspected. It does not pipe into the shell before verification.
- Running `nandocodego --version` during installation before the user has invoked the binary intentionally.
- Silently ignoring checksum failures.
- Writing to system paths without checking writability first.

### Release CI Workflow Design

```yaml
# .github/workflows/release.yml

name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  test:
    # Valid only after .github/workflows/ci.yml gains `workflow_call`.
    # If CI is not made reusable, duplicate release preflight jobs here instead.
    uses: ./.github/workflows/ci.yml

  release:
    needs: [test]
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
```

The `needs: [test]` dependency only works if `.github/workflows/ci.yml` is converted into a reusable workflow by adding `workflow_call` to its `on:` block. The current CI workflow is not reusable as written.

Implementation must choose one of these designs:

1. Add `workflow_call` to `.github/workflows/ci.yml`, then use `jobs.test.uses: ./.github/workflows/ci.yml`.
2. Keep CI unchanged and duplicate the release preflight jobs directly in `.github/workflows/release.yml`.

The first option keeps CI behavior centralized and is preferred if the reusable workflow conversion does not disrupt normal push and pull-request CI.

Either way, goreleaser must run only after build, vet, test, lint, security baseline, gosec, and govulncheck pass.

### Enhanced Doctor Output Format

```
nandocodego doctor

version:          nandocodego v0.1.0 (abc1234)
go:               go1.23.4
os/arch:          darwin/arm64

config dir:       ~/.nandocodego [ok, writable]
data dir:         ~/.local/share/nandocodego [ok, writable]
cache dir:        ~/.cache/nandocodego [ok, writable]
state dir:        ~/.local/state/nandocodego [ok, writable]
config file:      ~/.nandocodego/config.toml [found]

ollama:           http://localhost:11434 [not checked] (use --ollama to check)
models:           (not checked)

security:         all baseline files present
mcp servers:      2 configured [not checked] (use --mcp to check)
observability:    telemetry disabled
memory:           ~/.nandocodego/projects/my-project [12 entries]
hooks:            2 user hooks configured
```

With `--ollama`:

```
ollama:           http://localhost:11434 [reachable, 3 models]
  models:         qwen3:14b, llama3.2:3b, nomic-embed-text
```

With `--ollama` and Ollama unreachable (exit non-zero):

```
ollama:           http://localhost:11434 [UNREACHABLE]
```

## Implementation Plan

### Step 1 — goreleaser Configuration

Files:

- `.goreleaser.yml`

Implement:

- Write the full goreleaser YAML config as designed above.
- Run `goreleaser build --snapshot --clean` locally to verify all five targets build.
- Confirm each binary runs `--version` and prints the expected string.
- Confirm `checksums.txt` is generated.
- Verify no CGO symbols are present in the Linux arm64 binary using `file` command.

Notes:

- goreleaser itself is a CI-time toolchain dependency. It does not need to be in `go.mod`.
- goreleaser version should be pinned in the GitHub Action but kept flexible for local use (`goreleaser build --snapshot --clean` works with any compatible version).
- The `ignore` block must exclude `windows/arm64` because goreleaser would otherwise attempt to build it.

### Step 2 — CHANGELOG.md

Files:

- `CHANGELOG.md`

Implement:

- Keep a Changelog format with sections: `[Unreleased]`, `[0.1.0] - YYYY-MM-DD`.
- The `[Unreleased]` section is always present for future work.
- The `[0.1.0]` section summarizes all phases 0–17:
  - Added: full tool set, memory, hooks, MCP, sub-agents, skills, slash commands, tasks, concurrency, observability, distribution.
  - Fixed: any known bug fixes documented in phase logs.
  - Security: references to SECURITY.md.
- Subsequent entries will be added per release.

### Step 3 — install.sh

Files:

- `install.sh`

Implement:

- Full POSIX-compatible script (shebang `#!/bin/sh`, not `#!/bin/bash`).
- OS detection: `uname -s` → `linux` or `darwin`.
- Arch detection: `uname -m` → `amd64` or `arm64`.
- Error on unsupported OS/arch with a clear message pointing to the GitHub releases page.
- Version resolution: `--version` flag or query the GitHub API for the latest release.
- Install dir resolution: `--install-dir` flag or auto-detect.
- Download to `$(mktemp -d)` using `curl` with `-fsSL --retry 3`.
- Verify SHA256 before any copy.
- Copy and set `chmod 755`.
- Trap to clean up temp dir on EXIT.
- Print installation summary.

Security notes:

- Never pipe into sh.
- Never run the binary before the user explicitly runs it.
- Verify checksum line format against a regex before passing to `sha256sum --check` or `shasum -a 256 -c`.
- The GitHub releases download URL is HTTPS only.

### Step 4 — Release CI Workflow

Files:

- `.github/workflows/release.yml`

Implement:

- Triggered by `v*` tags only.
- `needs: [test]` using the existing CI workflow only after it has `workflow_call`, or duplicate the required checks directly in `release.yml`.
- goreleaser action pinned at v6.
- Environment variables: `GITHUB_TOKEN`; package-manager tokens only when their publishers are enabled.

Note on optional Homebrew step:

- If `HOMEBREW_TAP_GITHUB_TOKEN` or `SCOOP_BUCKET_GITHUB_TOKEN` is not configured in repository secrets, omit or disable those publisher blocks for the first release rather than letting goreleaser fail.
- Document the setup steps for adding the secret.

### Step 5 — Enhanced Doctor Command

Files:

- `internal/cli/doctor.go`
- `internal/cli/doctor_test.go`

Implement new fields:

- Version line: read from `version.Version`, `version.Commit`.
- Config file: check existence of `paths.ConfigDir() + "/config.toml"`.
- Cache dir: use `paths.CacheDir()` with writability check.
- State dir: use `paths.StateDir()` with writability check.
- Ollama section: gated behind `--ollama` flag. Use `llm/ollama.NewClient(url).ListModels(ctx)`.
- Memory section: count `.md` files directly under `paths.MemoryDir(wd)`. No LLM call.
- Hooks section: read hook count from user config path without executing hooks.
- MCP section: read server count from config. Reachability probe gated behind `--mcp` flag.
- Observability section: read from bootstrap `TelemetryEnabled` and `TelemetryEndpoint`.
- Security baseline: check existence of required Phase 0 files (already partially implemented; extend to cover all baseline files).

Exit behavior:

- Without any check flags: always exits zero unless a critical local path is unwritable.
- With `--ollama`: exits non-zero if Ollama is unreachable.
- With `--mcp`: exits non-zero if any enabled and trusted configured MCP server is unreachable. Disabled or untrusted servers should be reported but should not fail the command.

Doctor must remain fast. The only potentially slow operations are `--ollama` and `--mcp` probes, which are opt-in. All filesystem checks must complete within 100ms on a normal local disk.

### Step 6 — Homebrew Tap Bootstrap

Not a Go code change. Required manual steps:

1. Create `github.com/FernasFragas/homebrew-nandocodego` repository.
2. Create `Formula/` directory with a placeholder `nandocodego.rb`.
3. Create a Personal Access Token with `repo` scope for the tap repo.
4. Add it as `HOMEBREW_TAP_GITHUB_TOKEN` in the main repo's GitHub secrets.
5. After first release, goreleaser will update the formula automatically.

These steps are documented in the Phase 17 implementation note but are not automated.

### Step 7 — Scoop Bucket Bootstrap

Not a Go code change. Required manual steps:

1. Create `github.com/FernasFragas/scoop-nandocodego` repository.
2. Create `bucket/` directory.
3. Create a Personal Access Token with `repo` scope.
4. Add it as `SCOOP_BUCKET_GITHUB_TOKEN` in the main repo's GitHub secrets.

goreleaser generates and pushes the manifest JSON on each release.

### Step 8 — Tests and Verification

Required commands:

```sh
goreleaser build --snapshot --clean
ls -la dist/
./dist/nandocodego_darwin_arm64/nandocodego --version
./dist/nandocodego_linux_amd64_v1/nandocodego --version
cat dist/checksums.txt
go test ./internal/cli/...
go vet ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Doctor tests must cover:

- Default output format (no flags): all fields present, no network call.
- `--ollama` flag with a mock/stub Ollama server: reachable path.
- `--ollama` flag with no server: unreachable path, non-zero exit.
- Version field reflects injected ldflags.
- Memory field shows `[not initialized]` when memory dir is absent.
- Security baseline field lists any missing files.

## Implementation Todos

- [ ] Create `.goreleaser.yml` with five-target build matrix (linux/amd64, linux/arm64, darwin/arm64, darwin/amd64, windows/amd64).
- [ ] Set `CGO_ENABLED=0` for all build targets in `.goreleaser.yml`.
- [ ] Set `-trimpath -s -w` ldflags in `.goreleaser.yml`.
- [ ] Set version ldflags: `version.Version`, `version.Commit`, `version.BuildTime` in `.goreleaser.yml`.
- [ ] Configure `.goreleaser.yml` archives: `.tar.gz` for Linux/macOS, `.zip` for Windows.
- [ ] Configure `.goreleaser.yml` checksum with SHA256 algorithm and `checksums.txt` name.
- [ ] Configure `.goreleaser.yml` changelog with `use: github` and exclusion filters.
- [ ] Configure `.goreleaser.yml` release block with GitHub owner, name, and footer pointing to CHANGELOG.md.
- [ ] Configure `.goreleaser.yml` brews block with tap repository, commit author, homepage, description, test, and install stanzas.
- [ ] Configure `.goreleaser.yml` scoops block with bucket repository, commit author, and description.
- [ ] Add `ignore` block in `.goreleaser.yml` to exclude windows/arm64.
- [ ] Ignore build artifacts: add `dist/` to `.gitignore` if not already present.
- [ ] Run `goreleaser build --snapshot --clean` and verify five binaries produced.
- [ ] Run `--version` on each binary and verify version string format.
- [ ] Verify `checksums.txt` contains SHA256 entries for all uploaded release artifacts.
- [ ] Run `file dist/nandocodego_linux_arm64*/nandocodego` and confirm no CGO.
- [ ] Write `CHANGELOG.md` with `[Unreleased]` and `[0.1.0]` sections.
- [ ] Add all phases 0–17 to `[0.1.0]` section as Added entries.
- [ ] Add Security section to `[0.1.0]` referencing SECURITY.md.
- [ ] Write `install.sh` with shebang `#!/bin/sh`.
- [ ] Implement OS detection in `install.sh` using `uname -s`.
- [ ] Implement arch detection in `install.sh` using `uname -m` with x86_64 → amd64, aarch64 → arm64 mapping.
- [ ] Implement `--install-dir` and `--version` flag parsing in `install.sh`.
- [ ] Implement auto-install-dir selection in `install.sh` (`/usr/local/bin` or `~/.local/bin`).
- [ ] Implement version resolution from GitHub API in `install.sh` when `--version` not specified.
- [ ] Implement archive download with curl retry in `install.sh`.
- [ ] Implement `checksums.txt` download in `install.sh`.
- [ ] Implement SHA256 verification using `sha256sum` or `shasum -a 256` in `install.sh`.
- [ ] Implement exit-on-checksum-failure logic in `install.sh`.
- [ ] Implement binary extraction and copy in `install.sh`.
- [ ] Implement `trap EXIT` cleanup in `install.sh`.
- [ ] Test `install.sh` on macOS arm64 with a local mock server.
- [ ] Test `install.sh` on Linux amd64 in a Docker container.
- [ ] Write `.github/workflows/release.yml` triggered by `v*` tags.
- [ ] Either add `workflow_call` to CI and use it from `release.yml`, or duplicate release preflight checks directly in `release.yml`.
- [ ] Add `needs: [test]` dependency in `release.yml` so goreleaser waits for preflight checks.
- [ ] Pin goreleaser-action to v6 in `release.yml`.
- [ ] Set `GITHUB_TOKEN` in `release.yml`.
- [ ] Enable Homebrew/Scoop publishing only when tap/bucket repositories and tokens are configured.
- [ ] Document Homebrew tap setup steps in a comment block in `.goreleaser.yml`.
- [ ] Document Scoop bucket setup steps in a comment block in `.goreleaser.yml`.
- [ ] Enhance `internal/cli/doctor.go` to add version line with `version.Version` and `version.Commit`.
- [ ] Add cache dir and state dir rows to doctor output with writability status.
- [ ] Add config file row to doctor output showing `[found]` or `[not found]`.
- [ ] Add `--ollama` flag to doctor command.
- [ ] Implement Ollama reachability probe in doctor using `llm/ollama.NewClient(url).ListModels(ctx)`.
- [ ] Print `[not checked]` for Ollama line when `--ollama` not specified.
- [ ] Print `[reachable, N models]` and model list when `--ollama` specified and reachable.
- [ ] Print `[UNREACHABLE]` and exit non-zero when `--ollama` specified and unreachable.
- [ ] Add `--mcp` flag to doctor command.
- [ ] Implement MCP server count from config and reachability probe gated behind `--mcp`.
- [ ] Add observability state row to doctor output reading from bootstrap TelemetryEnabled.
- [ ] Redact auth tokens from observability endpoint URL in doctor output using Phase 16 redaction helpers.
- [ ] Add memory directory row to doctor output with `.md` file count.
- [ ] Show `[not initialized]` for memory row when memory directory does not exist.
- [ ] Add hooks configured count to doctor output.
- [ ] Update security baseline row to cover all Phase 0 required files.
- [ ] Write extended `internal/cli/doctor_test.go` covering all new fields.
- [ ] Add doctor test: default output format includes all fields, exits zero.
- [ ] Add doctor test: `--ollama` with reachable server exits zero.
- [ ] Add doctor test: `--ollama` with unreachable server exits non-zero.
- [ ] Add doctor test: memory row shows `[not initialized]` when directory absent.
- [ ] Add doctor test: version line reflects injected ldflags.
- [ ] Run `go test ./internal/cli/...` and confirm all pass.
- [ ] Run `go test -race ./internal/cli/...` and confirm clean.
- [ ] Run `tools/check-allowed-deps.sh` and confirm no new dependencies.
- [ ] Run `tools/check-network-policy.sh` and confirm no unauthorized endpoints.
- [ ] Update `docs/PHASE-LOG.md` with Phase 17 entry.

## Acceptance Criteria

- [ ] `goreleaser build --snapshot --clean` produces exactly five binaries without errors.
- [ ] Each binary's `--version` output matches format `nandocodego v<semver> (<short-sha>)`.
- [ ] Linux/amd64, Linux/arm64, Darwin/amd64, Darwin/arm64 archives are `.tar.gz`.
- [ ] Windows/amd64 archive is `.zip`.
- [ ] `checksums.txt` exists and contains one SHA256 entry per uploaded release artifact.
- [ ] CGO is disabled; `file` confirms no ELF dynamic linking to glibc in Linux binaries.
- [ ] All five binaries have the `-trimpath` flag in effect (no absolute paths in stack traces).
- [ ] `CHANGELOG.md` exists with `[Unreleased]` and at least one versioned section.
- [ ] `install.sh` is executable (`chmod +x`).
- [ ] `install.sh` verifies SHA256 before copying the binary; failure exits non-zero.
- [ ] `install.sh` never pipes code into the shell interpreter before verification.
- [ ] `install.sh` handles Darwin/arm64, Darwin/amd64, Linux/amd64, Linux/arm64 targets.
- [ ] `install.sh` prints a clear error for unsupported platforms.
- [ ] `.github/workflows/release.yml` triggers only on `v*` tags.
- [ ] Release workflow requires all test jobs to pass before goreleaser runs.
- [ ] goreleaser action is pinned at a specific major version.
- [ ] `nandocodego doctor` output includes version, go, os/arch, all four XDG dirs, config file, ollama, security, mcp, observability, memory, and hooks fields.
- [ ] All four XDG dirs show writability status.
- [ ] `nandocodego doctor` exits zero by default (no network probes).
- [ ] `nandocodego doctor --ollama` exits non-zero when Ollama is unreachable.
- [ ] `nandocodego doctor --ollama` lists installed model names when reachable.
- [ ] Doctor memory row shows entry count without LLM calls.
- [ ] Doctor observability row redacts auth tokens from endpoint URLs.
- [ ] Doctor test suite passes with race detector.
- [ ] If Homebrew publishing is enabled, the tap formula template is valid Ruby (goreleaser schema validates it).
- [ ] If Scoop publishing is enabled, the bucket manifest template is valid JSON.
- [ ] `go test ./...` passes after all Phase 17 changes.
- [ ] `tools/check-allowed-deps.sh` passes.
- [ ] `tools/check-network-policy.sh` passes.
- [ ] `docs/PHASE-LOG.md` contains a Phase 17 entry.

## Risks And Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| goreleaser cross-compilation fails for one target | High | Run `--snapshot --clean` locally before pushing; pin goreleaser version in CI. |
| HOMEBREW_TAP_GITHUB_TOKEN not set; release fails | Medium | Use `skip_upload: auto` in the brews block; document secret setup separately. |
| install.sh `sha256sum` not available on some systems | Medium | Detect `sha256sum` vs `shasum -a 256`; exit with instructions if neither found. |
| Binary size exceeds 50 MB limit from Phase 1 | Low | `-s -w -trimpath` reduces size significantly; CGO_ENABLED=0 avoids glibc overhead. |
| Doctor Ollama probe hangs indefinitely | Medium | Use a short context timeout (5 seconds) for the doctor `--ollama` probe. |
| Goreleaser version mismatch between local and CI | Low | Document the minimum goreleaser version in `.goreleaser.yml` comments. |
| Release workflow triggered by pre-release tags | Low | Use `prerelease: auto` in goreleaser release block; document tag naming convention. |
| Checksum file format mismatch between platforms | Medium | Test `install.sh` on both macOS and Linux; use standard BSD checksum format. |
| CHANGELOG.md gets out of sync with releases | Low | Use goreleaser changelog generation from GitHub release notes; CHANGELOG.md is the human-readable companion. |
| Doctor output format is too wide for narrow terminals | Low | Left-align all values; truncate long paths with `...` prefix. |

## Phase Log Template

When implementation finishes, append a Phase 17 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created and updated;
- goreleaser version used;
- five binary build verification results;
- checksum verification result;
- enhanced doctor output sample;
- doctor test results;
- manual install.sh smoke test result;
- design decisions;
- known constraints and deferred work (install.ps1, cosign, release signing);
- Homebrew and Scoop manual setup steps remaining;
- exit gate status.

## Exit Gate

Phase 17 is complete only when:

- all acceptance criteria above are met;
- `goreleaser build --snapshot --clean` produces all five binaries locally;
- each binary prints the correct `--version` string;
- `install.sh` passes checksum verification and installs successfully on at least one Linux and one macOS system;
- `nandocodego doctor` output contains all documented fields and `doctor --ollama` behaves correctly;
- all tests pass including the race detector;
- the phase log records the implementation, binary build results, and any deviations from this plan.
