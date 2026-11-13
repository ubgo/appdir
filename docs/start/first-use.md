# First use: resolve, read, ensure

appdir answers one question for a command-line tool: where do my files live? You call `appdir.New` once at start-up; it finds the global home (`~/.<app>`) and the project home (`.<app>/` in the project), and every later call asks it for a path. Reading paths never creates anything; `Ensure` creates a folder the first time you need it. Start here when you add appdir to a tool.

## Before you start

- Go 1.24 or newer, and `go get github.com/ubgo/appdir`. The module uses the standard library only.
- Import it as `"github.com/ubgo/appdir"`.
- The example uses a project home that `InitProject` made; [Create a home with init](init.md) shows that call.
- The example passes `appdir.WithRelease(true)` so the profile is prod and folders have their plain names. A `go run` build is a dev build, which would use `~/.lore-dev` and `.lore/profiles/dev/`; see [Release detection](../profiles/release-detection.md).

## Step 1 — Pick an app name

The app name becomes the home folder (`.lore`), the env prefix (`LORE_`) and the db file (`lore.db`), so `New` accepts only names matching `^[a-z0-9][a-z0-9._-]*$` and returns `ErrInvalidOption` for anything else.

<!-- snippet: examples/quickstart/main.go#name -->
```go
// checkName shows the one thing New checks about the app name: it must match
// ^[a-z0-9][a-z0-9._-]*$, because it becomes a folder and env names.
func checkName(w io.Writer) error {
	_, err := appdir.New(badAppName)
	if !errors.Is(err, appdir.ErrInvalidOption) {
		return fmt.Errorf("want ErrInvalidOption for %q, got %v", badAppName, err)
	}
	fmt.Fprintln(w, "New:", err)
	return nil
}
```

## Step 2 — Resolve with New

`New` reads the environment, the user's home folder and the working folder, then works out every home and path. It creates nothing and fails only for a bad option, an unknown profile or an unreadable working folder ([New](ds:block?id=new-mes9nm7s)); a missing project home is reported later, by the calls that need one. A real tool calls `appdir.New("lore", appdir.WithHome(flagHome), appdir.WithProfile(flagProfile))` with its flags and lets the seams default to the real machine. The example replaces the three outside inputs with seams so it never touches your real home:

<!-- snippet: examples/quickstart/main.go#open -->
```go
// open resolves every home for the app. The three seams keep the example
// away from the real machine: the user's home, the working folder and the
// environment all come from the caller. WithRelease(true) keeps the prod
// profile, so folders have their plain names (.lore, not .lore-dev).
func open(home, workDir string) (*appdir.Dirs, error) {
	d, err := appdir.New(appName,
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(workDir),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(true),
	)
	if err != nil {
		return nil, fmt.Errorf("appdir: %w", err)
	}
	return d, nil
}
```

`d.App()` returns the name given to `New` (`lore`), so messages and help text can name the tool without a second constant; the example prints it as the first line after `New`. `Global()` returns the global home, and `Project()` returns the project home or an error wrapping `ErrNoProject`. Both are covered in [Global and project homes](../homes/global-and-project.md).

## Step 3 — Read paths with Path and PathIn

`Path(kind)` returns a kind's path in its effective home: the db lives in the project home, the cache in the global home, and a kind kept in both homes (`config`) comes from the project when there is one ([Path](ds:block?id=dirs-path-asygw3vm)). `PathIn(kind, scope)` asks for one named home, `appdir.ScopeGlobal` or `appdir.ScopeProject`. A file kind's path is the file itself (`lore.db`), not its folder.

<!-- snippet: examples/quickstart/main.go#read -->
```go
// readPaths asks for paths. Path picks the kind's effective home (the db
// lives in the project, the cache is global); PathIn names the home.
func readPaths(w io.Writer, root string, d *appdir.Dirs) error {
	for _, k := range []appdir.Kind{appdir.KindDB, appdir.KindCache, appdir.KindConfig} {
		p, err := d.Path(k)
		if err != nil {
			return fmt.Errorf("path %s: %w", k, err)
		}
		fmt.Fprintf(w, "Path(%s): %s\n", k, show(root, p))
	}
	for _, s := range []appdir.Scope{appdir.ScopeGlobal, appdir.ScopeProject} {
		p, err := d.PathIn(appdir.KindConfig, s)
		if err != nil {
			return fmt.Errorf("path in %s: %w", s, err)
		}
		fmt.Fprintf(w, "PathIn(config, %s): %s\n", s, show(root, p))
	}
	return nil
}
```

## Step 4 — List every path with Paths

`Paths()` returns every kind as an `Entry` with its home, path, type, git rule and the rule that picked it. It is what a `paths --json` or `doctor` command prints ([Paths JSON](../reference/paths-json.md)). Kinds kept in both homes appear once per home.

<!-- snippet: examples/quickstart/main.go#paths -->
```go
// listPaths prints every kind with the home and the rule that picked it.
// Kinds kept in both homes (home.jsonc, the config files) appear twice.
func listPaths(w io.Writer, root string, d *appdir.Dirs) {
	for _, e := range d.Paths() {
		fmt.Fprintf(w, "%-13s %-8s %-8s %s\n", e.Kind, e.Home, e.Rule, show(root, e.Path))
	}
}
```

## Step 5 — Create folders with Ensure

