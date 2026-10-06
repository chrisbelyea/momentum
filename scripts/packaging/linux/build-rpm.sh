#!/usr/bin/env bash
# Build a system-wide RPM package from the release binary.
set -euo pipefail

VERSION="${VERSION:-v0.0.0}"
VERSION="${VERSION#v}"
VERSION="${VERSION:-0.0.0}"
ARCH="${ARCH:-x86_64}"
BINARY="${BINARY:?BINARY must name the extracted release executable}"
case "$ARCH" in x86_64|aarch64) ;; *) echo "Unsupported RPM architecture: $ARCH" >&2; exit 1;; esac
[[ -f "$BINARY" ]] || { echo "Missing binary: $BINARY" >&2; exit 1; }
command -v rpmbuild >/dev/null || { echo 'rpmbuild is required' >&2; exit 1; }
top="$(pwd)/dist/linux-rpm"
mkdir -p "$top"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
install -m 0755 "$BINARY" "$top/SOURCES/momentum-server"
install -m 0755 scripts/packaging/linux/run-momentum.sh "$top/SOURCES/run-momentum.sh"
install -m 0644 scripts/packaging/linux/momentum-package.service "$top/SOURCES/momentum.service"
cat > "$top/SPECS/momentum.spec" <<'EOF'
Name: momentum
Version: %{momentum_version}
Release: 1
Summary: Momentum task server
License: MIT
Requires: systemd, shadow-utils
BuildArch: %{momentum_arch}

%description
Momentum serves tasks securely over HTTPS on loopback by default.

%install
install -D -m 0755 %{_sourcedir}/momentum-server %{buildroot}%{_bindir}/momentum-server
install -D -m 0755 %{_sourcedir}/run-momentum.sh %{buildroot}%{_libdir}/momentum/run-momentum.sh
install -D -m 0644 %{_sourcedir}/momentum.service %{buildroot}%{_unitdir}/momentum.service

%pre
getent group momentum >/dev/null || groupadd -r momentum
getent passwd momentum >/dev/null || useradd -r -g momentum -d /var/lib/momentum -s /sbin/nologin momentum

%post
install -d -m 0700 -o momentum -g momentum /var/lib/momentum
systemctl daemon-reload >/dev/null 2>&1 || :
systemctl enable momentum.service >/dev/null 2>&1 || :
systemctl try-restart momentum.service >/dev/null 2>&1 || :

%preun
if [ "$1" -eq 0 ]; then
  systemctl stop momentum.service >/dev/null 2>&1 || :
  systemctl disable momentum.service >/dev/null 2>&1 || :
fi

%postun
systemctl daemon-reload >/dev/null 2>&1 || :
# /var/lib/momentum is not owned by this package; never remove user data.

%files
%{_bindir}/momentum-server
%{_libdir}/momentum/run-momentum.sh
%{_unitdir}/momentum.service
EOF
rpmbuild -bb "$top/SPECS/momentum.spec" --target "$ARCH" \
  --define "_topdir $top" --define "momentum_version $VERSION" --define "momentum_arch $ARCH" \
  --define '_unitdir /usr/lib/systemd/system' --define '_libdir /usr/lib' \
  --define '_build_id_links none'
mkdir -p dist/packages
find "$top/RPMS" -name '*.rpm' -exec cp -v {} dist/packages/ \;
test "$(find dist/packages -name '*.rpm' | wc -l)" -gt 0