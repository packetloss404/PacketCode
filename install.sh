#!/usr/bin/env bash
# Installer for packetcode — a keyboard-first multi-provider AI coding agent.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/packetloss404/packetcode/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/packetloss404/packetcode/main/install.sh | INSTALL_DIR="$HOME/.local/bin" bash
#
# Optional environment variables:
#   INSTALL_DIR  Where to install the binary. Default: /usr/local/bin
#                (override with INSTALL_DIR=$HOME/.local/bin to avoid sudo)
#   VERSION      Specific version to install (e.g. v0.1.0). Default: latest.
#   REQUIRE_SIGNATURE
#                Set to 1 to abort unless the Sigstore signature on
#                checksums.txt verifies. Off by default only because cosign is
#                not usually installed and releases before v0.6.0 are unsigned;
#                turn it on for anything unattended.

set -euo pipefail

REPO="packetloss404/packetcode"
BINARY="packetcode"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "packetcode: $1 is required" >&2
    exit 1
  fi
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "packetcode: sha256sum or shasum is required to verify downloads" >&2
    exit 1
  fi
}

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

need_cmd curl
need_cmd tar
need_cmd install

if [[ -z "${VERSION:-}" ]]; then
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
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

echo "Downloading packetcode ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o "$TMPDIR/$ARCHIVE"

echo "Verifying checksum..."
curl -fsSL "$CHECKSUMS_URL" -o "$TMPDIR/checksums.txt"

# Check the checksum file before believing it.
#
# Comparing the archive against checksums.txt proves the download was not
# corrupted in transit. It proves nothing about tampering: whoever could serve a
# modified archive could serve the matching checksums.txt beside it, and this
# script would confirm the forgery and install it. The signature is what makes
# the check terminate somewhere an attacker does not control.
verify_signature() {
  if ! command -v cosign >/dev/null 2>&1; then
    if [[ "${REQUIRE_SIGNATURE:-}" == "1" ]]; then
      echo "packetcode: REQUIRE_SIGNATURE=1 but cosign is not installed" >&2
      echo "  install it from https://docs.sigstore.dev/cosign/installation/" >&2
      exit 1
    fi
    echo "  note: cosign is not installed, so the checksum file's signature was not checked."
    echo "        The archive still matched checksums.txt. To verify properly, install"
    echo "        cosign (https://docs.sigstore.dev/cosign/installation/) and re-run."
    return
  fi

  if ! curl -fsSL "${CHECKSUMS_URL}.sig" -o "$TMPDIR/checksums.txt.sig" 2>/dev/null ||
     ! curl -fsSL "${CHECKSUMS_URL}.pem" -o "$TMPDIR/checksums.txt.pem" 2>/dev/null; then
    if [[ "${REQUIRE_SIGNATURE:-}" == "1" ]]; then
      echo "packetcode: REQUIRE_SIGNATURE=1 but ${VERSION} publishes no signature" >&2
      exit 1
    fi
    echo "  note: ${VERSION} has no signature (releases before v0.6.0 are unsigned)."
    return
  fi

  if cosign verify-blob "$TMPDIR/checksums.txt" \
      --signature "$TMPDIR/checksums.txt.sig" \
      --certificate "$TMPDIR/checksums.txt.pem" \
      --certificate-identity-regexp "^https://github.com/${REPO}/\.github/workflows/release\.yml@refs/tags/" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      >/dev/null 2>&1; then
    echo "  signature verified: checksums.txt was produced by ${REPO}'s release workflow."
  else
    # Always fatal, REQUIRE_SIGNATURE or not. A signature that is present and
    # does not verify is not a missing guarantee, it is a failed one.
    echo "packetcode: the signature on checksums.txt did NOT verify." >&2
    echo "  Refusing to install. This is what a substituted release looks like." >&2
    exit 1
  fi
}

verify_signature

EXPECTED_SHA="$(awk -v file="$ARCHIVE" '$2 == file {print $1}' "$TMPDIR/checksums.txt" | head -1)"
if [[ -z "$EXPECTED_SHA" ]]; then
  echo "packetcode: checksum for $ARCHIVE not found in checksums.txt" >&2
  exit 1
fi
ACTUAL_SHA="$(sha256_file "$TMPDIR/$ARCHIVE")"
if [[ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]]; then
  echo "packetcode: checksum mismatch for $ARCHIVE" >&2
  echo "  expected: $EXPECTED_SHA" >&2
  echo "  actual:   $ACTUAL_SHA" >&2
  exit 1
fi

tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR"

if [[ ! -f "$TMPDIR/$BINARY" ]]; then
  echo "packetcode: binary not found in archive" >&2
  exit 1
fi
chmod +x "$TMPDIR/$BINARY"

if [[ -e "$INSTALL_DIR" && ! -d "$INSTALL_DIR" ]]; then
  echo "packetcode: INSTALL_DIR exists but is not a directory: $INSTALL_DIR" >&2
  exit 1
fi

if [[ ! -d "$INSTALL_DIR" ]]; then
  if ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
    need_cmd sudo
    echo "Creating $INSTALL_DIR (sudo required)..."
    sudo mkdir -p "$INSTALL_DIR"
  fi
fi

if [[ -w "$INSTALL_DIR" ]]; then
  install -m 0755 "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
else
  need_cmd sudo
  echo "Installing to $INSTALL_DIR (sudo required)..."
  sudo install -m 0755 "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
fi

echo "packetcode ${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "warning: $INSTALL_DIR is not on PATH; add it to run '${BINARY}' without a full path." >&2
    ;;
esac
echo "  Run '${BINARY} --version' to verify."
