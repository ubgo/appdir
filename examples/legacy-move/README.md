# Example: legacy move

Shows `MoveLegacy` moving an old global home (`~/.demo-old`) to `~/.demo` on one disk and leaving `MOVED.txt`, then `already` on the second run, `both-exist`, `skipped` for a dev build and for `--home`, `none`, and `ErrBusy` while the old daemon's `run/daemon.lock` is held (held with `flock` in `lock_unix.go`; skipped on Windows).

Everything runs inside a temp root: the user's home is a folder under it (`WithHomeDir`) and the environment is a map (`WithEnv`), so the program never touches your real home or reads your shell's variables.

## Run it

```sh
go run ./examples/legacy-move
```

## Output

Paths are relative to the temp root.

```text
== first run moves, second run sees it done
status=moved from=home-move/.demo-old to=home-move/.demo note="renamed (same disk)"
status=already from=home-move/.demo-old to=home-move/.demo
.demo-old/MOVED.txt: moved to home-move/.demo
files in home-move:
  .demo-old/MOVED.txt
  .demo.moving.lock
  .demo/config.jsonc
  .demo/demo.db
  .demo/notes/today.md
  .demo/run/daemon.lock
== both homes exist
status=both-exist from=home-both/.demo-old to=home-both/.demo note="both exist; using home-both/.demo"
== skipped
dev build         status=skipped note="profile dev never runs a legacy move"
--home elsewhere  status=skipped note="home set by --home or DEMO_HOME"
no old folder     status=none
== old daemon still running
while the old daemon runs: old home home-busy/.demo-old is in use by a running daemon; will retry next run
next start: status=moved from=home-busy/.demo-old to=home-busy/.demo note="renamed (same disk)"
```

## Guides

- [Legacy move](../../docs/runtime/legacy-move.md)
