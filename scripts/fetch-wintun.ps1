$ErrorActionPreference = "Stop"

$version = "0.14.1"
$root = Split-Path -Parent $PSScriptRoot
$zip = Join-Path $env:TEMP "wintun-$version.zip"
$dst = Join-Path $env:TEMP "wintun-$version"

Write-Host "Downloading Wintun $version"
Invoke-WebRequest -UseBasicParsing "https://www.wintun.net/builds/wintun-$version.zip" -OutFile $zip
if (Test-Path $dst) { Remove-Item -Recurse -Force $dst }
Expand-Archive $zip -DestinationPath $dst

$targets = @{
    "amd64" = "windows-amd64"
    "arm64" = "windows-arm64"
}
foreach ($arch in $targets.Keys) {
    $source = Join-Path $dst "wintun\bin\$arch\wintun.dll"
    if (-not (Test-Path $source)) {
        throw "wintun.dll for $arch not found in the archive"
    }
    $dir = Join-Path $root "resources\$($targets[$arch])"
    New-Item -ItemType Directory -Force $dir | Out-Null
    Copy-Item $source (Join-Path $dir "wintun.dll") -Force
    Write-Host "Installed $dir\wintun.dll"
}

Write-Host "Done. Wintun is licensed under the terms shipped in the official archive."
