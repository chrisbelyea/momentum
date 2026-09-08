# Install a Momentum release as a per-user launcher, or as a Windows service.
# Service registration requires an elevated PowerShell prompt.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string]$BinaryPath,
    [string]$InstallDirectory = (Join-Path $env:LOCALAPPDATA 'Momentum'),
    [string]$DataDirectory = (Join-Path $env:APPDATA 'Momentum'),
    [switch]$RegisterService,
    [string]$ServiceName = 'Momentum',
    [string]$EncryptionKey
)

$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
    throw "Release binary was not found: $BinaryPath"
}

# A Windows service runs as LocalSystem unless an explicit service account is
# configured. Keep service defaults outside the interactive user's profile so
# SCM can read the binary and write the database/certificates reliably. The
# per-user launcher keeps its separate LOCALAPPDATA/APPDATA defaults.
if ($RegisterService) {
    if (-not $PSBoundParameters.ContainsKey('InstallDirectory')) {
        $InstallDirectory = Join-Path $env:ProgramFiles 'Momentum'
    }
    if (-not $PSBoundParameters.ContainsKey('DataDirectory')) {
        $DataDirectory = Join-Path $env:ProgramData 'Momentum'
    }
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
    if ([string]::IsNullOrWhiteSpace($EncryptionKey)) {
        throw 'Registering a production Windows service requires -EncryptionKey. Do not use MOMENTUM_DEV_MODE for a service.'
    }

    # Preserve explicit custom paths while granting only the service identity
    # the access it needs. This also makes custom paths deterministic instead
    # of relying on whichever ACLs happened to exist on the host.
    & icacls.exe $InstallDirectory /grant:r 'SYSTEM:(OI)(CI)(RX)' /T | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not grant the Windows service access to: $InstallDirectory" }
    & icacls.exe $DataDirectory /grant:r 'SYSTEM:(OI)(CI)(F)' /T | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not grant the Windows service access to: $DataDirectory" }

    New-Service -Name $ServiceName -BinaryPathName ('"{0}"' -f $target) -DisplayName 'Momentum Task Server' -StartupType Automatic
    # New-Service has no environment argument. Per-service environment values
    # are read by SCM when the service process starts.
    $serviceKey = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
    New-ItemProperty -Path $serviceKey -Name Environment -PropertyType MultiString -Force -Value @(
        "MOMENTUM_SERVICE_NAME=$ServiceName",
        "DB_PATH=$(Join-Path $DataDirectory 'momentum.db')",
        "PORT=8443",
        "MOMENTUM_DEV_CERT_DIR=$(Join-Path $DataDirectory 'dev-certs')",
        "MOMENTUM_ENCRYPTION_KEY=$EncryptionKey"
    ) | Out-Null
    Start-Service -Name $ServiceName
    Write-Host "Installed and started Windows service '$ServiceName'."
} else {
    Write-Host "Installed $target"
    Write-Host "Run: powershell -ExecutionPolicy Bypass -File `"$launcher`""
}
