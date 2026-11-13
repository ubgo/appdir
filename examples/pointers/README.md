# Example: pointer keys

Shows the `home:` and `db:` pointer keys of a project home: a relative `home:` in a committed `config.jsonc` with comments and trailing commas, the target created with `InitHome`, the missing-target error, an absolute pointer refused in the committed file and accepted in `config.local.jsonc`, a `db:` pointer, the one-hop warning, the non-string error, the dev profile (`home:` data under `<target>/profiles/dev`, `db:` ignored with a warning), `ConfigFiles()` under a broken pointer (the marker's own files stay listed), and an empty `home:` in `config.local.jsonc` that leaves the committed pointer in force.

Everything runs inside a temp folder: the user home is a folder in it (via `appdir.WithHomeDir`), the environment is empty (via `appdir.WithEnv`), and the real home is never touched. The temp root is printed as `$TMP`.

## Run it

```sh
go run ./examples/pointers
```

## Output

```text
== init
created $TMP/work/shop/.lore

== home: pointer, target missing
error: pointer points to $TMP/work/shop-private/lore, which does not exist; run 'lore init --home $TMP/work/shop-private/lore'
  sentinel: ErrPointerMissing

== home: pointer, target created with InitHome
profile: prod
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore
db:      $TMP/work/shop-private/lore/lore.db
backups: $TMP/work/shop-private/lore/backups

== absolute home: in config.jsonc
error: home: "$TMP/work/shop-private/lore" in $TMP/work/shop/.lore/config.jsonc is absolute; put it in config.local.jsonc
  sentinel: ErrPointerAbsolute

== absolute home: in config.local.jsonc
profile: prod
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore
db:      $TMP/work/shop-private/lore/lore.db
backups: $TMP/work/shop-private/lore/backups

== db: pointer
error: pointer points to $TMP/work/shop-db, which does not exist
  sentinel: ErrPointerMissing
profile: prod
project: $TMP/work/shop/.lore (rule project)
data:    $TMP/work/shop/.lore
db:      $TMP/work/shop-db/lore.db
backups: $TMP/work/shop/.lore/backups

== one hop only
profile: prod
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore
db:      $TMP/work/shop-private/lore/lore.db
backups: $TMP/work/shop-private/lore/backups
warning pointer-chain-ignored: home in $TMP/work/shop-private/lore/config.jsonc not followed: pointers go one hop only

== home: is not a string
error: $TMP/work/shop/.lore/config.local.jsonc: home must be a string path
  sentinel: ErrInvalidOption

== dev profile with home: and db:
profile: dev
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore/profiles/dev
db:      $TMP/work/shop-private/lore/profiles/dev/lore.db
backups: $TMP/work/shop-private/lore/profiles/dev/backups
warning pointer-ignored-for-profile: db: pointer in $TMP/work/shop/.lore/config.jsonc ignored for profile dev; the dev database stays at $TMP/work/shop-private/lore/profiles/dev/lore.db

== ConfigFiles under a broken pointer
Project: ErrPointerMissing
global  $TMP/userhome/.lore/config.jsonc
global  $TMP/userhome/.lore/config.local.jsonc
project $TMP/work/shop/.lore/config.jsonc
project $TMP/work/shop/.lore/config.local.jsonc

== empty home: in config.local.jsonc
profile: prod
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore
db:      $TMP/work/shop-private/lore/lore.db
backups: $TMP/work/shop-private/lore/backups
```

## Read more

- Guide: [Pointer keys](../../docs/pointers/pointer-keys.md)
- Guide: [Config files](../../docs/layout/config-files.md) (the JSONC reader)
