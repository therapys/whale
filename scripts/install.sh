#!/usr/bin/env sh
set -e

# whale installer: curl -fsSL https://raw.githubusercontent.com/therapys/whale/main/scripts/install.sh | sh

REPO="therapys/whale"
BINARY="whale"
DEST="/usr/local/bin"

info() { printf "[whale] %s\n" "$1"; }
fail() { printf "[whale] ERROR: %s\n" "$1" >&2; exit 1; }

# Detect OS/ARCH (Go style)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH=amd64;;
    aarch64|arm64) ARCH=arm64;;
    armv7l) ARCH=armv7;;
    *) fail "unsupported arch: $ARCH";;
esac

# Allow override via env: VERSION=v0.1.0 DEST=/usr/local/bin
VERSION=${VERSION:-latest}

# Try GitHub Releases first
if [ "$VERSION" = "latest" ]; then
    TAG=$(curl -fsSL https://api.github.com/repos/$REPO/releases/latest | sed -n 's/ *"tag_name": "\(.*\)",/\1/p' | head -n1)
else
    TAG=$VERSION
fi
[ -n "$TAG" ] || fail "could not resolve release tag"

ASSET="$BINARY-$OS-$ARCH.tar.gz"
URL="https://github.com/$REPO/releases/download/$TAG/$ASSET"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "downloading $URL"
if curl -fsSL "$URL" -o "$TMP/$ASSET"; then
    tar -C "$TMP" -xzf "$TMP/$ASSET"
    [ -f "$TMP/$BINARY" ] || fail "archive missing $BINARY"
    chmod +x "$TMP/$BINARY"
    sudo mv "$TMP/$BINARY" "$DEST/$BINARY" 2>/dev/null || mv "$TMP/$BINARY" "$DEST/$BINARY" || fail "move to $DEST failed (try sudo)"
    info "installed $BINARY to $DEST"
    exit 0
fi

# Fallback to go install if releases not available
if command -v go >/dev/null 2>&1; then
    info "fallback to 'go install'"
    GOFLAGS=${GOFLAGS:-}
    GOBIN=${GOBIN:-$HOME/go/bin}
    # Using main branch tip; users can set VERSION like v0.1.0
    if [ "$VERSION" = "latest" ]; then
        go install $GOFLAGS github.com/therapys/whale/cmd/whale@latest || fail "go install failed"
    else
        go install $GOFLAGS github.com/therapys/whale/cmd/whale@$VERSION || fail "go install failed"
    fi
    BINPATH="$GOBIN/$BINARY"
    [ -f "$BINPATH" ] || BINPATH="$(go env GOPATH)/bin/$BINARY"
    [ -f "$BINPATH" ] || fail "binary not found after go install"
    sudo mv "$BINPATH" "$DEST/$BINARY" 2>/dev/null || mv "$BINPATH" "$DEST/$BINARY" || fail "move to $DEST failed (try sudo)"
    info "installed $BINARY to $DEST"
    exit 0
fi

fail "no release asset and 'go' not found; see README for manual install"


