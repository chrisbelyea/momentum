#!/usr/bin/env bash
# Momentum Linux RPM package builder for #176 platform-native installers.
set -euo pipefail

VERSION="${VERSION:-0.0.0}"
ARCH="${ARCH:-x86_64}"
BINARY="${BINARY:-dist/momentum-server-linux-amd64}"
RPMNAME="momentum-${VERSION}-${ARCH}.rpm"

mkdir -p dist/linux-rpm/BUILD
install -m 0755 "$BINARY" dist/linux-rpm/BUILD/momentum-server

cat > dist/linux-rpm/BUILD/momentum.spec <<EOF
Name: momentum
Version: ${VERSION}
Release: 1
Summary: Momentum task server
License: MIT
Architecture: ${ARCH}
Group: Applications/Utilities
BuildArch: ${ARCH}

%description
Momentum brings your to-dos, reminders, and CalDAV tasks into one flow.

%prep

%build

%install
mkdir -p %{buildroot}/usr/bin %{buildroot}/lib/systemd/system %{buildroot}/var/lib/momentum
install -m 0755 %{_topdir}/BUILD/momentum-server %{buildroot}/usr/bin/momentum-server
install -m 0644 %{_topdir}/BUILD/momentum.service %{buildroot}/lib/systemd/system/momentum.service

%post
systemctl daemon-reload || true
systemctl enable momentum.service || true

%preun
if [ "$1" = "upgrade" ]; then
  systemctl stop momentum.service 2>/dev/null || true
fi

%postun
# Do not remove /var/lib/momentum on uninstall; user data is preserved.

%files
/usr/bin/momentum-server
/lib/systemd/system/momentum.service
%attr(700,momentum,momentum) /var/lib/momentum

%changelog
EOF

cp /dev/null dist/linux-rpm/BUILD/momentum.service
echo "Built RPM spec at dist/linux-rpm/BUILD/momentum.spec (run rpmbuild to package)"
