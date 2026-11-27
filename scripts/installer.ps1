$ErrorActionPreference = "Stop"

$requiredTags = "with_gvisor,with_quic,with_utls,with_grpc"
if (-not $env:GOFLAGS) {
    $env:GOFLAGS = "-tags=$requiredTags"
} elseif ($env:GOFLAGS -notmatch '-tags=') {
    $env:GOFLAGS += " -tags=$requiredTags"
}
$env:CGO_ENABLED = "0"

$platform = if ($args.Count -gt 0) { $args[0] } else { "windows/amd64" }
Write-Host "Building qbox installer for $platform"
go tool mygo build -platform $platform
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$out = Join-Path $PSScriptRoot "..\build"
$installer = Get-ChildItem -Path $out -Filter "*-setup-*.exe" -File -Recurse |
    Sort-Object LastWriteTime |
    Select-Object -Last 1
if (-not $installer) {
    Write-Error "MyGo did not create a Windows installer in $out"
    exit 1
}
Write-Host "Installer: $($installer.FullName)"
