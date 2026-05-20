#!/usr/bin/env bash
# Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
#
# Qorven bootstrapper — downloads the binary, then hands off to the
# full-screen installation wizard built into the binary itself.
#
#   curl -fsSL https://get.qorven.ai | sudo bash
#
# All flags after -- are forwarded to `qorven install`.
# Override QORVEN_BINARY=/path/to/bin to skip the download step.

set -euo pipefail

GITHUB_OWNER="${GITHUB_OWNER:-qorvenai}"
GITHUB_REPO="${GITHUB_REPO:-qorven}"
RELEASE_TAG="${RELEASE_TAG:-latest}"
INSTALL_DIR="/usr/local/bin"
QORVEN_BINARY="${QORVEN_BINARY:-}"
export DEBIAN_FRONTEND=noninteractive

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
die()  { printf "\n  ${RED}✗ ${BOLD}%s${NC}\n\n" "$*" >&2; exit 1; }
info() { printf "  ${CYAN}›${NC} %s\n" "$*"; }
ok()   { printf "  ${GREEN}✓${NC} %s\n" "$*"; }

# ── parse flags ───────────────────────────────────────────────────────────────
PASSTHROUGH=()
UNINSTALL=0
for arg in "$@"; do
  case "$arg" in
    --uninstall) UNINSTALL=1 ;;
    --help|-h)
      printf "Usage: curl -fsSL https://get.qorven.ai | sudo bash\n\n"
      printf "All options are forwarded to 'qorven install'.\n"
      printf "Run 'sudo qorven install --help' for the full flag list.\n\n"
      printf "Bootstrap-only flags:\n"
      printf "  --uninstall    Remove Qorven, service, and config (keeps data/DB)\n\n"
      exit 0 ;;
    *) PASSTHROUGH+=("$arg") ;;
  esac
done

# ── root check ────────────────────────────────────────────────────────────────
if [ "$(id -u)" -ne 0 ]; then
  die "needs root — retry with: curl -fsSL https://get.qorven.ai | sudo bash"
fi

# ── platform ──────────────────────────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
[ "$OS" != "linux" ] && die "Linux only. macOS: download from https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases"
case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

# ── uninstall (no binary needed) ──────────────────────────────────────────────
if [ "$UNINSTALL" = "1" ]; then
  printf "\n  ${BOLD}${RED}Uninstalling Qorven${NC}\n\n"
  systemctl stop    qorven 2>/dev/null || true
  systemctl disable qorven 2>/dev/null || true
  rm -f /etc/systemd/system/qorven.service
  systemctl daemon-reload 2>/dev/null || true
  rm -f "$INSTALL_DIR/qorven"
  rm -rf /etc/qorven
  ok "Qorven uninstalled (data and database not removed)"
  printf "\n  To also remove data:\n"
  printf "    sudo rm -rf /var/lib/qorven /var/log/qorven\n"
  printf "    sudo -u postgres psql -c 'DROP DATABASE qorven; DROP USER qorven;'\n\n"
  exit 0
fi

# ── detect existing install ───────────────────────────────────────────────────
EXISTING_VERSION=""
if command -v qorven >/dev/null 2>&1; then
  EXISTING_VERSION="$(qorven version 2>/dev/null | grep -oP 'v[\d.]+[^\s]*' | head -1 || true)"
fi

if [ -n "$EXISTING_VERSION" ]; then
  printf "\n"
  printf "  ${BOLD}Qorven is already installed${NC} (${EXISTING_VERSION})\n\n"

  # Resolve latest release for comparison
  LATEST_TAG="$(curl --proto '=https' --tlsv1.2 -fsSLm10 \
    -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases" \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": "\(.*\)".*/\1/' 2>/dev/null || echo "")"

  if [ -n "$LATEST_TAG" ] && [ "$LATEST_TAG" != "$EXISTING_VERSION" ]; then
    printf "  A newer version is available: ${GREEN}${BOLD}${LATEST_TAG}${NC}\n\n"
    printf "  What would you like to do?\n\n"
    printf "    ${BOLD}1)${NC} Update to ${LATEST_TAG}  ${GREEN}(recommended)${NC}\n"
    printf "    ${BOLD}2)${NC} Reinstall ${EXISTING_VERSION}  (repair / reconfigure)\n"
    printf "    ${BOLD}3)${NC} Cancel\n\n"
    printf "  Choice [1]: "
    read -r CHOICE </dev/tty || CHOICE="1"
    CHOICE="${CHOICE:-1}"
    case "$CHOICE" in
      2) info "reinstalling ${EXISTING_VERSION}…"; RELEASE_TAG="$EXISTING_VERSION" ;;
      3) printf "\n  Cancelled.\n\n"; exit 0 ;;
      *) info "updating to ${LATEST_TAG}…"; RELEASE_TAG="$LATEST_TAG" ;;
    esac
  else
    printf "  What would you like to do?\n\n"
    printf "    ${BOLD}1)${NC} Reinstall / repair current version  ${GREEN}(recommended)${NC}\n"
    printf "    ${BOLD}2)${NC} Cancel\n\n"
    printf "  Choice [1]: "
    read -r CHOICE </dev/tty || CHOICE="1"
    CHOICE="${CHOICE:-1}"
    [ "$CHOICE" = "2" ] && { printf "\n  Cancelled.\n\n"; exit 0; }
    info "reinstalling ${EXISTING_VERSION}…"
    RELEASE_TAG="${EXISTING_VERSION}"
  fi
  printf "\n"
