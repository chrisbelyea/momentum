# Database schema

`internal/db/schema/changelog/*.sql` is Momentum's canonical SQLite schema
changelog, applied in filename order. It defines the schema used by repositories and the release
binary. `internal/db/schema/init.sql` is a generated, embedded artifact only;
regenerate it with `scripts/db/generate-init-sql.sh` and validate it with
`scripts/db/generate-init-sql.sh --check`.

At startup, `InitializeSchema` records the schema version in
`momentum_schema_migrations`. It initializes fresh databases and upgrades the
pre-#59 development/v0.1.3 shapes (including the string-ID XML schema and the
partial `due_at` schema) without discarding user, backend, or task rows.

The old Liquibase YAML/XML changelogs and independent development DDL were
retired in #59. They must not be used to create or migrate Momentum SQLite
databases.
