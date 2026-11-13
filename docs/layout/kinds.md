# Kinds: the layout of a home

A kind is one named entry in a home: a file or a folder, with a path, a git rule and a scope (which home it lives in). appdir ships a standard set of kinds, and a tool changes that set with options of `New`: rename one, add its own, drop one it does not use. You want this page when your tool keeps something the standard does not list (templates, identity, thumbnails) or names a standard folder differently.

## Before you start

- Read [Global and project homes](../homes/global-and-project.md): a scope picks one of the two homes.
- Import `github.com/ubgo/appdir`. Kinds are typed: `appdir.Kind`, with constants for the standard ones (`appdir.KindDB`, …). Give your own kinds named constants too.
- The standard kinds ([standardKinds](ds:block?id=standardkinds-ptkt92ta)):

| Kind | Path | Type | Git | Scope | Flags |
|---|---|---|---|---|---|
| `home` | `home.jsonc` | file | commit | both | anchored |
| `config` | `config.jsonc` | file | commit | both | anchored |
| `config-local` | `config.local.jsonc` | file | ignore | both | anchored |
| `gitignore` | `.gitignore` | file | commit | project | anchored |
| `shared` | `shared/` | folder | commit | data scope | |
| `db` | `<app>.db` | file | ignore | data scope | |
| `backups` | `backups/` | folder | ignore | data scope | |
| `cache` | `cache/` | folder | ignore | global | disposable |
| `run` | `run/` | folder | ignore | data scope | |
| `logs` | `logs/` | folder | ignore | project-else-global in project mode, global in global mode | |

## Step 1 — Declare the layout

Every change is an option, applied in order by `New` ([buildKinds](ds:block?id=buildkinds-r6jj6d5s)):

- `WithKind(name, opts...)` on a new name adds a kind that starts as `Folder(name)`, `Ignore()`, in the data scope, with its own `<APP>_<KIND>_DIR`.
- `WithKind` on an existing name changes it in place. `Folder("data")` on `shared` is a rename: new path, same kind name, same git rule, same scope.
- `WithoutKind(name)` drops a kind from `Kinds()`, `Paths()`, the gitignore block and `Ensure` (which then returns `ErrUnknownKind`).
- Kind options: `Folder(p)` and `File(p)` set the type and path; `Commit()` and `Ignore()` set the git rule; `Scoped(s)` sets the home; `Disposable()` marks it safe to delete (it gets `CACHEDIR.TAG` and may fall back to the temp folder); `Anchored()` keeps it in the home root in every profile and gives it no env override.

<!-- snippet: examples/custom-layout/main.go#layout -->
```go
// layout is the tool's own layout: every change is an option of New, so
// paths --json, the gitignore block and Ensure all follow it.
func layout() []appdir.Option {
	return []appdir.Option{
		// Rename: shared/ becomes data/. Same kind, new path; it stays
		// committed and in the data scope.
		appdir.WithKind(appdir.KindShared, appdir.Folder("data")),
		// A new committed folder in the data scope.
		appdir.WithKind(kindTemplates, appdir.Commit()),
		// Login data belongs to the user, not to a project.
		appdir.WithKind(kindIdentity, appdir.File("identity.json"), appdir.Scoped(appdir.ScopeGlobal)),
		// A file next to home.jsonc in both profiles, with no env override.
		appdir.WithKind(kindSchema, appdir.File("schema.jsonc"), appdir.Commit(), appdir.Anchored()),
		// Safe to delete: gets CACHEDIR.TAG when Ensure creates it.
		appdir.WithKind(kindThumbs, appdir.Disposable()),
		// This tool keeps no backups.
		appdir.WithoutKind(appdir.KindBackups),
		// Commit prompt files, but never data/private/.
		appdir.WithGitCommit("prompts/*.md"),
		appdir.WithGitIgnore("data/private/"),
	}
}
```

`WithGitCommit` and `WithGitIgnore` are covered in [The .gitignore block](gitignore.md).

