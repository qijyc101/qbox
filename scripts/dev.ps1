$ErrorActionPreference = "Stop"

$env:GOFLAGS = "-tags=with_gvisor,with_quic,with_utls,with_grpc"
go tool mygo dev
