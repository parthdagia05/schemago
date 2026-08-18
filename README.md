# schemago

**A standalone database migration runner.** One tiny binary you run in your deploy
pipeline to apply schema changes *before* your app starts: safely, visibly, and the
same way every time. Not tied to any framework or language.

> Database changes should be a separate, deliberate, visible step in your deployment,
> not a side effect of your app turning on.

## Why schemago

The common shortcut, letting your app run migrations on boot, works until it doesn't:
a migration fails halfway and leaves the database half-updated, or twenty pods start at
once, fight over a schema lock, and freeze production. schemago pulls migrations out of
app startup and into one deliberate step.

The closest tool to schemago is **golang-migrate** (same shape: Go, single binary,
plain SQL, Postgres). schemago's wedge is the safe defaults it never shipped:

1. **Safe by default.** Every migration runs inside a transaction and rolls back
   cleanly on failure. No half-applied state, no hand-written `BEGIN`/`COMMIT`, no
   "dirty" version you have to manually force back.
2. **Deployment-safe.** Built-in advisory locking so twenty pods deploying at once
   can't race. Only one migration runs at a time; the rest wait, they don't freeze.
3. **Simple by default.** One static binary, plain `.sql` files. No JVM (unlike Flyway
   / Liquibase), no DSL to learn (unlike Atlas), no paid tier gating the safety features.

## Installation

`schemago` is distributed as a single static binary with zero runtime dependencies (no `libc` / dynamic library requirement).

### Download Pre-built Binary (Recommended)

Download the latest binary for your operating system and architecture from [GitHub Releases](https://github.com/parthdagia05/schemago/releases):

**Linux (amd64 / arm64)**:
```bash
curl -sSL https://github.com/parthdagia05/schemago/releases/latest/download/schemago_Linux_amd64.tar.gz | tar -xz -C /usr/local/bin schemago
```

**macOS (Apple Silicon / Intel)**:
```bash
curl -sSL https://github.com/parthdagia05/schemago/releases/latest/download/schemago_Darwin_arm64.tar.gz | tar -xz -C /usr/local/bin schemago
```

**Windows (amd64 / arm64)**:
Download the latest `schemago_Windows_amd64.zip` asset from the release page, unzip, and place `schemago.exe` in your system `PATH`.

### Build From Source

```bash
go install github.com/parthdagia05/schemago/cmd/schemago@latest
```

Or clone and build a static binary manually:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o schemago ./cmd/schemago
```

For release pipelines, cross-compilation matrix, and static binary verification instructions, see [docs/single_static_binary_release.md](docs/single_static_binary_release.md).

## Commands (planned v1)

| Command   | What it does                                                       |
|-----------|--------------------------------------------------------------------|
| `status`  | Which migrations have run and which are still pending.             |
| `plan`    | Preview exactly what's about to change before anything happens.    |
| `apply`   | Run the pending migrations, safely and one at a time.              |
| `dry-run` | Go through the motions without touching the real database.         |
| `version` | Display the current schemago release version.                      |

## Migration File Format & Directory Convention

`schemago` reads migration scripts from `./migrations` by default. Migrations use plain `.sql` files named `<version>_<description>.sql`:

- `0001_create_users_table.sql`
- `0002_add_email_index.sql`

Migrations are executed in strict numerical version order. For details on gap tolerance and duplicate version validation, see [docs/migration_convention.md](docs/migration_convention.md).

## Configuration & Database Connection

`schemago` connects to PostgreSQL using `pgx` (`github.com/jackc/pgx/v5`). Connection strings are resolved in the following priority order:

1. `--database-url` CLI flag (e.g. `schemago apply --database-url="postgres://user:pass@localhost:5432/dbname"`)
2. `DATABASE_URL` environment variable

For architectural details on driver selection and connection handling, see [docs/driver_choice.md](docs/driver_choice.md).

## Schema History Table

`schemago` tracks applied migrations inside the target database using the `schemago_migrations` table (version, name, applied_at, checksum). Checksums ensure already-applied migration files cannot be secretly modified on disk.

For schema specifications and integrity validation rules, see [docs/schema_history.md](docs/schema_history.md).

## Observability, Exit Codes & Structured Logging

`schemago` provides obvious, machine-readable error reporting for CI pipelines:

- **Statement & Line Tracking**: Every failure identifies the offending migration file, statement index, line number, SQL snippet, and database error cause.
- **Consistent Exit Codes**: Exit `0` for success, `1` for execution failure/pending status/checksum drift, `2` for usage errors.
- **Structured JSON Flag (`--json`)**: Output command results and errors as formatted JSON for automated log parsing in CI/CD pipelines.

For detailed specifications and examples, see [docs/observability.md](docs/observability.md).

## Integration Testing

`schemago` includes an end-to-end integration test suite against real PostgreSQL instances using `testcontainers-go` and CI service containers.

```bash
go test -v ./...
```

For test suite architecture and coverage details, see [docs/integration_testing.md](docs/integration_testing.md).

## Status

Early development. Postgres first; MySQL and more later. See the issue tracker for the
build plan.

## License

MIT. See [LICENSE](LICENSE).

