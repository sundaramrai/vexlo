Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not (Get-Command golangci-lint -ErrorAction SilentlyContinue)) {
    Write-Error "golangci-lint is not installed or not on PATH. Install it from https://golangci-lint.run/welcome/install/"
    exit 1
}

Write-Host "==> gofmt"
$unformatted = & gofmt -l cmd internal
if ($LASTEXITCODE -ne 0) {
    throw "gofmt failed"
}
if ($unformatted) {
    Write-Error "Unformatted files detected:`n$unformatted"
    exit 1
}

Write-Host "==> go vet"
& go vet ./...
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

Write-Host "==> golangci-lint"
& golangci-lint run
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

Write-Host "==> go build"
& go build ./...
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

Write-Host "All checks passed."
