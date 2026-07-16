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

## Commands (planned v1)

| Command   | What it does                                                       |
|-----------|--------------------------------------------------------------------|
| `status`  | Which migrations have run and which are still pending.             |
| `plan`    | Preview exactly what's about to change before anything happens.    |
| `apply`   | Run the pending migrations, safely and one at a time.              |
| `dry-run` | Go through the motions without touching the real database.         |

## Status

Early development. Postgres first; MySQL and more later. See the issue tracker for the
build plan.

## License

MIT. See [LICENSE](LICENSE).
