# Linux client design — Issue #132

Contract: `docs/native-client-contract.md` (merged PR #137). Framework: Vala + GTK 4; libsoup 3 for HTTPS, JSON-GLib for the versioned API and libsecret/Secret Service for session cookies. The intended native packages are deb for Ubuntu/Debian and rpm for Fedora; Arch is a later portability target. Ubuntu 24.04 glibc + distro GTK/libsecret is the proposed initial baseline, not an established release guarantee. The source tree under `client/linux` does not yet produce those packages.

Discovery is constrained to the packaged server at `https://127.0.0.1:8443` by default. Remote HTTPS origins require explicit entry and normal certificate validation; no HTTP or insecure TLS mode. Credentials use Secret Service keyed by origin/account/type. The approved headless Argon2id/AES-GCM fallback is intentionally not shipped without an independent security review. Session logout must revoke server-side before deleting the local record; failed revocation remains visible.

The client currently offers default-backend task listing, create, advance/reopen, delete, API compatibility checks and an open-conflict count. Full board/list parity, conflict recovery controls, actual deb/rpm integration, release archive interoperability and upgrade validation remain open requirements. See `client/linux/README.md` for source build and limitations.
