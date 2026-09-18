#!/usr/bin/env pwsh
# Momentum Windows MSI installer builder for #176 platform-native installers.
param(
    [string]$Version = "0.0.0",
    [string]$BinaryPath = "dist/momentum-server-windows-amd64.exe",
    [string]$OutputDir = "dist/windows-msi"
)

$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

$wixVersion = "3.14.1"
$wixBin = Join-Path $env:TEMP "wix"
if (-not (Test-Path (Join-Path $wixBin "candle.exe"))) {
    Write-Host "WiX Toolset required. Install WiX ${wixVersion} and set WIX path."
    exit 1
}

$wxs = Join-Path $OutputDir "Momentum.wxs"
@"
<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi">
  <Product Id="*" Name="Momentum" Language="1033" Version="$Version" Manufacturer="Momentum" UpgradeCode="2F5B6E3A-7C9D-4A1B-8E6F-0D3C5A9B2E71">
    <Package InstallerVersion="500" Compressed="yes" InstallScope="perMachine" />
    <MajorUpgrade DowngradeErrorMessage="A newer version of Momentum is already installed." />
    <Property Id="MOMENTUM_DATA_DIR" Value="[CommonAppDataFolder]Momentum" />
    <MediaTemplate EmbedCab="yes" />
    <Feature Id="ProductFeature" Title="Momentum" Level="1">
      <ComponentGroupRef Id="ProductComponents" />
    </Feature>
    <Directory Id="TARGETDIR" Name="SourceDir">
      <Directory Id="ProgramFiles64Folder">
        <Directory Id="INSTALLFOLDER" Name="Momentum">
          <ComponentGroup Id="ProductComponents">
            <Component Id="cmpMomentumServer" Guid="*">
              <File Id="filMomentumServer" Source="$BinaryPath" KeyPath="yes" />
            </Component>
          </ComponentGroup>
        </Directory>
      </Directory>
      <Directory Id="CommonAppDataFolder">
        <Directory Id="MOMENTUM_DATA" Name="Momentum" />
      </Directory>
    </Directory>
  </Product>
</Wix>
"@ | Set-Content -Path $wxs -Encoding UTF8

Write-Host "WXS authored at $wxs (run candle/light to build MSI)"
