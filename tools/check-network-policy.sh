#!/usr/bin/env bash
# check-network-policy.sh
# Scans source files for hardcoded HTTP/HTTPS URLs that violate the network policy.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

# Allowed hardcoded endpoints (per the outbound network policy)
ALLOWED_PATTERNS=(
  "http://localhost:11434"
  "http://127.0.0.1:11434"
  "https://localhost:11434"
  "https://127.0.0.1:11434"
  "https://ollama.com"
)

# Files to ignore (documentation, this plan, git internals, vendor, binaries)
IGNORE_PATTERNS=(
  ".git/"
  "vendor/"
  "node_modules/"
  ".github/"
  "SECURITY.md"
  "go-ollama-plan-AGENTS.md"
  "go-ollama-plan-HUMANS.md"
  "docs/"
  "book/"
  ".docs/"
  "*.md"
  "*.sum"
  "*.mod"
)

# Build grep exclude arguments
EXCLUDE_ARGS=()
for pattern in "${IGNORE_PATTERNS[@]}"; do
  EXCLUDE_ARGS+=(--exclude-dir="${pattern%%/*}" --exclude="$pattern")
done

# Find all http:// or https:// references in source files.
VIOLATIONS=()

while IFS=: read -r file _line_no line; do
  # Skip if empty
  if [[ -z "$file" ]] || [[ -z "$line" ]]; then
    continue
  fi
  if [[ "$line" =~ ^[[:space:]]*# ]]; then
    continue
  fi

  # Extract URLs from the line
  URLS=$(echo "$line" | grep -Eo "https?://[^[:space:]\"'<>]+" || true)
  
  # Check each URL
  while IFS= read -r url; do
    if [[ -z "$url" ]]; then
      continue
    fi
    
    # Check if this URL is allowed
    ALLOWED=false
    for allowed_pattern in "${ALLOWED_PATTERNS[@]}"; do
      if [[ "$url" == "$allowed_pattern"* ]]; then
        ALLOWED=true
        break
      fi
    done
    
    if [[ "$ALLOWED" == "false" ]]; then
      VIOLATIONS+=("$file: $url")
    fi
  done <<< "$URLS"
done < <(grep -rn -E 'https?://' \
  --include='*.go' \
  --include='*.ts' \
  --include='*.js' \
  --include='*.tsx' \
  --include='*.yaml' \
  --include='*.yml' \
  --include='*.toml' \
  --include='*.json' \
  "${EXCLUDE_ARGS[@]}" \
  . 2>/dev/null || true)

# Report results
if [[ ${#VIOLATIONS[@]} -eq 0 ]]; then
  echo "✓ No unauthorized hardcoded endpoints found"
  exit 0
else
  echo "✗ Found hardcoded endpoints that violate the network policy:"
  printf '  %s\n' "${VIOLATIONS[@]}"
  echo ""
  echo "Allowed endpoints:"
  printf '  %s\n' "${ALLOWED_PATTERNS[@]}"
  echo ""
  echo "To fix: remove hardcoded URLs or move them to configuration."
  echo "Documentation references are allowed in .md files under docs/ and .docs/"
  exit 1
fi
