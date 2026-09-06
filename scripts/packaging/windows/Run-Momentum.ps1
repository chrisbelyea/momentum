# Launch Momentum with writable per-user Windows paths.
[CmdletBinding()]
param(
    [string]$BinaryPath = (Join-Path $PSScriptRoot 'momentum-server.exe'),
    [string]$DataDirectory = (Join-Path $env:APPDATA 'Momentum'),
    [int]$Port = 8080
)

$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
    throw "Momentum binary was not found: $BinaryPath"
}

New-Item -ItemType Directory -Force -Path $DataDirectory | Out-Null
$env:DB_PATH = Join-Path $DataDirectory 'momentum.db'
$env:PORT = [string]$Port
& $BinaryPath
exit $LASTEXITCODE
