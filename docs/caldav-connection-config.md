# External CalDAV Connection Configuration

This document describes how to configure and validate external CalDAV server
connections in Momentum. The v0.2.2 release does not yet run an automatic
import/push scheduler for those backends; runtime synchronization is tracked in
[#68](https://github.com/chrisbelyea/momentum/issues/68) and
[#144](https://github.com/chrisbelyea/momentum/issues/144).
All `/backends` routes require an authenticated Momentum session. The examples
show request shapes; add the session cookie returned by `/auth/login` when
calling them.

## Overview

Momentum supports saving and validating external CalDAV server configurations in
addition to the internal task backend. Once runtime synchronization is available,
these configurations can connect tasks to CalDAV-compatible services such as:

- Nextcloud
- OwnCloud
- Radicale
- Apple Calendar Server
- And other CalDAV-compliant servers

## Security

All sensitive credentials (passwords, client certificates) are encrypted at rest using AES-256-GCM encryption. The encryption key must be configured via the `MOMENTUM_ENCRYPTION_KEY` environment variable.

### Setting up Encryption Key

**Production:**
```bash
export MOMENTUM_ENCRYPTION_KEY="your-strong-random-key-here"
```

**Development:**
Set `MOMENTUM_DEV_MODE=1` only for local development or integration tests when
no key is available. Production startup fails without an explicit key.

## API Endpoints

### List Backends
```
GET /backends
```

Returns all configured backends owned by the authenticated user.

### Get Backend
```
GET /backends/{backend_id}
```

Returns details of a specific backend. Sensitive data (passwords) are sanitized in responses.

### Create Backend
```
POST /backends
Content-Type: application/json

{
  "backend_type": "external_caldav",
  "name": "My CalDAV Server",
  "config": {
    "url": "https://caldav.example.com/dav/calendars/user@example.com/tasks",
    "username": "user@example.com",
    "password": "your-password"
  }
}
```

Creates a new backend configuration.

### Update Backend
```
PUT /backends/{backend_id}
Content-Type: application/json

{
  "backend_type": "external_caldav",
  "name": "Updated Name",
  "config": {
    "url": "https://caldav.example.com/dav/calendars/user@example.com/tasks",
    "username": "user@example.com",
    "password": "new-password"
  }
}
```

Updates an existing backend configuration.

### Delete Backend
```
DELETE /backends/{backend_id}
```

Deletes a backend configuration.

### Validate Connection
```
POST /backends/validate
Content-Type: application/json

{
  "backend_type": "external_caldav",
  "name": "Test Connection",
  "config": {
    "url": "https://caldav.example.com/dav/calendars/user@example.com/tasks",
    "username": "user@example.com",
    "password": "test-password"
  }
}
```

Validates a CalDAV connection without saving it. Returns:
```json
{
  "valid": true,
  "message": "Connection validated successfully"
}
```

Or on error:
```json
{
  "valid": false,
  "error": "connection failed: ..."
}
```

## Configuration Options

### Backend Types

- `internal`: Internal CalDAV server (default)
- `external_caldav`: External CalDAV server

### CalDAV Configuration

Required fields:
- `url`: CalDAV server URL
- At least one authentication method:
  - Basic authentication: `username` and `password`
  - Client certificate: `client_cert_path` and `client_key_path`

Optional fields:
- `calendar_path`: Specific calendar path (if different from main URL)
- `skip_tls_verify`: Skip TLS certificate verification (not recommended for production)

### Basic Authentication Example

```json
{
  "backend_type": "external_caldav",
  "name": "Nextcloud Tasks",
  "config": {
    "url": "https://nextcloud.example.com/remote.php/dav/calendars/username/tasks",
    "username": "username",
    "password": "app-specific-password"
  }
}
```

### Client Certificate Authentication Example

```json
{
  "backend_type": "external_caldav",
  "name": "Corporate CalDAV",
  "config": {
    "url": "https://caldav.corp.example.com/dav",
    "client_cert_path": "/path/to/client-cert.pem",
    "client_key_path": "/path/to/client-key.pem"
  }
}
```

## TLS Security

All external CalDAV connections use TLS 1.3+ by default with strong cipher suites. The following security measures are enforced:

- Minimum TLS version: 1.3
- Certificate verification enabled by default
- Connection timeouts configured
- Secure default cipher suites

### Disabling TLS Verification (Not Recommended)

Only for testing or development with self-signed certificates:

```json
{
  "config": {
    "url": "https://caldav.test.local",
    "username": "user",
    "password": "pass",
    "skip_tls_verify": true
  }
}
```

**Warning**: Never use `skip_tls_verify: true` in production environments.

## Database Schema

Backend configurations use integer SQLite-generated IDs and are owned by the
user identified by the authenticated session. Configuration is encrypted at
rest with AES-256-GCM. Do not create or alter the tables manually; the release
binary initializes and upgrades the canonical schema described in
[docs/database-schema.md](database-schema.md).

The canonical table definition is maintained in
[internal/db/schema/changelog](../internal/db/schema/changelog), generated
into the embedded initializer, and applied automatically by the server.

The encrypted configuration is managed by the backend repository and is never
returned in API responses.

## Environment Variables

- `MOMENTUM_ENCRYPTION_KEY`: Required for production. Used to encrypt/decrypt backend credentials.
- `DB_PATH`: Database file path (default: the OS user data directory)
- `PORT`: HTTPS server port (default: `8443`)
- `TLS_CERT`, `TLS_KEY`: Optional certificate/key paths; development certificates are generated when omitted

## Example Usage

### Using cURL

1. Create a backend:
```bash
curl -k -X POST https://localhost:8443/backends \
  -H "Content-Type: application/json" \
  -d '{
    "backend_type": "external_caldav",
    "name": "My CalDAV",
    "config": {
      "url": "https://caldav.example.com/dav",
      "username": "user",
      "password": "secret"
    }
  }'
```

2. List backends:
```bash
curl -k https://localhost:8443/backends
```

3. Validate connection:
```bash
curl -k -X POST https://localhost:8443/backends/validate \
  -H "Content-Type: application/json" \
  -d '{
    "backend_type": "external_caldav",
    "config": {
      "url": "https://caldav.example.com/dav",
      "username": "user",
      "password": "secret"
    }
  }'
```

## Testing

Run the test suite:
```bash
go test ./...
```

Run specific package tests:
```bash
go test ./internal/backend -v
go test ./internal/caldav -v
go test ./internal/crypto -v
go test ./internal/db -v
```

## Security Considerations

1. **Encryption Key Management**: Store the encryption key securely (e.g., using a secrets manager, not in version control)
2. **TLS Only**: All external connections require HTTPS/TLS
3. **Credential Storage**: All passwords and sensitive data are encrypted at rest
4. **Response Sanitization**: Passwords are never included in API responses
5. **Certificate Validation**: TLS certificate validation is enforced by default
6. **Connection Timeouts**: All connections have appropriate timeouts to prevent hanging

## Future Enhancements

- OAuth 2.0 support for CalDAV servers that support it
- OS keychain integration for client-side credential storage (see [credential-storage.md](credential-storage.md))
- Runtime import/push scheduling and release-binary external sync (see #68 and #144)
- Hosted-service certification beyond the pinned provider-compatible Nextcloud CI service
- Automatic credential rotation
- Multi-factor authentication support
