# Launch Momentum with writable per-user Windows paths.
[CmdletBinding()]
param(
    [string]$BinaryPath = (Join-Path $PSScriptRoot 'momentum-server.exe'),
    [string]$DataDirectory = (Join-Path $env:APPDATA 'Momentum'),
    [int]$Port = 8443,
    [string]$EncryptionKeyFile = (Join-Path $DataDirectory 'encryption.key')
)

$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
    throw "Momentum binary was not found: $BinaryPath"
}

New-Item -ItemType Directory -Force -Path $DataDirectory | Out-Null
$env:DB_PATH = Join-Path $DataDirectory 'momentum.db'
$env:PORT = [string]$Port
$env:MOMENTUM_DEV_CERT_DIR = Join-Path $DataDirectory 'dev-certs'

if ([string]::IsNullOrWhiteSpace($env:MOMENTUM_ENCRYPTION_KEY)) {
    if (Test-Path -LiteralPath $EncryptionKeyFile -PathType Leaf) {
        $env:MOMENTUM_ENCRYPTION_KEY = (Get-Content -LiteralPath $EncryptionKeyFile -Raw).Trim()
    } else {
        $bytes = [byte[]]::new(32)
        $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
        try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
        $env:MOMENTUM_ENCRYPTION_KEY = [Convert]::ToBase64String($bytes)
        [System.IO.File]::WriteAllText($EncryptionKeyFile, $env:MOMENTUM_ENCRYPTION_KEY, [System.Text.UTF8Encoding]::new($false))
        $acl = Get-Acl -LiteralPath $EncryptionKeyFile
        $acl.SetAccessRuleProtection($true, $false)
        $rule = [System.Security.AccessControl.FileSystemAccessRule]::new($env:USERNAME, 'FullControl', 'Allow')
        $acl.SetAccessRule($rule)
        Set-Acl -LiteralPath $EncryptionKeyFile -AclObject $acl
    }
}
if ([string]::IsNullOrWhiteSpace($env:MOMENTUM_ENCRYPTION_KEY)) {
    throw "Encryption key file is empty: $EncryptionKeyFile"
}
& $BinaryPath
exit $LASTEXITCODE
