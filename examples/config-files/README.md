# Example: config files

Lists `ConfigFiles()` in read order for the prod and dev profiles (with the dev profile's own `profiles/dev/config.local.jsonc`), with a `home:` pointer, with `--home`, with no `$HOME` and with `config.local.jsonc` dropped by `WithoutKind(KindConfigLocal)`; shows `Committed` inside a git work tree (a `.git` folder); merges one key with a toy loader so later files visibly win; and replaces the JSONC reader with `WithConfigReader` to show what appdir reads.

Everything runs inside a temp folder: the user home is a folder in it (via `appdir.WithHomeDir`), the environment is empty (via `appdir.WithEnv`), and the real home is never touched. The temp root is printed as `$TMP`.

## Run it

```sh
go run ./examples/config-files
```

## Output

```text
== init
created $TMP/shop/.lore

== prod (profile prod)
1. global  $TMP/userhome/.lore/config.jsonc
2. global  $TMP/userhome/.lore/config.local.jsonc local missing
3. project $TMP/shop/.lore/config.jsonc committed
4. project $TMP/shop/.lore/config.local.jsonc local
port = 4000 (from $TMP/shop/.lore/config.local.jsonc)

== dev (profile dev)
1. global  $TMP/userhome/.lore-dev/config.jsonc missing
2. global  $TMP/userhome/.lore-dev/config.local.jsonc local missing
3. project $TMP/shop/.lore/config.jsonc committed
4. project $TMP/shop/.lore/config.local.jsonc local
5. project $TMP/shop/.lore/profiles/dev/config.local.jsonc local profile=dev
port = 4100 (from $TMP/shop/.lore/profiles/dev/config.local.jsonc)

== home: pointer (profile prod)
1. global  $TMP/userhome/.lore/config.jsonc
2. global  $TMP/userhome/.lore/config.local.jsonc local missing
3. project $TMP/shop/.lore/config.jsonc committed
4. project $TMP/shop/.lore/config.local.jsonc local
5. project $TMP/shop-private/lore/config.jsonc committed
6. project $TMP/shop-private/lore/config.local.jsonc local missing
port = 5000 (from $TMP/shop-private/lore/config.jsonc)

== home: pointer (profile dev)
1. global  $TMP/userhome/.lore-dev/config.jsonc missing
2. global  $TMP/userhome/.lore-dev/config.local.jsonc local missing
3. project $TMP/shop/.lore/config.jsonc committed
4. project $TMP/shop/.lore/config.local.jsonc local
5. project $TMP/shop-private/lore/config.jsonc committed
6. project $TMP/shop-private/lore/config.local.jsonc local missing
7. project $TMP/shop-private/lore/profiles/dev/config.local.jsonc local profile=dev missing
port = 5000 (from $TMP/shop-private/lore/config.jsonc)

== --home (profile prod)
1. global  $TMP/elsewhere/config.jsonc missing
2. global  $TMP/elsewhere/config.local.jsonc local missing
port unset (code default)

== no $HOME (profile prod)
1. project $TMP/shop/.lore/config.jsonc committed
2. project $TMP/shop/.lore/config.local.jsonc local
3. project $TMP/shop-private/lore/config.jsonc committed
4. project $TMP/shop-private/lore/config.local.jsonc local missing
port = 5000 (from $TMP/shop-private/lore/config.jsonc)

== WithoutKind(config-local) (profile prod)
1. global  $TMP/userhome/.lore/config.jsonc
2. project $TMP/shop/.lore/config.jsonc committed
port = 3000 (from $TMP/shop/.lore/config.jsonc)

== WithConfigReader
read $TMP/userhome/.lore/home.jsonc
read $TMP/shop/.lore/config.jsonc
read $TMP/shop/.lore/config.local.jsonc
read $TMP/shop-private/lore/home.jsonc
read $TMP/shop-private/lore/config.jsonc
read $TMP/shop-private/lore/config.local.jsonc
read $TMP/shop-private/lore/home.jsonc
```

## Read more

- Guide: [Config files](../../docs/layout/config-files.md)
- Guide: [Pointer keys](../../docs/pointers/pointer-keys.md)
