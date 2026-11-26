$ErrorActionPreference = "Stop"

$requiredTags = "with_gvisor,with_quic,with_utls,with_grpc"
if (-not $env:GOFLAGS) {
    $env:GOFLAGS = "-tags=$requiredTags"
} elseif ($env:GOFLAGS -notmatch '-tags=') {
    $env:GOFLAGS += " -tags=$requiredTags"
}

go tool mygo dev
