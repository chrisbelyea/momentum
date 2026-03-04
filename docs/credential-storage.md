# Credential Storage: OS Keychain Integration

## Overview

Momentum clients store sensitive credentials (passwords, OAuth tokens, API keys) in the operating system's native credential store rather than in plain-text files or application databases. This ensures that credentials are protected by OS-level access controls and, where supported, by hardware-backed encryption.

This document describes the OS keychain mechanism for each supported client platform, the data stored in each, and guidance for client implementers.

> **Server-side storage**: The Momentum server encrypts credentials at rest using AES-256-GCM. See [security-implementation.md](security-implementation.md) for details on server-side credential handling.

## What Is Stored in the Keychain

Each client stores the following credential types in the OS keychain:

| Credential Type | Description |
|---|---|
| Momentum account password | Password used to authenticate with the Momentum server |
| OAuth access tokens | Short-lived tokens for external service integrations (e.g., Microsoft Graph, Google Tasks) |
| OAuth refresh tokens | Long-lived tokens used to obtain new access tokens |
| CalDAV passwords | Passwords for external CalDAV server connections |
| API keys | API keys for future integrations (Trello, Jira, GitHub) |

Credentials are keyed by a service identifier (`momentum`) and an account identifier (typically the user's account ID or email address).

## Platform-Specific Mechanisms

### Windows: Windows Credential Manager

**API**: [`Windows.Security.Credentials.PasswordVault`](https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.passwordvault) (WinRT)  
**Backing store**: Windows Credential Manager (`Control Panel > Credential Manager > Windows Credentials`)  
**Client**: Windows 11 (WinUI 3 + Windows App SDK)

Windows Credential Manager provides an encrypted credential store backed by the Data Protection API (DPAPI). Credentials are encrypted using keys derived from the user's Windows login credentials, ensuring they are inaccessible to other user accounts.

#### Key Characteristics

- Credentials are scoped to the current Windows user account.
- The store is encrypted with DPAPI using the user's logon credentials.
- Credentials roam with the user profile when Windows account sync is enabled.
- Access is controlled by the Windows Access Control List (ACL) subsystem.

#### WinRT API Usage (C# / Windows App SDK)

```csharp
using Windows.Security.Credentials;

// Store a credential
var vault = new PasswordVault();
vault.Add(new PasswordCredential("momentum", accountId, password));

// Retrieve a credential
var credential = vault.Retrieve("momentum", accountId);
credential.RetrievePassword();
string password = credential.Password;

// Remove a credential
vault.Remove(vault.Retrieve("momentum", accountId));
```

#### References

- [PasswordVault Class (WinRT)](https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.passwordvault)
- [Windows Credential Manager overview](https://learn.microsoft.com/en-us/windows/win32/secauthn/credential-manager)
- [Data Protection API (DPAPI)](https://learn.microsoft.com/en-us/windows/win32/seccng/cng-dpapi)

---

### macOS: Keychain Services

**API**: [Keychain Services](https://developer.apple.com/documentation/security/keychain_services) (Security framework)  
**Backing store**: macOS Keychain (login keychain or iCloud Keychain)  
**Clients**: macOS (Swift + AppKit), iOS/iPadOS (Swift + UIKit/SwiftUI)

The macOS Keychain is a secure, encrypted database managed by the Security framework. On Apple Silicon and modern Intel Macs, the keychain is backed by the Secure Enclave for hardware-level protection. Items can optionally sync to iCloud Keychain, making them available across a user's Apple devices.

#### Key Characteristics

- Items are encrypted at rest using hardware-backed keys where available (Secure Enclave on Apple Silicon).
- Per-app access controls: only the creating app (and apps it explicitly trusts) can read items.
- Supports access control policies (e.g., require biometric authentication before retrieval).
- iCloud Keychain sync is opt-in per item.
- Shared access groups allow credential sharing between macOS and iOS apps from the same developer.

#### Swift API Usage (Security framework)

```swift
import Security

let service = "momentum"

// Store a credential
func storeCredential(account: String, password: String) throws {
    let data = password.data(using: .utf8)!
    let query: [String: Any] = [
        kSecClass as String: kSecClassGenericPassword,
        kSecAttrService as String: service,
        kSecAttrAccount as String: account,
        kSecValueData as String: data
    ]
    // Delete existing item before adding (upsert pattern)
    SecItemDelete(query as CFDictionary)
    let status = SecItemAdd(query as CFDictionary, nil)
    guard status == errSecSuccess else {
        throw KeychainError.unexpectedStatus(status)
    }
}

// Retrieve a credential
func retrieveCredential(account: String) throws -> String {
    let query: [String: Any] = [
        kSecClass as String: kSecClassGenericPassword,
        kSecAttrService as String: service,
        kSecAttrAccount as String: account,
        kSecMatchLimit as String: kSecMatchLimitOne,
        kSecReturnData as String: true
    ]
    var item: CFTypeRef?
    let status = SecItemCopyMatching(query as CFDictionary, &item)
    guard status == errSecSuccess, let data = item as? Data,
          let password = String(data: data, encoding: .utf8) else {
        throw KeychainError.itemNotFound
    }
    return password
}

// Delete a credential
func deleteCredential(account: String) {
    let query: [String: Any] = [
        kSecClass as String: kSecClassGenericPassword,
        kSecAttrService as String: service,
        kSecAttrAccount as String: account
    ]
    SecItemDelete(query as CFDictionary)
}
```

#### References

- [Keychain Services (Apple Developer Documentation)](https://developer.apple.com/documentation/security/keychain_services)
- [Using the Keychain to Manage User Secrets](https://developer.apple.com/documentation/security/keychain_services/keychain_items/using_the_keychain_to_manage_user_secrets)
- [Sharing Access to Keychain Items Among a Collection of Apps](https://developer.apple.com/documentation/security/keychain_services/keychain_items/sharing_access_to_keychain_items_among_a_collection_of_apps)

---

### Linux: Secret Service API

**API**: [freedesktop.org Secret Service API](https://specifications.freedesktop.org/secret-service/) (D-Bus)  
**Backing stores**: GNOME Keyring (`gnome-keyring`), KDE Wallet (`kwallet`), or any compliant implementation  
**Client**: Linux desktop (Vala + GTK 4)  
**Library**: [`libsecret`](https://gnome.pages.gitlab.gnome.org/libsecret/)

The Secret Service API is a freedesktop.org standard D-Bus interface for securely storing secrets. It is implemented by `gnome-keyring` (GNOME/most desktops) and `kwallet` (KDE). The `libsecret` library provides a GLib-based client API that works with any compliant implementation, making it desktop-environment-agnostic.

#### Key Characteristics

- Secrets are encrypted at rest using keys derived from the user's login password (PAM integration).
- Access is controlled by the D-Bus session: only processes in the current user session can access secrets.
- On GNOME, the keyring unlocks automatically on login when PAM integration is configured.
- `libsecret` abstracts over different backends (gnome-keyring, kwallet, etc.) through the standard D-Bus interface.
- Secrets are organized into named collections (analogous to keychains/vaults); the default collection is used unless specified.

#### Vala / libsecret Usage

```vala
// Build dependency: libsecret-1-dev
// Vala VAPI: libsecret-1

// Schema is created once and reused (varargs constructor pattern)
private static Secret.Schema schema = new Secret.Schema (
    "org.momentum.credentials",
    Secret.SchemaFlags.NONE,
    "service", Secret.SchemaAttributeType.STRING,
    "account", Secret.SchemaAttributeType.STRING
);

// Store a credential
Secret.password_store_sync (
    schema,
    Secret.COLLECTION_DEFAULT,
    "Momentum credential for " + account_id,
    password,
    null,
    "service", "momentum",
    "account", account_id
);

// Retrieve a credential
string? password = Secret.password_lookup_sync (
    schema,
    null,
    "service", "momentum",
    "account", account_id
);

// Delete a credential
Secret.password_clear_sync (
    schema,
    null,
    "service", "momentum",
    "account", account_id
);
```

#### Fallback: Encrypted File Storage

On systems without a Secret Service implementation (e.g., minimal server environments or headless setups), the Linux client falls back to an AES-256-GCM encrypted local file. The encryption key is derived from a user-supplied passphrase using Argon2id. This fallback is not recommended for desktop environments where a Secret Service provider is available.

#### References

- [Secret Service API Specification](https://specifications.freedesktop.org/secret-service/)
- [libsecret Reference Manual](https://gnome.pages.gitlab.gnome.org/libsecret/)
- [GNOME Keyring documentation](https://wiki.gnome.org/Projects/GnomeKeyring)

---

### Web App (PWA): No OS Keychain Access

The Momentum web application (PWA) runs in a browser sandbox and does not have direct access to OS credential stores. Credentials submitted by the web app are:

1. Transmitted over HTTPS/TLS 1.3+ to the Momentum server.
2. Encrypted at rest on the server using AES-256-GCM (see [security-implementation.md](security-implementation.md)).

For long-lived sessions, the web app uses short-lived session tokens stored in `HttpOnly`, `Secure`, `SameSite=Strict` cookies, which are inaccessible to JavaScript and protected from CSRF attacks.

> **Future**: When the [Web Authentication API (WebAuthn)](https://www.w3.org/TR/webauthn/) is adopted, passkeys stored in the OS keychain can be used for passwordless authentication from the browser without exposing credentials to the web app.

---

## Credential Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│  User enters credential (login, add backend, OAuth flow)    │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│  Client stores credential in OS keychain                    │
│  (Windows Credential Manager / macOS Keychain /             │
│   Linux Secret Service)                                     │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│  Client retrieves credential from keychain when needed      │
│  (API call, sync operation, backend connection)             │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│  Credential transmitted over HTTPS/TLS 1.3+ only           │
│  Never written to disk in plaintext                         │
└─────────────────────────────────────────────────────────────┘
```

Credentials are removed from the OS keychain when:
- The user signs out of the Momentum client.
- The associated backend connection is deleted.
- The user explicitly revokes stored credentials.

## Security Checklist for Client Implementers

- [ ] Credentials are read from and written to the OS keychain only (never to plain-text files, `UserDefaults`, `SharedPreferences`, or unencrypted storage).
- [ ] Credential retrieval happens at the point of use, not cached in memory longer than necessary.
- [ ] After use, credential strings are zeroed/cleared from memory where the platform permits.
- [ ] OS keychain items use a consistent service name (`momentum`) and account identifier scheme.
- [ ] App entitlements (macOS/iOS) are configured to allow keychain access.
- [ ] The Secret Service schema (Linux) uses a stable schema name and attribute set.
- [ ] Fallback encrypted-file storage (Linux headless) uses Argon2id for key derivation.
- [ ] Credentials are deleted from the keychain on sign-out and on backend removal.
- [ ] No credentials appear in logs, crash reports, or analytics.

## References

- [Windows Credential Manager](https://learn.microsoft.com/en-us/windows/win32/secauthn/credential-manager)
- [macOS Keychain Services](https://developer.apple.com/documentation/security/keychain_services)
- [freedesktop.org Secret Service API](https://specifications.freedesktop.org/secret-service/)
- [libsecret](https://gnome.pages.gitlab.gnome.org/libsecret/)
- [Momentum Security Implementation](security-implementation.md)
- [Momentum Specification — Section 8: Security Requirements](../requirements/specification.md)
