# Advisory Locking Architecture

`schemago` uses PostgreSQL session-level advisory locking to guarantee deployment safety when multiple application pods or instances boot simultaneously.

## Purpose

When deploying microservices or web applications with multiple replicas (e.g., Kubernetes rolling deployments), multiple containers may attempt to execute `schemago apply` at the exact same moment.

Without advisory locking, concurrent migration runners risk:
1. Executing duplicate DDL statements simultaneously.
2. Aborting transactions due to lock contention or table creation collisions.
3. Writing duplicate or corrupt history records in `schemago_migrations`.

`schemago` solves this by acquiring a session-level PostgreSQL advisory lock before inspecting or applying pending migrations.

## Key Mechanics

1. **Deterministic Lock Keys**: Lock IDs are 64-bit signed integers derived deterministically from the schema history table name (e.g. `schemago:schemago_migrations`). Different history tables operate under independent locks.
2. **Session-Level Locking**: `schemago` uses `SELECT pg_advisory_lock($1)` on a dedicated database connection. Session-level locks remain active across per-migration transactions.
3. **Automatic Cleanup & Release**: Lock release (`SELECT pg_advisory_unlock($1)`) is executed upon completion, error rollback, or abnormal process exit (PostgreSQL automatically releases session advisory locks when the TCP connection closes).
4. **Leader / Waiter Behavior**:
   - The first runner acquires the lock and applies pending migrations.
   - Secondary waiters block at lock acquisition.
   - When the leader finishes and releases the lock, the waiter unblocks, re-evaluates the history table, finds zero pending migrations, and exits cleanly with exit code `0`.

## Bypassing Locking (Optional)

In single-process or CI/CD pipelines where external locking is managed at the orchestrator level, advisory locking can be disabled using the `--no-lock` flag:

```bash
schemago apply --no-lock
```
