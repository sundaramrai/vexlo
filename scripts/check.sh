#!/usr/bin/env bash
set -euo pipefail

if ! command -v golangci-lint >/dev/null 2>&1; then
  echo "golangci-lint is not installed or not on PATH. Install it from https://golangci-lint.run/welcome/install/"
  exit 1
fi

echo "==> gofmt"
unformatted="$(gofmt -l cmd internal)"
if [[ -n "$unformatted" ]]; then
  echo "Unformatted files detected:"
  echo "$unformatted"
  exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> golangci-lint"
golangci-lint run

echo "==> go build"
go build ./...

echo "All checks passed."
