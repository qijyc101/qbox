$ErrorActionPreference = "Stop"

$requiredTags = "with_gvisor,with_quic,with_utls,with_grpc"
if (-not $env:GOFLAGS) {
    $env:GOFLAGS = "-tags=$requiredTags"
} elseif ($env:GOFLAGS -notmatch '-tags=') {
    $env:GOFLAGS += " -tags=$requiredTags"
}
$env:CGO_ENABLED = "0"

$unformatted = gofmt -l main.go internal
if ($unformatted) {
    Write-Host "gofmt needed:"
    $unformatted
    exit 1
}

Write-Host "Tests without UI"
$packages = go list ./... | Where-Object { $_ -ne "qbox/internal/app" }
go test -count=1 $packages
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# Some Windows antivirus configurations block a freshly linked test
# executable in %TEMP%. Build it into the workspace and run it there.
$out = Join-Path $PSScriptRoot "..\build\test"
New-Item -ItemType Directory -Force $out | Out-Null
$appTest = Join-Path $out "app.test.exe"
Write-Host "Building UI tests"
go test -c -o $appTest ./internal/app
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Running UI tests"
& $appTest "-test.v"
$code = $LASTEXITCODE
Remove-Item $appTest -Force -ErrorAction SilentlyContinue
if ($code -ne 0) { exit $code }

Write-Host "go vet"
go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "mygo vet"
go tool mygo vet .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "All checks passed"
