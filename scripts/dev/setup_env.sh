#!/usr/bin/env bash
# -----------------------------------------------------------------------------
# TinyVM Development Environment Setup Script
# Installs QEMU/KVM binaries and configures local virtualization permissions.
# -----------------------------------------------------------------------------

set -euo pipefail

# ANSI Color codes for output formatting
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
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

# Determine target user (handles running under sudo)
TARGET_USER="${SUDO_USER:-$USER}"

# Ensure sudo helper
run_as_root() {
    if [[ $EUID -eq 0 ]]; then
        "$@"
    else
        if ! command -v sudo >/dev/null 2>&1; then
            log_error "sudo is required to install packages. Please run as root or install sudo."
            exit 1
        fi
        sudo "$@"
    fi
}

echo -e "${BOLD}=========================================${NC}"
echo -e "${BOLD}     TinyVM Development Setup            ${NC}"
echo -e "${BOLD}=========================================${NC}"
echo ""

# 1. Detect Host OS & Package Manager
log_info "Detecting host operating system..."
if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    log_info "Detected: ${PRETTY_NAME:-$NAME}"
else
    log_warn "Could not source /etc/os-release; attempting generic package detection."
fi

# 2. Check CPU Virtualization Support (VT-x / AMD-V)
log_info "Checking CPU hardware virtualization support..."
if grep -q -E '(vmx|svm)' /proc/cpuinfo; then
    log_success "Hardware virtualization (VT-x/AMD-V) is enabled in CPU."
else
    log_warn "Hardware virtualization flags (vmx/svm) not detected in /proc/cpuinfo."
    log_warn "If running inside a VM, ensure Nested Virtualization is enabled in your hypervisor."
fi

# 3. Install QEMU Packages
log_info "Installing required QEMU binaries and utilities..."

if command -v apt-get >/dev/null 2>&1; then
    log_info "Updating apt package index..."
    run_as_root apt-get update -y
    log_info "Installing qemu-system-x86, qemu-utils, and ovmf..."
    run_as_root apt-get install -y --no-install-recommends \
        qemu-system-x86 \
        qemu-utils \
        ovmf
elif command -v dnf >/dev/null 2>&1; then
    log_info "Using dnf to install QEMU packages..."
    run_as_root dnf install -y \
        qemu-system-x86-core \
        qemu-img \
        edk2-ovmf
elif command -v pacman >/dev/null 2>&1; then
    log_info "Using pacman to install QEMU packages..."
    run_as_root pacman -Sy --noconfirm \
        qemu-base \
        edk2-ovmf
else
    log_error "Unsupported package manager. Please manually install qemu-system-x86 and qemu-utils."
    exit 1
fi

# 4. Verify /dev/kvm and Group Permissions
log_info "Configuring KVM permissions for user: ${TARGET_USER}..."

if [[ -e /dev/kvm ]]; then
    log_success "Found /dev/kvm."
    # Ensure user is in kvm group
    if getent group kvm >/dev/null 2>&1; then
        if id -nG "$TARGET_USER" | grep -qw "kvm"; then
            log_success "User ${TARGET_USER} is already in the 'kvm' group."
        else
            log_info "Adding user ${TARGET_USER} to 'kvm' group..."
            run_as_root usermod -aG kvm "$TARGET_USER"
            log_warn "Added to 'kvm' group. You may need to log out and log back in (or run 'newgrp kvm') for this to take effect."
        fi
    fi
else
    log_warn "/dev/kvm not found. Make sure KVM kernel module is loaded (e.g. 'sudo modprobe kvm kvm_intel' or 'kvm_amd')."
fi

# 5. Verify Installed Binaries
echo ""
log_info "Verifying installed binaries..."

if command -v qemu-img >/dev/null 2>&1; then
    QEMU_IMG_VER=$(qemu-img --version | head -n 1)
    log_success "qemu-img: ${QEMU_IMG_VER}"
else
    log_error "qemu-img not found in PATH."
fi

if command -v qemu-system-x86_64 >/dev/null 2>&1; then
    QEMU_SYS_VER=$(qemu-system-x86_64 --version | head -n 1)
    log_success "qemu-system-x86_64: ${QEMU_SYS_VER}"
else
    log_error "qemu-system-x86_64 not found in PATH."
fi

echo ""
echo -e "${BOLD}=========================================${NC}"
echo -e "${GREEN}${BOLD}Setup completed successfully!${NC}"
echo -e "You are now ready to build and run TinyVM with KVM/QEMU."
echo -e "${BOLD}=========================================${NC}"
