# Liquibase Database Migrations

This directory contains Liquibase database migration files for Momentum.

## Overview

Momentum uses [Liquibase](https://www.liquibase.org/) for database schema management and migrations. This ensures:
- **Version control** for database schema changes
- **Consistent migrations** across environments (development, staging, production)
- **Cross-database support** (SQLite for self-hosted, PostgreSQL for production)
- **Rollback capabilities** for schema changes

## Directory Structure

```
liquibase/
├── liquibase.properties    # Default configuration for Liquibase
├── changelog.xml           # Master changelog that includes all changesets
├── changelogs/            # Individual changeset files
│   └── 001-initial-schema.xml
└── README.md              # This file
```

## Configuration

### liquibase.properties

The `liquibase.properties` file contains default settings for database connections and validation:

- **changeLogFile**: Points to the master changelog file
- **driver/url**: Database connection settings (defaults to SQLite)
- **liquibase.strictChecksums**: Ensures changesets cannot be modified after deployment

### Environment-Specific Configuration

Override connection settings via environment variables or command-line arguments:

```bash
# SQLite (default for self-hosted)
liquibase --url=jdbc:sqlite:momentum.db update

# PostgreSQL (production/SaaS)
liquibase \
  --url=jdbc:postgresql://localhost:5432/momentum \
  --username=momentum_user \
  --password=${DB_PASSWORD} \
  update
```

## Usage

### Running Migrations

Use the `scripts/db/migrate.sh` helper script:

```bash
# Apply all pending migrations
./scripts/db/migrate.sh update

# View migration status
./scripts/db/migrate.sh status

# Validate changelog files
./scripts/db/migrate.sh validate
```

Or run Liquibase directly:

```bash
liquibase --defaultsFile=liquibase/liquibase.properties update
```

### Validation

Liquibase validation checks that:
- All changelog files are well-formed XML/YAML
- Referenced files exist
- Changesets have valid structure
- No duplicate changeset IDs

**Validation runs automatically in CI** and will fail PRs with invalid changesets.

```bash
liquibase --defaultsFile=liquibase/liquibase.properties validate
```

### Creating New Migrations

1. Create a new changeset file in `changelogs/`:
   ```bash
   touch liquibase/changelogs/002-add-tags-table.xml
   ```

2. Add your changeset with a unique ID:
   ```xml
   <?xml version="1.0" encoding="UTF-8"?>
   <databaseChangeLog
       xmlns="http://www.liquibase.org/xml/ns/dbchangelog"
       xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
       xsi:schemaLocation="http://www.liquibase.org/xml/ns/dbchangelog
           http://www.liquibase.org/xml/ns/dbchangelog/dbchangelog-4.27.xsd">

       <changeSet id="002-create-tags-table" author="your-name">
           <comment>Add tags table for task labels</comment>
           <createTable tableName="tags">
               <column name="id" type="VARCHAR(36)">
                   <constraints primaryKey="true" nullable="false"/>
               </column>
               <column name="name" type="VARCHAR(255)">
                   <constraints nullable="false"/>
               </column>
           </createTable>
       </changeSet>
   </databaseChangeLog>
   ```

3. Include it in the master `changelog.xml`:
   ```xml
   <include file="changelogs/002-add-tags-table.xml" relativeToChangelogFile="true"/>
   ```

4. Validate your changes:
   ```bash
   liquibase --defaultsFile=liquibase/liquibase.properties validate
   ```

5. Test the migration:
   ```bash
   liquibase --defaultsFile=liquibase/liquibase.properties update
   ```

## CI/CD Integration

The CI workflow (`.github/workflows/ci.yml`) automatically:
1. Installs Liquibase
2. Runs `liquibase validate` on all PRs
3. Fails the build if validation errors are detected

This ensures that **bad changesets block PRs** before they can be merged.

## Best Practices

1. **Never modify deployed changesets**: Once a changeset is merged and deployed, create a new changeset for any corrections
2. **Use descriptive IDs**: Format as `NNN-description` (e.g., `001-create-users-table`)
3. **Add comments**: Explain the purpose of each changeset
4. **Test both databases**: Validate changes work on both SQLite and PostgreSQL
5. **Keep changesets atomic**: Each changeset should represent a single, complete change
6. **Use rollback when possible**: Define rollback steps for complex migrations

## Troubleshooting

### Validation Failures

If `liquibase validate` fails:

1. Check XML syntax:
   ```bash
   xmllint --noout liquibase/changelog.xml
   xmllint --noout liquibase/changelogs/*.xml
   ```

2. Verify file references in `changelog.xml` match actual files

3. Ensure changeset IDs are unique

4. Check that the changelog schema version matches your Liquibase version

### Common Errors

- **File not found**: Check paths in `<include>` tags are correct
- **Duplicate changeset ID**: Each `id` + `author` combination must be unique
- **Invalid XML**: Use an XML validator or IDE with XML support
- **Schema mismatch**: Ensure xmlns declarations match Liquibase version

## References

- [Liquibase Documentation](https://docs.liquibase.com/)
- [Liquibase Best Practices](https://www.liquibase.org/get-started/best-practices)
- [Database Change Log Format](https://docs.liquibase.com/concepts/changelogs/home.html)
- Momentum Specification: [requirements/specification.md](../requirements/specification.md) (Section 14: Tooling and CI/CD)

## Version

- Liquibase Version: 4.27.0 (as specified in `.github/workflows/ci.yml`)
- Changelog Schema: 4.27.xsd
