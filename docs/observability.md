# Observability, Exit Codes & Structured Error Output

`schemago` provides deterministic exit codes, detailed statement-level error reporting, and structured JSON output to make database migration pipeline failures obvious and machine-readable for both humans and CI/CD pipelines.

## 1. Statement and Line Error Tracking

When executing migration scripts in `apply` or `dry-run`, `schemago` parses SQL migration files into individual statements while tracking 1-based start line numbers.

When a SQL statement fails:
- **Offending Migration File**: Identifies the exact file name (e.g. `0002_create_posts.sql`).
- **Statement Index**: Identifies which statement failed in sequence (1-indexed).
- **Line Number**: Identifies the exact line number in the source file where the failing statement begins.
- **Statement Snippet**: Truncates and displays the failing SQL statement.
- **Database Error**: Preserves full cause from PostgreSQL/database driver.
- **Transactional Cleanliness**: Clearly confirms that the transaction was cleanly rolled back and zero persistent changes were committed.

### Human-Readable Failure Output Example

```text
Applying 2 pending migrations:

  ✓ 0001_create_users.sql (12ms)
  ✗ 0002_create_posts.sql FAILED (4ms)
    Error at statement 2 (line 9): pq: relation "invalid_table" does not exist
    Statement: CREATE TABLE posts (id INT PRIMARY KEY, user_id INT REFERENCES invalid_table(id))
    Migration rolled back cleanly.

Applied 1/2 migration(s). Execution stopped on failure.
```

---

## 2. Process Exit Codes

`schemago` uses strict, consistent process exit codes across all subcommands for reliable CI pipeline gating.

| Exit Code | Name | Description |
|-----------|------|-------------|
| `0` | `ExitSuccess` | Command completed successfully with zero errors, pending migrations, or drift issues. |
| `1` | `ExitFailure` | Execution failure, DB error, transaction rollback, pending migrations (`status`), or checksum drift (`status`). |
| `2` | `ExitUsage` | Command-line usage error, unknown subcommand, or invalid flag syntax. |

### Exit Code Behavior by Command

- **`schemago status`**: Returns `0` if all discovered migrations are cleanly applied with no pending scripts or checksum drift. Returns `1` if there are pending migrations or integrity drift (enabling CI deployment gating).
- **`schemago plan`**: Returns `0` after previewing pending migrations. Returns `1` on DB connection or config failure.
- **`schemago apply`**: Returns `0` when all pending migrations apply successfully. Returns `1` on any failure or rollback.
- **`schemago dry-run`**: Returns `0` when dry-run validation succeeds. Returns `1` on SQL errors or non-transactional statements.

---

## 3. Structured Logging (`--json`)

The global `--json` flag formats output from all commands as machine-readable JSON for logging frameworks and CI pipelines.

### Usage

```bash
schemago --json status
schemago --json plan
schemago --json apply
schemago --json dry-run
```

### Apply Failure JSON Output Example

```json
{
  "applied": [
    {
      "file": {
        "version": 1,
        "filename": "0001_create_users.sql",
        "path": "migrations/0001_create_users.sql",
        "checksum": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
      },
      "duration_ms": 12
    }
  ],
  "failed": {
    "file": {
      "version": 2,
      "filename": "0002_create_posts.sql",
      "path": "migrations/0002_create_posts.sql",
      "checksum": ""
    },
    "duration_ms": 4,
    "error": "failed to execute statement 2 (line 9) in migration \"0002_create_posts.sql\": pq: relation \"invalid_table\" does not exist",
    "statement_index": 2,
    "line_number": 9,
    "statement_sql": "CREATE TABLE posts (id INT PRIMARY KEY, user_id INT REFERENCES invalid_table(id))"
  },
  "total_pending": 2
}
```

### Structured CLI Error Format Example

```json
{
  "command": "apply",
  "error": "schemago apply error: missing database connection string",
  "exit_code": 1
}
```
