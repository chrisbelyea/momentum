#!/usr/bin/env bash
# Lightweight, dependency-free validation for native packaging files.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bash -n "${ROOT}/scripts/packaging/linux/run-momentum.sh" "${ROOT}/scripts/packaging/linux/install-user.sh"
grep -q 'ExecStart=%h/.local/bin/momentum-server' "${ROOT}/scripts/packaging/linux/momentum.service"
grep -q 'New-Service' "${ROOT}/scripts/packaging/windows/Install-Momentum.ps1"
grep -q '\$env:DB_PATH' "${ROOT}/scripts/packaging/windows/Run-Momentum.ps1"
echo 'Native packaging files validated.'
