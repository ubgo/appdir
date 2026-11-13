# Prod-db guard

A dev build that opens the installed tool's database can migrate it to a schema the release does not know, and your real data is gone. `CheckDB(path)` is the guard you call before opening a database and before any migration: a dev build gets `ErrProdDB` for the prod databases of this machine, `AccessReadOnly` with `<APP>_ALLOW_PROD=1`, and `AccessReadWrite` for everything else. A release build always gets `AccessReadWrite`.

## Before you start

- [Profiles](profiles.md) and [Release detection](release-detection.md): the guard is about dev builds.
- Import `"github.com/ubgo/appdir"` and `"errors"`.
- The example app is `demo`. A release run first creates the project home and the two prod databases: `home/.demo/demo.db` (global) and `home/proj/.demo/demo.db` (project).

## Step 1 — Check before you open

`CheckDB` resolves a relative path against the working folder `New` used (`WithWorkDir`), not the process's current folder, then applies the symlink rule (`VerifyFile`), then, in a dev build, resolves the paths the release profile would use on this machine and compares ([CheckDB](ds:block?id=dirs-checkdb-2r8b9xmq)). That prod view uses no `--home` and the environment with every `<APP>_*` variable removed (the overrides; other variables stay, so a `db:` pointer written with `${VAR}` still expands): what an installed binary started from the same folder would open. It covers the global home's db and the project db, including a `db:` pointer; a project view that fails does not skip the global db. Files are compared by identity (`os.SameFile`); a file that does not exist yet is compared by its real parent folder and its name (case-insensitive on macOS and Windows), and when that folder does not exist yet either (a fresh prod home), by real path.

<!-- snippet: examples/prod-db-guard/main.go#openDB -->
```go
// openDB is what a tool runs before it opens its database: CheckDB first,
// then open with the access it allows. ErrProdDB means "do not open at all".
func openDB(d *appdir.Dirs, path string) (appdir.DBAccess, error) {
	access, err := d.CheckDB(path)
	if errors.Is(err, appdir.ErrProdDB) {
		return access, err // the tool prints it and exits; never opens, never migrates
	}
	if err != nil {
		return access, fmt.Errorf("check %s: %w", path, err)
	}
	// AccessReadOnly: open read-only and skip migrations.
	return access, nil
}
```

## Step 2 — Try the prod and dev databases

A dev build is refused on both prod databases, gets read-only with `DEMO_ALLOW_PROD=1`, and read-write on its own database under `profiles/dev/`. A release build is never refused.

<!-- snippet: examples/prod-db-guard/main.go#guard -->
```go
// guard tries the prod databases from a dev build (refused, then read-only
// with DEMO_ALLOW_PROD=1), the dev build's own database, and a release build.
func guard(w io.Writer, s sandbox, prodGlobal, prodProject string) error {
	dev, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	allowed, err := s.open(fakeEnv{envAllowProd: allowProdOn}, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	release, err := s.open(nil, appdir.WithRelease(true))
	if err != nil {
		return err
	}
	own, err := dev.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	cases := []struct {
		label string
		d     *appdir.Dirs
		path  string
	}{
		{"dev build, prod global db", dev, prodGlobal},
		{"dev build, prod project db", dev, prodProject},
		{"dev build + DEMO_ALLOW_PROD=1, prod project db", allowed, prodProject},
		{"dev build, its own db", dev, own},
		{"release build, prod project db", release, prodProject},
	}
	for _, c := range cases {
		access, err := openDB(c.d, c.path)
		if err != nil {
			fmt.Fprintf(w, "%-48s %s -> %s\n", c.label, s.show(c.path), s.clean(err.Error()))
			continue
		}
		fmt.Fprintf(w, "%-48s %s -> %s\n", c.label, s.show(c.path), access)
	}
	return nil
}
```

## Step 3 — Survive a leaked DEMO_HOME

The usual way a dev build reaches prod data is a terminal opened by the installed tool that still exports `DEMO_HOME=~/.demo`. In a non-release profile appdir ignores an `<APP>_HOME` or `<APP>_<KIND>_DIR` that points inside the prod home, with warning `env-points-to-prod` ([Child processes and env leaks](child-processes.md)). If the tool still builds the prod path by hand, `CheckDB` refuses it.

<!-- snippet: examples/prod-db-guard/main.go#leak -->
```go
// envLeak starts a dev build in a shell opened by the installed tool, which
// exported DEMO_HOME=~/.demo. The value points into the prod home, so the
// dev build ignores it (with a warning) and keeps ~/.demo-dev; and if the
// tool still tries the prod db, CheckDB refuses it.
func envLeak(w io.Writer, s sandbox, prodGlobal string) error {
	d, err := s.open(fakeEnv{envHome: "~/.demo"}, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	for _, warn := range d.Warnings() {
		fmt.Fprintf(w, "warning %s: %s\n", warn.Code, s.clean(warn.Message))
	}
	fmt.Fprintf(w, "global home: %s\n", s.show(d.Global().Base))
	if _, err := openDB(d, prodGlobal); err != nil {
		fmt.Fprintf(w, "open %s: %s\n", s.show(prodGlobal), s.clean(err.Error()))
	}
	return nil
}
```

## Step 4 — Follow a db: pointer

A `db:` key in the project's `config.local.jsonc` moves the prod project database ([Pointer keys](../pointers/pointer-keys.md)). The prod view follows it, so the dev build is refused on the target even before the file exists. The dev build itself ignores `db:` (warning `pointer-ignored-for-profile`) and keeps its database under `profiles/dev/`.

