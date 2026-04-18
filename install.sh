#!/usr/bin/env bash
# Installer for packetcode — a keyboard-first multi-provider AI coding agent.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/packetcode/packetcode/main/install.sh | bash
#
# Optional environment variables:
#   INSTALL_DIR  Where to install the binary. Default: /usr/local/bin
#                (override with INSTALL_DIR=$HOME/.local/bin to avoid sudo)
#   VERSION      Specific version to install (e.g. v0.1.0). Default: latest.

set -euo pipefail

REPO="packetcode/packetcode"
BINARY="packetcode"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)   ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)
    echo "packetcode: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
  *)
    echo "packetcode: unsupported OS: $OS (use the .exe from GitHub Releases on Windows)" >&2
    exit 1
    ;;
esac

if [[ -z "${VERSION:-}" ]]; then
  if ! command -v curl >/dev/null 2>&1; then
    echo "packetcode: curl is required to discover the latest version" >&2
    exit 1
  fi
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | head -1 \
    | sed -E 's/.*"(v[^"]+)".*/\1/')"
  if [[ -z "$VERSION" ]]; then
    echo "packetcode: could not determine latest version" >&2
    exit 1
  fi
fi

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ARCHIVE="${BINARY}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

echo "Downloading packetcode ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o "$TMPDIR/$ARCHIVE"
tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR"

if [[ ! -f "$TMPDIR/$BINARY" ]]; then
  echo "packetcode: binary not found in archive" >&2
  exit 1
fi
chmod +x "$TMPDIR/$BINARY"

if [[ -w "$INSTALL_DIR" ]]; then
  mv "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
else
  echo "Installing to $INSTALL_DIR (sudo required)..."
  sudo mv "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
fi

echo "✓ packetcode ${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
echo "  Run '${BINARY} --version' to verify."
