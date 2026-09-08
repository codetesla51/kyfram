#!/bin/sh
set -e
# kyfram installer — clones source and builds for the host (no prebuilt binaries to track)
# usage:
#   curl -fsSL https://raw.githubusercontent.com/codetesla51/kyfram/master/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/codetesla51/kyfram/master/install.sh | sh -s -- v0.2.0
#   VERSION=v0.2.0 sh install.sh
REPO="codetesla51/kyfram"
BIN="kyfram"
VERSION="${VERSION:-${1:-latest}}"
command -v go >/dev/null 2>&1 || { echo "go not found — install Go 1.25+" >&2; exit 1; }
command -v git >/dev/null 2>&1 || { echo "git not found — install git" >&2; exit 1; }
INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then INSTALL_DIR="$HOME/.local/bin"; mkdir -p "$INSTALL_DIR"; fi
if [ "$VERSION" = "latest" ]; then TAG="master"; else case "$VERSION" in v*) TAG="$VERSION" ;; *) TAG="v$VERSION" ;; esac; fi
echo "installing kyfram ($TAG) → $INSTALL_DIR/$BIN"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT INT TERM
if [ "$TAG" = "master" ]; then
  git clone --depth 1 "https://github.com/${REPO}.git" "$TMPDIR"
else
  git clone --depth 1 --branch "$TAG" "https://github.com/${REPO}.git" "$TMPDIR"
fi
(cd "$TMPDIR" && go build -o "$INSTALL_DIR/$BIN" ./cmd/kyfram)
echo "kyfram ready: $INSTALL_DIR/$BIN --help"
