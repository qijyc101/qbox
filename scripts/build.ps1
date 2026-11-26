$ErrorActionPreference = "Stop"

$requiredTags = "with_gvisor,with_quic,with_utls,with_grpc"
if (-not $env:GOFLAGS) {
    $env:GOFLAGS = "-tags=$requiredTags"
} elseif ($env:GOFLAGS -notmatch '-tags=') {
    $env:GOFLAGS += " -tags=$requiredTags"
}
$env:CGO_ENABLED = "0"

$platform = if ($args.Count -gt 0) { $args[0] } else { "windows/amd64" }
Write-Host "Building qbox for $platform"
go tool mygo build -platform $platform -skip-installer
