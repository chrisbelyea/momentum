-- Preserve whether DTSTART and DUE were date-only values (schema version 5).
ALTER TABLE tasks ADD COLUMN due_date_only INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN start_date_only INTEGER NOT NULL DEFAULT 0;
