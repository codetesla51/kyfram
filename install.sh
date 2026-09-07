#!/bin/sh
set -e
# ponytail: download prebuilt binary, fallback to go install if no release
REPO="codetesla51/kyfram"
BIN="kyfram"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in x86_64|x86-64|amd64) ARCH="amd64" ;; aarch64|arm64) ARCH="arm64" ;; *) echo "unsupported arch: $ARCH" >&2; exit 1 ;; esac
case "$OS" in linux|darwin) ;; *) echo "unsupported os: $OS" >&2; exit 1 ;; esac
FILE="${BIN}-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${FILE}"
INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then INSTALL_DIR="$HOME/.local/bin"; mkdir -p "$INSTALL_DIR"; fi
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
  echo "installed $INSTALL_DIR/$BIN ($OS/$ARCH) from release"
else
  rm -f "$TMP"
  if ! command -v go >/dev/null 2>&1; then
    echo "no prebuilt binary and go not found — install go or check releases at https://github.com/$REPO/releases" >&2
    exit 1
  fi
  echo "no prebuilt binary, building from source with go install..."
  go install "github.com/${REPO}/cmd/kyfram@latest"
  # go install location
  GOBIN=$(go env GOBIN 2>/dev/null); [ -z "$GOBIN" ] && GOBIN=$(go env GOPATH 2>/dev/null)/bin
  if [ -x "$GOBIN/$BIN" ] && [ "$GOBIN" != "$INSTALL_DIR" ]; then
    echo "built $GOBIN/$BIN — add $GOBIN to PATH or move to $INSTALL_DIR"
  else
    echo "built $GOBIN/$BIN"
  fi
  INSTALL_DIR="$GOBIN"
fi
if "$INSTALL_DIR/$BIN" --help >/dev/null 2>&1; then
  echo "kyfram ready: $INSTALL_DIR/$BIN --help"
elif command -v "$BIN" >/dev/null 2>&1; then
  echo "kyfram ready: $(command -v $BIN)"
else
  echo "installed to $INSTALL_DIR — ensure it is on PATH"
fi
