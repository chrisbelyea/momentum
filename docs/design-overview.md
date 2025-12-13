# Design Overview

This document outlines the design and architecture of Momentum.

## Overview

Momentum brings your to-dos, reminders, and CalDAV tasks into one flow — visible as lists or boards. Designed for people who want to see their life in motion.

## Architecture

Momentum follows a **client-server architecture** with shared core logic to minimize code duplication across platform-native clients.

```mermaid
graph TB
    subgraph "Clients"
        WebApp["Web App<br/>(PWA + htmx)"]
        WindowsApp["Windows 11<br/>(WinUI + App SDK)"]
        MacApp["macOS<br/>(Swift)"]
        iOSApp["iOS/iPadOS<br/>(Swift)"]
        LinuxApp["Linux<br/>(Vala + GTK)"]
    end
    
    subgraph "Momentum Server"
        API["REST/Web API"]
        Auth["Authentication/<br/>Authorization"]
        Sync["Sync Orchestrator"]
        InternalCAL["Internal CalDAV Server"]
        Storage["Data Storage<br/>(SQLite/PostgreSQL)"]
    end
    
    subgraph "External Backends"
        ExtCAL["External CalDAV<br/>Servers"]
        iCloud["iCloud Reminders"]
        MS["Microsoft To-Do/<br/>Outlook/Planner"]
        Google["Google Tasks"]
        Other["Trello/Jira/<br/>GitHub Issues"]
    end
    
    WebApp --> API
    WindowsApp --> API
    MacApp --> API
    iOSApp --> API
    LinuxApp --> API
    
    API --> Auth
    API --> Sync
    API --> InternalCAL
    Auth --> Storage
    InternalCAL --> Storage
    
    Sync --> InternalCAL
    Sync --> ExtCAL
    Sync --> iCloud
    Sync --> MS
    Sync --> Google
    Sync --> Other
    Sync --> Storage
```

### Deployment Models

- **SaaS**: Hosted Momentum server with managed infrastructure, authentication, and updates
- **Self-hosted**: Single-executable server running as:
  - Linux: systemd service
  - Windows: Windows service
  - Default DB: SQLite (single-file); optional PostgreSQL for multi-user scenarios

### Communication

- **HTTPS/TLS 1.3+ only**: All network traffic is encrypted; plaintext HTTP is rejected
- **REST/Web API**: Client-server communication via standard HTTP methods
- **CalDAV protocol**: Standard RFC 4791 for task (VTODO) operations

## Key Components

### 1. Server Components

#### Authentication & Authorization
- User account management with Argon2id password hashing
- OAuth/OIDC support for external service integrations
- Client certificate validation for CalDAV servers
- Per-user backend access control
- Encrypted credential storage at rest

#### Internal CalDAV Server
- RFC 4791 compliant CalDAV implementation
- Manages VTODO entities as canonical task format
- Local operation (no OS firewall prompts)
- Default backend for new users
- Supports full VTODO lifecycle: create, read, update, delete

#### Sync Orchestrator
- Idempotent sync operations for reliability
- Per-backend sync state tracking with checkpoints
- Conflict detection and resolution
- Rate limiting and error handling
- Incremental sync to minimize data transfer
- Handles schema differences across backends

#### Data Storage (RDBMS)
- **SQLite**: Default for single-user/self-hosted installations
- **PostgreSQL**: Optional for larger deployments
- Stores:
  - User accounts and authentication data
  - Backend configurations and credentials (encrypted)
  - Sync state and checkpoints
  - Task metadata and mappings
- Migrations managed via Liquibase for cross-DB compatibility

### 2. Client Components

#### Web Application
- Progressive Web App (PWA) with offline capabilities
- Installable on desktop and mobile browsers
- Uses htmx for progressive enhancement where feasible
- Responsive design for desktop and mobile viewports
- Shared UI patterns across all clients

#### Native Desktop Clients
- **Windows 11**: WinUI 3 + Windows App SDK
- **macOS**: Swift with AppKit
- **Linux**: Vala + GTK 4 (.deb/.rpm/Arch packages)

#### Native Mobile Clients
- **iOS/iPadOS**: Swift with UIKit/SwiftUI
- Shared Swift code between macOS and iOS where practical

#### Shared Core Logic
- Task model and VTODO mapping
- Validation and business rules
- Sync coordination logic
- Backend abstraction layer
- Shared across all clients to ensure consistency

### 3. External Integrations

#### CalDAV Servers
- Connect via URL, username/password, or client certificates
- Full VTODO synchronization
- Label/tag mapping to CATEGORIES

#### Future Backend Integrations
- iCloud Reminders (CloudKit API)
- Microsoft To-Do, Outlook Tasks, Planner (Microsoft Graph API)
- Google Tasks (Google Tasks API)
- Trello (Trello API)
- Jira (Jira REST API)
- GitHub Issues and Projects (GitHub GraphQL API)

### 4. Task Model

