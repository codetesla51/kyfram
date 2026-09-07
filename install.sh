#!/bin/sh
set -e
# kyfram installer — detects OS/arch, installs prebuilt binary or builds from source
# usage:
#   curl -fsSL https://raw.githubusercontent.com/codetesla51/kyfram/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/codetesla51/kyfram/main/install.sh | sh -s -- v0.1.1
#   VERSION=v0.1.1 sh install.sh
REPO="codetesla51/kyfram"
BIN="kyfram"
VERSION="${VERSION:-${1:-latest}}"
# normalize: strip leading v for url? keep as-is, GitHub tags include v
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in x86_64|x86-64|amd64) ARCH="amd64" ;; aarch64|arm64) ARCH="arm64" ;; *) echo "unsupported arch: $ARCH" >&2; exit 1 ;; esac
case "$OS" in linux|darwin) ;; *) echo "unsupported os: $OS" >&2; exit 1 ;; esac
FILE="${BIN}-${OS}-${ARCH}"
if [ "$VERSION" = "latest" ]; then
  URL="https://github.com/${REPO}/releases/latest/download/${FILE}"
else
  # allow v prefix or not
  case "$VERSION" in v*) ;; *) VERSION="v$VERSION" ;; esac
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILE}"
fi
INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then INSTALL_DIR="$HOME/.local/bin"; mkdir -p "$INSTALL_DIR"; fi
# detect existing install for update message
if command -v "$BIN" >/dev/null 2>&1 || [ -x "$INSTALL_DIR/$BIN" ]; then
  echo "updating kyfram ($VERSION) for $OS/$ARCH → $INSTALL_DIR/$BIN"
else
  echo "installing kyfram ($VERSION) for $OS/$ARCH → $INSTALL_DIR/$BIN"
fi
TMP=$(mktemp)
downloaded=""
if command -v curl >/dev/null 2>&1; then
  if curl -fsSL "$URL" -o "$TMP" 2>/dev/null; then downloaded=1; fi
elif command -v wget >/dev/null 2>&1; then
  if wget -qO "$TMP" "$URL" 2>/dev/null; then downloaded=1; fi
fi
if [ "$downloaded" = "1" ] && [ -s "$TMP" ]; then
  chmod +x "$TMP"
  mv "$TMP" "$INSTALL_DIR/$BIN"
  echo "installed $INSTALL_DIR/$BIN ($VERSION)"
else
  rm -f "$TMP"
  if ! command -v go >/dev/null 2>&1; then
    echo "no prebuilt binary at $URL and go not found" >&2
    echo "install go or check https://github.com/$REPO/releases" >&2
    exit 1
  fi
  echo "no prebuilt binary, building from source (go install $VERSION)..."
  # go install tag: latest or vX.Y.Z (fallback to direct if proxy not yet indexed)
  TAG="$VERSION"
  [ "$TAG" = "latest" ] && TAG="latest"
  if ! go install "github.com/${REPO}/cmd/kyfram@$TAG" 2>/dev/null; then
    echo "retry with GOPROXY=direct..."
    GOPROXY=direct GOSUMDB=off go install "github.com/${REPO}/cmd/kyfram@$TAG"
  fi
  GOBIN=$(go env GOBIN 2>/dev/null); [ -z "$GOBIN" ] && GOBIN=$(go env GOPATH 2>/dev/null)/bin
  if [ -x "$GOBIN/$BIN" ]; then
    if [ "$GOBIN" != "$INSTALL_DIR" ] && [ -w "$INSTALL_DIR" ]; then
      cp "$GOBIN/$BIN" "$INSTALL_DIR/$BIN" 2>/dev/null || true
      echo "built $GOBIN/$BIN → $INSTALL_DIR/$BIN"
    else
      echo "built $GOBIN/$BIN — ensure $GOBIN is on PATH"
      INSTALL_DIR="$GOBIN"
    fi
  fi
fi
if "$INSTALL_DIR/$BIN" --help >/dev/null 2>&1; then
  echo "kyfram ready: $INSTALL_DIR/$BIN --help"
elif command -v "$BIN" >/dev/null 2>&1; then
  echo "kyfram ready: $(command -v $BIN)"
else
  echo "installed to $INSTALL_DIR — add it to PATH"
fi
