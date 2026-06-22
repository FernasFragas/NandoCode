#!/usr/bin/env bash
# verify-phase-0.sh
# Runs all Phase 0 acceptance criteria checks.
# This script will be extended in Phase 1 to include Go build/test/lint/scan.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

echo "=================================================="
echo "Phase 0 Verification"
echo "=================================================="
echo ""

# Track failures
FAILURES=0

# Helper function for checks
check_file() {
  local file="$1"
  if [[ -f "$file" ]]; then
    echo "✓ $file exists"
  else
    echo "✗ $file MISSING"
    FAILURES=$((FAILURES + 1))
  fi
}

check_executable() {
  local file="$1"
  if [[ -x "$file" ]]; then
    echo "✓ $file is executable"
  else
    echo "✗ $file is NOT executable"
    FAILURES=$((FAILURES + 1))
  fi
}

run_check() {
  local name="$1"
  local cmd="$2"
  echo ""
  echo "Running: $name"
  if eval "$cmd"; then
    echo "✓ $name passed"
  else
    echo "✗ $name FAILED"
    FAILURES=$((FAILURES + 1))
  fi
}

# Check required files exist
echo "Checking required files..."
check_file "SECURITY.md"
check_file "tools/allowed-deps.txt"
check_file "tools/check-allowed-deps.sh"
check_file "tools/check-network-policy.sh"
check_file ".github/workflows/ci.yml"
check_file ".github/dependabot.yml"
check_file ".github/dependency-review-config.yml"
check_file ".github/ISSUE_TEMPLATE/security-hardening.md"
check_file "docs/PHASE-LOG.md"
check_file "tools/verify-phase-0.sh"

# Check scripts are executable
echo ""
echo "Checking script permissions..."
check_executable "tools/check-allowed-deps.sh"
check_executable "tools/check-network-policy.sh"
check_executable "tools/verify-phase-0.sh"

# Run policy checks
run_check "Dependency allowlist check" "./tools/check-allowed-deps.sh"
run_check "Network policy check" "./tools/check-network-policy.sh"

# Phase 1+ checks (run only if go.mod exists)
if [[ -f go.mod ]]; then
  echo ""
  echo "=================================================="
  echo "Go module detected - running Phase 1+ checks"
  echo "=================================================="
  
  run_check "Go build" "go build ./..."
  run_check "Go vet" "go vet ./..."
  run_check "Go test (race detector)" "go test -race -timeout=120s ./..."
  
  # Check for linter (may not be installed in all environments)
  if command -v golangci-lint &> /dev/null; then
    run_check "golangci-lint" "golangci-lint run"
  else
    echo ""
    echo "⚠️  golangci-lint not found; skipping (install: https://golangci-lint.run/usage/install/)"
  fi
  
  # Check for security scanners
  if command -v gosec &> /dev/null; then
    run_check "gosec" "gosec ./..."
  else
    echo ""
    echo "⚠️  gosec not found; skipping (install: go install github.com/securego/gosec/v2/cmd/gosec@latest)"
  fi
  
  if command -v govulncheck &> /dev/null; then
    run_check "govulncheck" "govulncheck ./..."
  else
    echo ""
    echo "⚠️  govulncheck not found; skipping (install: go install golang.org/x/vuln/cmd/govulncheck@latest)"
  fi
else
  echo ""
  echo "⚠️  go.mod not found; skipping Go checks (will be enabled in Phase 1)"
fi

# Final report
echo ""
echo "=================================================="
if [[ $FAILURES -eq 0 ]]; then
  echo "✅ All Phase 0 checks passed!"
  echo "=================================================="
  exit 0
else
  echo "❌ $FAILURES check(s) failed"
  echo "=================================================="
  exit 1
fi