#### Canonical VTODO Fields
- **UID**: Unique identifier
- **SUMMARY**: Task title
- **DESCRIPTION**: Detailed task description
- **STATUS**: Task status (e.g., NEEDS-ACTION, IN-PROCESS, COMPLETED, CANCELLED)
- **DTSTART**: Start date/time
- **DUE**: Due date/time
- **PRIORITY**: Task priority (1-9)
- **CATEGORIES**: Labels/tags
- **RELATED-TO**: Subtask relationships
- **ATTACH**: Attachments (backend-dependent)

#### Backend Mapping
- Internal mapping layer translates between VTODO and backend-specific formats
- Preserves metadata during transfers between backends
- Handles schema differences and limitations

## Sync Orchestration

The Sync Orchestrator is responsible for keeping tasks synchronized between the internal CalDAV server and external backends. It implements incremental synchronization and robust conflict resolution to ensure data consistency and reliability.

### Incremental Sync Strategy

Momentum uses incremental synchronization to minimize network traffic and improve performance. Rather than fetching all tasks on every sync, only changes since the last successful sync are transferred.

#### Sync State Tracking

For each backend, the system maintains:
- **Last sync timestamp**: When the last successful sync completed
- **Sync token/checkpoint**: Backend-specific cursor or ETag for incremental queries
- **Entity version map**: Mapping of task UIDs to their last-known modification timestamps

#### Sync Process

```mermaid
sequenceDiagram
    participant Client
    participant API
    participant Sync as Sync Orchestrator
    participant Storage as Data Storage
    participant Backend as External Backend
    
    Client->>API: Request sync for backend
    API->>Sync: Initiate sync
    Sync->>Storage: Load sync state (last token, timestamp)
    
    alt First sync (no state)
        Sync->>Backend: Fetch all tasks
    else Incremental sync
        Sync->>Backend: Fetch changes since last sync token
    end
    
    Backend-->>Sync: Return changed tasks + new sync token
    
    Sync->>Sync: Detect conflicts (compare timestamps)
    
    alt No conflicts
        Sync->>Storage: Update tasks
        Sync->>Storage: Save new sync state
    else Conflicts detected
        Sync->>Sync: Apply conflict resolution policy
        Sync->>Storage: Update tasks with resolution
        Sync->>Storage: Save new sync state
        Sync->>Storage: Log conflict events
    end
    
    Sync-->>API: Sync complete (status + conflicts)
    API-->>Client: Return sync result
```

#### Sync Triggers

Synchronization can be triggered by:
- **Manual**: User explicitly requests sync via UI
- **Periodic**: Scheduled background sync (configurable interval, default 15 minutes)
- **Push notification**: Backend-initiated webhook or push notification (when supported)
- **On-connect**: When backend connection is first established or restored
- **After local change**: Immediate push of local changes to backend (optional, configurable)

#### Bidirectional Sync

Sync operates bidirectionally to ensure changes flow in both directions:

1. **Pull phase**: Fetch changes from external backend to internal CalDAV
2. **Push phase**: Send local changes from internal CalDAV to external backend
3. **Reconciliation**: Resolve any conflicts that arise from concurrent modifications

### Conflict Resolution Policies

Conflicts occur when the same task is modified in multiple locations between syncs. Momentum detects conflicts by comparing modification timestamps and ETags.

#### Conflict Detection

A conflict is detected when:
- Local modification timestamp > last sync timestamp **AND**
- Remote modification timestamp > last sync timestamp **AND**
- Local and remote task versions differ

#### Resolution Strategies

Momentum supports multiple conflict resolution strategies, configurable per backend:

##### 1. Last-Write-Wins (Default)
- Compare modification timestamps
- The version with the most recent timestamp wins
- Older version is preserved in conflict history
- **Use case**: Default for most backends; simple and predictable

##### 2. Server-Wins
- Remote backend version always takes precedence
- Local changes are discarded (but logged)
- **Use case**: When external backend is authoritative (e.g., shared team calendars)

##### 3. Client-Wins
- Local internal CalDAV version takes precedence
- Remote changes are discarded (but logged)
- **Use case**: When user prefers local edits to override external sources

##### 4. Manual Resolution
- Conflict is flagged for user review
- Both versions are preserved
- User chooses which version to keep or manually merges changes
- **Use case**: Critical tasks where data loss is unacceptable

##### 5. Field-Level Merge (Advanced)
- Attempt to merge non-conflicting field changes
- If same field modified in both versions, fall back to last-write-wins for that field
- **Use case**: Complex scenarios with multiple concurrent editors

#### Conflict Resolution Flow

