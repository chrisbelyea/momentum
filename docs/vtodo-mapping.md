# Canonical VTODO Mapping

This document defines the mapping between CalDAV VTODO fields (as defined in RFC 5545) and Momentum's internal task model. This mapping ensures data portability, round-trip fidelity, and consistent behavior across all backends.

## Overview

Momentum uses CalDAV VTODO as the canonical task format. All internal operations work with the task model defined here, and backend-specific adapters translate to/from this format. This approach ensures:

- **Portability**: Tasks can be exported and imported in standard iCalendar format
- **Fidelity**: No data loss when syncing between Momentum and CalDAV servers
- **Consistency**: Same behavior across all backends through a unified internal model
- **Extensibility**: Clear foundation for adding non-CalDAV backend integrations

## Task Model

Momentum's internal task model represents the essential fields needed for kanban-based task management. Each field maps to one or more VTODO properties.

### Core Fields

| Internal Field | Type | Required | Description |
|----------------|------|----------|-------------|
| `uid` | String | Yes | Unique identifier for the task |
| `title` | String | Yes | Short task title/summary |
| `description` | String | No | Detailed description of the task |
| `status` | Enum | Yes | Current task status |
| `dueDate` | DateTime | No | When the task is due |
| `startDate` | DateTime | No | When the task should start |
| `priority` | Integer | No | Task priority level (0-9) |
| `tags` | String[] | No | Labels/categories for the task |
| `completedDate` | DateTime | No | When the task was completed |
| `createdDate` | DateTime | Yes | When the task was created |
| `modifiedDate` | DateTime | Yes | Last modification timestamp |
| `sequence` | Integer | Yes | Version/revision counter for conflict detection |

### Extended Fields

| Internal Field | Type | Required | Description |
|----------------|------|----------|-------------|
| `percentComplete` | Integer | No | Completion percentage (0-100) |
| `url` | String | No | Associated URL/link |
| `location` | String | No | Physical location associated with task |
| `relatedTo` | String[] | No | UIDs of related tasks (for subtasks) |
| `attachments` | Attachment[] | No | File attachments (backend-dependent) |

## VTODO to Task Model Mapping

This section defines how VTODO properties map to internal task fields.

### Mandatory Properties

#### UID (Unique Identifier)
- **VTODO Property**: `UID`
- **Task Field**: `uid`
- **Type**: String
- **Required**: Yes
- **Rules**:
  - Must be globally unique (RFC 5545 recommends using domain-based or UUID format)
  - Generated on task creation if not provided
  - Never changes for the lifetime of the task
  - Format: UUID v4 (e.g., `550e8400-e29b-41d4-a716-446655440000`)
- **Round-trip**: Preserved exactly

#### SUMMARY (Title)
- **VTODO Property**: `SUMMARY`
- **Task Field**: `title`
- **Type**: String
- **Required**: Yes
- **Rules**:
  - Single-line text (newlines converted to spaces on import)
  - Maximum length: 255 characters (truncated with ellipsis if longer)
  - Empty strings treated as "(No Title)"
- **Round-trip**: Preserved with normalization (whitespace trimmed, single-line)

#### DTSTAMP (Created Date)
- **VTODO Property**: `DTSTAMP`
- **Task Field**: `createdDate`
- **Type**: DateTime (UTC)
- **Required**: Yes
- **Rules**:
  - Automatically set to current UTC time on creation
  - Never modified after creation (immutable)
  - Always stored in UTC timezone
- **Round-trip**: Preserved exactly

#### LAST-MODIFIED (Modified Date)
- **VTODO Property**: `LAST-MODIFIED`
- **Task Field**: `modifiedDate`
- **Type**: DateTime (UTC)
- **Required**: Yes
- **Rules**:
  - Updated to current UTC time on any modification
  - Used for conflict detection in sync operations
  - Always stored in UTC timezone
- **Round-trip**: Updated on each modification

#### SEQUENCE (Revision Number)
- **VTODO Property**: `SEQUENCE`
- **Task Field**: `sequence`
- **Type**: Integer
- **Required**: Yes
- **Rules**:
  - Starts at 0 for new tasks
  - Incremented on each significant modification
  - Used for conflict detection and resolution
  - Must be monotonically increasing
- **Round-trip**: Preserved and incremented

### Status Mapping

#### STATUS (Task Status)
- **VTODO Property**: `STATUS`
- **Task Field**: `status`
- **Type**: Enum
- **Required**: Yes (defaults to `NEEDS-ACTION` if not specified)

**Status Value Mapping:**

