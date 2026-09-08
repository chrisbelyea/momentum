#!/usr/bin/env bash
# Lightweight, dependency-free validation for native packaging files.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bash -n "${ROOT}/scripts/packaging/linux/run-momentum.sh" "${ROOT}/scripts/packaging/linux/install-user.sh" "${ROOT}/scripts/packaging/linux/test-launcher.sh"
grep -q 'PORT:-8443' "${ROOT}/scripts/packaging/linux/run-momentum.sh"
grep -q 'EnvironmentFile=' "${ROOT}/scripts/packaging/linux/momentum.service"
grep -q 'ReadWritePaths=' "${ROOT}/scripts/packaging/linux/momentum.service"
grep -q 'New-Service' "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
grep -q '\$env:DB_PATH' "${ROOT}/scripts/packaging/windows/Run-Momentum.ps1"
grep -q '\[int\]\$Port = 8443' "${ROOT}/scripts/packaging/windows/Run-Momentum.ps1"
grep -q 'MOMENTUM_ENCRYPTION_KEY' "${ROOT}/scripts/packaging/windows/Run-Momentum.ps1"
grep -q '\[string\]\$EncryptionKey' "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
grep -q 'MOMENTUM_SERVICE_NAME' "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
grep -q "Join-Path \$env:ProgramFiles 'Momentum'" "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
grep -q "Join-Path \$env:ProgramData 'Momentum'" "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
grep -q 'icacls.exe' "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
test -f "${ROOT}/scripts/packaging/windows/Test-Launcher.ps1"
test -f "${ROOT}/scripts/packaging/windows/Test-TaskWorkflow.ps1"
echo 'Native packaging files validated.'
