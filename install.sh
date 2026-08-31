#!/bin/sh
# prompter installer script
# Usage: curl -fsSL https://raw.githubusercontent.com/dotcommander/prompter/main/install.sh | sh

set -e

REPO="dotcommander/prompter"
BINARY="prompter"

# Detect Operating System
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  darwin)  TARGET_OS="darwin" ;;
  linux)   TARGET_OS="linux" ;;
  msys*|mingw*|cygwin*) TARGET_OS="windows" ;;
  *)
    echo "Unsupported operating system: $OS" >&2
    exit 1
    ;;
esac

# Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) TARGET_ARCH="amd64" ;;
  arm64|aarch64) TARGET_ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# Determine destination directory
if [ -n "$BINDIR" ]; then
  DEST_DIR="$BINDIR"
elif [ -w "/usr/local/bin" ]; then
  DEST_DIR="/usr/local/bin"
elif [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
  DEST_DIR="$HOME/.local/bin"
elif [ -d "$HOME/bin" ] || mkdir -p "$HOME/bin" 2>/dev/null; then
  DEST_DIR="$HOME/bin"
else
  DEST_DIR="/usr/local/bin"
fi

mkdir -p "$DEST_DIR" 2>/dev/null || true

# If Go is installed, use go install as reliable primary/fallback
if command -v go >/dev/null 2>&1; then
  echo "Installing prompter via Go toolchain..."
  go install "github.com/${REPO}@latest"
  echo "✓ Successfully installed prompter!"
  exit 0
fi

# Otherwise, download the latest GitHub release tarball
TAG=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
  TAG="latest"
fi

TARBALL="prompter_${TARGET_OS}_${TARGET_ARCH}.tar.gz"
if [ "$TAG" = "latest" ]; then
  URL="https://github.com/${REPO}/releases/latest/download/${TARBALL}"
else
  URL="https://github.com/${REPO}/releases/download/${TAG}/${TARBALL}"
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

echo "Downloading prompter ($TARGET_OS/$TARGET_ARCH)..."
if curl -fsSL "$URL" -o "$TMP_DIR/$TARBALL" 2>/dev/null; then
  tar -xzf "$TMP_DIR/$TARBALL" -C "$TMP_DIR"
  if [ -w "$DEST_DIR" ]; then
    mv "$TMP_DIR/$BINARY" "$DEST_DIR/$BINARY"
  else
    echo "Elevated permissions required to install to $DEST_DIR"
    sudo mv "$TMP_DIR/$BINARY" "$DEST_DIR/$BINARY"
  fi
  chmod +x "$DEST_DIR/$BINARY"
  echo "✓ Successfully installed prompter to $DEST_DIR/$BINARY"
else
  echo "Could not download prebuilt release from $URL" >&2
  echo "Please install via Go: go install github.com/${REPO}@latest" >&2
  exit 1
fi

case ":$PATH:" in
  *":$DEST_DIR:"*) ;;
  *)
    echo "\nNOTE: $DEST_DIR is not in your \$PATH. Add it to your shell profile:"
    echo "  export PATH=\"$DEST_DIR:\$PATH\""
    ;;
esac
