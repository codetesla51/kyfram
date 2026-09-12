#!/bin/sh
set -e
cd "$(dirname "$0")/.."
# local CI gate — mirrors .gitlab-ci.yml (build, vet, test, lint if installed)
go mod download
go build ./...
go vet ./...
go test ./...
if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run
else
  echo "skip: golangci-lint not installed"
fi
