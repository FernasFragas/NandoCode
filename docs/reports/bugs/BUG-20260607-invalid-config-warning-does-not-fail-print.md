# BUG-20260607-invalid-config-warning-does-not-fail-print

## Summary

`nandocodego --print` does not fail when the project config TOML is malformed. It emits a parse warning to stderr and continues with the run, which diverges from the Lane A scenario expectation that invalid config should fail with actionable error text.

## Severity

- Severity: `sev4_low`
- Disposition: `documentation_drift`
- Area: `cli`

## Environment

- Commit: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- OS: `Darwin 25.5.0 arm64`
- Go version: `go1.26.2`
- Ollama version: stubbed local Ollama-compatible endpoint on `127.0.0.1:18081`
- Model: `stub-local`
- Relevant env vars:
  - `NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/config`
  - `NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/data`
  - `NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/cache`
  - `NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/state`

## Preconditions

- Project workspace at `/private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/workspace`
- Malformed project config at `.nandocodego/config.toml` with content `default_model = [`
- Local stub Ollama endpoint available on `http://127.0.0.1:18081`

## Reproduction Steps

1. Create a workspace with `.nandocodego/config.toml` containing malformed TOML: `default_model = [`.
2. Run:
   `env NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/config NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/data NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/cache NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/state /Users/fernando/Desktop/to_sync/ai_projects_etc/go-nandocode-llm/bin/nandocodego --ollama-url http://127.0.0.1:18081 --model stub-local --print hello`

## Expected Result

The command should fail immediately with a clear config-load error.

## Actual Result

The command prints a parse warning and then completes the `--print` run successfully.

## Evidence

- Output summary:
  - `Config warning: project config parse error at /private/tmp/nandocodego-e2e-ab-badcfg.EhFSxS/workspace/.nandocodego/config.toml: toml: array is incomplete`
  - `stub-response model=stub-local num_ctx=4096`
- Artifact paths: none recorded beyond this report
- Sanitization notes: no secrets present

## Frequency

- always
- attempt count: `1`

## Evidence Level

- `E0`

## Impacted Scenarios

- `A-009`

## Regression Risk

Malformed config may be easy to miss because the command continues with defaults or flags, which can mask configuration mistakes during troubleshooting.

## Suspected Root Cause

`internal/cli/print.go` treats `config.Load(...)` parse problems as warnings when a result is returned, instead of enforcing a hard failure for malformed project config.

## Recommended Fix Direction

Decide whether malformed config should be fatal for CLI commands that load config. If the intended behavior is warning-only, update the E2E plan and user-facing docs. If the intended behavior is fail-fast, return a non-zero exit instead of continuing.

## Related Files

- `internal/cli/print.go`
- `internal/config/loader.go`
- `docs/AGENT-E2E-TEST-AND-BUG-REPORT-PLAN.md`

## Retest Plan

1. Keep a malformed `.nandocodego/config.toml`.
2. Re-run the exact `--print` command above.
3. Verify whether the command now fails non-zero with only a config error, or whether the documentation was updated to make warning-only behavior explicit.

## Closure Criteria

Either:

- malformed config causes `--print` to fail with a non-zero exit and actionable error text, or
- the source-of-truth plan/docs are updated so scenario `A-009` no longer expects failure.
