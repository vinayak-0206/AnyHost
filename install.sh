#!/bin/bash
# GoTunnel Installer
# Usage: curl -sSL https://raw.githubusercontent.com/vinayak-0206/AnyHost/main/install.sh | bash

set -e

REPO="vinayak-0206/AnyHost"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="gotunnel"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    darwin) OS="darwin" ;;
    linux)  OS="linux" ;;
    *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

echo "GoTunnel Installer"
echo "=================="
echo ""
echo "Detected: ${OS}/${ARCH}"

# Get latest release URL
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/gotunnel-${OS}-${ARCH}"

echo "Downloading from: ${DOWNLOAD_URL}"
echo ""

# Create temp directory
TMP_DIR=$(mktemp -d)
TMP_FILE="${TMP_DIR}/${BINARY_NAME}"

# Download
if command -v curl &> /dev/null; then
    curl -sSL -o "$TMP_FILE" "$DOWNLOAD_URL"
elif command -v wget &> /dev/null; then
    wget -q -O "$TMP_FILE" "$DOWNLOAD_URL"
else
    echo "Error: curl or wget required"
    exit 1
fi

# Make executable
chmod +x "$TMP_FILE"

# Install
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
fi

# Cleanup
rm -rf "$TMP_DIR"

echo ""
echo "Installed successfully!"
echo ""
echo "Quick start:"
echo "  1. gotunnel config add-authtoken <your-token>"
echo "  2. gotunnel http 3000"
echo ""
