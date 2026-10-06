# Build a real per-machine WiX 3 MSI. The service is manual until configured.
param(
    [string]$Version = '0.0.0',
    [string]$BinaryPath,
    [string]$OutputDir = 'dist/windows-msi'
)
$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) { throw "Missing Windows binary: $BinaryPath" }
$Version = $Version.TrimStart('v')
if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw "MSI requires a three-part numeric version: $Version" }
$wixBin = if ($env:WIX) { Join-Path $env:WIX 'bin' } else { Join-Path ${env:ProgramFiles(x86)} 'WiX Toolset v3.14\bin' }
if (-not (Test-Path (Join-Path $wixBin 'candle.exe'))) { throw 'WiX Toolset 3.14 candle.exe is required' }
if (-not (Test-Path (Join-Path $wixBin 'light.exe'))) { throw 'WiX Toolset 3.14 light.exe is required' }
$OutputDir = [IO.Path]::GetFullPath($OutputDir)
$BinaryPath = [IO.Path]::GetFullPath($BinaryPath)
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$source = [Security.SecurityElement]::Escape($BinaryPath)
$wxs = Join-Path $OutputDir 'Momentum.wxs'
@"
<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi">
  <Product Id="*" Name="Momentum" Language="1033" Version="$Version" Manufacturer="Momentum" UpgradeCode="2F5B6E3A-7C9D-4A1B-8E6F-0D3C5A9B2E71">
    <Package InstallerVersion="500" Compressed="yes" InstallScope="perMachine" Platform="x64" />
    <MajorUpgrade DowngradeErrorMessage="A newer version of Momentum is already installed." />
    <MediaTemplate EmbedCab="yes" />
    <Feature Id="ProductFeature" Title="Momentum" Level="1">
      <ComponentRef Id="ServerComponent" />
    </Feature>
    <Directory Id="TARGETDIR" Name="SourceDir">
      <Directory Id="ProgramFiles64Folder">
        <Directory Id="INSTALLFOLDER" Name="Momentum">
          <Component Id="ServerComponent" Guid="*" Win64="yes">
            <File Id="ServerFile" Source="$source" Name="momentum-server.exe" KeyPath="yes" />
            <ServiceInstall Id="MomentumService" Name="Momentum" DisplayName="Momentum Task Server" Description="Momentum HTTPS task server" Type="ownProcess" Start="demand" ErrorControl="normal" Account="LocalSystem" />
            <ServiceControl Id="MomentumServiceControl" Name="Momentum" Stop="both" Remove="uninstall" Wait="yes" />
          </Component>
        </Directory>
      </Directory>
    </Directory>
  </Product>
</Wix>
"@ | Set-Content -Path $wxs -Encoding UTF8
$obj = Join-Path $OutputDir 'Momentum.wixobj'
$msi = Join-Path $OutputDir "momentum-$Version-windows-amd64.msi"
& (Join-Path $wixBin 'candle.exe') -nologo -arch x64 -out $obj $wxs
if ($LASTEXITCODE -ne 0) { throw 'WiX candle failed' }
& (Join-Path $wixBin 'light.exe') -nologo -out $msi $obj
if ($LASTEXITCODE -ne 0) { throw 'WiX light failed' }
if (-not (Test-Path -LiteralPath $msi -PathType Leaf)) { throw "MSI not created: $msi" }
Write-Host "Built $msi"