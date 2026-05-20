#!/usr/bin/env bash
# dev.sh — start backend (air hot-reload) + Next.js in a tmux session.
#
# ┌─────────────────────────────────────────────────────────────────────────┐
# │  DEV ENVIRONMENT — ONE SOURCE OF TRUTH                                  │
# │                                                                          │
# │  USE THIS URL:  http://localhost:3000  ← always, for everything          │
# │                                                                          │
# │  Port :4200  = Go backend (air hot-reload). API + WS only. No UI.       │
# │  Port :3000  = Next.js dev server. Proxies /api/* → :4200 automatically. │
# │                                                                          │
# │  DO NOT open :4200 directly — it serves the OLD embedded UI (no HMR,    │
# │  no live code changes). DO NOT use the tailscale IP for dev testing.     │
# │                                                                          │
# │  Tailscale / nginx :80 → proxies to :4200 (Go binary, no live UI).      │
# │  Only use the tailscale IP after deploying a release binary to EC2.      │
# └─────────────────────────────────────────────────────────────────────────┘
#
# Usage:
#   ./dev.sh          — start (or attach if already running)
#   ./dev.sh stop     — kill the session and all processes
#   ./dev.sh logs     — tail both log files
#
# Logs:
#   /tmp/qorven-backend.log
#   /tmp/qorven-next.log
#
# The session name is "qorven-dev" — tmux attach-session -t qorven-dev to re-attach.

set -euo pipefail

REPO="$(cd "$(dirname "$0")" && pwd)"
SESSION="qorven-dev"
BACKEND_LOG="/tmp/qorven-backend.log"
NEXT_LOG="/tmp/qorven-next.log"

# ── helpers ──────────────────────────────────────────────────────────────────

kill_port() {
  local port="$1"
  local pid
  pid=$(lsof -ti TCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
  if [[ -n "$pid" ]]; then
    echo "  Killing process on :$port (pid $pid)"
    kill "$pid" 2>/dev/null || true
    sleep 1
    # SIGKILL if still alive
    pid=$(lsof -ti TCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
    [[ -n "$pid" ]] && kill -9 "$pid" 2>/dev/null || true
  fi
}

# ── stop ─────────────────────────────────────────────────────────────────────

if [[ "${1:-}" == "stop" ]]; then
  echo "Stopping qorven-dev..."
  tmux kill-session -t "$SESSION" 2>/dev/null || true
  kill_port 4200
  kill_port 3000
  echo "Done."
  exit 0
fi

# ── logs ─────────────────────────────────────────────────────────────────────

if [[ "${1:-}" == "logs" ]]; then
  tail -f "$BACKEND_LOG" "$NEXT_LOG"
  exit 0
fi

# ── start ─────────────────────────────────────────────────────────────────────

# If session already running, just attach.
if tmux has-session -t "$SESSION" 2>/dev/null; then
  echo "Session '$SESSION' already running — attaching."
  echo "(detach with Ctrl+B then D)"
  exec tmux attach-session -t "$SESSION"
fi

# Kill anything already holding the ports so air doesn't walk to :4201.
kill_port 4200
kill_port 3000

echo "Starting qorven-dev tmux session..."

# Window 0: backend via air
tmux new-session -d -s "$SESSION" -n "backend" -x 220 -y 50
tmux send-keys -t "$SESSION:backend" \
  "cd '$REPO/backend' && air -c .air.toml 2>&1 | tee '$BACKEND_LOG'" Enter

# Window 1: Next.js dev server
tmux new-window -t "$SESSION" -n "nextjs"
tmux send-keys -t "$SESSION:nextjs" \
  "cd '$REPO/web' && pnpm dev --port 3000 2>&1 | tee '$NEXT_LOG'" Enter

# Select the backend window by default
tmux select-window -t "$SESSION:backend"

echo ""
echo "  Session : $SESSION"
echo "  Backend : air hot-reload on :4200  (log: $BACKEND_LOG)"
echo "  Next.js : pnpm dev on :3000        (log: $NEXT_LOG)"
echo ""
echo "  Attach  : tmux attach-session -t $SESSION"
echo "  Stop    : ./dev.sh stop"
echo "  Logs    : ./dev.sh logs"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  DEV URL  →  http://localhost:3000  (use this only)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
