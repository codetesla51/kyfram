#!/bin/sh
set -e
# kyfram installer bootstrap — fetches source, hands over to the
# interactive Go installer (cmd/install, tracked in-repo).
# usage:
#   curl -fsSL https://codetesla51.gitlab.io/kyfram/install.sh | sh
#   curl -fsSL https://codetesla51.gitlab.io/kyfram/install.sh | sh -s -- v0.3.0
#   VERSION=v0.3.0 sh install.sh
REPO="codetesla51/kyfram"
VERSION="${VERSION:-${1:-latest}}"
command -v go >/dev/null 2>&1 || { echo "go not found — install Go 1.25+ from https://go.dev/dl" >&2; exit 1; }
command -v git >/dev/null 2>&1 || { echo "git not found — install git" >&2; exit 1; }
case "$VERSION" in latest) TAG="master" ;; v*) TAG="$VERSION" ;; *) TAG="v$VERSION" ;; esac
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT INT TERM
git clone --depth 1 ${TAG:+--branch "$TAG"} "https://gitlab.com/${REPO}.git" "$TMPDIR" >/dev/null
# Piped stdin can't prompt, so the bootstrap always answers yes.
(cd "$TMPDIR" && go run ./cmd/install --yes --version "$VERSION")
