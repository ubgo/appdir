# Child processes and env leaks

Environment variables cross the line between your installed tool and your dev build in both directions. A shell opened by the installed tool still exports `<APP>_HOME=~/.<app>`; a worker your dev build starts must run as `dev` too. appdir handles the first case itself (the env-leak filter) and gives you three helpers for the second: `ChildEnv`, `CleanEnv` and `ForProfile`. Read this page when your tool starts other processes, wraps dev tasks, or needs another profile's paths.

## Before you start

- [Profiles](profiles.md): the profile order and the dev folders.
- [Overrides](../homes/overrides.md): what `<APP>_HOME` and `<APP>_<KIND>_DIR` do normally.
- Import `"github.com/ubgo/appdir"` (and `"os/exec"` to start a child).

## Step 1 — Let the env-leak filter ignore prod paths

In a non-release profile, an `<APP>_HOME` or `<APP>_<KIND>_DIR` whose value lies inside the release profile's global home (`~/.<app>`) is ignored, with warning `env-points-to-prod`. The dev build keeps its own folders ([pointsToProd](ds:block?id=dirs-pointstoprod-9gdwvqpz)). Values that point anywhere else still work. Print `Warnings()` once at start-up so the user sees why their variable had no effect ([paths --json and warnings](../reference/paths-json.md)).

<!-- snippet: examples/profiles/main.go#leak -->
```go
// envLeak runs a dev build in a shell that still carries variables meant for
// the installed tool. Values that point into the prod home are ignored with a
// warning; the dev build keeps its own folders.
func envLeak(w io.Writer, s sandbox) error {
	env := fakeEnv{envHome: "~/.demo", envDBDir: "~/.demo/data"}
	d, err := s.open(env, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	for _, warn := range d.Warnings() {
		fmt.Fprintf(w, "warning %s: %s\n", warn.Code, s.clean(warn.Message))
	}
	db, err := d.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "profile %s: global %s, db %s\n", d.Profile(), s.show(d.Global().Base), s.show(db))
	return nil
}
```

## Step 2 — Pass the profile to child processes

`ChildEnv(environ)` returns `environ` with `<APP>_PROFILE` set to the current profile, replacing any value already there ([ChildEnv](ds:block?id=dirs-childenv-myszxsrp)). Use it as `cmd.Env` for every child your tool starts, so a worker started by the dev build runs as dev. `CleanEnv(environ, prefix)` returns `environ` without any `<PREFIX>_*` variable ([CleanEnv](ds:block?id=cleanenv-276nsr8w)); it is a pure function for dev task wrappers that must start from a clean slate.

`ForProfile(p)` resolves the same app with the same options and environment, as if `--profile p` were given ([ForProfile](ds:block?id=dirs-forprofile-g2gegg5p)). It reads only, so a dev build can compute the prod paths (for `doctor`, or for a discovery file) without opening prod's files.

<!-- snippet: examples/profiles/main.go#child -->
```go
// childProcesses passes the profile on: ChildEnv sets DEMO_PROFILE for a
// child the tool starts, CleanEnv strips every DEMO_* variable (what a dev
// task wrapper does), and ForProfile resolves another profile's paths
// without opening its files. A real tool passes os.Environ().
func childProcesses(w io.Writer, s sandbox) error {
	parent := []string{"PATH=/usr/bin", envProfile + "=" + string(appdir.ProfileProd), envHome + "=~/.demo"}
	d, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	cmd := exec.Command(workerBinary) // built, never started here
	cmd.Env = d.ChildEnv(parent)
	fmt.Fprintf(w, "ChildEnv: %s\n", strings.Join(cmd.Env, " "))
	fmt.Fprintf(w, "CleanEnv: %s\n", strings.Join(appdir.CleanEnv(parent, envPrefix), " "))

	prod, err := d.ForProfile(appdir.ProfileProd)
	if err != nil {
		return err
	}
	db, err := prod.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "ForProfile(prod): profile %s, global %s, db %s\n", prod.Profile(), s.show(prod.Global().Base), s.show(db))
	return nil
}
```

## Try it

```sh
go run ./examples/profiles
```

The sections this page covers:

```text
== env leak filter
warning env-points-to-prod: DEMO_HOME points into the prod home home/.demo; ignored for profile dev
warning env-points-to-prod: DEMO_DB_DIR points into the prod home home/.demo; ignored for profile dev
profile dev: global home/.demo-dev, db home/proj/.demo/profiles/dev/demo.db
== child processes
ChildEnv: PATH=/usr/bin DEMO_HOME=~/.demo DEMO_PROFILE=dev
CleanEnv: PATH=/usr/bin
ForProfile(prod): profile prod, global home/.demo, db home/proj/.demo/demo.db
```

The example prints paths relative to its temp root; a real run prints absolute paths.

## Pitfalls

- `ChildEnv` sets only `<APP>_PROFILE`. A leaked `<APP>_HOME` stays in the child's environment (see `DEMO_HOME=~/.demo` above); the child, being dev, ignores it when it points into prod. To drop it, run `CleanEnv` first and `ChildEnv` on the result.
- `CleanEnv` matches by prefix plus `_`: `CleanEnv(env, "DEMO")` removes `DEMO_HOME` but keeps `DEMOX=1`. It removes every variable with that prefix, including ones that are not appdir's.
- The filter runs only in a non-release profile. In prod, `<APP>_HOME=~/.<app>` is honoured like any other value.
- The filter compares against the prod global home only. An `<APP>_HOME` pointing at a prod project home is not filtered; the [prod-db guard](prod-db-guard.md) still refuses the prod project database.
- `ForProfile` with an unknown profile returns `ErrUnknownProfile`. It keeps your `--home` option, so under `--home` both profiles resolve to the same folder.

## Related

- [Prod-db guard](prod-db-guard.md): the second line of defence.
- [Run files and sockets](../runtime/run-files-and-sockets.md): `DaemonDiscovery` uses `ForProfile`.
- Example: [examples/profiles](../../examples/profiles).
- Design: [SPEC 8.5 Env leak](../SPEC.md#85-env-leak) and [SPEC 8.6 Another profile](../SPEC.md#86-another-profile).
