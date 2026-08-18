# Integration Testing Against Real PostgreSQL

`schemago` features an end-to-end integration test suite (`internal/integration`) executed directly against a real PostgreSQL instance to validate core claims: transaction rollback safety, PostgreSQL advisory locking, schema history tracking, dry-run isolation, and CLI subcommands.

## Running Tests

To run the full test suite (including unit and real PostgreSQL integration tests):

```bash
go test -v ./...
```

### PostgreSQL Execution Strategy

1. **Automatic Container Provisioning (Default)**:
   If no PostgreSQL environment variables are set, tests automatically leverage [`testcontainers-go`](https://github.com/testcontainers/testcontainers-go) to spin up a ephemeral `postgres:16-alpine` Docker container.

2. **Targeting Existing PostgreSQL Instance (CI / Local Docker)**:
   Set `TEST_POSTGRES_DSN` or `DATABASE_URL` environment variables to run tests against an existing PostgreSQL instance:

```bash
TEST_POSTGRES_DSN="postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" go test -v ./...
```

## Test Coverage & Scenarios

The integration test suite (`internal/integration/postgres_integration_test.go`) covers:

- **Fresh Apply**: Verifies execution of pending migrations on a clean PostgreSQL database, creation of target schema objects, and populated history records in `schemago_migrations`.
- **Idempotent Re-apply**: Confirms re-running `schemago apply` on an up-to-date database is an idempotent no-op.
- **Mid-Migration Failure & Transaction Rollback**: Ensures statement execution errors within a migration cleanly roll back all uncommitted DDL/DML statements within the transaction while leaving prior successfully applied migrations untouched.
- **Concurrent Apply & PostgreSQL Advisory Locking**: Simulates multiple concurrent runner instances executing `schemago apply` simultaneously using `pg_advisory_lock` / `pg_try_advisory_lock` to ensure zero race conditions, deadlocks, or duplicate migration execution.
- **Dry-Run DB Cleanliness**: Verifies `schemago dry-run` simulates execution without creating tables or modifying database state, guaranteeing zero persistent side-effects.
- **CLI Subcommand E2E**: Tests `status`, `plan`, `dry-run`, and `apply` commands, verifying exit codes, standard output/error formatting, and `--json` mode.
