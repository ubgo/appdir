# Global and project homes

Every tool built on appdir has two homes. The global home `~/.<app>` holds what belongs to the user on this machine (the cache, user settings, a global tool's data). The project home `.<app>/` sits in a project folder and holds what belongs to that project (its database, its committed settings). `New` resolves both at once, and every kind of file lives in one of them, or in both. Read this page to know which home a path comes from and how to inspect both.

## Before you start

- [First use](../start/first-use.md): `New`, `Path`, `Ensure`.
- A project home made with `InitProject` ([Create a home with init](../start/init.md)).
- Import `"github.com/ubgo/appdir"`.

## Step 1 — Know where the global home is

The global home is `~/.<app>` with rule `global`: `~/.lore` for the release profile and `~/.lore-dev` for a dev build ([Names](../profiles/names.md)). appdir does not read `XDG_CONFIG_HOME`, `XDG_DATA_HOME` or `XDG_STATE_HOME`: one folder per tool is easier to find, back up and delete than three. `--home` and `<APP>_HOME` replace it ([Overrides](overrides.md)), and with no user home it is unset ([Fallbacks](fallbacks.md)). The order is in [resolveHomes](ds:block?id=dirs-resolvehomes-2zu5rbrz).

The example resolves the same folder as a release build and as a dev build. You never build this path yourself: read it from `Global().Base`, or better, ask `Path(kind)` for the file you need.

<!-- snippet: examples/quickstart/main.go#global -->
```go
// globalHome shows where the global home is for each kind of build: a
// release build uses ~/.lore, a dev build (go run, go build without the
// tool's release flag) uses ~/.lore-dev, so trying a change never touches
// the installed tool's data.
func globalHome(w io.Writer, root, home, workDir string) error {
	for _, release := range []bool{true, false} {
		d, err := appdir.New(appName,
			appdir.WithHomeDir(func() (string, error) { return home, nil }),
			appdir.WithWorkDir(workDir),
			appdir.WithEnv(func(string) (string, bool) { return "", false }),
			appdir.WithRelease(release),
		)
		if err != nil {
			return fmt.Errorf("appdir: %w", err)
		}
		g := d.Global()
		fmt.Fprintf(w, "release=%-5v profile %-4s global home %s (rule %s)\n", release, d.Profile(), show(root, g.Base), g.Rule)
	}
	return nil
}
```

## Step 2 — Inspect both homes

`Global()` returns the global `Home`. `Project()` returns the project `Home`, or an error wrapping `ErrNoProject` when there is none, and `HasProject()` says which. A `Home` has:

- `Base`: the folder holding `home.jsonc` and the config files.
- `Data`: the folder holding every other kind. It equals `Base` for the release profile; for a dev build the project's `Data` is `.lore/profiles/dev/`, so dev data never mixes with prod data ([Profiles](../profiles/profiles.md)).
- `Rule`: what picked it (`global`, `project`, `worktree`, `pointer`, `flag`, `env-home`, `defaults`).
- `Fallback` and `Note`: set when a fallback was used, with a reason.
- `Marker`: the `.lore/` that was found (project only; it differs from `Base` under a `home:` pointer).
- `Info`: the parsed `home.jsonc`, or nil when the folder has none yet.

<!-- snippet: examples/quickstart/main.go#homes -->
```go
// printHomes shows both homes at once. Base holds home.jsonc and the config
// files, Data every other kind (they differ only for a non-release profile).
// Info is the parsed home.jsonc, nil when the folder has none yet. App is
// the name New was given, for messages that name the tool.
func printHomes(w io.Writer, root string, d *appdir.Dirs) error {
	fmt.Fprintf(w, "app:          %s\n", d.App())
	g := d.Global()
	fmt.Fprintf(w, "global home:  %s (rule %s, fallback %v, info %v)\n", show(root, g.Base), g.Rule, g.Fallback, g.Info != nil)
	fmt.Fprintf(w, "has project:  %v\n", d.HasProject())
	p, err := d.Project()
	if err != nil {
		return fmt.Errorf("project home: %w", err)
	}
	fmt.Fprintf(w, "project home: %s (rule %s, data %s)\n", show(root, p.Base), p.Rule, show(root, p.Data))
	fmt.Fprintf(w, "project info: app=%s layout=%d scope=%s\n", p.Info.App, p.Info.Layout, p.Info.Scope)
	return nil
}
```

## Step 3 — Know which home a kind uses

Each kind has a scope ([standard kinds](ds:block?id=standardkinds-ptkt92ta)):

| Scope | Kinds by default | Home used by `Path` |
|---|---|---|
| `both` | `home`, `config`, `config-local` | project if there is one, else global; `Paths` lists both |
| `global` | `cache` | always the global home |
| data scope (`project` in `ModeProject`) | `shared`, `db`, `backups`, `run` | the project home, or `ErrNoProject` |
| `project-else-global` | `logs` (in `ModeProject`) | the project home, else the global home with `Fallback: true` |
| `project` | `gitignore` | the project home only |

The mode sets the data scope. `ModeProject` (the default) puts data in the project home and fails loudly outside a project. A tool that works the same everywhere, such as a personal launcher, passes `WithMode(appdir.ModeGlobal)` and keeps its data in `~/.<app>`. Kinds, scopes, `WithMode` and `Scoped` are covered in [Kinds](../layout/kinds.md).

The example runs from the user's home, where there is no project home, once per mode. `Paths()` reports each kind's home, its `Fallback` flag and, for a kind that cannot resolve, its error in `Entry.Error` (`Paths` itself never fails):

<!-- snippet: examples/quickstart/main.go#scopes -->
```go
// kindHomes runs from the user's home, where there is no project home, and
// prints the home each kind lands in. In ModeProject (the default) the db is
// project data and fails with ErrNoProject, the logs fall back to the global
// home, and the cache is global anyway. In ModeGlobal every data kind lives
// in the global home, so nothing fails.
func kindHomes(w io.Writer, root, home string) error {
	for _, mode := range []appdir.Mode{appdir.ModeProject, appdir.ModeGlobal} {
		d, err := appdir.New(appName,
			appdir.WithHomeDir(func() (string, error) { return home, nil }),
			appdir.WithWorkDir(home), // no .lore/ here or above
			appdir.WithEnv(func(string) (string, bool) { return "", false }),
			appdir.WithRelease(true),
			appdir.WithMode(mode),
		)
		if err != nil {
			return fmt.Errorf("appdir: %w", err)
		}
		for _, e := range d.Paths() {
			switch e.Kind {
			case appdir.KindDB, appdir.KindLogs, appdir.KindCache:
			default:
				continue // the three kinds that show the three outcomes
			}
			if e.Error != "" {
				fmt.Fprintf(w, "%-7s %-6s error: %s\n", mode, e.Kind, e.Error)
				continue
			}
			fmt.Fprintf(w, "%-7s %-6s %-7s fallback=%-5v %s\n", mode, e.Kind, e.Home, e.Fallback, show(root, e.Path))
		}
	}
	return nil
}
```

It prints:

```text
project db     error: no .lore/ here; run 'lore init' or pass --home
project cache  global  fallback=false home/.lore/cache
project logs   global  fallback=true  home/.lore/logs
global  db     global  fallback=false home/.lore/lore.db
global  cache  global  fallback=false home/.lore/cache
global  logs   global  fallback=false home/.lore/logs
```

In `ModeProject` the db fails with `ErrNoProject`, the logs land in the global home with `fallback=true`, and the cache is global anyway. In `ModeGlobal` nothing depends on a project.

## Step 4 — Rename the home folder if you must

`WithDirName(name)` changes the folder name of both homes; the default is `.` plus the app name. The env names do not change with it. The overrides example shows it next to `WithEnvPrefix`:

<!-- snippet: examples/overrides/main.go#names -->
```go
// names prints the variable names a tool shows in help and doctor output,
// for the defaults and with WithEnvPrefix / WithDirName, and a dir name
// that is refused.
func (s setup) names(w io.Writer) error {
	d, err := s.open(nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "EnvPrefix: %s  EnvName(HOME): %s\n", d.EnvPrefix(), d.EnvName("HOME"))
	fmt.Fprintf(w, "KindEnv(cache): %s  KindEnv(config): %q (anchored)\n", d.KindEnv(appdir.KindCache), d.KindEnv(appdir.KindConfig))
	d, err = s.open(nil, appdir.WithEnvPrefix(prefix), appdir.WithDirName(dirName))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "with prefix %s: KindEnv(cache): %s\n", prefix, d.KindEnv(appdir.KindCache))
	fmt.Fprintf(w, "with dir name %s: global home %s\n", dirName, s.label(d.Global().Base))
	// The dir name must be one plain folder name: "..", "." or a path with a
	// separator would put the home outside the user's home.
	if _, err := s.open(nil, appdir.WithDirName(badDirName)); errors.Is(err, appdir.ErrInvalidOption) {
		// open adds its own "appdir:" prefix; print the library's message.
		fmt.Fprintf(w, "with dir name %s: error: %v\n", badDirName, errors.Unwrap(err))
	} else {
		return fmt.Errorf("dir name %q: want ErrInvalidOption, got %v", badDirName, err)
	}
	return nil
}
```

## Try it

```sh
go run ./examples/quickstart
```

The first lines show both homes (paths relative to the temp root):

```text
== New: resolve, create nothing
app:          lore
global home:  home/.lore (rule global, fallback false, info false)
has project:  true
project home: home/projects/shop/.lore (rule project, data home/projects/shop/.lore)
project info: app=lore layout=1 scope=project
```

Further down, the `== global home per build` block is Step 1 (Step 3's block is quoted above):

```text
== global home per build
release=true  profile prod global home home/.lore (rule global)
release=false profile dev  global home home/.lore-dev (rule global)
```

The global home has no `Info` because it does not exist yet: `New` creates nothing. `go run ./examples/overrides` prints `with dir name .lore-data: global home home/.lore-data` for Step 4.

## Pitfalls

- **`Global()` can have an empty `Base`.** With no user home and no override its rule is `defaults` and both paths are empty. Check `Rule` before joining paths to it ([Fallbacks](fallbacks.md)).
- **The project's `Base` and `Data` differ in a dev build.** Join kind paths to `Data`, or better, ask `Path(kind)`; joining to `Base` puts dev data next to prod data.
- **`Path` hides the global copy of a both-homes kind inside a project.** Use `PathIn(kind, appdir.ScopeGlobal)`, or `ConfigFiles()` for the config read order ([Config files](../layout/config-files.md)).
- **`WithDirName` takes one plain folder name.** An empty name, `.`, `..`, or a name with `/`, `\` or NUL makes `New` fail with `ErrInvalidOption` (`appdir: dir name ".." must be one plain path segment`), so a home can never land above the user's home. If the name comes from user input, show that error as is; `go run ./examples/overrides` prints it.
- **The global home is never a project home.** Running inside the user's home does not turn `~/.lore` into a project ([Lookup](lookup.md)).

## Related

- [How the project home is found](lookup.md)
- [Overrides: --home and env](overrides.md)
- [Fallbacks](fallbacks.md)
- [Kinds and scopes](../layout/kinds.md)
- Examples: [examples/quickstart](../../examples/quickstart/README.md), [examples/overrides](../../examples/overrides/README.md)
- SPEC: [4.3 Global home](../SPEC.md#43-global-home), [4.6 Profile folders](../SPEC.md#46-profile-folders), [5.2 Mode and scope](../SPEC.md#52-mode-and-scope)
