#!/bin/bash
# kubectl-managedjob - Installation Script
# Usage: curl -sSL https://raw.githubusercontent.com/lukaszraczylo/jobs-manager-operator/main/scripts/install-plugin.sh | bash
#
# Or with a specific version:
# curl -sSL https://raw.githubusercontent.com/lukaszraczylo/jobs-manager-operator/main/scripts/install-plugin.sh | bash -s -- v1.0.0

set -e

# Configuration
GITHUB_REPO="lukaszraczylo/jobs-manager-operator"
BINARY_NAME="kubectl-managedjob"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Detect OS and architecture
detect_platform() {
    local os arch

    case "$(uname -s)" in
        Darwin)
            os="darwin"
            ;;
        Linux)
            os="linux"
            ;;
        MINGW*|MSYS*|CYGWIN*)
            os="windows"
            ;;
        *)
            error "Unsupported operating system: $(uname -s)"
            ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64)
            arch="amd64"
            ;;
        arm64|aarch64)
            arch="arm64"
            ;;
        *)
            error "Unsupported architecture: $(uname -m)"
            ;;
    esac

    echo "${os}_${arch}"
}

# Get the latest release version from GitHub
get_latest_version() {
    local response version curl_opts

    # Use GitHub token if available (higher rate limit)
    curl_opts=(-sS)
    if [[ -n "${GITHUB_TOKEN:-}" ]]; then
        curl_opts+=(-H "Authorization: token ${GITHUB_TOKEN}")
    fi

    # Fetch with error handling
    response=$(curl "${curl_opts[@]}" "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>&1)

    # Check for rate limiting
    if echo "$response" | grep -q "API rate limit exceeded"; then
        error "GitHub API rate limit exceeded.

You have a few options:
  1. Wait ~1 hour for the rate limit to reset
  2. Specify a version manually:
     curl -sSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/scripts/install-plugin.sh | bash -s -- v0.0.33
  3. Use a GitHub token (set GITHUB_TOKEN environment variable)"
    fi

    # Check for other API errors
    if echo "$response" | grep -q '"message":'; then
        local msg
        msg=$(echo "$response" | grep '"message":' | sed -E 's/.*"message": *"([^"]+)".*/\1/')
        error "GitHub API error: $msg"
    fi

    # Extract version
    version=$(echo "$response" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    if [[ -z "$version" ]]; then
        error "Failed to fetch latest version from GitHub. Response: $response"
    fi

    echo "$version"
}

# Download and install the plugin
download_and_install() {
    local version="$1"
    local platform="$2"
    local tmp_dir

    tmp_dir=$(mktemp -d)
    trap "rm -rf $tmp_dir" EXIT

    # Construct download URL
    local archive_ext="tar.gz"
    if [[ "$platform" == windows_* ]]; then
        archive_ext="zip"
    fi

    # Archive name format from goreleaser: jobs-manager-operator_VERSION_OS_ARCH.tar.gz
    local archive_name="jobs-manager-operator_${version#v}_${platform}.${archive_ext}"
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${archive_name}"

    info "Downloading ${archive_name}..."

    if ! curl -sSL -o "$tmp_dir/release.${archive_ext}" "$download_url"; then
        error "Failed to download release from: $download_url"
    fi

    info "Extracting archive..."
    if [[ "$archive_ext" == "zip" ]]; then
        if ! unzip -q "$tmp_dir/release.zip" -d "$tmp_dir"; then
            error "Failed to extract archive"
        fi
    else
        if ! tar -xzf "$tmp_dir/release.tar.gz" -C "$tmp_dir"; then
            error "Failed to extract archive"
        fi
    fi

    # Find the binary
    local binary_path
    binary_path=$(find "$tmp_dir" -name "$BINARY_NAME" -type f | head -1)

    if [[ -z "$binary_path" ]]; then
        error "Could not find $BINARY_NAME in the archive"
    fi

    # Determine install location
    local install_path="$INSTALL_DIR/$BINARY_NAME"

    # Check if we need sudo
    if [[ -w "$INSTALL_DIR" ]]; then
        info "Installing to ${install_path}..."
        cp "$binary_path" "$install_path"
        chmod +x "$install_path"
    else
        info "Installing to ${install_path} (requires sudo)..."
        sudo cp "$binary_path" "$install_path"
        sudo chmod +x "$install_path"
    fi

    success "Installed $BINARY_NAME to $install_path"
}

# Verify installation
verify_installation() {
    if command -v "$BINARY_NAME" &> /dev/null; then
        success "Installation verified!"
        info "Run '$BINARY_NAME --help' to get started"
        echo ""
        "$BINARY_NAME" --help 2>/dev/null | head -10 || true
    else
        warn "Installation complete, but $BINARY_NAME is not in PATH"
        warn "You may need to add $INSTALL_DIR to your PATH"
        echo ""
        echo "Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
        echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
    fi
}

# Handle --uninstall flag
uninstall() {
    local install_path="$INSTALL_DIR/$BINARY_NAME"

    echo ""
    echo "Uninstalling $BINARY_NAME..."

    if [[ -f "$install_path" ]]; then
        if [[ -w "$install_path" ]]; then
            rm "$install_path"
        else
            sudo rm "$install_path"
        fi
        success "Removed $install_path"
    else
        warn "$BINARY_NAME not found at $install_path"
    fi

    success "Uninstallation complete"
}

# Main installation flow
main() {
    local version="${1:-}"

    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║        kubectl-managedjob - Installation Script           ║"
    echo "║   Kubernetes workflow visualization plugin for kubectl    ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""

    # Check required dependencies
    if ! command -v curl &> /dev/null; then
        error "curl is required but not installed"
    fi

    if ! command -v tar &> /dev/null; then
        error "tar is required but not installed"
    fi

    # Detect platform
    local platform
    platform=$(detect_platform)
    info "Detected platform: $platform"

    # Get version
    if [[ -z "$version" ]]; then
        info "Fetching latest release..."
        version=$(get_latest_version)
    fi
    info "Installing version: $version"

    # Download and install
    download_and_install "$version" "$platform"

    # Verify
    verify_installation

    echo ""
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║                  Installation Complete!                   ║"
    echo "╠═══════════════════════════════════════════════════════════╣"
    echo "║  Usage:                                                   ║"
    echo "║    kubectl managedjob visualize <name> -n <namespace>     ║"
    echo "║    kubectl managedjob list -n <namespace>                 ║"
    echo "║    kubectl managedjob status <name> -n <namespace>        ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo ""
}

# Handle flags
case "${1:-}" in
    --uninstall)
        uninstall
        exit 0
        ;;
    --help|-h)
        echo "kubectl-managedjob Installation Script"
        echo ""
        echo "Usage:"
        echo "  install-plugin.sh [VERSION]    Install kubectl-managedjob (latest or specific version)"
        echo "  install-plugin.sh --uninstall  Remove kubectl-managedjob"
        echo "  install-plugin.sh --help       Show this help"
        echo ""
        echo "Environment Variables:"
        echo "  INSTALL_DIR    Installation directory (default: /usr/local/bin)"
        echo "  GITHUB_TOKEN   GitHub token for higher API rate limits"
        echo ""
        echo "Examples:"
        echo "  # Install latest version"
        echo "  curl -sSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/scripts/install-plugin.sh | bash"
        echo ""
        echo "  # Install specific version"
        echo "  curl -sSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/scripts/install-plugin.sh | bash -s -- v0.0.33"
        echo ""
        echo "  # Install to custom directory"
        echo "  INSTALL_DIR=~/bin curl -sSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/scripts/install-plugin.sh | bash"
        exit 0
        ;;
esac

main "$@"
