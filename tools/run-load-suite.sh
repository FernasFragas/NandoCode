#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/private/tmp/go-nandocodego-gocache}"

# Keep tests hermetic without overriding NANDOCODEGO_* runtime variables that
# some tests intentionally assert as unset or inherited.
unset NANDOCODEGO_CONFIG_HOME NANDOCODEGO_DATA_HOME NANDOCODEGO_CACHE_HOME NANDOCODEGO_STATE_HOME

echo "Running load/perf suite..."
go test ./internal/agent/... ./internal/tui/... ./internal/analysis/... -count=20
go test -race ./internal/agent/... ./internal/tasks/... ./internal/state/... ./internal/tui/...
go test ./internal/tui/... -bench 'Render|Transcript|AssistantDelta' -benchmem
go test ./internal/mentions/... ./internal/tools/dirwalk/... -run 'Large|Directory|Budget'
go test ./internal/agent/... ./internal/analysis/... -run 'Load|Large|Deterministic|Checkpoint|Retrieve'

echo "Load/perf suite passed."
