# External CalDAV Connection Configuration

This document describes how to configure and use external CalDAV server connections in Momentum.

## Overview

Momentum supports connecting to external CalDAV servers in addition to the internal CalDAV server. This allows you to sync tasks from various CalDAV-compatible services like:

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
If no encryption key is set, the server will use a default development key (not secure for production).

## API Endpoints

### List Backends
```
GET /backends?user_id={user_id}
```

Returns all configured backends for a user.

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
  "user_id": "user-123",
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
  "user_id": "user-123",
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

Backend configurations are stored in the `backends` table:

```sql
CREATE TABLE backends (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    backend_type VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    config_encrypted TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

The `config_encrypted` field stores the JSON-serialized configuration encrypted with AES-256-GCM.

## Environment Variables

- `MOMENTUM_ENCRYPTION_KEY`: Required for production. Used to encrypt/decrypt backend credentials.
- `DB_PATH`: Database file path (default: `momentum.db`)
- `PORT`: HTTP server port (default: `8080`)

## Example Usage

### Using cURL

1. Create a backend:
```bash
curl -X POST http://localhost:8080/backends \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
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
curl http://localhost:8080/backends?user_id=user-123
```

3. Validate connection:
```bash
curl -X POST http://localhost:8080/backends/validate \
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
- Connection pooling and retry logic
- Automatic credential rotation
- Multi-factor authentication support