| VTODO STATUS | Internal Status | Kanban Column | Description |
|--------------|-----------------|---------------|-------------|
| `NEEDS-ACTION` | `TODO` | Todo | Task not started |
| `IN-PROCESS` | `IN_PROGRESS` | In Progress | Task actively being worked on |
| `COMPLETED` | `DONE` | Done | Task finished |
| `CANCELLED` | `CANCELLED` | (archived) | Task abandoned/cancelled |

**Rules**:
- Default value: `NEEDS-ACTION` (maps to `TODO`)
- Status transitions are validated:
  - Valid: `TODO` → `IN_PROGRESS` → `DONE`
  - Valid: Any status → `CANCELLED`
  - Invalid transitions are rejected with an error
- When `STATUS` is `COMPLETED`:
  - `COMPLETED` property must be set with completion timestamp
  - `PERCENT-COMPLETE` should be set to 100
- **Round-trip**: Preserved with validation

### Date/Time Properties

#### DUE (Due Date)
- **VTODO Property**: `DUE`
- **Task Field**: `dueDate`
- **Type**: DateTime
- **Required**: No
- **Rules**:
  - Can be date-only (YYYYMMDD) or date-time (YYYYMMDDTHHmmss)
  - Date-only values interpreted as end-of-day in user's timezone
  - All-day tasks use date-only format
  - Timezone info preserved if provided; converted to UTC for storage
- **Round-trip**: Preserved with timezone conversion

#### DTSTART (Start Date)
- **VTODO Property**: `DTSTART`
- **Task Field**: `startDate`
- **Type**: DateTime
- **Required**: No
- **Rules**:
  - Can be date-only or date-time
  - Must be before or equal to `DUE` if both are present
  - Validation error if `DTSTART` > `DUE`
- **Round-trip**: Preserved with validation

#### COMPLETED (Completion Date)
- **VTODO Property**: `COMPLETED`
- **Task Field**: `completedDate`
- **Type**: DateTime (UTC)
- **Required**: Only if `STATUS` is `COMPLETED`
- **Rules**:
  - Automatically set when status changes to `DONE`
  - Must be present when `STATUS` is `COMPLETED`
  - Stored in UTC timezone
  - Cleared if status changes from `DONE` to another status
- **Round-trip**: Preserved with status synchronization

### Priority

#### PRIORITY (Task Priority)
- **VTODO Property**: `PRIORITY`
- **Task Field**: `priority`
- **Type**: Integer (0-9)
- **Required**: No
- **Rules**:
  - RFC 5545 defines priority scale:
    - `0`: Undefined (no priority specified)
    - `1`: Highest priority
    - `2-4`: High priority
    - `5`: Medium priority
    - `6-8`: Low priority
    - `9`: Lowest priority
  - Values outside 0-9 are clamped to this range
  - Default: `0` (undefined)
- **UI Mapping**:
  - High: `1-4` → Red/Urgent indicator
  - Medium: `5` → Yellow/Normal indicator
  - Low: `6-9` → Blue/Low indicator
  - None: `0` → No indicator
- **Round-trip**: Preserved with range validation (values clamped to 0-9)

### Description

#### DESCRIPTION (Task Description)
- **VTODO Property**: `DESCRIPTION`
- **Task Field**: `description`
- **Type**: String (multi-line)
- **Required**: No
- **Rules**:
  - Supports multi-line text with `\n` line breaks
  - Maximum length: 10,000 characters
  - Supports basic formatting (preserved as plain text)
  - Empty or whitespace-only treated as null
- **Round-trip**: Preserved exactly (including whitespace and line breaks)

### Categories/Tags

#### CATEGORIES (Labels/Tags)
- **VTODO Property**: `CATEGORIES`
- **Task Field**: `tags`
- **Type**: String array
- **Required**: No
- **Rules**:
  - Comma-separated list in VTODO format
  - Each tag is trimmed of whitespace
  - Case-sensitive (but UI may offer case-insensitive search)
  - Empty categories ignored
  - Duplicate tags removed
  - Maximum tag length: 50 characters
  - Maximum tags per task: 20
- **Format**: `CATEGORIES:work,urgent,client-project`
- **Round-trip**: Preserved with normalization (trimmed, deduplicated)

### Progress

#### PERCENT-COMPLETE (Completion Percentage)
- **VTODO Property**: `PERCENT-COMPLETE`
- **Task Field**: `percentComplete`
- **Type**: Integer (0-100)
- **Required**: No
- **Rules**:
  - Value range: 0-100
  - Must be 100 when `STATUS` is `COMPLETED`
  - Must be less than 100 when `STATUS` is not `COMPLETED`
  - Validation errors for out-of-range values
