# Web Kanban Board

This directory contains the web-based kanban board UI for Momentum.

## Features

- **Three-column layout**: Todo, In Progress, Done
- **Accessible task controls**: Create, edit, delete, and change status with keyboard-friendly form controls; drag-and-drop remains an optional shortcut
- **Rich task fields**: Edit title, description, status, due date, priority, and tags
- **Backend selection**: Choose an owned backend from the board; backend and list filter state is represented in shareable URLs
- **Persistent state**: All changes are saved to the database and persist across page reloads
- **Responsive design**: Works on desktop and mobile devices
- **Accessible feedback**: Mutation errors and stale concurrent edits are announced through ARIA live regions and reconciled with server state

## Current scope

The server requires an authenticated session for the board, list, and task API. Each task mutation may include an `If-Match` version from the rendered task card. If another device changes the task first, the server returns `409 Conflict`; the browser announces the conflict and reloads the authoritative board instead of silently overwriting the newer change. Failed optimistic status moves are likewise reloaded from the server.

1. **Authentication**: Currently uses hard-coded backend_id=1. Future versions will integrate proper user authentication and authorization.
2. **Router**: Uses manual URL parsing. Consider migrating to gorilla/mux or similar router for more robust parameter handling.
3. **Accessibility**: Replace alert() with ARIA live regions for screen reader compatibility.
4. **Error Recovery**: Improve error handling to revert specific task cards instead of full page reload.
5. **WebSocket Support**: Add real-time updates for collaborative editing scenarios.

The historical scaffold notes above are superseded by the authenticated,
accessible workflow described in `## Current scope`; live push updates remain
outside the current release scope.

## Architecture

### Template Structure
- `web/templates/index.html` - Main kanban board template with embedded CSS and JavaScript

### API Endpoints
- `GET /` - Render the kanban board
- `GET /list` - Render the filtered/sorted list view
- `POST /api/tasks` - Create a task
- `PUT /api/tasks/{id}` - Update task fields
- `DELETE /api/tasks/{id}` - Delete a task
- `PATCH /api/tasks/{id}/status` - Update task status

### Status Mapping
The kanban board maps task statuses to columns as follows:
- **Todo**: `NEEDS-ACTION`
- **In Progress**: `IN-PROCESS`
- **Done**: `COMPLETED`

## Development

### Running the Server
```bash
# Initialize the database with test data
./scripts/db/init_dev.sh

# Build and run the server
go build -o server ./cmd/server
DB_PATH=momentum_dev.db PORT=8080 ./server
```

Then open http://localhost:8080 in your browser.

### Testing
```bash
# Run all tests
go test ./...

# Run only web handler tests
go test ./internal/web
```

## Usage

1. **View tasks**: Open the web interface to see tasks organized by status
2. **Move tasks**: Click and drag a task card to another column
3. **Persistence**: Changes are automatically saved - refresh the page to verify

## Technical Details

### Progressive Enhancement
The UI follows the principle of progressive enhancement:
- Server-side rendering with Go templates
- Vanilla JavaScript for drag-and-drop (no heavy frameworks)
- Graceful degradation if JavaScript is disabled

### Error Handling
- Invalid status values are rejected by the server
- Failed API calls trigger page reload to restore correct state
- User-friendly error messages for all failure scenarios

### Security
- Status validation prevents invalid state
- All API calls use JSON over HTTPS in production
- No client secrets or sensitive data in frontend code
