# Install a Momentum release as a per-user launcher, or as a Windows service.
# Service registration requires an elevated PowerShell prompt.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string]$BinaryPath,
    [string]$InstallDirectory = (Join-Path $env:LOCALAPPDATA 'Momentum'),
    [string]$DataDirectory = (Join-Path $env:APPDATA 'Momentum'),
    [switch]$RegisterService,
    [string]$ServiceName = 'Momentum'
)

$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
    throw "Release binary was not found: $BinaryPath"
}

New-Item -ItemType Directory -Force -Path $InstallDirectory, $DataDirectory | Out-Null
$target = Join-Path $InstallDirectory 'momentum-server.exe'
Copy-Item -LiteralPath $BinaryPath -Destination $target -Force

$launcher = Join-Path $InstallDirectory 'Run-Momentum.ps1'
$runScript = Join-Path $PSScriptRoot 'Run-Momentum.ps1'
Copy-Item -LiteralPath $runScript -Destination $launcher -Force

if ($RegisterService) {
    if (-not ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Registering a Windows service requires an elevated PowerShell prompt.'
    }
    if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
        throw "A service named '$ServiceName' already exists; remove it explicitly before reinstalling."
    }
    New-Service -Name $ServiceName -BinaryPathName ('"{0}"' -f $target) -DisplayName 'Momentum Task Server' -StartupType Automatic
    Start-Service -Name $ServiceName
    Write-Host "Installed and started Windows service '$ServiceName'."
} else {
    Write-Host "Installed $target"
    Write-Host "Run: powershell -ExecutionPolicy Bypass -File `"$launcher`""
}
