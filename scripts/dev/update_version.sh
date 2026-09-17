#!/usr/bin/env bash
# -----------------------------------------------------------------------------
# TinyVM Interactive Version Update Script
# Updates the primary VERSION file, Go internal version, and re-compiles the binary.
# -----------------------------------------------------------------------------

set -euo pipefail

# ANSI color codes
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    CYAN=''
    BOLD=''
    NC=''
fi

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Resolve repository root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

VERSION_FILE="${REPO_ROOT}/VERSION"
GO_VERSION_FILE="${REPO_ROOT}/internal/version/version.go"
MAKEFILE="${REPO_ROOT}/Makefile"
BINARY_PATH="${REPO_ROOT}/tinyvm"

# Validate files exist
if [[ ! -f "$GO_VERSION_FILE" ]]; then
    log_error "Go version file not found at: ${GO_VERSION_FILE}"
    exit 1
fi

# Read current version
if [[ -f "$VERSION_FILE" ]]; then
    CURRENT_VERSION="$(tr -d ' \t\r\n' < "$VERSION_FILE")"
else
    # Fallback to internal/version/version.go
    CURRENT_VERSION="$(grep -oE 'Version = "[^"]+"' "$GO_VERSION_FILE" | cut -d'"' -f2 || echo "0.1.0-dev")"
fi

# Clean current version (remove leading 'v' if present)
CURRENT_VERSION="${CURRENT_VERSION#v}"

# SemVer validation regex
SEMVER_REGEX="^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$"

# Parse current version components
CORE_VERSION="${CURRENT_VERSION%%-*}"
CORE_VERSION="${CORE_VERSION%%+*}"

PRERELEASE=""
if [[ "$CURRENT_VERSION" == *-* ]]; then
    PRERELEASE="${CURRENT_VERSION#*-}"
    PRERELEASE="${PRERELEASE%%+*}"
fi

IFS='.' read -r MAJOR MINOR PATCH <<< "$CORE_VERSION"
MAJOR="${MAJOR:-0}"
MINOR="${MINOR:-1}"
PATCH="${PATCH:-0}"

# Compute candidate bumps
BUMP_PATCH="${MAJOR}.${MINOR}.$((PATCH + 1))"
BUMP_MINOR="${MAJOR}.$((MINOR + 1)).0"
BUMP_MAJOR="$((MAJOR + 1)).0.0"

# Print usage / help
show_help() {
    cat << EOF
TinyVM Version Update Utility

Usage:
  $(basename "$0") [options] [bump-type | target-version]

Bump Types:
  patch       Bump patch version (${CURRENT_VERSION} -> ${BUMP_PATCH})
  minor       Bump minor version (${CURRENT_VERSION} -> ${BUMP_MINOR})
  major       Bump major version (${CURRENT_VERSION} -> ${BUMP_MAJOR})
  release     Strip prerelease tag (${CURRENT_VERSION} -> ${CORE_VERSION})
  <semver>    Specify explicit version (e.g. 0.2.0, 1.0.0-rc.1)

Options:
  -b, --build       Rebuild binary immediately after version update (default: interactive prompt)
  --no-build        Do not rebuild binary
  -t, --test        Run test suite ('go test ./...') before building
  -c, --commit      Commit updated version files in git
  -g, --tag         Create git annotated tag for the new version
  -d, --dry-run     Show planned updates without modifying files
  -y, --yes         Non-interactive mode, assume Yes for confirmations
  -h, --help        Show this help message

Running without arguments launches the interactive menu.
EOF
}

# Default flags
ARG_BUMP=""
FLAG_BUILD=""
FLAG_TEST=false
FLAG_COMMIT=false
FLAG_TAG=false
FLAG_DRY_RUN=false
FLAG_YES=false

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            show_help
            exit 0
            ;;
        -b|--build)
            FLAG_BUILD=true
            shift
            ;;
        --no-build)
            FLAG_BUILD=false
            shift
            ;;
        -t|--test)
            FLAG_TEST=true
            shift
            ;;
        -c|--commit)
            FLAG_COMMIT=true
            shift
            ;;
        -g|--tag)
            FLAG_TAG=true
            shift
            ;;
        -d|--dry-run)
            FLAG_DRY_RUN=true
            shift
            ;;
        -y|--yes)
            FLAG_YES=true
            shift
            ;;
        patch|minor|major|release)
            ARG_BUMP="$1"
            shift
            ;;
        *)
            if [[ "$1" =~ $SEMVER_REGEX ]] || [[ "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
                ARG_BUMP="${1#v}"
                shift
            else
                log_error "Unknown argument or invalid semver: $1"
                show_help
                exit 1
            fi
            ;;
    esac
done

NEW_VERSION=""

# Selection logic
if [[ -n "$ARG_BUMP" ]]; then
    case "$ARG_BUMP" in
        patch)
            NEW_VERSION="$BUMP_PATCH"
            ;;
        minor)
            NEW_VERSION="$BUMP_MINOR"
            ;;
        major)
            NEW_VERSION="$BUMP_MAJOR"
            ;;
        release)
            NEW_VERSION="$CORE_VERSION"
            ;;
        *)
            NEW_VERSION="$ARG_BUMP"
            ;;
    esac