## Step 2 — Read the layout in effect

`Kinds()` returns the layout after every edit, in a stable order: the standard kinds first (minus the dropped ones), then new kinds in the order they were added. `KindEnv(kind)` gives the `<APP>_<KIND>_DIR` name (kind name upper-cased, `-` turned into `_`), or `""` for an anchored kind. A tool's `paths --json` and `doctor` print these (see [paths --json and doctor](../reference/paths-json.md)).

<!-- snippet: examples/custom-layout/main.go#kinds -->
```go
// showKinds prints the layout in effect, in the order Kinds returns it.
func (ex *example) showKinds() error {
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	fmt.Fprintf(ex.w, "== kinds (mode %s)\n", d.Mode())
	for _, k := range d.Kinds() {
		var flags []string
		if k.Anchored {
			flags = append(flags, "anchored")
		}
		if k.Disposable {
			flags = append(flags, "disposable")
		}
		env := d.KindEnv(k.Name)
		if env == "" {
			env = "-"
		}
		line := fmt.Sprintf("%-12s %-18s %-6s %-6s %-19s %-18s %s",
			k.Name, k.Path, k.Type, k.Git, k.Scope, env, strings.Join(flags, ","))
		fmt.Fprintln(ex.w, strings.TrimRight(line, " "))
	}
	return nil
}
```

## Step 3 — Pick the mode

The mode sets the data scope, the scope every kind without an explicit one gets: `project` for `ModeProject` (the default), `global` for `ModeGlobal`. `Mode()` reports it. `ModeProject` is the default because it fails safe: a global tool that forgets `WithMode(ModeGlobal)` gets a loud `ErrNoProject` outside a project, while the opposite mistake would quietly write project data into `~/.<app>`. In project mode, `logs` is `project-else-global`: the project's `logs/` when there is a project home, the global one otherwise.

<!-- snippet: examples/custom-layout/main.go#modes -->
```go
// showModes compares the data scope of the two modes. A global tool passes
// WithMode(ModeGlobal); the default ModeProject fails safe outside projects.
func (ex *example) showModes() error {
	fmt.Fprintln(ex.w, "\n== mode and scope")
	for _, m := range []appdir.Mode{appdir.ModeProject, appdir.ModeGlobal} {
		d, err := ex.newDirs(appdir.WithMode(m))
		if err != nil {
			return err
		}
		scopes := map[appdir.Kind]appdir.Scope{}
		for _, k := range d.Kinds() {
			scopes[k.Name] = k.Scope
		}
		fmt.Fprintf(ex.w, "%-7s db=%s logs=%s cache=%s\n",
			d.Mode(), scopes[appdir.KindDB], scopes[appdir.KindLogs], scopes[appdir.KindCache])
	}
	return nil
}
```

The four scopes:

| Scope | Lives in |
|---|---|
| `global` | the global home `~/.<app>` |
| `project` | the project home; `ErrNoProject` when there is none |
| `project-else-global` | the project home when there is one, else the global home (the entry is marked `Fallback`) |
| `both` | one copy per home; `Path` gives the project one when there is a project, `PathIn` picks |

## Step 4 — Invalid names and paths