```mermaid
sequenceDiagram
    participant Sync as Sync Orchestrator
    participant Storage as Data Storage
    participant Policy as Resolution Policy
    participant History as Conflict History
    
    Sync->>Sync: Detect conflict (task modified in both locations)
    Sync->>Storage: Load local version (with timestamp)
    Sync->>Storage: Load remote version (with timestamp)
    
    Sync->>Policy: Apply resolution strategy
    
    alt Last-Write-Wins
        Policy->>Policy: Compare timestamps
        Policy-->>Sync: Newer version wins
    else Server-Wins
        Policy-->>Sync: Remote version wins
    else Client-Wins
        Policy-->>Sync: Local version wins
    else Manual Resolution
        Policy-->>Sync: Flag for user review
        Sync->>Storage: Store both versions
        Sync->>Storage: Create conflict record
    else Field-Level Merge
        Policy->>Policy: Merge non-conflicting fields
        Policy->>Policy: Apply last-write-wins to conflicting fields
        Policy-->>Sync: Merged version
    end
    
    Sync->>History: Log conflict event (both versions, resolution)
    Sync->>Storage: Save resolved version
    Sync->>Storage: Update sync state
```

#### Conflict History

All conflicts are logged for audit and recovery purposes:
- **Timestamp**: When conflict occurred
- **Task UID**: Which task conflicted
- **Local version**: Full snapshot of local task state
- **Remote version**: Full snapshot of remote task state
- **Resolution applied**: Which strategy was used
- **Winning version**: Final resolved state
- **User notified**: Whether user was alerted (for manual resolution)

Users can access conflict history to:
- Review past conflicts
- Recover discarded changes if needed
- Understand sync behavior
- Adjust resolution policies

### Idempotency and Error Handling

#### Idempotent Operations

All sync operations are designed to be idempotent—they can be safely retried without duplicating data or causing inconsistencies:
- Task creates use stable UIDs; duplicate creates are detected and ignored
- Task updates are applied based on modification timestamp comparison
- Task deletes are tracked by UID; redundant deletes are no-ops
- Sync tokens are updated atomically only after successful sync completion

#### Error Handling

The sync orchestrator handles various error conditions gracefully:

- **Network errors**: Retry with exponential backoff; preserve local state
- **Authentication failures**: Alert user; pause sync until credentials are refreshed
- **Rate limiting**: Respect rate limit headers; schedule retry after cooldown period
- **Backend unavailable**: Temporary disable backend; retry periodically with exponential backoff
- **Data validation errors**: Log error; skip invalid task; continue with remaining tasks
- **Partial sync failures**: Commit successful changes; preserve sync token for resumed incremental sync

#### Transactional Guarantees

- Sync state updates are transactional: either all changes commit or none do
- If sync fails mid-operation, the system rolls back to the last known good state
- The next sync attempt resumes from the last successful checkpoint

### Performance Considerations

- **Batch operations**: Group multiple task updates into single backend API calls when possible
- **Parallel syncing**: Multiple backends can sync concurrently (with per-backend rate limiting)
- **Delta encoding**: Only transmit changed fields when backend supports partial updates
- **Compression**: Enable gzip/deflate compression for API requests and responses
- **Connection pooling**: Reuse HTTP connections across sync operations
- **Lazy conflict detection**: Only check for conflicts on tasks that were locally modified

## Design Principles

### 1. Standards-First
- **CalDAV VTODO as canonical format**: Ensures data portability and vendor neutrality
- **RFC compliance**: Follow established protocols and standards
- **Open formats**: Avoid proprietary lock-in

### 2. Security by Default
- **TLS-only communication**: HTTPS/TLS 1.3+ with strong cipher suites exclusively
- **Encrypted credentials**: OS keychain integration on clients; encrypted at rest on server
- **Least privilege**: Minimal permission scopes; per-user backend access
- **Regular security updates**: Dependency audits and vulnerability scanning in CI

### 3. Separation of Concerns
- **Shared core logic**: Business rules and models shared across platforms
- **Platform-native UIs**: Leverage native UI frameworks for best user experience
- **Backend abstraction**: Generic task interface with backend-specific adapters

### 4. Simplicity
- **Single-executable server**: No complex installation or dependencies
- **Minimal configuration**: Environment variables and simple config files
- **SQLite by default**: Zero-configuration database for self-hosting
- **Progressive enhancement**: Web app works without JavaScript, enhanced with it

### 5. Reliability
- **Idempotent operations**: Sync operations can be safely retried
- **Conflict detection**: Detect and resolve conflicts during synchronization
- **Incremental sync**: Only transfer changed data
- **Graceful degradation**: Handle backend failures without data loss

### 6. Performance
- **Responsive UI**: Board interactions remain smooth with 1,000+ tasks
- **Efficient sync**: Minimize network traffic and database operations
- **Lazy loading**: Load data on demand where appropriate
- **Optimistic UI updates**: Update UI immediately, sync in background

### 7. Portability
- **Cross-platform**: Native clients for Windows, macOS, Linux, iOS, iPadOS, and web
- **Data export**: Users can export tasks to standard formats
- **Backend transfers**: Move tasks between services without data loss
- **Self-contained**: Self-hosted server runs anywhere with minimal dependencies

### 8. Extensibility
- **Plugin architecture**: Future support for custom integrations
- **API-first design**: Well-defined REST API for third-party tools
- **Modular backends**: Easy to add new task source integrations
- **Configurable workflows**: Users can customize status columns and transitions