fi

# ── download or use provided binary ───────────────────────────────────────────
if [ -n "$QORVEN_BINARY" ]; then
  info "using provided binary: $QORVEN_BINARY"
  [ -f "$QORVEN_BINARY" ] || die "QORVEN_BINARY='$QORVEN_BINARY' not found"
  cp "$QORVEN_BINARY" "$INSTALL_DIR/qorven"
  chmod +x "$INSTALL_DIR/qorven"
else
  if [ "$RELEASE_TAG" = "latest" ]; then
    info "resolving latest release…"
    RELEASE_TAG="$(curl --proto '=https' --tlsv1.2 -fsSL \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases" \
      | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": "\(.*\)".*/\1/')"
    [ -z "$RELEASE_TAG" ] && die "Could not resolve latest release. Check: https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases"
    info "latest: ${RELEASE_TAG}"
  fi

  BINARY_URL="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/download/${RELEASE_TAG}/qorven-${OS}-${ARCH}"
  info "downloading ${RELEASE_TAG} for ${OS}/${ARCH}…"
  TMP_BIN="$(mktemp /tmp/qorven.XXXXXX)"
  if ! curl -fsSL --progress-bar "$BINARY_URL" -o "$TMP_BIN" 2>&1; then
    rm -f "$TMP_BIN"
    die "Download failed: $BINARY_URL\n  Set QORVEN_BINARY=/path/to/bin to use a local binary."
  fi
  chmod +x "$TMP_BIN"
  mv "$TMP_BIN" "$INSTALL_DIR/qorven"
fi

# ── install telemetry (fire-and-forget, never blocks install) ─────────────────
_track_install() {
  local ver="${RELEASE_TAG}"
  local os="${OS:-linux}"
  local arch="${ARCH:-amd64}"
  local kernel; kernel="$(uname -r 2>/dev/null | cut -d- -f1 || echo unknown)"
  local distro; distro="$(. /etc/os-release 2>/dev/null && echo "${ID:-unknown}-${VERSION_ID:-}" || echo unknown)"
  local cpu_cores; cpu_cores="$(nproc 2>/dev/null || echo 0)"
  local mem_gb; mem_gb="$(awk '/MemTotal/{printf "%d", $2/1024/1024}' /proc/meminfo 2>/dev/null || echo 0)"
  local hostname_hash; hostname_hash="$(hostname 2>/dev/null | sha256sum 2>/dev/null | cut -c1-8 || echo unknown)"
  local provider; provider="$(curl -sfm2 http://169.254.169.254/latest/meta-data/placement/region 2>/dev/null && echo aws || \
                               curl -sfm2 http://metadata.google.internal/computeMetadata/v1/project/project-id -H 'Metadata-Flavor:Google' 2>/dev/null && echo gcp || \
                               curl -sfm2 http://169.254.169.254/hetzner/v1/metadata 2>/dev/null && echo hetzner || echo bare)"
  curl -fsSLm5 \
    "https://get.qorven.ai/t?v=${ver}&os=${os}&arch=${arch}&kernel=${kernel}&distro=${distro}&cores=${cpu_cores}&mem=${mem_gb}&host=${hostname_hash}&cloud=${provider}" \
    -o /dev/null 2>/dev/null || true
}
_track_install &

ok "binary ready: $INSTALL_DIR/qorven  ($(du -sh "$INSTALL_DIR/qorven" 2>/dev/null | cut -f1))"
printf "\n"

# ── hand off to the installation wizard ───────────────────────────────────────
exec "$INSTALL_DIR/qorven" install "${PASSTHROUGH[@]}"
