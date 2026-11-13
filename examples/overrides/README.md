# overrides

Every way a user moves folders: `WithHome` (`--home`, also relative and with `~`), `LORE_HOME`, `LORE_CACHE_DIR` and `LORE_DB_DIR` (a file kind keeps its name), the names `EnvPrefix`, `EnvName` and `KindEnv` return, `WithEnvPrefix` and `WithDirName` (and the `..` dir name it refuses), values that never become a path (an unset or empty `${VAR}`, `~user`), and a `$VAR` without braces that stays literal text. The last block shows a `--home` with an unset variable: every kind fails with that error, the cache included. The environment is a map passed with `WithEnv`.

## Run

```sh
go run ./examples/overrides
```

Output (paths relative to the temp root):

```text
== no override
config  project   home/projects/shop/.lore/config.jsonc
db      project   home/projects/shop/.lore/lore.db
cache   global    home/.lore/cache
== --home ~/try
config  flag      home/try/config.jsonc
db      flag      home/try/lore.db
cache   flag      home/try/cache
== --home scratch (relative)
config  flag      home/projects/shop/scratch/config.jsonc
db      flag      home/projects/shop/scratch/lore.db
cache   flag      home/projects/shop/scratch/cache
== LORE_HOME=${FAST}/lore
config  env-home  fast/lore/config.jsonc
db      env-home  fast/lore/lore.db
cache   env-home  fast/lore/cache
with --home ~/try: global home/try (rule flag)
dev build, LORE_HOME=~/.lore/x: global home/.lore-dev (rule global)
  warning env-points-to-prod: LORE_HOME points into the prod home home/.lore; ignored for profile dev
== LORE_CACHE_DIR and LORE_DB_DIR
config  project   home/projects/shop/.lore/config.jsonc
db      env-kind  fast/db/lore.db
cache   env-kind  home/fast-cache
== env names
EnvPrefix: LORE  EnvName(HOME): LORE_HOME
KindEnv(cache): LORE_CACHE_DIR  KindEnv(config): "" (anchored)
with prefix NOTES: KindEnv(cache): NOTES_CACHE_DIR
with dir name .lore-data: global home home/.lore-data
with dir name ..: error: appdir: dir name ".." must be one plain path segment
== bad values
${FAST} unset:  error: LORE_CACHE_DIR: ${FAST} is not set
${FAST} empty:  error: LORE_CACHE_DIR: ${FAST} is not set
~user:          error: LORE_CACHE_DIR: "~bob/cache": the ~user form is not supported
$FAST literal:  cache: home/projects/shop/$FAST/cache
LORE_HOME empty: cache: home/.lore/cache
== --home with an unset ${VAR}
db     error: --home: ${NOPE} is not set
cache  error: --home: ${NOPE} is not set
```

`go test ./examples/overrides` runs the same program in a test folder and checks these lines.

## Guide

- [Overrides: --home and env](../../docs/homes/overrides.md)
- [Global and project homes](../../docs/homes/global-and-project.md) (`WithDirName`)
