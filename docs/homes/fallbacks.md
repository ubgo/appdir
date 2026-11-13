# Fallbacks: no home, read-only home, root

Tools run in places with no user home (a CI container, a systemd unit with no `HOME`), on read-only disks, and under `sudo`. appdir keeps them working where it is safe and fails with a clear sentence where it is not: config falls back to code defaults, the cache moves to a private temp folder, data kinds fail and say which variable to set, and root is stopped from writing root-owned files into a user's home. Read this page to know what your tool does in each of those places.

## Before you start

- [First use](../start/first-use.md) and [Global and project homes](global-and-project.md).
- Import `"github.com/ubgo/appdir"`.
- The example is a global tool (`WithMode(appdir.ModeGlobal)`), so the db lives in the global home and the failures show without a project. It gets the user's home from `WithHomeDir`:

<!-- snippet: examples/fallbacks/main.go#open -->
```go
// open resolves a global tool's homes with the given home lookup. A failing
// lookup is what os.UserHomeDir returns in a container with no $HOME.
func open(homeDir func() (string, error), work string, extra ...appdir.Option) (*appdir.Dirs, error) {
	opts := append([]appdir.Option{
		appdir.WithMode(appdir.ModeGlobal),
		appdir.WithHomeDir(homeDir),
		appdir.WithWorkDir(work),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(true), // prod profile: plain folder names
	}, extra...)
	d, err := appdir.New(appName, opts...)
	if err != nil {
		return nil, fmt.Errorf("appdir: %w", err)
	}
	return d, nil
}
```

## Step 1 — Run with no user home

When the home lookup fails (`os.UserHomeDir` returns an error) and the user passed no `--home` or `<APP>_HOME`, `New` still succeeds. The global home gets rule `defaults` with `Fallback: true` and empty paths, and each kind does what is safe ([per-kind rules](ds:block?id=dirs-entryin-zhkmadk3)):

- Anchored kinds (`home.jsonc`, the config files) have no path and rule `defaults`: the tool uses its code defaults.
- Global data kinds (here the db) fail with `ErrNoHome`: `no home folder; set LORE_HOME`.
- Disposable kinds (the cache) use `<tmp>/<Name(app)>-<uid>/` with rule `temp` and `Fallback: true`. `<tmp>` is `os.TempDir()`, `<uid>` is the numeric uid (the user name on Windows), and `Name(app)` adds the profile for a dev build (`lore-dev-1001`).

<!-- snippet: examples/fallbacks/main.go#nohome -->
```go
// noHome shows each kind when the home lookup fails. New still succeeds:
// config is "use code defaults", the db names the missing home, the cache
// goes to <tmp>/<app>-<uid>/.
func noHome(w io.Writer, root, work string) error {
	d, err := open(func() (string, error) { return "", errors.New(noHomeMsg) }, work)
	if err != nil {
		return err
	}
	g := d.Global()
	fmt.Fprintf(w, "global home: rule=%s fallback=%v note=%q\n", g.Rule, g.Fallback, g.Note)
	printKinds(w, root, d)
	return nil
}
```

The first `Ensure` on the cache creates the temp folder with mode `0700`. If that folder already exists and is a symlink or belongs to another user, `Ensure` refuses with `ErrUnsafeTemp` instead of writing into a folder someone else controls ([ensurePrivateDir](ds:block?id=dirs-ensureprivatedir-ars7qvkj)). The example's tests show both, under a private `TMPDIR`.

## Step 2 — Turn the temp fallback off

`WithTempFallback(false)` turns off both temp fallbacks (no home, and read-only home in Step 3). Disposable kinds then fail like data kinds. Use it when cache data must never land in a shared temp folder.

<!-- snippet: examples/fallbacks/main.go#notemp -->
```go
// noHomeNoTemp turns the temp fallback off: the cache now fails like the db.
func noHomeNoTemp(w io.Writer, root, work string) error {
	d, err := open(func() (string, error) { return "", errors.New(noHomeMsg) }, work,
		appdir.WithTempFallback(false))
	if err != nil {
		return err
	}
	printKinds(w, root, d)
	return nil
}
```

## Step 3 — Handle a read-only home

Reads still work in a home the user cannot write. `Ensure` fails with the OS error, the folder it tried to create, and a hint naming the two variables that move it: `<APP>_HOME` for everything, `<APP>_<KIND>_DIR` for that kind. Disposable kinds are tried once more in the temp folder when creating fails with a read-only or permission error, unless the temp fallback is off ([Ensure](ds:block?id=dirs-ensure-mxbz3fmd)). The example turns it off so `go run` writes nothing outside its temp root; its test `TestEnsureReadOnlyFallsBack` shows the cache landing in the temp folder.

