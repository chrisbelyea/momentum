# Security Implementation for External CalDAV Connections

## Overview

This document describes the security measures implemented for external CalDAV connection configuration in Momentum.

## Secure Secret Storage

### Encryption

All sensitive credentials (passwords, client certificate paths) are encrypted at rest using **AES-256-GCM** (Galois/Counter Mode), which provides:

- **Confidentiality**: Data is encrypted and unreadable without the key
- **Authentication**: Built-in authentication tag prevents tampering
- **Integrity**: Any modifications to the ciphertext are detected

### Key Derivation

The encryption key is derived from a master key using **SHA-256** hash function, ensuring:
- Consistent 32-byte key length for AES-256
- One-way derivation (master key cannot be recovered from derived key)

### Nonce Generation

Each encryption operation uses a unique random nonce:
- Generated using `crypto/rand` (cryptographically secure random number generator)
- Prevents reuse of nonce/key pairs
- Stored as part of the ciphertext

### Implementation Details

Location: `internal/crypto/encryption.go`

```go
// Encrypt encrypts plaintext using AES-256-GCM
func Encrypt(plaintext string) (string, error)

// Decrypt decrypts ciphertext using AES-256-GCM
func Decrypt(ciphertext string) (string, error)
```

The encrypted data format:
```
[nonce (12 bytes)][ciphertext + auth tag]
```

Base64 encoded for storage in the database.

## TLS Configuration

### Minimum Requirements

- **TLS Version**: 1.3 minimum (configurable, but enforced by default)
- **Certificate Validation**: Enabled by default
- **Strong Cipher Suites**: Modern cipher suites only (managed by Go's crypto/tls)

### Connection Settings

Location: `internal/caldav/client.go`

```go
tlsConfig := &tls.Config{
    MinVersion:         tls.VersionTLS13,
    InsecureSkipVerify: config.SkipTLSVerify, // false by default
}
```

### Timeouts

All HTTP connections have appropriate timeouts:
- **Total Request Timeout**: 30 seconds
- **TLS Handshake Timeout**: 10 seconds
- **Response Header Timeout**: 10 seconds
- **Expect Continue Timeout**: 1 second

## Database Security

### Encrypted Storage

Backend configurations are stored with encryption:
- Sensitive fields are encrypted before storage
- Only encrypted data is stored in `config_encrypted` column
- Decryption happens in application layer

### Access Control

- Foreign key constraints ensure data integrity
- Cascade delete prevents orphaned data
- User-based access control at application layer

## API Security

### Response Sanitization

Location: `internal/models/backend.go`

```go
// SanitizeForResponse removes sensitive data before sending to client
func (b *Backend) SanitizeForResponse() {
    if b.Config != nil {
        b.Config.Password = ""
    }
}
```

All API responses sanitize sensitive data:
- Passwords are never returned in API responses
- Only encrypted data stored in database

### Validation

Location: `internal/models/backend.go`

```go
func (b *Backend) Validate() error
```

Comprehensive validation before storage:
- Required fields checked
- Backend type validation
- Configuration completeness verification
- Authentication method validation

## Authentication Methods

### Basic Authentication

- Username and password stored encrypted
- Transmitted over TLS only
- Password never logged or returned in responses

### Client Certificate Authentication

- Certificate and key paths stored encrypted
- Certificates loaded at connection time
- TLS mutual authentication supported

## Testing

### Security Tests

All security features are thoroughly tested:

1. **Encryption Tests** (`internal/crypto/encryption_test.go`):
   - Round-trip encryption/decryption
   - Nonce uniqueness
   - Invalid ciphertext handling
   - Empty string handling
   - Special character handling

2. **Repository Tests** (`internal/db/backend_repository_test.go`):
   - Encryption round-trip in database
   - Verification that data is encrypted in database
   - Config decryption on retrieval

3. **Client Tests** (`internal/caldav/client_test.go`):
   - TLS configuration verification
   - Connection validation with authentication
   - Certificate handling

4. **API Tests** (`internal/backend/handler_test.go`):
   - Response sanitization
   - Password removal from responses
   - Validation enforcement

## Environment Variables

### Required for Production

```bash
MOMENTUM_ENCRYPTION_KEY="your-strong-random-key-here"
```

**Important**: 
- Use a strong, random key (at least 32 characters)
- Store securely (e.g., secrets manager, not in code)
- Never commit to version control
- Rotate periodically

### Key Generation

Generate a secure key:
```bash
# Linux/macOS
openssl rand -base64 32

# Or using Go
go run -c 'package main; import "crypto/rand"; import "encoding/base64"; import "fmt"; func main() { b := make([]byte, 32); rand.Read(b); fmt.Println(base64.StdEncoding.EncodeToString(b)) }'
```

## Compliance

This implementation follows security best practices from:

- **OWASP**: Secure credential storage guidelines
- **NIST**: Cryptographic standards (AES-256-GCM, TLS 1.3)
- **CalDAV RFC 4791**: CalDAV protocol security considerations
- **RFC 8446**: TLS 1.3 specification

## Security Checklist

- [x] All credentials encrypted at rest (AES-256-GCM)
- [x] TLS 1.3+ enforced for external connections
- [x] Certificate validation enabled by default
- [x] Passwords never returned in API responses
- [x] Comprehensive input validation
- [x] Secure random nonce generation
- [x] Connection timeouts configured
- [x] No weak cipher suites
- [x] Foreign key constraints for data integrity
- [x] Comprehensive security tests

## Client-Side Credential Storage

Native clients (Windows, macOS, iOS/iPadOS, Linux desktop) store credentials in the OS keychain rather than on disk or in application databases. See [credential-storage.md](credential-storage.md) for the platform-specific mechanisms and implementation guidance.

## Known Limitations

1. **Key Storage**: Encryption key must be managed by operator (future: integrate with key management systems)
2. **Key Rotation**: No automatic key rotation (future enhancement)
3. **Certificate Revocation**: No OCSP stapling (future enhancement)
4. **Rate Limiting**: No built-in rate limiting on API endpoints (future enhancement)

## Reporting Security Issues

Security vulnerabilities should be reported according to the guidelines in `SECURITY.md` in the repository root.

## References

- [NIST SP 800-38D: GCM Mode](https://csrc.nist.gov/publications/detail/sp/800-38d/final)
- [RFC 8446: TLS 1.3](https://datatracker.ietf.org/doc/html/rfc8446)
- [RFC 4791: CalDAV](https://datatracker.ietf.org/doc/html/rfc4791)
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)
- [Client-Side Credential Storage (OS Keychains)](credential-storage.md)