<!-- snippet: examples/prod-db-guard/main.go#pointer -->
```go
// pointer moves the prod project db elsewhere with a db: key in the
// project's config.local.jsonc. The prod view CheckDB builds follows the
// pointer, so a dev build refuses the target even before the file exists.
// The dev build itself ignores db: and keeps its database under profiles/.
func pointer(w io.Writer, s sandbox) error {
	target := filepath.Join(s.root, sharedDir, sharedDB)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create db folder: %w", err)
	}
	cfg := fmt.Sprintf("{ \"db\": %q }\n", filepath.ToSlash(target))
	if err := os.WriteFile(filepath.Join(s.project, homeFolder, localConfig), []byte(cfg), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", localConfig, err)
	}
	dev, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	for _, warn := range dev.Warnings() {
		fmt.Fprintf(w, "warning %s: %s\n", warn.Code, s.clean(warn.Message))
	}
	own, err := dev.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	for _, p := range []string{target, own} {
		access, err := openDB(dev, p)
		if err != nil {
			fmt.Fprintf(w, "dev build, %s -> %s\n", s.show(p), s.clean(err.Error()))
			continue
		}
		fmt.Fprintf(w, "dev build, %s -> %s\n", s.show(p), access)
	}
	return nil
}
```

## Step 5 — Pass a relative path, or none of the folders yet

Two cases look safe and are not. A relative name is joined to the folder `New` was given (`WithWorkDir`, or the process's folder when you passed none), not re-read from the process at check time, so a tool that opens `.demo/demo.db` from the project is refused exactly as if it had passed the absolute path. And on a machine where the installed tool never ran, `~/.demo` does not exist: there is no file or folder to compare by identity, so `CheckDB` compares the real path and still refuses. Without that, the first dev run on a fresh machine would create the prod database and migrate it to the dev schema, and the release would later open a database it cannot read.

The example runs the fresh-home case first, before the release run creates anything:

<!-- snippet: examples/prod-db-guard/main.go#fresh -->
```go
// freshHome runs a dev build on a machine where the installed tool never ran,
// so ~/.demo does not exist yet. The prod db path has no file and no folder
// to compare by identity; CheckDB compares it by real path and still refuses,
// so a dev build cannot create and migrate the prod db in a fresh home.
func freshHome(w io.Writer, s sandbox) error {
	prodHome := filepath.Join(s.home, homeFolder)
	if _, err := os.Stat(prodHome); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("want no %s yet, stat says %v", s.show(prodHome), err)
	}
	dev, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	prodDB := filepath.Join(prodHome, appName+dbExt)
	fmt.Fprintf(w, "dev build, %s (folder missing) -> %s\n", s.show(prodDB), s.verdict(openDB(dev, prodDB)))
	return nil
}
```

Then, after the prod databases exist, a relative name from the project:

<!-- snippet: examples/prod-db-guard/main.go#relative -->
```go
// relativePath passes CheckDB a relative name. It is joined to the folder New
// was given (WithWorkDir, here the project), not to the process's current
// folder, so the guard checks the same file the tool will open from there.
func relativePath(w io.Writer, s sandbox) error {
	dev, err := s.open(nil, appdir.WithRelease(false))
	if err != nil {
		return err
	}
	rel := filepath.Join(homeFolder, appName+dbExt) // .demo/demo.db, the prod project db
	fmt.Fprintf(w, "dev build, %s from %s -> %s\n", filepath.ToSlash(rel), s.show(s.project), s.verdict(openDB(dev, rel)))
	return nil
}
```

Output:

```text
== fresh machine: no prod home yet
dev build, home/.demo/demo.db (folder missing) -> dev build refuses the prod database home/.demo/demo.db; set DEMO_ALLOW_PROD=1 to open it read-only
== relative path
dev build, .demo/demo.db from home/proj -> dev build refuses the prod database home/proj/.demo/demo.db; set DEMO_ALLOW_PROD=1 to open it read-only
```

## Try it

```sh
go run ./examples/prod-db-guard
```

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

The example prints paths relative to its temp root; a real run prints absolute paths.

## Pitfalls

- `AccessReadOnly` is a promise you keep: open the database read-only and never run migrations. appdir cannot enforce it.
- The guard is about the build, not the profile. A dev build run with `--profile prod` resolves the prod paths and is still refused on them; a release build run with `--profile dev` is never refused.
- Call `CheckDB` with the exact path you will open. It knows only the prod paths appdir resolves (the global db and the project db or its `db:` target); a copy of the prod database somewhere else is not refused.
- A relative path is resolved against `WithWorkDir`, not against the folder the process is in when it calls `CheckDB`. If your tool changes folder after `New` (`os.Chdir`) and then opens a relative name, pass `CheckDB` the absolute path you will really open, or resolve `Dirs` again.
- Do not skip `CheckDB` because the prod home is missing ("nothing to protect yet"): that first dev run is the one that would create the prod database with the dev schema. The guard refuses it (Step 5).
- The prod view is built without `--home` and without any `DEMO_*` variable. A dev build run with `--home /tmp/x` is still refused on `~/.demo/demo.db`, and a prod database that only exists because the installed tool runs with `DEMO_HOME` set elsewhere is not known to the guard.
- `CheckDB` also fails with `ErrSymlinkEscape` when the path is a symlink that leads outside its home ([Files and permissions](../layout/files-and-permissions.md)).

## Related

- [Child processes and env leaks](child-processes.md): the env-leak filter.
- [Errors](../reference/errors.md): `ErrProdDB` and its exit code.
- Example: [examples/prod-db-guard](../../examples/prod-db-guard).
- Design: [SPEC 8.4 Prod-db guard](../SPEC.md#84-prod-db-guard).
