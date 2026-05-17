#!/usr/bin/env bash
# check-sse-names.sh — FU-004: verify every broadcast/sendSSE literal string
# is either a namespaced name (contains ".") or a key in the legacyAliases map.
#
# Usage: scripts/check-sse-names.sh
# Exit code: 0 = all good, 1 = unknown names found.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EVENTS_FILE="$REPO_ROOT/backend/internal/api/events/types.go"
GATEWAY_DIR="$REPO_ROOT/backend/internal/gateway"

# Extract known legacy alias keys from the Go source.
# Matches lines like:  "phase":          TypeBuildPhase,
known=$(grep -oP '"[a-z_]+"\s*:\s*Type' "$EVENTS_FILE" | grep -oP '"[^"]+"' | tr -d '"')

# Extract all literal string arguments to broadcast() and sendSSE() calls.
# Matches: broadcast("agent_started", ...) or sendSSE("phase", ...)
used=$(grep -rhoP '(?:broadcast|sendSSE)\(\s*"([^"]+)"' "$GATEWAY_DIR" \
    --include="*.go" | grep -oP '"[^"]+"' | tr -d '"' | sort -u)

fail=0
while IFS= read -r name; do
    # Namespaced names (contain ".") are always valid.
    [[ "$name" == *.* ]] && continue
    if ! echo "$known" | grep -qx "$name"; then
        echo "UNKNOWN SSE name: '$name' — add to legacyAliases in api/events/types.go" >&2
        fail=1
    fi
done <<< "$used"

if [[ $fail -eq 0 ]]; then
    echo "check-sse-names: all broadcast/sendSSE names are known ✓"
fi
exit $fail
