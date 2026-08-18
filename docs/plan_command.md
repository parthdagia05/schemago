# Command: `plan`

This document details the architecture, design, and execution model for the `schemago plan` command.

## Overview

The `schemago plan` command provides a read-only preview of the migration scripts that would be executed by an `apply` run. It computes the diff between local migration files and applied migration history in the target database without modifying the target schema or applying any user migration scripts.

## Usage

```bash
# Basic plan preview
schemago plan --database-url="postgres://user:pass@localhost:5432/dbname"

# Show full SQL file contents for each pending migration
schemago plan --database-url="postgres://user:pass@localhost:5432/dbname" --sql

# Custom migration directory and history table name
schemago plan --dir="./custom_migrations" --table="schema_history"
```

## Operation Lifecycle

1. **Configuration Resolution**: Parses `--database-url`, `--dir`, `--table`, and `--sql` flags or environment variables (`DATABASE_URL`, `MIGRATIONS_DIR`, `MIGRATIONS_TABLE`).
2. **Database Handshake**: Connects to the PostgreSQL instance and pings the database within the configured context timeout.
3. **History Verification**: Ensures the `schemago_migrations` history tracking table is present and queries applied migration records (`SELECT version, name, applied_at, checksum FROM ...`).
4. **File Discovery**: Scans the configured migrations directory (`./migrations` by default), validating filenames and computing SHA-256 file checksums.
5. **Pending Calculation & Drift Check**:
   - Calculates pending migrations (`discovered - applied`).
   - Validates that applied migrations still exist on disk.
   - Verifies SHA-256 checksum consistency for previously applied files.
6. **Plan Formatting**:
   - If no pending migrations exist, prints `"Nothing to apply. Database is up to date."`.
   - If pending migrations exist, prints the ordered list of pending files (and SQL contents if `--sql` is enabled).

## Safety Guarantees

- **No Schema Changes**: `schemago plan` does not execute any DDL or DML statements from migration files.
- **Integrity Validation**: Immediately alerts if a previously applied migration file has been edited post-execution or deleted from disk.
