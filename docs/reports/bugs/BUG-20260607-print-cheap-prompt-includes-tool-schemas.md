# BUG-20260607-print-cheap-prompt-includes-tool-schemas

## Summary

Cheap non-interactive `--print` prompts still persist prompt dumps with all built-in tool schemas present, even when the prompt is a trivial no-context request that should qualify for `ToolModeNone` behavior. The user-visible completion succeeds, but the request shape recorded in prompt-dump metadata contradicts the expected cheap-prompt optimization.

## Severity

- Severity: `sev3_medium`
- Disposition: `confirmed`
- Area: `cli`

## Environment

- Commit: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- OS: `macOS 26.5.1 (Darwin 25.5.0 arm64)`
- Go version: `go version go1.26.2 darwin/arm64`
- Ollama version: client `0.30.5`
- Model: `qwen3.6:35b`
- Relevant env vars:
  - `NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/config`
  - `NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/data`
  - `NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/cache`
  - `NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/state`
  - `NANDOCODEGO_PROMPT_DUMP=metadata`

## Preconditions

- Isolated config/data/cache/state directories created under `/private/tmp/nandocodego-lane-cd.Bk2o0Z`
- Local Ollama runtime reachable outside the sandbox
- Local model `qwen3.6:35b` available
- Working directory set to `/private/tmp/nandocodego-lane-cd.Bk2o0Z/workspace`

## Reproduction Steps

1. Run:
   ```sh
   env NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/config NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/data NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/cache NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/state NANDOCODEGO_PROMPT_DUMP=metadata /Users/fernando/Desktop/to_sync/ai_projects_etc/go-nandocode-llm/bin/nandocodego --model qwen3.6:35b --print 'Respond with exactly OK.' --json
   ```
2. Inspect the persisted prompt dump:
   ```sh
   sed -n '1,220p' /private/tmp/nandocodego-lane-cd.Bk2o0Z/state/prompt-dumps/latest.json
   ```
3. Rerun with a second trivial prompt:
   ```sh
   env NANDOCODEGO_CONFIG_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/config NANDOCODEGO_DATA_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/data NANDOCODEGO_CACHE_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/cache NANDOCODEGO_STATE_HOME=/private/tmp/nandocodego-lane-cd.Bk2o0Z/state NANDOCODEGO_PROMPT_DUMP=metadata /Users/fernando/Desktop/to_sync/ai_projects_etc/go-nandocode-llm/bin/nandocodego --model qwen3.6:35b --print 'Reply with exactly YES.' --json
   ```
4. Inspect `latest.json` again.

## Expected Result

For a trivially cheap prompt with no explicit file/workspace context, the final model request shape should omit tool schemas and the persisted prompt dump should reflect that omission.

## Actual Result

Both prompts completed successfully without tool use, but `latest.json` recorded:

- `"tool_count": 9`
- `"tool_names": ["Bash", "FileEdit", "FileRead", "FileWrite", "Glob", "Grep", "TodoRead", "TodoWrite", "WebFetch"]`

on each run.

## Evidence

- Command output summary:
  - First run returned `{"content":"OK","tool_uses":[]...}`
  - Second run returned `{"content":"YES","tool_uses":[]...}`
- Prompt dump references:
  - `/private/tmp/nandocodego-lane-cd.Bk2o0Z/state/prompt-dumps/latest.json` after each repro
- Relevant observed fields:
  - `message_count: 1`
  - `tool_count: 9`
  - `history_policy: "default"`
  - `evidence_pack.Packed: false`
- Sanitization notes:
  - Only public test prompts were used
  - No private code or credentials were included

## Frequency

- always
- attempt count: `2`

## Evidence Level

- `E2`

## Impacted Scenarios

- `C-012`

## Regression Risk

This likely affects all cheap `--print` prompts and may also indicate divergence between print mode and TUI routing behavior for request shaping, token budgeting, and prompt-dump inspection.

## Suspected Root Cause

Evidence suggests the print path constructs `agent.Input` directly in `internal/cli/print.go` without applying the retrieval-route decision that sets `ToolModeNone` for general prompts.

## Recommended Fix Direction

Make the print path apply the same routing/tool-mode decision used by the TUI before calling the agent, and add an integration-level regression check that inspects prompt-dump metadata for a cheap `--print` prompt.

## Related Files

- `internal/cli/print.go`
- `internal/retrievalroute/route.go`
- `internal/agent/stream.go`
- `docs/AGENT-E2E-TEST-AND-BUG-REPORT-PLAN.md`

## Retest Plan

1. Rerun both trivial `--print` commands above.
2. Inspect `latest.json`.
3. Confirm `tool_count` is `0` or otherwise proves tool schemas were omitted for the cheap prompt path.
4. Rerun one explicit-context prompt to confirm normal tool exposure is preserved where required.

## Closure Criteria

- Cheap non-interactive prompts no longer persist tool schemas in prompt-dump metadata.
- The fix is proven with command output plus prompt-dump evidence.
- An adjacent explicit-context or tool-using prompt still behaves correctly after the change.
