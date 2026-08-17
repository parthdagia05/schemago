# Schema History Table & Migration Tracking

This document outlines the architecture and schema of the migration history table in `schemago`.

## Overview

To track which migrations have been applied, `schemago` maintains a lightweight schema history table inside the target database.

By default, the table is named `schemago_migrations`. It is automatically created on the first run of database operations if absent.

## History Table Schema

```sql
CREATE TABLE IF NOT EXISTS schemago_migrations (
    version BIGINT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    checksum VARCHAR(64) NOT NULL
);
```

### Column Specification

1. **`version`** (`BIGINT PRIMARY KEY`): The numeric version identifier extracted from the migration filename (e.g. `1` or `20260817000001`).
2. **`name`** (`VARCHAR(255)`): The full migration filename (e.g. `0001_create_users_table.sql`).
3. **`applied_at`** (`TIMESTAMPTZ`): UTC timestamp recording when the migration was applied to the database.
4. **`checksum`** (`VARCHAR(64)`): The hex-encoded SHA-256 hash of the migration file content at the time of execution.

## Pending Calculation & Checksum Verification

When determining the set of pending migrations:

1. **Discovery**: `schemago` scans the migration directory and sorts discovered files by version ascending.
2. **Database Query**: `schemago` reads all records from `schemago_migrations`.
3. **Pending Set**: Pending migrations are calculated as `discovered - applied`.
4. **Integrity Validation**: For already applied migrations, `schemago` verifies that the stored checksum matches the local file checksum. If an applied migration file on disk has been edited post-application, execution halts with a `ErrChecksumMismatch` error to preserve deployment safety.
