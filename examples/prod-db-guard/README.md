# Example: prod-db guard

Shows `CheckDB`: a dev build refuses the prod global and project databases (`ErrProdDB`), opens them read-only with `DEMO_ALLOW_PROD=1`, opens its own database read-write; a release build is never refused. A relative path is checked against the working folder `New` was given, and a dev build is refused even before the prod home exists. It also shows the env-leak filter ignoring `DEMO_HOME=~/.demo` and the prod view following a `db:` pointer.

Everything runs inside a temp root: the user's home is a folder under it (`WithHomeDir`) and the environment is a map (`WithEnv`), so the program never touches your real home or reads your shell's variables.

## Run it

```sh
go run ./examples/prod-db-guard
```

## Output

Paths are relative to the temp root.

```text
== fresh machine: no prod home yet
dev build, home/.demo/demo.db (folder missing) -> dev build refuses the prod database home/.demo/demo.db; set DEMO_ALLOW_PROD=1 to open it read-only
== dev build vs release build
dev build, prod global db                        home/.demo/demo.db -> dev build refuses the prod database home/.demo/demo.db; set DEMO_ALLOW_PROD=1 to open it read-only
dev build, prod project db                       home/proj/.demo/demo.db -> dev build refuses the prod database home/proj/.demo/demo.db; set DEMO_ALLOW_PROD=1 to open it read-only
dev build + DEMO_ALLOW_PROD=1, prod project db   home/proj/.demo/demo.db -> read-only
dev build, its own db                            home/proj/.demo/profiles/dev/demo.db -> read-write
release build, prod project db                   home/proj/.demo/demo.db -> read-write
== relative path
dev build, .demo/demo.db from home/proj -> dev build refuses the prod database home/proj/.demo/demo.db; set DEMO_ALLOW_PROD=1 to open it read-only
== env leak: DEMO_HOME=~/.demo in a dev shell
warning env-points-to-prod: DEMO_HOME points into the prod home home/.demo; ignored for profile dev
global home: home/.demo-dev
open home/.demo/demo.db: dev build refuses the prod database home/.demo/demo.db; set DEMO_ALLOW_PROD=1 to open it read-only
== db: pointer in the project's config.local.jsonc
warning pointer-ignored-for-profile: db: pointer in home/proj/.demo/config.local.jsonc ignored for profile dev; the dev database stays at home/proj/.demo/profiles/dev/demo.db
dev build, shared-data/team.db -> dev build refuses the prod database shared-data/team.db; set DEMO_ALLOW_PROD=1 to open it read-only
dev build, home/proj/.demo/profiles/dev/demo.db -> read-write
```

## Guides

- [Prod-db guard](../../docs/profiles/prod-db-guard.md)
- [Child processes and env leaks](../../docs/profiles/child-processes.md)