- **Round-trip**: Preserved with validation

### Related Tasks

#### RELATED-TO (Subtasks/Dependencies)
- **VTODO Property**: `RELATED-TO`
- **Task Field**: `relatedTo`
- **Type**: String array (UIDs)
- **Required**: No
- **Rules**:
  - Contains UIDs of related tasks
  - Relationship type specified via `RELTYPE` parameter:
    - `PARENT`: This task is a parent (has subtasks)
    - `CHILD`: This task is a subtask of another
    - `SIBLING`: Related tasks at same level
  - Backend support varies; stored but may not be actionable
- **Format**: `RELATED-TO;RELTYPE=PARENT:parent-task-uid`
- **Round-trip**: Preserved if backend supports relationships

### Additional Properties

#### URL (Associated Link)
- **VTODO Property**: `URL`
- **Task Field**: `url`
- **Type**: String (valid URL)
- **Required**: No
- **Rules**:
  - Must be a valid URL format
  - Maximum length: 2048 characters
- **Round-trip**: Preserved with URL validation

#### LOCATION (Physical Location)
- **VTODO Property**: `LOCATION`
- **Task Field**: `location`
- **Type**: String
- **Required**: No
- **Rules**:
  - Free-form text
  - Maximum length: 255 characters
- **Round-trip**: Preserved exactly

#### ATTACH (Attachments)
- **VTODO Property**: `ATTACH`
- **Task Field**: `attachments`
- **Type**: Attachment array
- **Required**: No
- **Rules**:
  - Backend-dependent feature
  - Can be URI or inline binary (base64)
  - Momentum prefers URI references over inline
  - Inline attachments may be stored separately and converted to URIs
- **Format**: `ATTACH;FMTYPE=image/png:https://example.com/file.png`
- **Round-trip**: Best-effort (backend-dependent)

## Task Model to VTODO Mapping

When converting from internal task model to VTODO format:

### Required VTODO Structure

Every VTODO must contain:

```
BEGIN:VTODO
UID:<unique-identifier>
DTSTAMP:<creation-timestamp-utc>
LAST-MODIFIED:<modification-timestamp-utc>
SEQUENCE:<revision-number>
SUMMARY:<task-title>
STATUS:<status-value>
END:VTODO
```

### Complete Example

```
BEGIN:VTODO
UID:550e8400-e29b-41d4-a716-446655440000
DTSTAMP:20231201T120000Z
LAST-MODIFIED:20231205T153000Z
SEQUENCE:3
SUMMARY:Implement user authentication
DESCRIPTION:Add OAuth2 support for Google and GitHub providers.\nInclude unit tests and documentation.
STATUS:IN-PROCESS
PRIORITY:2
CATEGORIES:backend,security,high-priority
DTSTART:20231201T120000Z
DUE:20231208T170000Z
PERCENT-COMPLETE:45
URL:https://github.com/example/momentum/issues/42
RELATED-TO;RELTYPE=CHILD:parent-task-uid-12345
END:VTODO
```

### Optional Properties

Include when non-null/non-default:

- `DESCRIPTION`: When `description` field is not empty
- `PRIORITY`: When `priority` is not 0
- `CATEGORIES`: When `tags` array is not empty
- `DTSTART`: When `startDate` is set
- `DUE`: When `dueDate` is set
- `COMPLETED`: When `status` is `DONE` (with UTC timestamp)
- `PERCENT-COMPLETE`: When set and not 0
- `URL`: When `url` field is set
- `LOCATION`: When `location` field is set
- `RELATED-TO`: When `relatedTo` array is not empty
- `ATTACH`: When `attachments` array is not empty

## Round-Trip Fidelity Guarantees

The following guarantees ensure no data loss when syncing tasks between Momentum and CalDAV servers:

### Guaranteed Fields

These fields are **guaranteed** to round-trip without loss:

1. **UID**: Always preserved exactly
2. **SUMMARY** (title): Preserved with normalization (whitespace, single-line)
3. **DESCRIPTION**: Preserved exactly including formatting
4. **STATUS**: Preserved with validation
5. **CATEGORIES** (tags): Preserved with normalization (trimmed, deduplicated)
6. **DUE** (dueDate): Preserved with timezone conversion
7. **PRIORITY**: Preserved with range validation (0-9)
8. **DTSTART** (startDate): Preserved with timezone conversion
9. **COMPLETED** (completedDate): Preserved when status is DONE
10. **DTSTAMP** (createdDate): Immutable after creation
11. **LAST-MODIFIED** (modifiedDate): Updated on each change
12. **SEQUENCE**: Incremented on each change