`Ensure(kind)` is the lazy creation point: it creates a folder kind's folder, or a file kind's parent folder, with mode `0700`, and returns the path ([Ensure](ds:block?id=dirs-ensure-mxbz3fmd)). Call it right before you write. For the db it returns `…/lore.db`; opening the file is your job. A cache folder also gets a `CACHEDIR.TAG` ([Files and permissions](../layout/files-and-permissions.md)).

<!-- snippet: examples/quickstart/main.go#ensure -->
```go
// ensure creates folders on first use. For a folder kind it creates the
// folder; for a file kind (the db) it creates the parent folder only and
// returns the file path for the caller to open.
func ensure(w io.Writer, root string, d *appdir.Dirs) error {
	for _, k := range []appdir.Kind{appdir.KindCache, appdir.KindLogs, appdir.KindDB} {
		p, err := d.Ensure(k)
		if err != nil {
			return fmt.Errorf("ensure %s: %w", k, err)
		}
		fmt.Fprintf(w, "Ensure(%s): %s\n", k, show(root, p))
	}
	fmt.Fprintf(w, "global home exists: %v\n", exists(d.Global().Base))
	return nil
}
```

## Try it

```sh
go run ./examples/quickstart
```

Paths are printed relative to the example's temp root (`home/` is the fake user home):

```text
== app name rule
New: appdir: app name "My App" must match ^[a-z0-9][a-z0-9._-]*$
== New: resolve, create nothing
app:          lore
global home:  home/.lore (rule global, fallback false, info false)
has project:  true
project home: home/projects/shop/.lore (rule project, data home/projects/shop/.lore)
project info: app=lore layout=1 scope=project
== Path and PathIn
Path(db): home/projects/shop/.lore/lore.db
Path(cache): home/.lore/cache
Path(config): home/projects/shop/.lore/config.jsonc
PathIn(config, global): home/.lore/config.jsonc
PathIn(config, project): home/projects/shop/.lore/config.jsonc
== Paths (what paths --json lists)
home          global   global   home/.lore/home.jsonc
home          project  project  home/projects/shop/.lore/home.jsonc
config        global   global   home/.lore/config.jsonc
config        project  project  home/projects/shop/.lore/config.jsonc
config-local  global   global   home/.lore/config.local.jsonc
config-local  project  project  home/projects/shop/.lore/config.local.jsonc
gitignore     project  project  home/projects/shop/.lore/.gitignore
shared        project  project  home/projects/shop/.lore/shared
db            project  project  home/projects/shop/.lore/lore.db
backups       project  project  home/projects/shop/.lore/backups
cache         global   global   home/.lore/cache
run           project  project  home/projects/shop/.lore/run
logs          project  project  home/projects/shop/.lore/logs
== reads never create
global home exists: false
== Ensure: create on first use
Ensure(cache): home/.lore/cache
Ensure(logs): home/projects/shop/.lore/logs
Ensure(db): home/projects/shop/.lore/lore.db
global home exists: true
== global home per build
release=true  profile prod global home home/.lore (rule global)
release=false profile dev  global home home/.lore-dev (rule global)
== which home a kind uses, outside a project
project db     error: no .lore/ here; run 'lore init' or pass --home
project cache  global  fallback=false home/.lore/cache
project logs   global  fallback=true  home/.lore/logs
global  db     global  fallback=false home/.lore/lore.db
global  cache  global  fallback=false home/.lore/cache
global  logs   global  fallback=false home/.lore/logs
```

The global home did not exist after `New`, `Path` and `Paths`; the first `Ensure(cache)` created it. The last two blocks (the global home per build, and which home a kind uses outside a project) belong to [Global and project homes](../homes/global-and-project.md).

## Pitfalls

- **`New` succeeding does not mean there is a project.** A missing project home is stored and returned by `Path`, `PathIn` and `Ensure` for project kinds (`no .lore/ here; run 'lore init' or pass --home`). Check `HasProject()` or the error of the call you need; commands that need only the cache still work outside a project.
- **Reads never create, so a path from `Path` may not exist yet.** Call `Ensure` before writing, not `os.MkdirAll` on your own: `Ensure` applies the `0700` mode, the root guard, the layout-version guard and the temp fallback ([Fallbacks](../homes/fallbacks.md)).
- **`Ensure` on a file kind creates only the folder.** It returns the file path; the file appears when your code writes it.
- **`Path` for a both-homes kind hides the global copy.** `Path(config)` returns the project file inside a project. To read both config files in order, use `ConfigFiles()` ([Config files](../layout/config-files.md)), not `Path`.
- **An unknown kind is an error, not an empty path.** `Path("nope")` returns `ErrUnknownKind`; kinds you add with `WithKind` are known ([Kinds](../layout/kinds.md)).
- **A dev build uses other folders.** Without `WithRelease`, `go run` and `go build` builds are dev builds and use `~/.lore-dev` and `.lore/profiles/dev/` ([Profiles](../profiles/profiles.md)).

## Related

- [Create a home with init](init.md)
- [Global and project homes](../homes/global-and-project.md)
- [How the project home is found](../homes/lookup.md)
- [Options reference](../reference/options.md) and [Errors reference](../reference/errors.md)
- Example: [examples/quickstart](../../examples/quickstart/README.md)
- SPEC: [3.1 Constructor](../SPEC.md#31-constructor), [3.4 Methods](../SPEC.md#34-methods), [4.7 Per-kind result](../SPEC.md#47-per-kind-result)
