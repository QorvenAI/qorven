#!/usr/bin/env bash
# Qorven Web UI — setup and start script
# Usage: ./start-web.sh [port]
#
# This script:
# 1. Installs Node.js dependencies (if needed)
# 2. Creates .env.local from gateway config
# 3. Builds the production bundle
# 4. Starts the web server

set -e

PORT="${1:-3000}"
DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

echo "⚡ Qorven Web UI"
echo ""

# Check Node.js
if ! command -v node &>/dev/null; then
    echo "✗ Node.js not found"
    echo "  Install: curl -fsSL https://rpm.nodesource.com/setup_22.x | sudo bash - && sudo dnf install -y nodejs"
    exit 1
fi
echo "✓ Node.js $(node --version)"

# Install dependencies
if [ ! -d node_modules ]; then
    echo "→ Installing dependencies..."
    npm install 2>&1 | tail -1
fi

# Create .env.local from gateway config
QORVEN_HOME="${QORVEN_HOME:-$HOME/.qorven}"
if [ -f "$QORVEN_HOME/.env" ]; then
    # Extract gateway token
    TOKEN=$(grep "QORVEN_GATEWAY_TOKEN" "$QORVEN_HOME/.env" 2>/dev/null | cut -d= -f2)
    cat > .env.local << EOF
NEXT_PUBLIC_API_URL=http://localhost:4200
NEXT_PUBLIC_API_TOKEN=${TOKEN}
EOF
    echo "✓ Config loaded from $QORVEN_HOME/.env"
else
    if [ ! -f .env.local ]; then
        cat > .env.local << EOF
NEXT_PUBLIC_API_URL=http://localhost:4200
NEXT_PUBLIC_API_TOKEN=
EOF
        echo "⚠ No gateway config found — edit .env.local manually"
    fi
fi

# Build
echo "→ Building..."
npx next build 2>&1 | grep -E "✓|○|ƒ|Error" | tail -10
if [ ! -f .next/BUILD_ID ]; then
    echo "✗ Build failed"
    exit 1
fi
echo "✓ Build complete"

# Start
echo ""
echo "→ Starting on port $PORT..."
exec npx next start -H 0.0.0.0 -p "$PORT"
