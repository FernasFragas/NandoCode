# BUG-20260607-smoke-server-script-fails-on-empty-auth-array

## Summary

`tools/smoke-server.sh` exits before any HTTP request when `TOKEN` is unset on the default macOS Bash 3.2 shell. The script expands `"${_auth[@]}"` under `set -u` while `_auth` is still an empty array, triggering an unbound-variable failure.

## Severity

- Severity: `sev4_low`
- Disposition: `confirmed`
- Area: `test_harness`

## Environment

- Commit: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- OS: `Darwin 25.5.0 arm64`
- Go version: `go version go1.26.2 darwin/arm64`
- Relevant env vars:
  - `TOKEN` unset

## Preconditions

- Default macOS Bash `3.2.57(1)-release`
- No `TOKEN` environment variable exported

## Reproduction Steps

1. Run:
   `tools/smoke-server.sh`

## Expected Result

The script should create a session and run the smoke flow without requiring `TOKEN` to be set.

## Actual Result

The script exits immediately with:

- `tools/smoke-server.sh: line 12: _auth[@]: unbound variable`

## Evidence

- Command output summary: immediate shell failure before the first `curl`
- Related code path: empty-array expansion in `tools/smoke-server.sh`
- Sanitization notes: no secrets present

## Frequency

- always
- attempt count: `1`

## Evidence Level

- `E1`

## Impacted Scenarios

- `G-006`
- `G-007`
- coordinator smoke automation

## Regression Risk

This breaks the documented smoke harness on a default macOS developer machine and can hide actual server behavior behind a shell-compatibility failure.

## Suspected Root Cause

The script assumes empty-array expansion is safe under `set -u`, which is not true on Bash 3.2 for the current `_auth` pattern.

## Recommended Fix Direction

Guard the auth arguments without relying on empty-array expansion under `set -u`, or initialize the request path in a shell-compatible way.

## Related Files

- `tools/smoke-server.sh`

## Retest Plan

1. Leave `TOKEN` unset.
2. Re-run `tools/smoke-server.sh`.
3. Confirm the script reaches the server calls and completes the smoke flow.

## Closure Criteria

- The smoke script runs successfully with `TOKEN` unset on the default supported Bash version.