Kind names must match `^[a-z][a-z0-9-]*$`. Paths must be relative, slash-separated, clean, with no `.`, `..` or empty segment, no `\` and no `:`. Anything else makes `New` fail with `ErrInvalidOption`, so a bad layout is caught at start-up, not on first write.

<!-- snippet: examples/custom-layout/main.go#bad-kind -->
```go
// badKind shows that a kind path may not leave the home.
func (ex *example) badKind() error {
	fmt.Fprintln(ex.w, "\n== invalid kind")
	_, err := ex.newDirs(appdir.WithKind("escape", appdir.Folder("../outside")))
	fmt.Fprintf(ex.w, "error: %v\n", err)
	fmt.Fprintf(ex.w, "is ErrInvalidOption: %t\n", errors.Is(err, appdir.ErrInvalidOption))
	return nil
}
```

## Step 5 — Flip a git rule, and ask for a kind that is gone

`Ignore()` is the opposite of `Commit()`: on a standard committed kind it takes the kind out of git, and the gitignore block stops re-including its folder. A kind removed with `WithoutKind` is unknown from then on: `Path`, `PathIn`, `Ensure` and the other calls that take a kind return `ErrUnknownKind` (`appdir: unknown kind "<name>"`) instead of guessing a path, so a typo or a call left over from an older layout fails loudly.

<!-- snippet: examples/custom-layout/main.go#kind-edits -->
```go
// kindEdits turns the committed shared kind into an ignored one with
// Ignore(), then asks for backups, which layout() dropped: every call that
// takes a kind answers ErrUnknownKind instead of guessing a path.
func (ex *example) kindEdits() error {
	fmt.Fprintln(ex.w, "\n== Ignore() and unknown kinds")
	d, err := ex.newDirs(appdir.WithKind(appdir.KindShared, appdir.Ignore()))
	if err != nil {
		return err
	}
	spec, _ := kindSpec(d, appdir.KindShared)
	fmt.Fprintf(ex.w, "shared git rule: %s; block re-includes data/: %t\n",
		spec.Git, strings.Contains(d.GitignoreBlock(), "!data/\n"))
	_, err = d.Path(appdir.KindBackups)
	fmt.Fprintf(ex.w, "Path(backups): %v\n", err)
	_, err = d.Ensure(appdir.KindBackups)
	fmt.Fprintf(ex.w, "Ensure(backups): %v\n", err)
	fmt.Fprintf(ex.w, "is ErrUnknownKind: %t\n", errors.Is(err, appdir.ErrUnknownKind))
	return nil
}
```

## Step 6 — Drop the home file

`home.jsonc` is the `home` kind, and it can be dropped like any other. Without it, `InitProject` and `InitHome` create only the folder (`0700`) and write no `home.jsonc`; outside a git work tree they write no `.gitignore` either, so the new `.lore/` is empty. Lookup still finds the folder by name, but there is no `app` or `layout` to read: the layout version guard never fires, and a `home:` pointer target is recognised by its folder name instead: it must be named `.lore` (`../shop-private/.lore`, made with `InitHome`); any other folder fails with `ErrPointerNotHome`. Keep the `home` kind unless your tool needs neither the layout guard nor freely named pointer targets.

<!-- snippet: examples/custom-layout/main.go#no-home-file -->
```go
// noHomeFile drops the home kind: init then creates only the .lore folder
// and writes no home.jsonc (and, outside a git work tree, no .gitignore).
func (ex *example) noHomeFile() error {
	fmt.Fprintln(ex.w, "\n== WithoutKind(home) and init")
	d, err := ex.newDirs(appdir.WithoutKind(appdir.KindHome))
	if err != nil {
		return err
	}
	marker, err := d.InitProject(filepath.Join(ex.root, plainDir))
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	entries, err := os.ReadDir(marker)
	if err != nil {
		return fmt.Errorf("read %s: %w", marker, err)
	}
	fmt.Fprintf(ex.w, "init %s: folder %s, %d files, home.jsonc %s\n",
		ex.rel(marker), mode(marker), len(entries), mode(filepath.Join(marker, homeInfoFile)))
	return nil
}
```

## Try it

```sh
go run ./examples/custom-layout
```

The part of the output this page covers (the temp root is printed as `$TMP`):

```text
== kinds (mode project)
home         home.jsonc         file   commit both                -                  anchored
config       config.jsonc       file   commit both                -                  anchored
config-local config.local.jsonc file   ignore both                -                  anchored
gitignore    .gitignore         file   commit project             -                  anchored
shared       data               folder commit project             LORE_SHARED_DIR
db           lore.db            file   ignore project             LORE_DB_DIR
cache        cache              folder ignore global              LORE_CACHE_DIR     disposable
run          run                folder ignore project             LORE_RUN_DIR
logs         logs               folder ignore project-else-global LORE_LOGS_DIR
templates    templates          folder commit project             LORE_TEMPLATES_DIR
identity     identity.json      file   ignore global              LORE_IDENTITY_DIR
schema       schema.jsonc       file   commit project             -                  anchored
thumbs       thumbs             folder ignore project             LORE_THUMBS_DIR    disposable