### Best-Effort Fields

These fields are preserved when possible, but may be limited by backend capabilities:

1. **PERCENT-COMPLETE**: May not be supported by all CalDAV servers
2. **RELATED-TO**: Subtask relationships may not be supported
3. **ATTACH**: Attachments depend on server capabilities
4. **URL**: Preserved but may not be actionable in all UIs
5. **LOCATION**: Preserved but may not be used

### Normalization Rules

To ensure consistency and avoid spurious changes:

1. **Whitespace**: Leading/trailing whitespace trimmed from text fields
2. **Line endings**: Normalized to `\n` in multi-line fields
3. **Timezones**: All timestamps stored in UTC internally
4. **Case**: Tag/category case preserved (search may be case-insensitive)
5. **Empty values**: Empty strings, empty arrays, and whitespace-only treated as null
6. **Duplicates**: Duplicate tags removed automatically

### Conflict Detection

Momentum uses these properties to detect and resolve conflicts:

1. **LAST-MODIFIED**: Compare timestamps to identify the latest version
2. **SEQUENCE**: Version counter; higher value wins in case of conflict
3. **DTSTAMP**: Creation timestamp for initial conflict resolution

**Conflict Resolution Strategy**:
- If `SEQUENCE` differs: higher sequence wins
- If `SEQUENCE` equal but `LAST-MODIFIED` differs: latest timestamp wins
- If both equal: server version wins (conservative approach)
- User notified of conflicts via sync status UI

## Validation Rules

All tasks must satisfy these validation rules before being stored or synced:

### Field-Level Validations

1. **UID**: Must be present, non-empty, globally unique
2. **SUMMARY**: Must be present, non-empty after trimming (max 255 chars)
3. **STATUS**: Must be one of: `NEEDS-ACTION`, `IN-PROCESS`, `COMPLETED`, `CANCELLED`
4. **PRIORITY**: Must be 0-9 (clamped if out of range)
5. **PERCENT-COMPLETE**: Must be 0-100 if present
6. **SEQUENCE**: Must be non-negative integer
7. **Dates**: `DTSTART` must be ≤ `DUE` if both present
8. **Tags**: Each tag max 50 chars, max 20 tags per task
9. **Description**: Max 10,000 characters
10. **URL**: Must be valid URL format if present

### Cross-Field Validations

1. If `STATUS` is `COMPLETED`:
   - `COMPLETED` date must be present
   - `PERCENT-COMPLETE` should be 100 (auto-set if not)
2. If `STATUS` is not `COMPLETED`:
   - `COMPLETED` date must be absent (cleared if present)
   - `PERCENT-COMPLETE` must be < 100
3. Status transitions must be valid:
   - Any status can transition to `CANCELLED`
   - `TODO` → `IN_PROGRESS` → `DONE` is the normal flow
   - Direct `TODO` → `DONE` is allowed (skipping in-progress)

### Import Validation

When importing VTODO from external sources:

1. **UID generation**: If missing, generate UUID v4
2. **Default values**: Apply defaults for missing required fields
3. **Sanitization**: Remove invalid characters, trim whitespace
4. **Range clamping**: Clamp numeric values to valid ranges
5. **Error handling**: Log validation errors; either reject or auto-correct based on severity

## Backend-Specific Considerations

### CalDAV Servers

- **Full support**: All standard VTODO properties supported
- **Extensions**: Server-specific extensions stored but not interpreted
- **Sync**: Efficient sync using `REPORT` queries with time-range filters

### Future Backend Integrations

For non-CalDAV backends (iCloud, Microsoft To-Do, Google Tasks, etc.):

1. **Adapter Pattern**: Each backend has an adapter that translates to/from canonical VTODO
2. **Field Mapping**: Adapters map backend-specific fields to VTODO equivalents
3. **Capability Detection**: Adapters declare which features are supported
4. **Graceful Degradation**: Unsupported fields stored in metadata but not synced
5. **Metadata Preservation**: Store original backend data for lossless round-trips within same backend

## Change Log

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-13 | Momentum Team | Initial canonical VTODO mapping specification |

## References

- [RFC 5545: Internet Calendaring and Scheduling Core Object Specification (iCalendar)](https://tools.ietf.org/html/rfc5545)
- [RFC 4791: Calendaring Extensions to WebDAV (CalDAV)](https://tools.ietf.org/html/rfc4791)
- [Momentum Specification](../requirements/specification.md)
- [Design Overview](./design-overview.md)
