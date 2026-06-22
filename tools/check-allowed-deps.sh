#!/usr/bin/env bash
# check-allowed-deps.sh
# Validates that all direct dependencies in go.mod are in the allowlist.

set -euo pipefail

ALLOWLIST_FILE="tools/allowed-deps.txt"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

# If go.mod doesn't exist yet, exit successfully (Phase 0 may run before Phase 1)
if [[ ! -f go.mod ]]; then
  echo "✓ go.mod does not exist yet; skipping dependency check"
  exit 0
fi

# Read allowlist, filtering out comments and blank lines
ALLOWED_DEPS=$(grep -v '^\s*#' "$ALLOWLIST_FILE" | grep -v '^\s*$' || true)

# Get the main module path to exclude it
MAIN_MODULE=$(go list -m)

# Get all direct dependencies (not indirect)
DIRECT_DEPS=$(go list -m -f '{{if not .Indirect}}{{.Path}}{{end}}' all | grep -v "^$MAIN_MODULE$" || true)

# Track violations
VIOLATIONS=()

# Check each direct dependency
while IFS= read -r dep; do
  if [[ -z "$dep" ]]; then
    continue
  fi
  
  # Check if this dep is in the allowlist
  # We need to handle module paths that might be prefixes (e.g., github.com/foo/bar/v2)
  FOUND=false
  while IFS= read -r allowed; do
    if [[ -z "$allowed" ]]; then
      continue
    fi
    # Exact match or allowed path is a prefix
    if [[ "$dep" == "$allowed" ]] || [[ "$dep" == "$allowed"/* ]]; then
      FOUND=true
      break
    fi
  done <<< "$ALLOWED_DEPS"
  
  if [[ "$FOUND" == "false" ]]; then
    VIOLATIONS+=("$dep")
  fi
done <<< "$DIRECT_DEPS"

# Report results
if [[ ${#VIOLATIONS[@]} -eq 0 ]]; then
  echo "✓ All direct dependencies are allowlisted"
  exit 0
else
  echo "✗ Found non-allowlisted direct dependencies:"
  printf '  %s\n' "${VIOLATIONS[@]}"
  echo ""
  echo "To fix: add these modules to $ALLOWLIST_FILE with justification,"
  echo "or remove them from go.mod if they are not needed."
  exit 1
fi
