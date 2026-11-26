$ErrorActionPreference = "Stop"

$env:GOFLAGS = "-tags=with_gvisor,with_quic,with_utls,with_grpc"
$env:CGO_ENABLED = "0"

Write-Host "Tests without UI"
go test ./internal/config ./internal/subscription ./internal/rulesets ./internal/boxconf ./internal/core ./internal/platform

# Some Windows antivirus configurations block a freshly linked test
# executable in %TEMP%. Build it into the workspace and run it there.
$out = Join-Path $PSScriptRoot "..\build\test"
New-Item -ItemType Directory -Force $out | Out-Null
$appTest = Join-Path $out "app.test.exe"
Write-Host "Building UI tests"
go test -c -o $appTest ./internal/app

Write-Host "Running UI tests"
& $appTest "-test.v"
$code = $LASTEXITCODE
Remove-Item $appTest -Force -ErrorAction SilentlyContinue
if ($code -ne 0) { exit $code }

Write-Host "go vet"
go vet ./...

Write-Host "mygo vet"
go tool mygo vet .

Write-Host "All checks passed"
