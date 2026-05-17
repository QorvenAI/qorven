#!/usr/bin/env bash
# Install Go static analysis tools required by `make verify` and the pre-commit hook.
# Run once after cloning: ./scripts/install-dev-tools.sh
set -euo pipefail

echo "==> Installing Go dev tools…"
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/kisielk/errcheck@latest

echo "==> Tools installed:"
staticcheck --version
errcheck --version 2>/dev/null || echo "errcheck: ok (no --version flag)"

echo ""
echo "==> Pre-commit hook is at .git/hooks/pre-commit — already executable."
echo "    All future commits will auto-run: go vet, go build, staticcheck, tsc."
