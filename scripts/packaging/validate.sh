#!/usr/bin/env bash
# Lightweight, dependency-free validation for native packaging files.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bash -n "${ROOT}/scripts/packaging/linux/run-momentum.sh" "${ROOT}/scripts/packaging/linux/install-user.sh" "${ROOT}/scripts/packaging/linux/uninstall-user.sh" "${ROOT}/scripts/packaging/linux/test-launcher.sh" "${ROOT}/scripts/packaging/linux/test-systemd-service.sh"
grep -q 'PORT:-8443' "${ROOT}/scripts/packaging/linux/run-momentum.sh"
grep -q 'EnvironmentFile=' "${ROOT}/scripts/packaging/linux/momentum.service"
grep -q 'ReadWritePaths=' "${ROOT}/scripts/packaging/linux/momentum.service"
grep -q 'MOMENTUM_SERVICE_NAME' "${ROOT}/scripts/packaging/linux/install-user.sh"
grep -q -- '--install-only' "${ROOT}/scripts/packaging/linux/install-user.sh"
grep -q 'New-Service' "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
grep -q '\$env:DB_PATH' "${ROOT}/scripts/packaging/windows/Run-Momentum.ps1"
grep -q '\[int\]\$Port = 8443' "${ROOT}/scripts/packaging/windows/Run-Momentum.ps1"
grep -q 'MOMENTUM_ENCRYPTION_KEY' "${ROOT}/scripts/packaging/windows/Run-Momentum.ps1"
grep -q '\[string\]\$EncryptionKey' "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
grep -q 'MOMENTUM_SERVICE_NAME' "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
test -f "${ROOT}/scripts/packaging/windows/Test-Launcher.ps1"
test -x "${ROOT}/scripts/packaging/linux/uninstall-user.sh"
test -x "${ROOT}/scripts/packaging/linux/test-systemd-service.sh"
echo 'Native packaging files validated.'
