# Migration practices

`internal/store` opens SQLite, applies embedded Goose migrations, and stops
startup if migration fails. The runtime then works with one current data shape.
Use this guide when changing persisted data; the
[architecture map](architecture.md) locates the affected application and public
adapters.

## Change path

1. Read the current schema and application queries before editing. Identify
   the existing records, history, receipts, checkpoints, and user decisions the
   change must preserve.
2. Add a new numbered SQL file under `internal/store/migrations`. For a
   byte-accurate JSON transformation that SQLite SQL cannot express clearly,
   register a numbered transactional Go migration with the Goose provider.
   Do not edit a migration that may already have run in a user's database.
   Guidance migrations may embed a versioned Markdown template, as migration 12
   does with `internal/store/defaults/agents-v12.md`. Keep that template immutable
   for historical replay; a later default gets a new template and migration.
   Upgrade owner instructions only on exact prior-default equality; preserve
   customized or empty values and all previously captured Run context.
3. Update the supported schema-version bound in `internal/store/store.go` for
   the new version. Startup must continue to reject a database from a newer
   unsupported runtime.
4. Change application queries and public adapters to use the new canonical
   shape. Keep legacy conversion in the migration rather than adding ongoing
   dual-shape handling to runtime paths.
5. Extend `internal/store/store_test.go` with a populated prior-version
   database, then open it through `store.Open`. Assert the converted values and
   preserved state that matter to the change. Check a fresh database as well.

`TestWatcherInterestMigrationPreservesHistory` is an example of the upgrade
test: it creates version 3 data, opens the current store, and checks Watcher
cursor and links, historical Run snapshots, pending proposals, and owner
guidance. `TestFutureSchemaFailsWithoutReset` checks rejection of a newer
schema. Add assertions for the actual state at risk in a new migration; copying
the existing assertions alone does not prove a different conversion.

## Verification

Build `web/dist` before Go tests on a clean checkout because Go embeds it:

```powershell
npm --prefix web ci
npm --prefix web run build
go test ./internal/store ./internal/app
go test ./...
```

For a migration that changes a public response or run flow, also exercise the
affected adapter test and the relevant public-interface path. Run
`scripts/verify.ps1` for the complete Windows gate. Report the source schema
version and data cases exercised; fresh-database tests do not prove an upgrade
preserves existing records.
