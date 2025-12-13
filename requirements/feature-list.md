# Feature List

This document outlines the planned features for Momentum.

## Core Features

- Kanban board as default view with drag-and-drop between status columns.
- List view with sort/filter.
- Create, update, delete tasks (CalDAV VTODOs as canonical).
- Tagging via CalDAV labels.
- Unified view across multiple backends.

## Task Management

- Task fields: title, description, due date, priority, status, tags.
- Configurable board columns per user.
- Search/filter by backend, status, tags, text.
- Bulk operations (complete, move, transfer).

## CalDAV Integration

- Internal CalDAV server enabled by default (local-only, no firewall prompts).
- Connect to external CalDAV servers via URL + credentials/certificates.
- Transfer tasks between backends preserving metadata.
- Incremental sync and conflict handling.

## User Interface

- Web app (desktop/mobile) with installable PWA, prefer htmx.
- Native clients: Windows (WinUI), macOS/iOS/iPadOS (Swift), Linux (Vala+GTK).
- Consistent UX and shared core logic across clients.

## Future Enhancements

- Integrations: iCloud Reminders, Microsoft To-Do/Outlook, Microsoft Planner, Google Tasks, Trello, Jira, GitHub Issues/Projects.
- Teams/sharing, advanced permissions.
- Attachments and subtasks (where backend supports).

## References

- Vision: [requirements/vision.md](requirements/vision.md)
- Specification: [requirements/specification.md](requirements/specification.md)
