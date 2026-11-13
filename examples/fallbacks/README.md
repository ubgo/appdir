# fallbacks

What a global tool gets with no user home (`WithHomeDir` returning an error): config uses code defaults, the db fails with `ErrNoHome`, the cache uses the temp folder; then the same with `WithTempFallback(false)`; then `Ensure` in a home made read-only with `chmod 0500` (skipped as root). The program only prints where the temp fallback goes; its tests create it under a private `TMPDIR` and also show the read-only fallback of the cache and `ErrUnsafeTemp` for a symlinked temp folder. The root guard (`ErrRootOwned`) needs root and is described in the guide only.

## Run

```sh
go run ./examples/fallbacks
```

Output (paths relative to the temp root; the temp folder is shown as `$TMPDIR`, the uid as `<uid>`):

```text
== no $HOME
global home: rule=defaults fallback=true note="no home folder"
config  rule=defaults fallback=true (none: code defaults)
db      rule=global   error: no home folder; set LORE_HOME
cache   rule=temp     fallback=true $TMPDIR/lore-<uid>/cache
== no $HOME, WithTempFallback(false)
config  rule=defaults fallback=true (none: code defaults)
db      rule=global   error: no home folder; set LORE_HOME
cache   rule=global   error: no home folder; set LORE_HOME
== read-only home
Ensure(db): create ro-home/.lore: mkdir ro-home/.lore: permission denied (set LORE_HOME or LORE_DB_DIR)
Ensure(cache): create ro-home/.lore/cache: mkdir ro-home/.lore: permission denied (set LORE_HOME or LORE_CACHE_DIR)
```

`go test ./examples/fallbacks` runs the same program in a test folder and checks these lines.

## Guide

- [Fallbacks: no home, read-only home, root](../../docs/homes/fallbacks.md)
