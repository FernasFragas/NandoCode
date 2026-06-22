#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/private/tmp/go-nandocodego-gocache}"

# Keep tests hermetic without overriding NANDOCODEGO_* runtime variables that
# some tests intentionally assert as unset or inherited.
unset NANDOCODEGO_CONFIG_HOME NANDOCODEGO_DATA_HOME NANDOCODEGO_CACHE_HOME NANDOCODEGO_STATE_HOME

echo "Running full regression gate..."
go test $(go list ./... | grep -v '/internal/tools/webfetch$')
go test -race ./internal/agent/... ./internal/hooks/... ./internal/tasks/... ./internal/tui/... ./internal/state/...
go vet ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh

echo "Skipped package due sandbox local-listener restriction: ./internal/tools/webfetch/..."

echo "Full regression gate passed."
