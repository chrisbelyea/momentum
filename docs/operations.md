# Operating Momentum

This guide describes the supported single-user deployment of the Momentum `v0.2.2`
release on Linux and Windows. The release binary is self-contained: it embeds the
web assets and initializes SQLite on startup. The server is the supported client
for this release; native clients and automatic external-backend synchronization
remain tracked work in [#68](https://github.com/chrisbelyea/momentum/issues/68),
[#144](https://github.com/chrisbelyea/momentum/issues/144), and [#71](https://github.com/chrisbelyea/momentum/issues/71).

## Before first use

1. Download the archive for the operating system from the
   [v0.2.2 release](https://github.com/chrisbelyea/momentum/releases/tag/v0.2.2).
2. Verify its SHA-256 digest against `checksums.txt`.
3. Extract the archive to a directory owned by the account that will run Momentum.
4. Use the platform launcher below. It creates a writable data directory and a
   per-install encryption key. Keep the key with the database; losing it makes
   encrypted backend configuration unreadable.

The binary generates a self-signed development certificate on first use. This is
appropriate for local evaluation only. Use an operator-managed certificate for a
network deployment; see [tls-setup.md](tls-setup.md).

## Linux

For an interactive launch from the extracted archive:

```bash
chmod +x momentum-server scripts/packaging/linux/run-momentum.sh
scripts/packaging/linux/run-momentum.sh ./momentum-server
```

The launcher defaults to `~/.config/Momentum/momentum.db`,
`~/.config/Momentum/encryption.key`, and HTTPS port `8443`. Set
`MOMENTUM_DATA_DIR`, `DB_PATH`, `PORT`, or `MOMENTUM_ENCRYPTION_KEY_FILE` to
override those paths. The generated certificate is stored below the data
directory by the launcher.

The archive also includes `install-user.sh` for a per-user systemd service:

```bash
scripts/packaging/linux/install-user.sh ./momentum-server
systemctl --user status momentum.service
```

This helper is subject to the Linux service lifecycle work in
[#151](https://github.com/chrisbelyea/momentum/issues/151). If user systemd is
not available or the service does not start, use `run-momentum.sh` directly.
Do not copy the old root-level `/etc/systemd/system` example from historical
documentation; the release helper intentionally stays within the invoking
user's directories.

## Windows

From PowerShell in the extracted archive:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\scripts\packaging\windows\Install-Momentum.ps1 -BinaryPath .\momentum-server.exe
& "$env:LOCALAPPDATA\Momentum\Run-Momentum.ps1"
```

The per-user launcher defaults to `%APPDATA%\Momentum\momentum.db`, stores its
development certificate and encryption key beneath that directory, and listens
on HTTPS port `8443`.

The `-RegisterService` option exists for evaluation, but the v0.2.2 helper's
default profile is a per-user launcher. Windows Service Control Manager normally
runs services as `LocalSystem`, which cannot use an interactive user's profile
directory. Do not deploy the service option with default paths until
[#142](https://github.com/chrisbelyea/momentum/issues/142) is resolved; use the
per-user launcher for the current release.

## Create the first account

The v0.2.2 root page requires an authenticated session, while registration and
login are JSON endpoints. The following creates the first account and stores its
session cookie in `cookies.txt`:

```bash
curl -k -c cookies.txt -X POST https://localhost:8443/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"use-a-password-at-least-12-characters"}'
curl -k -b cookies.txt https://localhost:8443/
```

Open the same URL in a browser after importing or accepting the development
certificate. Browser-native login/onboarding is tracked in [#145](https://github.com/chrisbelyea/momentum/issues/145).

Registration creates a local task backend. Task creation, editing, deletion,
and status changes are available from the authenticated web board and list.

## Fresh install and upgrade

The release binary owns SQLite initialization and upgrades. On startup it applies
the canonical changelog and upgrades the legacy pre-#59 layouts without replacing
existing rows. Do not use `scripts/db/init_dev.sh` or run a second migration tool
against a live user database; those are development/fixture helpers. See
[database-schema.md](database-schema.md) for the schema policy.

For either a fresh install or an upgrade:

1. Stop any existing Momentum process/service.
2. Preserve the database and encryption key as described below.
3. Replace the executable (keep the previous executable for rollback).
4. Start the new executable with the same data path and encryption key.
5. Verify `GET /health`, sign in, and create/update/delete a disposable task.

The release repository's complete binary workflow is reproducible with:

```bash
PORT=18443 scripts/release/integration-test.sh ./momentum-server
```

That command uses a temporary database and development mode; it is a validation
check, not a production migration command.

## Backup

Stop Momentum before a file-level backup. Include the database's `-wal` and `-shm`
files if present, and copy the encryption key used by the process. Prefer SQLite's
backup API when `sqlite3` is available:

```bash
sqlite3 "$DB_PATH" ".backup '$BACKUP_DIR/momentum.db'"
cp "$MOMENTUM_ENCRYPTION_KEY_FILE" "$BACKUP_DIR/encryption.key"
```

On Windows, stop the service/process first and copy `momentum.db`, any adjacent
`momentum.db-wal`/`momentum.db-shm` files, and `encryption.key` to a protected
backup directory. Never publish the key or database in an issue, log, or archive.

## Restore

1. Stop Momentum.
2. Keep the failed database aside; do not overwrite it until the backup is verified.
3. Restore `momentum.db` and any matching WAL/SHM files, or restore the SQLite
   backup produced above.
4. Restore the exact encryption key used when that database was written.
5. Start Momentum and verify health, login, and a representative task workflow.

If the key is missing or changed, the server may start but encrypted backend
configuration cannot be decrypted. A restore is not complete until credentials
are verified.

## Rollback

Rollback is a coordinated binary/database operation:

1. Stop the new process.
2. Preserve logs and the post-upgrade database for diagnosis.
3. Restore the pre-upgrade database backup and its matching encryption key.
4. Restore the previous executable and start it with the same configuration.
5. Verify health and the task workflow before exposing the server again.

Never point an older executable at a database after an unverified migration. Keep
the pre-upgrade backup until the new release has passed production verification.

## Scope and limitations

- The server's default v0.2.2 listener binds all interfaces. The generated
  certificate authenticates only localhost/loopback, so use a host firewall or a
  trusted reverse proxy for any network deployment. Loopback-by-default binding
  is tracked in [#143](https://github.com/chrisbelyea/momentum/issues/143).
- External CalDAV backend configuration and validation are available through the
  authenticated `/backends` JSON API. Runtime import/push scheduling is not yet
  part of the release binary.
- PWA assets are embedded and served, but browser installability and offline
  mutation are not claimed. See [pwa.md](pwa.md).
