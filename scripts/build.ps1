$ErrorActionPreference = "Stop"

$env:GOFLAGS = "-tags=with_gvisor,with_quic,with_utls,with_grpc"
$env:CGO_ENABLED = "0"

$platform = if ($args.Count -gt 0) { $args[0] } else { "windows/amd64" }
Write-Host "Building qbox for $platform"
go tool mygo build -platform $platform -skip-installer
