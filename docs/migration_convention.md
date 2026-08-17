# Migration File Format & Directory Convention

This document specifies the standard conventions for structuring, naming, and ordering migration files in `schemago`.

## Directory Structure

By default, `schemago` looks for migration files in the `./migrations` directory relative to the current working directory.

```text
my-project/
├── migrations/
│   ├── 0001_create_users_table.sql
│   └── 0002_add_email_index.sql
└── schemago.toml (optional)
```

The migration directory location can be overridden via command-line flags or environment variables in subcommands.

## File Naming Format

Every migration file must adhere strictly to the following naming pattern:

```text
<version>_<description>.sql
```

### Components

1. **`<version>`**: A leading sequence of numeric digits representing the migration version.
   - Standard 4-digit zero-padded integers (e.g. `0001`, `0002`) or 14-digit timestamps (e.g. `20260817000001`).
   - Version values must parse as positive 64-bit integers (`int64`).
2. **`_`**: A single underscore separator separating version and description.
3. **`<description>`**: A concise snake_case description of the schema change (e.g., `create_users_table`).
4. **`.sql`**: Plain SQL file extension.

### Examples

- `0001_create_users_table.sql`
- `0002_add_email_index.sql`
- `20260817000001_initial_schema.sql`

## Ordering and Validation Rules

1. **Numerical Sorting**: Migrations are ordered strictly by their integer version value ascending.
2. **Duplicate Rejection**: Multiple files sharing the exact same numeric version (e.g. `0001_foo.sql` and `0001_bar.sql`) will trigger a hard validation error and abort execution.
3. **Gap Tolerance**: Non-consecutive version numbers (e.g., `0001` followed by `0003`) are permitted to accommodate team development branches.
4. **Phase 1 Scope**: Phase 1 uses single `.sql` files per migration. Each file is executed within an isolated transaction. Dual `.up.sql` / `.down.sql` pairing is reserved for Phase 2 rollback features.
