#!/usr/bin/env bash
# Build a system-wide Debian package from the release binary.
set -euo pipefail

VERSION="${VERSION:-v0.0.0}"
VERSION="${VERSION#v}"
VERSION="${VERSION:-0.0.0}"
ARCH="${ARCH:-amd64}"
BINARY="${BINARY:?BINARY must name the extracted release executable}"
case "$ARCH" in amd64|arm64) ;; *) echo "Unsupported DEB architecture: $ARCH" >&2; exit 1;; esac
[[ -f "$BINARY" ]] || { echo "Missing binary: $BINARY" >&2; exit 1; }
mkdir -p dist
staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT
mkdir -p "$staging/DEBIAN" "$staging/usr/bin" "$staging/usr/lib/momentum" "$staging/lib/systemd/system"
chmod 0755 "$staging"
install -m 0755 "$BINARY" "$staging/usr/bin/momentum-server"
install -m 0755 scripts/packaging/linux/run-momentum.sh "$staging/usr/lib/momentum/run-momentum.sh"
install -m 0644 scripts/packaging/linux/momentum-package.service "$staging/lib/systemd/system/momentum.service"
cat > "$staging/DEBIAN/control" <<EOF
Package: momentum
Version: $VERSION
Section: utils
Priority: optional
Architecture: $ARCH
Maintainer: Momentum maintainers <https://github.com/chrisbelyea/momentum>
Depends: adduser, systemd
Description: Momentum task server (system service)
 Momentum serves tasks securely over HTTPS on loopback by default.
EOF
cat > "$staging/DEBIAN/preinst" <<'EOF'
#!/bin/sh
set -e
if ! getent group momentum >/dev/null; then addgroup --system momentum; fi
if ! getent passwd momentum >/dev/null; then
  adduser --system --ingroup momentum --home /var/lib/momentum --no-create-home --disabled-login momentum
fi
EOF
cat > "$staging/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
install -d -m 0700 -o momentum -g momentum /var/lib/momentum
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
  if [ "$1" = configure ]; then systemctl enable momentum.service || true; systemctl try-restart momentum.service || true; fi
fi
EOF
cat > "$staging/DEBIAN/prerm" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = remove ] || [ "$1" = deconfigure ]; then
  if command -v systemctl >/dev/null 2>&1; then systemctl stop momentum.service || true; systemctl disable momentum.service || true; fi
fi
EOF
cat > "$staging/DEBIAN/postrm" <<'EOF'
#!/bin/sh
set -e
if command -v systemctl >/dev/null 2>&1; then systemctl daemon-reload || true; fi
# Never delete /var/lib/momentum, including on purge: DB, keys, config, certs belong to the user.
EOF
chmod 0755 "$staging/DEBIAN/"{preinst,postinst,prerm,postrm}
dpkg-deb --root-owner-group --build "$staging" "dist/momentum_${VERSION}_${ARCH}.deb"