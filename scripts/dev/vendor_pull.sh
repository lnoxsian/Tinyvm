#!/usr/bin/env bash
# -----------------------------------------------------------------------------
# TinyVM Vendor Assets Update Script
# Automatically resolves, downloads, and stages the latest stable versions of:
#   1. noVNC (official git repository release tag)
#   2. xterm.js & xterm-addon-fit (latest stable release)
#   3. htmx (latest stable release)
# -----------------------------------------------------------------------------

set -euo pipefail

# ANSI color formatting
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED='' GREEN='' YELLOW='' BLUE='' CYAN='' BOLD='' NC=''
fi

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VENDOR_DIR="${ROOT_DIR}/web/vendor"

mkdir -p "${VENDOR_DIR}/novnc" "${VENDOR_DIR}/xterm"

echo -e "${BOLD}${CYAN}=== TinyVM Vendor Assets Auto-Pull ===${NC}\n"

# -----------------------------------------------------------------------------
# 1. Pull Latest Stable noVNC from Official Git Repository
# -----------------------------------------------------------------------------
log_info "Querying latest stable release tag for noVNC..."
NOVNC_REPO="https://github.com/novnc/noVNC.git"
NOVNC_TAG=$(git ls-remote --tags --refs "${NOVNC_REPO}" \
    | awk -F'/' '{print $3}' \
    | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
    | sort -V \
    | tail -n 1)

if [[ -z "${NOVNC_TAG}" ]]; then
    log_error "Failed to resolve latest stable noVNC release tag from ${NOVNC_REPO}"
    exit 1
fi
log_info "Found latest stable noVNC version: ${BOLD}${NOVNC_TAG}${NC}"

TMP_CLONE_DIR=$(mktemp -d -p "${ROOT_DIR}" .tmp-novnc-XXXXXX)
cleanup() {
    rm -rf "${TMP_CLONE_DIR}"
}
trap cleanup EXIT

log_info "Cloning noVNC ${NOVNC_TAG} (shallow depth 1)..."
git clone --depth 1 --branch "${NOVNC_TAG}" --quiet "${NOVNC_REPO}" "${TMP_CLONE_DIR}/noVNC"

log_info "Staging noVNC assets into ${VENDOR_DIR}/novnc..."
mkdir -p "${VENDOR_DIR}/novnc"

# Copy core engine, third-party vendor dependencies, UI app, and HTML wrappers per EMBEDDING.md
cp -r "${TMP_CLONE_DIR}/noVNC/core" "${VENDOR_DIR}/novnc/"
cp -r "${TMP_CLONE_DIR}/noVNC/vendor" "${VENDOR_DIR}/novnc/"
cp -r "${TMP_CLONE_DIR}/noVNC/app" "${VENDOR_DIR}/novnc/"
cp "${TMP_CLONE_DIR}/noVNC/vnc.html" "${VENDOR_DIR}/novnc/"
cp "${TMP_CLONE_DIR}/noVNC/vnc_lite.html" "${VENDOR_DIR}/novnc/"

if [[ -f "${TMP_CLONE_DIR}/noVNC/defaults.json" ]]; then
    cp "${TMP_CLONE_DIR}/noVNC/defaults.json" "${VENDOR_DIR}/novnc/"
else
    echo "{}" > "${VENDOR_DIR}/novnc/defaults.json"
fi

if [[ -f "${TMP_CLONE_DIR}/noVNC/mandatory.json" ]]; then
    cp "${TMP_CLONE_DIR}/noVNC/mandatory.json" "${VENDOR_DIR}/novnc/"
else
    echo "{}" > "${VENDOR_DIR}/novnc/mandatory.json"
fi

log_success "Staged noVNC (${NOVNC_TAG}) successfully."

# -----------------------------------------------------------------------------
# 2. Pull Latest Stable xterm.js & xterm-addon-fit
# -----------------------------------------------------------------------------
log_info "Resolving latest stable xterm.js..."
XTERM_URL_INFO=$(curl -sI "https://unpkg.com/xterm" | grep -i "^location:" | head -n 1 || true)
XTERM_VER=$(echo "${XTERM_URL_INFO}" | sed -E 's/.*xterm@([^/]+).*/\1/' | tr -d '\r' | tr -d '\n')
if [[ -z "${XTERM_VER}" ]]; then
    XTERM_VER="latest"
fi
log_info "Pulling xterm.js (${XTERM_VER})..."

curl -sSL -o "${VENDOR_DIR}/xterm/xterm.js" "https://unpkg.com/xterm/lib/xterm.js"
curl -sSL -o "${VENDOR_DIR}/xterm/xterm.css" "https://unpkg.com/xterm/css/xterm.css"

FIT_URL_INFO=$(curl -sI "https://unpkg.com/xterm-addon-fit" | grep -i "^location:" | head -n 1 || true)
FIT_VER=$(echo "${FIT_URL_INFO}" | sed -E 's/.*xterm-addon-fit@([^/]+).*/\1/' | tr -d '\r' | tr -d '\n')
if [[ -z "${FIT_VER}" ]]; then
    FIT_VER="latest"
fi
log_info "Pulling xterm-addon-fit (${FIT_VER})..."
curl -sSL -o "${VENDOR_DIR}/xterm/xterm-addon-fit.js" "https://unpkg.com/xterm-addon-fit/lib/xterm-addon-fit.js"

log_success "Staged xterm.js (${XTERM_VER}) & fit addon (${FIT_VER}) successfully."

# -----------------------------------------------------------------------------
# 3. Pull Latest Stable HTMX
# -----------------------------------------------------------------------------
log_info "Resolving latest stable HTMX..."
HTMX_URL_INFO=$(curl -sI "https://unpkg.com/htmx.org" | grep -i "^location:" | head -n 1 || true)
HTMX_VER=$(echo "${HTMX_URL_INFO}" | sed -E 's/.*htmx.org@([^/]+).*/\1/' | tr -d '\r' | tr -d '\n')
if [[ -z "${HTMX_VER}" ]]; then
    HTMX_VER="latest"
fi
log_info "Pulling HTMX (${HTMX_VER})..."
curl -sSL -o "${VENDOR_DIR}/htmx.min.js" "https://unpkg.com/htmx.org/dist/htmx.min.js"

log_success "Staged HTMX (${HTMX_VER}) successfully."

# -----------------------------------------------------------------------------
# Summary
# -----------------------------------------------------------------------------
echo ""
log_success "All vendor assets updated and staged in web/vendor/:"
echo "  - noVNC:           ${NOVNC_TAG}"
echo "  - xterm.js:        ${XTERM_VER}"
echo "  - xterm-addon-fit: ${FIT_VER}"
echo "  - htmx:            ${HTMX_VER}"
echo ""
echo -e "You can now rebuild the binary with: ${BOLD}go build -o bin/tinyvm ./cmd/tinyvm${NC}\n"
