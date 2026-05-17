#!/usr/bin/env bash
# Copyright 2026 Tekky AI Academy LLP. Licensed under FSL-1.1-ALv2.
#
# scripts/dev-up.sh — preflight for `make dev` / docker compose.
#
# What it does, in order:
#   1. Check if the configured API and web ports are free. If occupied,
#      identify the process holding them and offer to kill it. The
#      backend's port probe will handle a stubborn occupant by walking
#      +1..+10, but it's much friendlier to surface the conflict here
#      instead of burying it in a log line.
#   2. Verify required tooling (go, node, pnpm, docker) is present.
#   3. Check that ~/.qorven exists and is writable — the backend
#      writes runtime.json there on boot.
#   4. Print a short "here's what'll happen next" summary.
#
# Usage:
#   scripts/dev-up.sh                  # interactive preflight
#   scripts/dev-up.sh --kill           # non-interactive; auto-kill occupants
#   scripts/dev-up.sh --ports 4200 3000  # override default port pair
#   scripts/dev-up.sh --check-only     # report and exit — don't prompt

set -uo pipefail  # no -e: check_port returning 1 for "busy" is a
                   # reportable state, not a fatal abort. We want the
                   # script to keep going and surface all the findings.

API_PORT="${QORVEN_API_PORT:-4200}"
WEB_PORT="${QORVEN_WEB_PORT:-3000}"
AUTO_KILL=0
CHECK_ONLY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --kill) AUTO_KILL=1; shift ;;
    --check-only) CHECK_ONLY=1; shift ;;
    --ports) API_PORT="$2"; WEB_PORT="$3"; shift 3 ;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

c_red=$'\033[31m'; c_yel=$'\033[33m'; c_grn=$'\033[32m'; c_dim=$'\033[2m'; c_rst=$'\033[0m'

# port_holder_pid <port>  — prints the PID holding <port> on stdout, or
# nothing if free. Tries `lsof` first (most portable on macOS + Linux);
# falls back to `ss` which is always present on modern Linux.
port_holder_pid() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -ti TCP:"$port" -sTCP:LISTEN 2>/dev/null || true
  elif command -v ss >/dev/null 2>&1; then
    # ss output: users:(("pname",pid=1234,fd=9))
    ss -ltnp "sport = :$port" 2>/dev/null | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -1
  else
    echo ""
  fi
}

port_holder_name() {
  local pid="$1"
  [[ -z "$pid" ]] && return
  if command -v ps >/dev/null 2>&1; then
    ps -p "$pid" -o comm= 2>/dev/null || echo "pid $pid"
  fi
}

check_port() {
  local label="$1" port="$2"
  local pid; pid="$(port_holder_pid "$port")"
  if [[ -z "$pid" ]]; then
    printf "  %s%s:%s  free%s\n" "$c_grn" "$label" "$port" "$c_rst"
    return 0
  fi
  local name; name="$(port_holder_name "$pid")"
  printf "  %s%s:%s  BUSY%s (pid %s, %s)\n" "$c_yel" "$label" "$port" "$c_rst" "$pid" "$name"

  if (( CHECK_ONLY )); then return 1; fi

  local do_kill=$AUTO_KILL
  if (( ! do_kill )); then
    read -r -p "    kill it? [y/N] " ans
    [[ "$ans" =~ ^[Yy]$ ]] && do_kill=1
  fi

  if (( do_kill )); then
    kill "$pid" 2>/dev/null || true
    # Give it 3 seconds to exit gracefully before escalating.
    for i in 1 2 3; do
      sleep 1
      if [[ -z "$(port_holder_pid "$port")" ]]; then
        printf "    %sreleased%s\n" "$c_grn" "$c_rst"
        return 0
      fi
    done
    printf "    %sSIGKILL%s\n" "$c_red" "$c_rst"
    kill -9 "$pid" 2>/dev/null || true
    sleep 1
    if [[ -z "$(port_holder_pid "$port")" ]]; then
      printf "    %sreleased%s\n" "$c_grn" "$c_rst"
      return 0
    fi
    printf "    %sfailed to free :%s — the backend's port probe will try +1..+10%s\n" "$c_yel" "$port" "$c_rst"
    return 1
  else
    printf "    %sleaving as-is — the backend will probe +1..+10 on bind%s\n" "$c_dim" "$c_rst"
    return 1
  fi
}

echo "Qorven preflight"
echo ""
echo "Ports:"
check_port "api " "$API_PORT" || true
check_port "web " "$WEB_PORT" || true
echo ""

echo "Tooling:"
for tool in go node pnpm docker; do
  if command -v "$tool" >/dev/null 2>&1; then
    v="$($tool --version 2>/dev/null | head -1)"
    printf "  %s%-7s%s %s\n" "$c_grn" "$tool" "$c_rst" "$v"
  else
    printf "  %s%-7s%s not found\n" "$c_yel" "$tool" "$c_rst"
  fi
done
echo ""

runtime_dir="${HOME}/.qorven"
if [[ ! -d "$runtime_dir" ]]; then
  mkdir -p "$runtime_dir"
  printf "Created %s\n" "$runtime_dir"
fi
if [[ ! -w "$runtime_dir" ]]; then
  printf "%s%s is not writable — runtime.json discovery will fail%s\n" "$c_red" "$runtime_dir" "$c_rst"
  exit 1
fi

if (( CHECK_ONLY )); then exit 0; fi

cat <<EOF

Ready. Next steps:
  • Backend dev:      cd backend && make dev-watch   (hot reload)
  • Frontend dev:     cd web && pnpm dev
  • Everything:       docker compose up -d

The backend will bind to :${API_PORT} if free, or walk +1..+10 if busy.
The web client reads the actual port from /__qorven_runtime at boot.
EOF