== mode and scope
project db=project logs=project-else-global cache=global
global  db=global logs=global cache=global

== invalid kind
error: appdir: kind "escape": path "../outside" has an empty, . or .. segment
is ErrInvalidOption: true

== Ignore() and unknown kinds
shared git rule: ignore; block re-includes data/: false
Path(backups): appdir: unknown kind "backups"
Ensure(backups): appdir: unknown kind "backups"
is ErrUnknownKind: true

== WithoutKind(home) and init
init $TMP/plain/.lore: folder 0700, 0 files, home.jsonc missing
```

`backups` is gone (`WithoutKind`), `shared` now has the path `data` but keeps its name and its env variable `LORE_SHARED_DIR`.

## Pitfalls

- A rename changes the path, not the name: the env variable stays `LORE_SHARED_DIR` and code keeps calling `Path(appdir.KindShared)`. Users who read `data/` on disk may expect `LORE_DATA_DIR`; document it in your tool.
- In `ModeProject` the `run` kind has the project scope, so a machine-wide daemon run outside any project gets `ErrNoProject` from `RunPath` and `Socket`. A daemon that is not tied to a project either uses `WithMode(appdir.ModeGlobal)` or moves just that kind: `WithKind(appdir.KindRun, appdir.Scoped(appdir.ScopeGlobal))` ([Choosing by app type](../choosing/by-app-type.md)).
- Options apply in order. `WithoutKind(x)` followed by `WithKind(x)` adds `x` back as a brand-new kind (folder, ignored, data scope), not the standard one.
- Dropping an anchored kind changes the commands built on it: without `config` or `config-local`, `ConfigFiles()` leaves that file out of every home; without `home`, `InitProject` and `InitHome` only create the folder and write no `home.jsonc`, and a `home:` pointer target must be named `.<app>`. A `home` kind moved to a nested path (`File("meta/home.json")`) gets its parent folder created by init.
- A new kind is ignored by git unless you pass `Commit()`. A committed kind's files reach git, so never commit a kind that may hold secrets.
- `Anchored()` on a new kind keeps the data scope unless you also pass `Scoped`; the standard anchored kinds are `both`. An anchored kind has no `<APP>_<KIND>_DIR`, and `--home` / `<APP>_HOME` is the only way to move it.
- `Scoped(s)` takes only the four scopes above; any other value is `ErrInvalidOption` from `New`.
- The data kinds of a non-release profile live under `.<app>/profiles/<profile>/` in the project home; anchored kinds do not (see [Profiles](../profiles/profiles.md)).
- `Disposable()` also lets `Ensure` fall back to `<tmp>/<app>-<uid>/` when the folder cannot be created (no `$HOME`, read-only home). Do not mark a kind disposable if losing it loses work (see [Fallbacks](../homes/fallbacks.md)).

## Related

- [The .gitignore block](gitignore.md): how commit rules turn into `.gitignore` lines.
- [Files and permissions](files-and-permissions.md): what `Ensure` creates.
- [Overrides](../homes/overrides.md): `<APP>_<KIND>_DIR` and `--home`.
- [Options](../reference/options.md): every option with its default.
- Example: [examples/custom-layout](../../examples/custom-layout).
- SPEC: [5.1 Standard kinds](../SPEC.md#51-standard-kinds), [5.2 Mode and scope](../SPEC.md#52-mode-and-scope), [5.3 Author-declared kinds](../SPEC.md#53-author-declared-kinds).
