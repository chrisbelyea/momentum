#!/usr/bin/env bash
# Momentum Linux DEB package builder for #176 platform-native installers.
set -euo pipefail

VERSION="${VERSION:-0.0.0}"
ARCH="${ARCH:-amd64}"
BINARY="${BINARY:-dist/momentum-server-linux-${ARCH}}"
PKGDIR="${PKGDIR:-dist/linux-deb}"
DEBNAME="momentum_${VERSION}_${ARCH}.deb"

mkdir -p "$PKGDIR/DEBIAN"
cat > "$PKGDIR/DEBIAN/control" <<EOF
Package: momentum
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${ARCH}
Maintainer: Momentum <maintainers@momentum.example>
Description: Momentum task server (native DEB package)
 Momentum brings your to-dos, reminders, and CalDAV tasks into one flow.
EOF

mkdir -p "$PKGDIR/usr/bin" "$PKGDIR/lib/systemd/system" "$PKGDIR/etc/systemd/system"
install -m 0755 "$BINARY" "$PKGDIR/usr/bin/momentum-server"

cat > "$PKGDIR/lib/systemd/system/momentum.service" <<'EOF'
[Unit]
Description=Momentum Task Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/momentum-server
Environment="DB_PATH=/var/lib/momentum/momentum.db"
Environment="MOMENTUM_ENCRYPTION_KEY_FILE=/var/lib/momentum/encryption.key"
WorkingDirectory=/var/lib/momentum
User=momentum
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

cat > "$PKGDIR/DEBIAN/preinst" <<'EOF'
#!/bin/sh
set -e
# Preserve existing data directory on upgrade.
if [ ! -d /var/lib/momentum ]; then
  mkdir -p /var/lib/momentum
  chmod 700 /var/lib/momentum
  chown momentum:momentum /var/lib/momentum
fi
EOF

cat > "$PKGDIR/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
  systemctl enable momentum.service || true
fi
EOF

cat > "$PKGDIR/DEBIAN/prerm" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = "upgrade" ]; then
  systemctl stop momentum.service 2>/dev/null || true
fi
EOF

cat > "$PKGDIR/DEBIAN/postrm" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = "purge" ]; then
  echo "Purge requested: user data in /var/lib/momentum is intentionally kept unless explicitly removed."
fi
EOF

chmod 0755 "$PKGDIR/DEBIAN/preinst" "$PKGDIR/DEBIAN/postinst" "$PKGDIR/DEBIAN/prerm" "$PKGDIR/DEBIAN/postrm"
dpkg-deb --build "$PKGDIR" "dist/${DEBNAME}"
echo "Built dist/${DEBNAME}"
