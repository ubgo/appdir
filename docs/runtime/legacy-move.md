# Legacy move

When a tool renames its global home (runbay used to live in `~/.pm`), its users still have their data in the old folder. `MoveLegacy(ctx)` moves the old global home to the new one on the first run of the new version, leaves a `MOVED.txt` note in the old place, and never deletes user data. It refuses to move while the old version's daemon still runs, and every other case (already moved, both folders exist, dev build, `--home`) is a status, not an error.

## Before you start

- [Global and project homes](../homes/global-and-project.md): only the global home is ever moved; committed project folders never are.
- [Profiles](../profiles/profiles.md): only the release profile moves.
- Import `"github.com/ubgo/appdir"`, `"context"` and `"errors"`.
- The example app is `demo`; its old home was `~/.demo-old`. Each case runs in its own fake user home under the temp root.

## Step 1 — Name the old home

`WithLegacyHome(appdir.LegacyHome{Dir: ".demo-old"})` names the old folder under the user's home (one path segment). `DaemonLocks` lists lock files inside it to probe; empty means `run/daemon.lock`. You may pass several `WithLegacyHome` options; the first old folder that exists is used.

<!-- snippet: examples/legacy-move/main.go#open -->
```go
// open resolves the app for one fake user home. WithLegacyHome names the old
// folder; DaemonLocks is left empty, so run/daemon.lock is probed.
// WithRelease(true): only the release profile ever runs a legacy move.
func open(home string, opts ...appdir.Option) (*appdir.Dirs, error) {
	base := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(home),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(true),
		appdir.WithLegacyHome(appdir.LegacyHome{Dir: oldDirName}),
	}
	return appdir.New(appName, append(base, opts...)...)
}
```

## Step 2 — Call MoveLegacy at start-up

Call it once, early, before anything opens or creates files in the home ([MoveLegacy](ds:block?id=dirs-movelegacy-aw98ygmb)). `ErrBusy` is not fatal: print it, keep going, and the next start tries again.

<!-- snippet: examples/legacy-move/main.go#move -->
```go
// migrate is what the tool runs at startup, before it opens anything in its
// home. ErrBusy is not fatal: the old daemon still runs, so the tool keeps
// using the old layout for now and tries again on its next start.
func migrate(ctx context.Context, d *appdir.Dirs) (appdir.MoveResult, error) {
	res, err := d.MoveLegacy(ctx)
	if errors.Is(err, appdir.ErrBusy) {
		return res, err // print it and go on; nothing was moved
	}
	if err != nil {
		return res, fmt.Errorf("legacy move: %w", err)
	}
	return res, nil
}
```

## Step 3 — Move on the first run

The steps ([moveOne](ds:block?id=dirs-moveone-9bhmva3m)): take `<parent>/<base(new)>.moving.lock` with an OS lock (non-blocking; held means another process is moving, `ErrBusy`); check again under the lock; probe each daemon lock in the old home without creating it (held → `ErrBusy`); on the same disk, one `rename(old, new)`; then recreate the old folder holding only `MOVED.txt` (`moved to <new>`). The result is `moved` with `From`, `To` and a note. The next run sees an old folder holding only `MOVED.txt` and reports `already`.

<!-- snippet: examples/legacy-move/main.go#first -->
```go
// firstRun moves ~/.demo-old to ~/.demo (one rename on the same disk), then
// runs again: the old folder now holds only MOVED.txt, so the result is
// "already". The .moving.lock file next to the home stays by design.
func firstRun(ctx context.Context, w io.Writer, root string) error {
	home, err := makeHome(root, caseMove, false)
	if err != nil {
		return err
	}
	for range 2 {
		d, err := open(home)
		if err != nil {
			return err
		}
		res, err := migrate(ctx, d)
		if err != nil {
			return err
		}
		printResult(w, root, res)
	}
	note, err := os.ReadFile(filepath.Join(home, oldDirName, movedNote))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s/%s: %s", oldDirName, movedNote, clean(root, string(note)))
	return printTree(w, root, home)
}
```

## Step 4 — Handle the cases that do not move

- `both-exist`: the old folder has data and the new home exists too. Nothing moves, the new home is used, and `Note` says so; show it in `doctor`.
- `skipped`: a non-release profile (a dev build has its own `~/.<app>-dev` home), a home set by `--home` or `<APP>_HOME`, or no user home folder.
- `none`: no `WithLegacyHome`, or no old folder exists.

<!-- snippet: examples/legacy-move/main.go#both -->
```go
// bothExist has an old folder with data and a new home already in place:
// nothing moves, the new home is used, and Note says so for doctor.
func bothExist(ctx context.Context, w io.Writer, root string) error {
	home, err := makeHome(root, caseBoth, true)
	if err != nil {
		return err
	}
	d, err := open(home)
	if err != nil {
		return err
	}
	res, err := migrate(ctx, d)
	if err != nil {
		return err
	}
	printResult(w, root, res)
	return nil
}
```

