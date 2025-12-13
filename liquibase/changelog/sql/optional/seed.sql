-- Development seed data (optional)
-- NOTE: This file is NOT referenced by any Liquibase changeset.
-- To load development seed data, run this file manually:
--   sqlite3 momentum-dev.db < liquibase/changelog/sql/optional/seed.sql
-- DO NOT run in production environments.

INSERT INTO users(email) VALUES ('demo@momentum.local');
