# Liquibase Migrations

## Setup
- Requires Java 17+ (CI uses Temurin 21).
- Download Liquibase CLI or use container.
- SQLite JDBC driver required for local dev (bundled with Liquibase or download separately).

## Local Development (SQLite)
The default configuration uses SQLite for local development and testing:
```sh
# Validate changeLogs
liquibase --defaultsFile=liquibase/liquibase.properties validate

# Apply migrations
liquibase --defaultsFile=liquibase/liquibase.properties update

# Rollback last changeset
liquibase --defaultsFile=liquibase/liquibase.properties rollbackCount 1
```

The database file `momentum-dev.db` will be created in the project root.

## CI/Production (PostgreSQL)
Override connection settings via environment variables:
```sh
export LIQUIBASE_URL=jdbc:postgresql://localhost:5432/momentum
export LIQUIBASE_USERNAME=momentum
export LIQUIBASE_PASSWORD=momentum
export LIQUIBASE_DRIVER=org.postgresql.Driver

liquibase --defaultsFile=liquibase/liquibase.properties update
```

## Cross-Database Compatibility Notes
- **CURRENT_TIMESTAMP**: Works identically in SQLite and PostgreSQL for TIMESTAMP fields.
- **BIGINT**: Supported in both; SQLite stores as INTEGER internally but maintains compatibility.
- **CLOB**: SQLite treats as TEXT; PostgreSQL as large text type. Both support large text storage.
- **CASCADE on DELETE**: Supported in both databases for foreign key constraints.
- **Indexes**: Both databases support multi-column and single-column indexes.

## Wrapper Script
Use the convenience wrapper:
```sh
# Validate
bash scripts/db/migrate.sh validate

# Update
bash scripts/db/migrate.sh update

# Rollback
bash scripts/db/migrate.sh rollbackCount 1
```

## Configuration
- See `liquibase/liquibase.properties` for defaults.
- Override via env vars in CI/local as needed.
