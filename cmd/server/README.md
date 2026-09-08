# Momentum Server

Internal CalDAV server for Momentum task management.

## Features

- **VTODO CRUD Operations**: Create, Read, Update, and Delete tasks locally
- **RESTful API**: Simple HTTP endpoints for task management
- **Authentication**: Registration, login, logout, session ownership, and route protection
- **TLS**: TLS 1.3 minimum with auto-generated development certificates or operator-supplied certificates
- **SQLite Storage**: Lightweight, file-based database (PostgreSQL support via existing schema)
- **Single Executable**: Compiles to a single binary with no external dependencies

## Building

```bash
# Build the server
go build -o bin/momentum-server ./cmd/server

# Run tests
go test ./...
```

## Running

### Quick Start

After building, simply run:

```bash
./bin/momentum-server
```

The server initializes or upgrades the SQLite database automatically and auto-generates
a self-signed TLS certificate on first run when `TLS_CERT` and `TLS_KEY` are omitted.
It reuses that certificate on subsequent runs. Open `https://localhost:8443` in your
browser (accept the security warning — this is expected for a development certificate).
The root page requires an authenticated session. Register the first account with the
JSON endpoint or use an existing client; browser-native onboarding is tracked in
[#145](https://github.com/chrisbelyea/momentum/issues/145).

### Prerequisites

1. Start the server (it initializes or upgrades the canonical SQLite schema automatically):

```bash
# Default: uses the OS user data directory, listens on https://localhost:8443
./bin/momentum-server

# Custom configuration via environment variables
DB_PATH=/path/to/database.db PORT=8443 ./bin/momentum-server
```

### Using custom TLS certificates

Set `TLS_CERT` and `TLS_KEY` to your certificate and key files to skip auto-generation:

```bash
export TLS_CERT=/path/to/cert.pem
export TLS_KEY=/path/to/key.pem
./bin/momentum-server
```

See [docs/tls-setup.md](../../docs/tls-setup.md) for development and production
certificate setup instructions.

## Configuration

The server is configured via environment variables:

- `DB_PATH`: Path to SQLite database file (default: OS user data directory)
- `PORT`: HTTPS server port (default: `8443`)
- `TLS_CERT`: Path to TLS certificate PEM file (optional; auto-generated for development if not set)
- `TLS_KEY`: Path to TLS private key PEM file (optional; auto-generated for development if not set)
- `HTTP_REDIRECT_PORT`: If set, starts an HTTP server that redirects to HTTPS
- `EXTERNAL_HOST`: Public hostname used for HTTP→HTTPS redirect URLs (default: `localhost:<PORT>`)
- `MOMENTUM_ENCRYPTION_KEY`: Encryption key for backend credentials (required in production)
- `MOMENTUM_DEV_MODE`: Set to `1` only for local development/integration tests when no encryption key is available
- `MOMENTUM_DEV_CERT_DIR`: Directory for the auto-generated development certificate (packaged launchers set this inside the data directory)

## API Endpoints

### Health Check

```
GET /health
```

Returns server health status.

### List Tasks

```
GET /caldav/tasks?backend_id={backend_id}
```

Lists all tasks for a given backend.

**Query Parameters:**
- `backend_id` (required): Backend identifier

**Response:** JSON array of task objects

### Get Task

```
GET /caldav/tasks/{id}
```

Retrieves a single task by ID.

**Response:** JSON task object

### Create Task

```
POST /caldav/tasks
Content-Type: application/json

{
  "backend_id": 1,
  "title": "Task title",
  "description": "Task description",
  "status": "NEEDS-ACTION",
  "priority": 5,
  "due_at": "2024-12-31T23:59:59Z"
}
```

Creates a new task.

**Required Fields:**
- `backend_id`: Backend identifier (integer)
- `title`: Task title

**Optional Fields:**
- `description`: Task description
- `status`: Task status (default: `NEEDS-ACTION`)
- `priority`: Priority level (integer)
- `due_at`: Due date in ISO 8601 format
- `tags_json`: Tags as JSON string
- `external_id`: External system identifier

**Response:** Created task object with generated ID

### Update Task

```
PUT /caldav/tasks/{id}
Content-Type: application/json

{
  "backend_id": 1,
  "title": "Updated title",
  "status": "IN-PROCESS",
  "priority": 7
}
```

Updates an existing task.

**Required Fields:**
- `backend_id`: Backend identifier (integer)
- `title`: Task title
- `status`: Task status

**Response:** Updated task object

### Delete Task

```
DELETE /caldav/tasks/{id}
```

Deletes a task by ID.

**Response:** 204 No Content on success

## Task Statuses

Valid task status values (from VTODO specification):

- `NEEDS-ACTION`: Task needs to be started
- `IN-PROCESS`: Task is in progress
- `COMPLETED`: Task is completed
- `CANCELLED`: Task is cancelled

## Example Usage

```bash
# Authenticate first and send the returned session cookie with protected routes.
# The examples below use integer backend/task IDs and https://localhost:8443.

# Create a task
curl -k -X POST https://localhost:8443/caldav/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "backend_id": 1,
    "title": "Buy groceries",
    "status": "NEEDS-ACTION",
    "priority": 5
  }'

# List tasks
curl -k "https://localhost:8443/caldav/tasks?backend_id=1"

# Get a task
curl -k https://localhost:8443/caldav/tasks/{task-id}

# Update a task
curl -k -X PUT https://localhost:8443/caldav/tasks/{task-id} \
  -H "Content-Type: application/json" \
  -d '{
    "backend_id": 1,
    "title": "Buy groceries and cook dinner",
    "status": "IN-PROCESS",
    "priority": 8
  }'

# Delete a task
curl -k -X DELETE https://localhost:8443/caldav/tasks/{task-id}
```

## Architecture

The server follows a layered architecture:

- **cmd/server**: Application entry point and HTTP server setup
- **internal/caldav**: HTTP handlers and CalDAV protocol logic
- **internal/db**: Database repository layer
- **internal/models**: Data models and domain logic
- **pkg/vtodo**: RFC 5545-safe VTODO parsing, validation, and generation

## Testing

Run tests with:

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test ./... -cover

# Run tests with verbose output
go test ./... -v
```

## Development

### Code Organization

- Keep HTTP handlers thin - business logic belongs in the repository layer
- Use the existing `models.Task` struct to maintain consistency with the database schema
- Follow Go standard project layout conventions

### Current release boundaries

- The release binary includes the CalDAV client and backend configuration/validation APIs,
  but automatic external-backend synchronization is not yet wired into the running
  server. Track [#68](https://github.com/chrisbelyea/momentum/issues/68) and
  [#144](https://github.com/chrisbelyea/momentum/issues/144).
- Browser-native account onboarding is not in v0.2.2; use `/auth/register` and
  `/auth/login` until [#145](https://github.com/chrisbelyea/momentum/issues/145) is merged.
- Scheduled synchronization, PWA installability automation, and an in-browser
  conflict queue remain future work; see the active roadmap in `docs/status.md`.

## References

- [Requirements Specification](../requirements/specification.md) - Sections 4, 9
- [Design Overview](../docs/design-overview.md) - CalDAV Server section
- [VTODO Mapping](../docs/vtodo-mapping.md) - Task field mappings
- [RFC 4791](https://tools.ietf.org/html/rfc4791) - CalDAV specification
- [RFC 5545](https://tools.ietf.org/html/rfc5545) - iCalendar specification
