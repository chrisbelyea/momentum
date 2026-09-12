[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string]$BinaryPath,
    [string]$LauncherPath = (Join-Path $PSScriptRoot 'Run-Momentum.ps1'),
    [int]$Port = 18446
)

$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) { throw "Binary not found: $BinaryPath" }
if (-not (Test-Path -LiteralPath $LauncherPath -PathType Leaf)) { throw "Launcher not found: $LauncherPath" }

$dataDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("momentum-packaging-" + [Guid]::NewGuid().ToString('N'))
$stdoutPath = Join-Path $dataDirectory 'server.stdout.log'
$stderrPath = Join-Path $dataDirectory 'server.stderr.log'
New-Item -ItemType Directory -Force -Path $dataDirectory | Out-Null
$process = $null

function ConvertTo-ProcessArgument {
    param([Parameter(Mandatory = $true)] [string]$Value)

    # Windows PowerShell 5.1 joins Start-Process -ArgumentList into one
    # command line. Quote path arguments using CommandLineToArgvW-compatible
    # escaping so installation and temp paths containing spaces survive.
    if ($Value -notmatch '[\s"]') { return $Value }
    $escaped = [regex]::Replace($Value, '(\\*)"', '$1$1\"')
    $escaped = [regex]::Replace($escaped, '(\\+)$', '$1$1')
    return '"' + $escaped + '"'
}

try {
    $argumentList = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File',
        (ConvertTo-ProcessArgument $LauncherPath), '-BinaryPath',
        (ConvertTo-ProcessArgument $BinaryPath), '-DataDirectory',
        (ConvertTo-ProcessArgument $dataDirectory), '-Port', $Port)
    $process = Start-Process -FilePath 'powershell.exe' -ArgumentList $argumentList -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -PassThru
    $ready = $false
    for ($i = 0; $i -lt 20; $i++) {
        try {
            & curl.exe -kfsS "https://127.0.0.1:$Port/health" | Out-Null
            if ($LASTEXITCODE -eq 0) {
                $ready = $true
                break
            }
        } catch {
            Start-Sleep -Seconds 1
        }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) {
        Get-Content -LiteralPath $stdoutPath, $stderrPath -ErrorAction SilentlyContinue
        throw 'Windows packaged launcher did not become healthy'
    }
    if (-not (Test-Path -LiteralPath (Join-Path $dataDirectory 'encryption.key'))) { throw 'launcher did not persist encryption key' }
    if (-not (Test-Path -LiteralPath (Join-Path $dataDirectory 'momentum.db'))) { throw 'launcher did not create database' }
    Write-Host 'Windows packaged launcher passed first-run health/configuration checks.'
} finally {
    if ($process -and -not $process.HasExited) { Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue }
    Remove-Item -LiteralPath $dataDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
