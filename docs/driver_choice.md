# Architecture Record: PostgreSQL Driver Selection & Connection Configuration

This document records the architectural decision regarding external driver selection for PostgreSQL database connectivity in `schemago`, alongside configuration precedence and connection lifecycle management.

## Driver Selection: `github.com/jackc/pgx/v5`

`schemago` uses `github.com/jackc/pgx/v5` via its `database/sql` compatibility layer (`github.com/jackc/pgx/v5/stdlib`).

### Rationale & Comparison

| Criteria | `jackc/pgx/v5` | `lib/pq` |
| :--- | :--- | :--- |
| **Maintenance Status** | Actively maintained & feature-complete | Maintenance mode (legacy) |
| **Performance** | High performance with native binary encoding | Standard text protocol |
| **`database/sql` Compatibility** | Full support via `pgx/v5/stdlib` | Native `database/sql` driver |
| **Type Support** | Full support for modern Postgres types | Basic type mapping |

Using `pgx/v5/stdlib` allows `schemago` to retain standard Go `database/sql` abstractions while gaining the reliability, security patches, and performance optimizations of the `pgx` ecosystem.

## Configuration Precedence

Database connection strings are resolved using the following order of priority:

1. **CLI Flag (`--database-url`)**: Explicit flag provided to `schemago` commands.
2. **Environment Variable (`DATABASE_URL`)**: Fallback connection string set in the environment.

If neither is supplied, `schemago` returns an actionable error instructing the user on how to provide a valid connection string.

## Health Check & Timeout Enforcement

- **Connection Timeout**: Every connection ping operation operates under a 5-second contextual deadline by default.
- **Actionable Error Messaging**: Raw connection, network, and driver errors are parsed and wrapped into user-friendly diagnostic messages (e.g., host lookup failures, connection refused, authentication errors, or missing parameters).
