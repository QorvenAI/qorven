#!/usr/bin/env bash
# Copyright 2026 Tekky AI Academy LLP. Licensed under FSL-1.1-ALv2.
#
# scripts/build.sh — produce a single Qorven binary with the web UI
# embedded. Run from repo root.
#
#   ./scripts/build.sh                 # local-arch binary → dist/qorven
#   GOOS=linux GOARCH=amd64 ./scripts/build.sh
#   GOOS=darwin GOARCH=arm64 ./scripts/build.sh
#
# The output binary contains:
#   - Backend API, agent loop, channel handlers, migrations
#   - Next.js static export of the web UI (served by the same
#     listener; no Node runtime required)
#
# Two modes:
#   1. Default: embed the web UI. Runs `pnpm install` + `pnpm build`
#      in QORVEN_STATIC mode, copies web/out into the embed dir.
#   2. SKIP_WEB=1: skip the web build. Useful when iterating on the
#      backend; the binary falls back to the external web/ folder
#      (or a .embedded sentinel if nothing's there).

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WEB=$ROOT/web
BACKEND=$ROOT/backend
EMBED=$BACKEND/internal/webui/dist
OUT=${OUT:-$ROOT/dist/qorven}

GOOS=${GOOS:-$(go env GOOS)}
GOARCH=${GOARCH:-$(go env GOARCH)}
VERSION=${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}

mkdir -p "$(dirname "$OUT")"

if [[ -z "${SKIP_WEB:-}" ]]; then
  echo "==> Building web UI (static export)…"
  (
    cd "$WEB"
    if ! command -v pnpm >/dev/null 2>&1; then
      echo "pnpm not found. Install with: npm install -g pnpm" >&2
      exit 1
    fi
    pnpm install --frozen-lockfile
    QORVEN_STATIC=1 pnpm build
  )
  echo "==> Copying web/out → $EMBED"
  rm -rf "$EMBED"
  mkdir -p "$EMBED"
  cp -r "$WEB/out/." "$EMBED/"
  # Restore the gitignored sentinel files so `git status` stays clean.
  cat > "$EMBED/.gitignore" <<'EOF'
*
!.gitignore
!.embedded
EOF
  cat > "$EMBED/.embedded" <<'EOF'
This file exists so go:embed always has at least one file to match.
Release builds overwrite this directory with the Next.js static export.
EOF
fi

echo "==> Building Go binary (GOOS=$GOOS GOARCH=$GOARCH)…"
(
  cd "$BACKEND"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
    -ldflags "-s -w -X 'github.com/qorvenai/qorven/cmd.Version=$VERSION'" \
    -o "$OUT" .
)

size=$(du -h "$OUT" | awk '{print $1}')
echo "==> Built $OUT ($size, $GOOS/$GOARCH, version $VERSION)"
