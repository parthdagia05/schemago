# Command: `apply`

This document details the architecture, design, and execution model for the `schemago apply` command.

## Overview

The `schemago apply` command executes pending database migrations sequentially against the target database.

The core differentiator of `schemago` is **safe by default**: each migration runs inside its own isolated database transaction (`BEGIN ... COMMIT`). If an individual migration encounters an error during execution or recording, its transaction is cleanly rolled back (`ROLLBACK`). Execution stops immediately, ensuring previously committed migrations remain safely applied while the failed migration leaves no partial or dirty state in the database.

## Usage

```bash
# Basic apply execution
schemago apply --database-url="postgres://user:pass@localhost:5432/dbname"

# Apply with custom migration directory and history table name
schemago apply --dir="./custom_migrations" --table="schema_history"
```

## Operation Lifecycle

1. **Configuration Resolution**: Parses `--database-url`, `--dir`, and `--table` flags or environment variables (`DATABASE_URL`, `MIGRATIONS_DIR`, `MIGRATIONS_TABLE`).
2. **Database Handshake**: Connects to the PostgreSQL instance and verifies connectivity within the context timeout.
3. **History Table Verification**: Ensures the `schemago_migrations` tracking table exists, creating it automatically if missing.
4. **File Discovery**: Scans the configured migrations directory, validating version sequences and computing SHA-256 checksums.
5. **Pending Calculation & Integrity Verification**:
   - Calculates pending migrations (`discovered - applied`).
   - Verifies that previously applied migrations exist on disk and match stored SHA-256 checksums.
6. **Transactional Migration Execution**:
   - If no pending migrations exist, prints `"Nothing to apply. Database is up to date."` and exits with code `0`.
   - For each pending migration file:
     - Scans SQL content for non-transactional statements (`CREATE INDEX CONCURRENTLY`, `VACUUM`, etc.). If detected, execution halts with `ErrNonTransactional`.
     - Begins an isolated database transaction (`tx, err := db.BeginTx(ctx, nil)`).
     - Executes the migration's SQL statements inside the transaction.
     - Inserts the migration record into the history table (`schemago_migrations`) within the **same transaction**.
     - Commits the transaction (`tx.Commit()`).
7. **Failure Recovery & Rollback**:
   - If SQL execution, history recording, or commit fails:
     - Triggers transaction rollback (`tx.Rollback()`).
     - Reports the exact error and migration filename.
     - Stops execution of remaining pending migrations.
     - Exits with code `1`.

## Safety Guarantees

- **Transactional Atomicity**: Every migration and its history table record commit together atomically in a single transaction.
- **Zero Dirty State**: A failed migration rolls back completely. The database schema remains in a clean, consistent state.
- **Safe Re-execution**: After resolving the cause of a failed migration, re-running `schemago apply` safely resumes from the failed migration without dead-ends.
