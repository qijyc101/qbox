$ErrorActionPreference = "Stop"

$requiredTags = "with_gvisor,with_quic,with_utls,with_grpc"
if (-not $env:GOFLAGS) {
    $env:GOFLAGS = "-tags=$requiredTags"
} elseif ($env:GOFLAGS -notmatch '-tags=') {
    $env:GOFLAGS += " -tags=$requiredTags"
}
$env:CGO_ENABLED = "0"

Write-Host "staticcheck"
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "govulncheck"
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