<!-- snippet: examples/fallbacks/main.go#readonly -->
```go
// readOnlyHome makes a home the user cannot write to and calls Ensure.
// The error names the folder and the variables that move it.
func readOnlyHome(w io.Writer, root, work string) error {
	if os.Geteuid() == 0 {
		fmt.Fprintln(w, "skipped: running as root, folder permissions do not apply")
		return nil
	}
	home := filepath.Join(root, readOnlyDir)
	if err := os.Mkdir(home, readOnly); err != nil {
		return fmt.Errorf("create read-only home: %w", err)
	}
	defer func() { _ = os.Chmod(home, writable) }() // let the caller remove root
	d, err := open(func() (string, error) { return home, nil }, work,
		appdir.WithTempFallback(false)) // off, so this run writes nothing to the system temp folder
	if err != nil {
		return err
	}
	for _, k := range []appdir.Kind{appdir.KindDB, appdir.KindCache} {
		if _, err := d.Ensure(k); err != nil {
			fmt.Fprintf(w, "Ensure(%s): %s\n", k, label(root, err.Error()))
			continue
		}
		return fmt.Errorf("Ensure(%s) wrote into a read-only home", k)
	}
	return nil
}
```

## Step 4 — Know the root guard

Before any write, appdir checks the effective uid ([checkWrite](ds:block?id=dirs-checkwrite-n23fhubq)). If it is 0 and the nearest existing folder of the target home belongs to another uid, the write fails with `ErrRootOwned`: `running as root but <your home>/.lore belongs to uid 1000; pass --allow-root`. This catches `sudo lore …` on Linux, where sudo keeps `HOME` pointing at your home and would otherwise leave root-owned files there that your next normal run cannot write. A tool that really means it passes its `--allow-root` flag as `WithAllowRoot(true)`. The check runs in `Ensure`, `InitProject`, `InitHome` and the other write calls, never in reads. On Windows it does nothing: an elevated process is the same user. The example does not run it, because it needs root.

## Try it

```sh
go run ./examples/fallbacks
```

Paths are relative to the example's temp root; the temp folder is shown as `$TMPDIR` and the uid as `<uid>`. As root, the read-only block prints a skip line instead:

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

The temp fallback, the read-only fallback and `ErrUnsafeTemp` are exercised by `go test ./examples/fallbacks`, which points `TMPDIR` at a test folder first.

## Pitfalls

- **A project tool outside a project is not saved by the fallbacks.** In `ModeProject` the db is a project kind, so with no home it fails with `ErrNoProject`, not `ErrNoHome`; only kinds in the global home use the rules above.
- **`~` in an override needs a home.** With no user home, `LORE_HOME=~/x` fails with `ErrNoHome` (`"~/x" needs a home folder`). Use an absolute path in containers.
- **A broken override is not "no home".** The temp fallback and code defaults apply only when there is no user home and no override. A `--home` or `<APP>_HOME` with an unset `${VAR}` fails every kind, the cache included, with the flag or variable name in the message (`--home: ${NOPE} is not set`); [Overrides, Step 6](overrides.md#step-6--report-a-broken-override) shows the code and its output from `go run ./examples/overrides`.
- **The temp folder is per user and shared by every project.** Two projects of the same app share `<tmp>/lore-<uid>/cache` in the fallback. Do not put project data in a disposable kind.
- **Some systems clean the temp folder.** systemd-tmpfiles and macOS remove old files from `/tmp`; a fallback cache can vanish between runs. That is why only disposable kinds may use it.
- **The root guard cannot be shown by a normal example.** It needs uid 0, and running the read-only step as root succeeds because root ignores folder permissions; the example skips that step as root.
- **Do not catch `ErrRootOwned` and retry with `WithAllowRoot(true)`.** The flag exists for an explicit user choice; passing it always brings back the root-owned files the guard prevents.

## Related

- [Overrides: --home and env](overrides.md)
- [Global and project homes](global-and-project.md)
- [Files and permissions](../layout/files-and-permissions.md) (modes, `CACHEDIR.TAG`)
- [Errors reference](../reference/errors.md)
- Example: [examples/fallbacks](../../examples/fallbacks/README.md)
- SPEC: [4.3 Global home](../SPEC.md#43-global-home), [4.7 Per-kind result](../SPEC.md#47-per-kind-result), [4.8 Writing: sudo, root, read-only](../SPEC.md#48-writing-sudo-root-read-only)