<!-- snippet: examples/legacy-move/main.go#skipped -->
```go
// skipped shows the cases that never move: a dev build (its own ~/.demo-dev
// home), a home given by --home (or DEMO_HOME), and no old folder at all.
func skipped(ctx context.Context, w io.Writer, root string) error {
	home, err := makeHome(root, caseFresh, false)
	if err != nil {
		return err
	}
	cases := []struct {
		label string
		home  string
		opts  []appdir.Option
	}{
		{"dev build", home, []appdir.Option{appdir.WithRelease(false)}},
		{"--home elsewhere", home, []appdir.Option{appdir.WithHome(filepath.Join(root, elsewhere))}},
		{"no old folder", filepath.Join(root, elsewhere), nil},
	}
	for _, c := range cases {
		d, err := open(c.home, c.opts...)
		if err != nil {
			return err
		}
		res, err := migrate(ctx, d)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-17s ", c.label)
		printResult(w, root, res)
	}
	return nil
}
```

## Step 5 — Wait for the old daemon

If the old version's daemon is still running, moving its home under it would break it. `MoveLegacy` opens each daemon lock that exists (never creating one) and tries the lock; held means busy. The example holds the lock with `flock` the way the old daemon would (Unix only; on Windows the example skips this case).

<!-- snippet: examples/legacy-move/main.go#busy -->
```go
// busy holds the old daemon's lock while the new version starts: MoveLegacy
// probes run/daemon.lock (it never creates it), sees it held and returns
// ErrBusy. Once the old daemon exits, the next start moves the home.
func busy(ctx context.Context, w io.Writer, root string) error {
	home, err := makeHome(root, caseBusy, false)
	if err != nil {
		return err
	}
	release, err := holdLock(filepath.Join(home, oldDirName, filepath.FromSlash(daemonLock)))
	if err != nil {
		fmt.Fprintf(w, "skipped: %v\n", err)
		return nil
	}
	d, err := open(home)
	if err != nil {
		release()
		return err
	}
	_, err = migrate(ctx, d)
	release() // the old daemon exits
	if !errors.Is(err, appdir.ErrBusy) {
		return fmt.Errorf("want ErrBusy, got %v", err)
	}
	fmt.Fprintf(w, "while the old daemon runs: %s\n", clean(root, err.Error()))
	res, err := migrate(ctx, d)
	if err != nil {
		return err
	}
	fmt.Fprint(w, "next start: ")
	printResult(w, root, res)
	return nil
}
```

## Moving across disks

When the old and new homes are on different disks (device ids differ, or cannot be read), a rename is impossible, so `MoveLegacy` copies instead. It copies the old folder into `<new>.moving-tmp` (when the old home itself is a symlink, its target is copied, not the link; inside it files, folders, symlinks as links, modes kept; sockets and pipes skipped as live state), checks that the copy has the same file count and bytes, runs your `WithLegacyVerify(func(src, dst string) error)` check (for example a database integrity check), writes a `.legacy-move` marker, renames the copy to the new home, renames the old folder to `<old>.moved-<YYYYMMDD>`, removes the marker, and leaves `MOVED.txt` in a new old folder. The old data is kept, never deleted.

Crash safety: a same-disk rename is atomic. A crash during the copy leaves `<new>.moving-tmp`, which the next run deletes (it is appdir's own partial copy; the old folder is intact) before copying again. A crash after the copy was renamed into place leaves `.legacy-move` in the new home; the next run sees it and finishes the remaining steps instead of reporting `both-exist`. A crash after the old folder was renamed to `<old>.moved-<date>` leaves `.legacy-move` (holding the old path) in the new home and no old folder; the next run matches that path, removes the marker, writes `MOVED.txt` and reports `moved`, instead of `none` forever. The example runs on one disk, so it shows only the rename path.

## Try it

```sh
go run ./examples/legacy-move
```

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

The example prints paths relative to its temp root; `MoveResult` and `MOVED.txt` hold absolute paths.

## Pitfalls

- Call `MoveLegacy` before `Ensure`, `InitHome` or anything else that creates the new home. Once the new home exists, the result is `both-exist` and nothing moves.
- Only daemon locks are probed. An old daemon that holds no lock file (only a pid file, say) is not detected; list the lock files it does hold in `DaemonLocks`.
- An old home that is a symlink to another disk (`~/.pm -> /Volumes/data/pm`) is followed: its contents are copied into the new home, and the link is what gets renamed to `<old>.moved-<date>`, so the data on the other disk stays where it was until you remove it. Before v0.1.1 the copy took the link alone, so the new home pointed back at the old disk.
- `<new>.moving.lock` stays next to the home after the move, by design: deleting a lock file races with a process that already opened it.
- A dev build never moves anything, so you cannot try the move with `go run` unless you pass `WithRelease(true)`.
- `MoveResult.From` and `To` are `omitempty` in JSON; a `none` result with no `WithLegacyHome` has neither.
- `ErrBusy` has two messages: `another process is moving <old>` (the move lock) and `old home <old> is in use by a running daemon; will retry next run` (a daemon lock).

## Related

- [Run files and sockets](run-files-and-sockets.md): the daemon lock in `run/`.
- [Errors](../reference/errors.md): `ErrBusy`.
- Example: [examples/legacy-move](../../examples/legacy-move).
- Design: [SPEC 9 Legacy move](../SPEC.md#9-legacy-move-section-8).
