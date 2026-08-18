# Command: `dry-run`

This document details the architecture, design, and execution model for the `schemago dry-run` command.

## Overview

The `schemago dry-run` command executes the full migration apply pipeline against the target database without committing any changes. It executes pending migrations sequentially inside an isolated transaction that is guaranteed to be rolled back at completion or upon failure.

This enables developers and CI/CD pipelines to validate SQL migration syntax, foreign key constraints, table dependencies, and schema changes against the real database engine while leaving the target database completely untouched.

## Usage

```bash
# Basic dry-run execution
schemago dry-run --database-url="postgres://user:pass@localhost:5432/dbname"

# Dry-run with custom migration directory and history table name
schemago dry-run --dir="./custom_migrations" --table="schema_history"

# Dry-run disabling advisory lock
schemago dry-run --no-lock
```

## Operation Lifecycle

1. **Configuration Resolution**: Parses `--database-url`, `--dir`, `--table`, and `--no-lock` flags or environment variables (`DATABASE_URL`, `MIGRATIONS_DIR`, `MIGRATIONS_TABLE`).
2. **Database Handshake**: Connects to the target database and verifies connectivity within the configured timeout.
3. **Advisory Lock Acquisition**: Unless `--no-lock` is specified, acquires a session-level advisory lock to prevent race conditions during dry-run validation.
4. **History Table Verification**: Ensures the `schemago_migrations` tracking table exists.
5. **File Discovery**: Scans the configured migrations directory, validating numerical version ordering and computing SHA-256 checksums.
6. **Pending Calculation & Drift Check**:
   - Calculates pending migrations (`discovered - applied`).
   - Verifies SHA-256 checksum consistency for previously applied migrations.
7. **Single-Transaction Dry-Run Execution**:
   - If no pending migrations exist, outputs `"[DRY-RUN] Nothing to apply. Database is up to date."` and exits with code `0`.
   - Begins a single outer database transaction (`tx, err := db.BeginTx(ctx, nil)`).
   - Defers a mandatory rollback (`defer tx.Rollback()`) to ensure no changes are committed.
   - For each pending migration file:
     - Scans for non-transactional statements (`CREATE INDEX CONCURRENTLY`, `VACUUM`, etc.). If detected, execution halts with `ErrNonTransactional`.
     - Executes SQL statements on `tx`.
     - Simulates history recording by inserting the record into `schemago_migrations` within `tx`.
   - Upon completion or failure, the transaction rolls back cleanly, leaving zero persistent changes in the target database.
8. **Formatted Output**: Output lines and summaries are clearly tagged with `[DRY-RUN]` to distinguish dry-run validation from real apply operations.

## Safety Guarantees

- **Zero Persistent Changes**: Every DDL statement and history record in dry-run is executed inside a transaction that is unconditionally rolled back.
- **SQL Error Catching**: Detects syntax errors, constraint violations, and invalid DDL statements across chained pending migrations before applying changes in production.
- **Clear Distinction**: Formatted output explicitly marks all results with `[DRY-RUN]`, eliminating confusion with live migration applies.