else
    # Interactive Menu
    echo -e "${BOLD}=========================================${NC}"
    echo -e "${BOLD}      TinyVM Version Management          ${NC}"
    echo -e "${BOLD}=========================================${NC}"
    echo -e "Current Version: ${CYAN}${BOLD}${CURRENT_VERSION}${NC}\n"

    echo "Select target version:"
    if [[ -n "$PRERELEASE" ]]; then
        echo -e "  ${BOLD}0)${NC} Release version      (${YELLOW}${CORE_VERSION}${NC})"
    fi
    echo -e "  ${BOLD}1)${NC} Patch release        (${GREEN}${BUMP_PATCH}${NC})"
    echo -e "  ${BOLD}2)${NC} Minor release        (${GREEN}${BUMP_MINOR}${NC})"
    echo -e "  ${BOLD}3)${NC} Major release        (${GREEN}${BUMP_MAJOR}${NC})"
    echo -e "  ${BOLD}4)${NC} Pre-release / Dev    (${CYAN}${BUMP_PATCH}-dev${NC})"
    echo -e "  ${BOLD}5)${NC} Custom SemVer        (Enter manually)"
    echo -e "  ${BOLD}q)${NC} Quit"
    echo ""

    while true; do
        read -r -p "Enter choice: " choice
        case "$choice" in
            0)
                if [[ -n "$PRERELEASE" ]]; then
                    NEW_VERSION="$CORE_VERSION"
                    break
                else
                    echo -e "${RED}Option 0 is only available for pre-releases.${NC}"
                fi
                ;;
            1)
                NEW_VERSION="$BUMP_PATCH"
                break
                ;;
            2)
                NEW_VERSION="$BUMP_MINOR"
                break
                ;;
            3)
                NEW_VERSION="$BUMP_MAJOR"
                break
                ;;
            4)
                read -r -p "Enter pre-release suffix [default: dev]: " suffix
                suffix="${suffix:-dev}"
                suffix="${suffix#-}" # remove leading hyphen if user typed it
                NEW_VERSION="${BUMP_PATCH}-${suffix}"
                break
                ;;
            5)
                while true; do
                    read -r -p "Enter custom version (e.g. 0.2.0 or 1.0.0-rc.1): " custom_ver
                    custom_ver="${custom_ver#v}"
                    if [[ "$custom_ver" =~ $SEMVER_REGEX ]]; then
                        NEW_VERSION="$custom_ver"
                        break 2
                    else
                        echo -e "${RED}Invalid SemVer format. Must follow X.Y.Z or X.Y.Z-tag${NC}"
                    fi
                done
                ;;
            q|Q|quit|exit)
                log_info "Operation cancelled by user."
                exit 0
                ;;
            *)
                echo -e "${RED}Invalid option. Please choose from above.${NC}"
                ;;
        esac
    done
fi

