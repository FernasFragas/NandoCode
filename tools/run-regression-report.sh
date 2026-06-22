#!/usr/bin/env bash
set -u

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR" || exit 1

NOW_UTC="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
REPORT_DIR="$ROOT_DIR/.tmp-regression"
REPORT_PATH="$REPORT_DIR/regression-report-$(date -u '+%Y%m%d-%H%M%S').md"
LOG_FAST="$REPORT_DIR/fast.log"
LOG_FULL="$REPORT_DIR/full.log"
LOG_LOAD="$REPORT_DIR/load.log"
LOG_WEBFETCH="$REPORT_DIR/webfetch.log"

mkdir -p "$REPORT_DIR"

run_step() {
	local name="$1"
	local cmd="$2"
	local log_path="$3"
	printf '==> %s\n' "$name"
	# shellcheck disable=SC2086
	bash -lc "$cmd" >"$log_path" 2>&1
	local code=$?
	if [ $code -eq 0 ]; then
		printf '    PASS: %s\n' "$name"
	else
		printf '    FAIL: %s (exit %d)\n' "$name" "$code"
	fi
	return $code
}

FAST_STATUS="pass"
FULL_STATUS="pass"
LOAD_STATUS="pass"
WEBFETCH_STATUS="pass"

if ! run_step "Fast Regression Gate" "tools/run-regression-fast.sh" "$LOG_FAST"; then
	FAST_STATUS="fail"
fi
if ! run_step "Full Regression Gate" "tools/run-regression-full.sh" "$LOG_FULL"; then
	FULL_STATUS="fail"
fi
if ! run_step "Load Suite" "tools/run-load-suite.sh" "$LOG_LOAD"; then
	LOAD_STATUS="fail"
fi
if ! run_step "WebFetch Package (sandbox-sensitive)" "go test ./internal/tools/webfetch/..." "$LOG_WEBFETCH"; then
	WEBFETCH_STATUS="blocked_or_fail"
fi

overall="pass"
if [ "$FAST_STATUS" != "pass" ] || [ "$FULL_STATUS" != "pass" ] || [ "$LOAD_STATUS" != "pass" ]; then
	overall="fail"
fi

{
	echo "# Regression Run - $(date -u '+%Y-%m-%d')"
	echo
	echo "Environment:"
	echo "- Generated at (UTC): $NOW_UTC"
	echo "- Commit: $(git rev-parse HEAD 2>/dev/null || echo unknown)"
	echo "- Branch: $(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"
	echo "- OS: $(uname -s) $(uname -r)"
	echo "- Arch: $(uname -m)"
	echo "- Go version: $(go version 2>/dev/null || echo unavailable)"
	echo "- Ollama version: $(ollama --version 2>/dev/null || echo unavailable)"
	echo "- Config/data/cache/state dirs isolated: script-managed by underlying gate runners"
	echo
	echo "Automated:"
	echo "- Fast gate: $FAST_STATUS"
	echo "- Full gate: $FULL_STATUS"
	echo "- Load suite: $LOAD_STATUS"
	echo "- WebFetch package: $WEBFETCH_STATUS"
	echo "- Overall: $overall"
	echo
	echo "## Fast Gate Log"
	echo
	echo '```text'
	cat "$LOG_FAST"
	echo '```'
	echo
	echo "## Full Gate Log"
	echo
	echo '```text'
	cat "$LOG_FULL"
	echo '```'
	echo
	echo "## Load Suite Log"
	echo
	echo '```text'
	cat "$LOG_LOAD"
	echo '```'
	echo
	echo "## WebFetch Package Log"
	echo
	echo '```text'
	cat "$LOG_WEBFETCH"
	echo '```'
	echo
	echo "## Findings"
	echo
	echo "- Severity:"
	echo "- Area:"
	echo "- Repro:"
	echo "- Expected:"
	echo "- Actual:"
	echo "- Logs/trace:"
	echo "- Owner:"
	echo "- Blocker for Phase 22: yes/no"
} >"$REPORT_PATH"

printf '\nReport written: %s\n' "$REPORT_PATH"
if [ "$overall" = "pass" ]; then
	exit 0
fi
exit 1
