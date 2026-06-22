#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/private/tmp/go-nandocodego-gocache}"

# Keep tests hermetic without overriding NANDOCODEGO_* runtime variables that
# some tests intentionally assert as unset or inherited.
unset NANDOCODEGO_CONFIG_HOME NANDOCODEGO_DATA_HOME NANDOCODEGO_CACHE_HOME NANDOCODEGO_STATE_HOME

echo "Running fast regression gate..."
go test ./internal/agent/... \
  ./internal/analysis/... \
  ./internal/bootstrap/... \
  ./internal/cli/... \
  ./internal/commands/... \
  ./internal/config/... \
  ./internal/hooks/... \
  ./internal/memory/... \
  ./internal/observability/... \
  ./internal/permissions/... \
  ./internal/state/... \
  ./internal/tools/agenttool/... \
  ./internal/tools/bash/... \
  ./internal/tools/builtin/... \
  ./internal/tools/dirwalk/... \
  ./internal/tools/fileedit/... \
  ./internal/tools/fileread/... \
  ./internal/tools/filewrite/... \
  ./internal/tools/glob/... \
  ./internal/tools/grep/... \
  ./internal/tools/skilltool/... \
  ./internal/tools/tasktool/... \
  ./internal/tools/todo/... \
  ./internal/tui/...

echo "Skipped package due sandbox local-listener restriction: ./internal/tools/webfetch/..."

echo "Fast regression gate passed."