# Validate computed target version
if ! [[ "$NEW_VERSION" =~ $SEMVER_REGEX ]]; then
    log_error "Calculated version '$NEW_VERSION' is not a valid Semantic Version."
    exit 1
fi

echo ""
echo -e "${BOLD}Target Update Summary:${NC}"
echo -e "  Current Version:  ${YELLOW}${CURRENT_VERSION}${NC}"
echo -e "  New Version:      ${GREEN}${BOLD}${NEW_VERSION}${NC}"
echo -e "  Files affected:"
echo -e "    - ${CYAN}${VERSION_FILE}${NC}"
echo -e "    - ${CYAN}${GO_VERSION_FILE}${NC}"
echo -e "    - ${CYAN}${BINARY_PATH}${NC} (compiled binary)"

# If dry-run, output summary and exit
if [[ "$FLAG_DRY_RUN" == true ]]; then
    log_warn "DRY-RUN: Version would be updated to '${NEW_VERSION}'. No files were modified."
    exit 0
fi

# Interactive prompt for build if not specified via flag
if [[ -z "$FLAG_BUILD" ]]; then
    if [[ "$FLAG_YES" == true ]] || ! [[ -t 0 ]]; then
        FLAG_BUILD=true
    else
        read -r -p "Rebuild TinyVM binary now? [Y/n]: " rebuild_choice || rebuild_choice="y"
        case "$rebuild_choice" in
            [nN]*) FLAG_BUILD=false ;;
            *)     FLAG_BUILD=true ;;
        esac
    fi
fi

# Confirmation prompt
if [[ "$FLAG_YES" != true ]]; then
    if [[ -t 0 ]]; then
        read -r -p "Proceed with update? [Y/n]: " confirm_choice || confirm_choice="y"
        case "$confirm_choice" in
            [nN]*)
                log_info "Aborted by user."
                exit 0
                ;;
        esac
    fi
fi

# 1. Update VERSION file
log_info "Updating ${VERSION_FILE}..."
echo "$NEW_VERSION" > "$VERSION_FILE"

# 2. Update internal/version/version.go
log_info "Updating ${GO_VERSION_FILE}..."
awk -v new_ver="$NEW_VERSION" '
/^[[:space:]]*Version[[:space:]]*=[[:space:]]*"[^"]*"/ {
    sub(/Version[[:space:]]*=[[:space:]]*"[^"]*"/, "Version = \"" new_ver "\"")
}
{ print }
' "$GO_VERSION_FILE" > "${GO_VERSION_FILE}.tmp"
mv "${GO_VERSION_FILE}.tmp" "$GO_VERSION_FILE"

# 3. Run tests if requested
if [[ "$FLAG_TEST" == true ]]; then
    log_info "Running test suite..."
    (cd "$REPO_ROOT" && go test ./...)
    log_success "Tests passed."
fi

# 4. Rebuild binary
if [[ "$FLAG_BUILD" == true ]]; then
    log_info "Rebuilding TinyVM binary via 'make build'..."
    (cd "$REPO_ROOT" && make build)

    if [[ -x "$BINARY_PATH" ]]; then
        echo ""
        log_success "Binary recompiled successfully! Verifying output:"
        "$BINARY_PATH" version
        echo ""
    else
        log_warn "Binary not found at expected path: ${BINARY_PATH}"
    fi
fi

# 5. Git commit & tag if requested
if [[ "$FLAG_COMMIT" == true ]] && command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    log_info "Creating git commit..."
    git -C "$REPO_ROOT" add "$VERSION_FILE" "$GO_VERSION_FILE" "$MAKEFILE"
    git -C "$REPO_ROOT" commit -m "chore: bump version to v${NEW_VERSION}"
    log_success "Created commit: chore: bump version to v${NEW_VERSION}"

    if [[ "$FLAG_TAG" == true ]]; then
        log_info "Creating git tag v${NEW_VERSION}..."
        git -C "$REPO_ROOT" tag -a "v${NEW_VERSION}" -m "Release v${NEW_VERSION}"
        log_success "Created tag: v${NEW_VERSION}"
    fi
fi

log_success "Version successfully updated to v${NEW_VERSION}!"
