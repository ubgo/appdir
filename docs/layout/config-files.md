# Config files

appdir does not load settings; it tells your config loader which files to read and in which order. `ConfigFiles()` returns that list: the global home's files, then the project's, then a `home:` pointer target's, then the dev profile's own file, later winning per key. Each entry says which home it belongs to, whether it is the machine-local file and whether it is committed. You want this page when you wire appdir to a config loader, or replace the JSONC reader appdir uses for `home.jsonc` and pointer keys.

## Before you start

- Read [Global and project homes](../homes/global-and-project.md) and, for the pointer target files, [Pointer keys](../pointers/pointer-keys.md).
- Import `github.com/ubgo/appdir`, plus your config loader. The example below uses a ten-line toy loader that merges one key, `port`, so the order is visible.
- The read order ([ConfigFiles](ds:block?id=dirs-configfiles-nzbecu5q)):

| # | File | `Home` | `Local` | `Committed` | `Profile` |
|---|---|---|---|---|---|
| 1 | `~/.<app>/config.jsonc` | global | no | never | |
| 2 | `~/.<app>/config.local.jsonc` | global | yes | never | |
| 3 | `.<app>/config.jsonc` | project | no | inside a git work tree | |
| 4 | `.<app>/config.local.jsonc` | project | yes | never | |
| 5 | `<target>/config.jsonc` (with `home:`) | project | no | target inside a git work tree | |
| 6 | `<target>/config.local.jsonc` (with `home:`) | project | yes | never | |
| 7 | `<data>/config.local.jsonc` (non-release profile) | project | yes | never | the profile |

`<data>` is the project's data folder: `.<app>/profiles/<profile>/`, or `<target>/profiles/<profile>/` with a `home:` pointer. In a non-release profile the global home is the profile's own (`~/.<app>-dev`).

## Step 1 — List the files and merge them

Missing files are listed too, so a loader can say where a setting would go; treat a missing file as empty. `Committed` lets the loader refuse secrets in a committed file; `Home` lets it refuse a global-only setting in a project file. Those policies are the loader's, not appdir's.

<!-- snippet: examples/config-files/main.go#list -->
```go
// list prints ConfigFiles in read order and what a loader that lets later
// files win per key makes of "port".
func (ex *example) list(title string, extra ...appdir.Option) error {
	d, err := appdir.New(appName, ex.options(extra...)...)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	fmt.Fprintf(ex.w, "\n== %s (profile %s)\n", title, d.Profile())
	files := d.ConfigFiles()
	for i, f := range files {
		var flags []string
		if f.Local {
			flags = append(flags, "local")
		}
		if f.Committed {
			flags = append(flags, "committed")
		}
		if f.Profile != "" {
			flags = append(flags, "profile="+string(f.Profile))
		}
		if !exists(f.Path) {
			flags = append(flags, "missing")
		}
		line := fmt.Sprintf("%d. %-7s %s %s", i+1, f.Home, ex.rel(f.Path), strings.Join(flags, " "))
		fmt.Fprintln(ex.w, strings.TrimRight(line, " "))
	}
	port, from, err := loadPort(files)
	if err != nil {
		return fmt.Errorf("load port: %w", err)
	}
	if from == "" {
		fmt.Fprintln(ex.w, "port unset (code default)")
		return nil
	}
	fmt.Fprintf(ex.w, "port = %d (from %s)\n", port, ex.rel(from))
	return nil
}

// loadPort is a toy config loader: it reads the files in order, skips the
// missing ones, and lets a later file win per key. A real tool hands the
// list to its config loader instead. JSON numbers decode as float64, so the
// value is checked and turned into an int here, at the boundary. from is
// the file that set the port, empty when no file did.
func loadPort(files []appdir.ConfigFile) (port int, from string, err error) {
	for _, f := range files {
		m, err := readJSON(f.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue // a missing file counts as empty
		}
		if err != nil {
			return 0, "", err
		}
		v, ok := m[portKey]
		if !ok {
			continue
		}
		n, ok := v.(float64)
		if !ok {
			return 0, "", fmt.Errorf("%s: %q is %T, want a number", f.Path, portKey, v)
		}
		port, from = int(n), f.Path
	}
	return port, from, nil
}
```

## Step 2 — The dev profile's own file

A dev build reads the same shared project files as prod, so it gets the team settings. It may add `.<app>/profiles/dev/config.local.jsonc`, read last, so it wins over both shared files: a dev build can take port 4100 while the installed build keeps 4000. The entry has `Profile: "dev"`. The file is gitignored with the rest of `profiles/`, and a release build never lists it (owner decision, 2026-11-10).

<!-- snippet: examples/config-files/main.go#dev -->
```go
// dev lists the files of the dev profile: the same shared files, then the
// profile's own config.local.jsonc, read last so it wins.
func (ex *example) dev() error {
	devFile := filepath.Join(ex.marker, profilesDir, string(appdir.ProfileDev), localConfigFile)
	if err := os.MkdirAll(filepath.Dir(devFile), dirPerm); err != nil {
		return fmt.Errorf("create: %w", err)
	}
	if err := os.WriteFile(devFile, []byte(`{ "port": 4100 }`+"\n"), filePerm); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return ex.list("dev", appdir.WithProfile(string(appdir.ProfileDev)))
}
```

## Step 3 — With a `home:` pointer

The marker's two files are read first (they hold the pointer), then the target's two files. The dev profile's file moves into the target with the rest of the dev data.

<!-- snippet: examples/config-files/main.go#pointer -->
```go
// pointer moves the project home with a home: key in the local file: the
// target's two files come after the marker's, and the dev profile's own
// file moves into the target with the rest of the dev data.
func (ex *example) pointer() error {
	for _, dir := range []string{ex.private, filepath.Join(ex.root, privateDir, gitDir)} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	d, err := appdir.New(appName, ex.options()...)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	if err := d.InitHome(ex.private, appdir.ScopeProject); err != nil {
		return fmt.Errorf("init home: %w", err)
	}
	local := `{ "port": 4000, "home": "../../shop-private/lore" }` + "\n"
	if err := os.WriteFile(filepath.Join(ex.marker, localConfigFile), []byte(local), filePerm); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := os.WriteFile(filepath.Join(ex.private, configFile), []byte(`{ "port": 5000 }`+"\n"), filePerm); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := ex.list("home: pointer"); err != nil {
		return err
	}
	return ex.list("home: pointer", appdir.WithProfile(string(appdir.ProfileDev)))
}
```

## Step 4 — `--home` and no `$HOME`

With `--home` or `<APP>_HOME`, there is one home for every scope and its two files are listed once, marked `Home: "global"`. With no home folder (no `$HOME`), the global files are left out and the project files remain.

<!-- snippet: examples/config-files/main.go#overrides -->
```go
// homeFlag lists the files of one home named by --home: two files, once.
func (ex *example) homeFlag() error {
	return ex.list("--home", appdir.WithHome(filepath.Join(ex.root, otherHome)))
}

// noHome drops the global files when there is no home folder.
func (ex *example) noHome() error {
	noHomeDir := func() (string, error) { return "", errors.New(noHomeMsg) }
	return ex.list("no $HOME", appdir.WithHomeDir(noHomeDir))
}
```

## Step 5 — Replace the JSONC reader

appdir itself reads only `home.jsonc` and the files that may hold `home:` / `db:`. It reads them with a built-in JSONC reader that accepts `//` and `/* */` comments and trailing commas, and walks strings properly (`//` inside a string is text, escapes are honoured); it fails on an unterminated comment or string. A committed file like this one is read as is:

<!-- snippet: examples/pointers/main.go#committed-pointer -->
```go
// committedPointer is a team-shared config.jsonc: JSONC, so it may hold
// comments and trailing commas. The home: value is relative to the folder
// holding this file (shop/.lore), so ../../shop-private/lore is the
// private repo next to shop on every machine that clones both.
const committedPointer = `{
  // Our lore data lives in the private repo next to this one.
  /* A relative path works on every machine. */
  "home": "../../shop-private/lore",
  "note": "a // inside a string is not a comment",
}
`
```

`WithConfigReader(fn)` replaces that reader, so appdir and your loader agree on one syntax ([SPEC 6.3](../SPEC.md#63-config-reader-seam)). The reader returns the file as a map; for a missing file it must return an error that wraps `fs.ErrNotExist`. Here a recording reader shows what appdir reads during `New`:

<!-- snippet: examples/config-files/main.go#reader -->
```go
// reader passes the tool's own reader to appdir. appdir calls it for
// home.jsonc and for the files that may hold home:/db:, never for the
// settings themselves. A missing file must give an error that wraps
// fs.ErrNotExist, as os.ReadFile does.
func (ex *example) reader() error {
	fmt.Fprintln(ex.w, "\n== WithConfigReader")
	var read []string
	recording := func(path string) (map[string]any, error) {
		read = append(read, ex.rel(path))
		return readJSON(path)
	}
	if _, err := appdir.New(appName, ex.options(appdir.WithConfigReader(recording))...); err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	for _, p := range read {
		fmt.Fprintf(ex.w, "read %s\n", p)
	}
	return nil
}

// readJSON is a strict JSON reader: the tool's choice, here the stdlib.
func readJSON(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err // keeps fs.ErrNotExist for a missing file
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}
```

## Step 6 — Drop a config kind

A tool that keeps no machine-only settings drops the file with `WithoutKind(appdir.KindConfigLocal)` (or `KindConfig` for the committed one). The dropped file is left out of `ConfigFiles()` in every home, never listed as the home folder itself, so the loader needs no special case. Pointer keys are read only from the config kinds that remain.

<!-- snippet: examples/config-files/main.go#dropped -->
```go
// dropped lists the files of a tool that keeps no machine-only settings:
// WithoutKind(KindConfigLocal) leaves config.local.jsonc out of every home,
// so the loader never reads it (and a home: key in it is not read either).
func (ex *example) dropped() error {
	return ex.list("WithoutKind(config-local)", appdir.WithoutKind(appdir.KindConfigLocal))
}
```

## Try it

```sh
go run ./examples/config-files
```

Output (the temp root is printed as `$TMP`):

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

With the pointer, the target's `config.jsonc` (port 5000) comes after the marker's files, and in the dev profile the old `shop/.lore/profiles/dev/config.local.jsonc` is no longer read: the dev file now lives in the target.

## Pitfalls

- The target's files win over the marker's, including the marker's `config.local.jsonc`. A machine-local setting in `.<app>/config.local.jsonc` is overridden by the same key in the target's `config.jsonc`; put it in the target's `config.local.jsonc` instead.
- Adding a `home:` pointer moves the dev profile's own file from `.<app>/profiles/dev/` to `<target>/profiles/dev/`; an existing dev file stops being read.
- `Committed` is a guess from the folder, not from git: an entry named `.git` in the home or any parent, up to the filesystem root. A home under a stray `.git` higher up (a dotfiles repo) shows `Committed: true`; a file excluded by `.gitignore` still shows `true` if it is the `config.jsonc` slot.
- Under `--home` / `<APP>_HOME` the two files are marked `Home: "global"`, and `Committed` is always `false`, even inside a repo.
- With a pointer error the list keeps the global files and the marker's own `config.jsonc` and `config.local.jsonc` (they hold the key to fix), and drops the target's and the profile's files; `Project()` returns the error, and every project kind fails with it (see [Pointer keys](../pointers/pointer-keys.md)). A loader should stop and show that error rather than merge a partial list.
- A config kind dropped with `WithoutKind` is left out of the list in every home (Step 6), and its `home:` / `db:` keys are no longer read: in the example the `home:` pointer that sat in `config.local.jsonc` stops applying, so the list ends at the marker's `config.jsonc`. Move a pointer to the remaining file before you drop one.
- `WithConfigReader` is used for `home.jsonc` and pointer keys only; your loader reads the settings. If your reader returns a plain error for a missing file (not wrapping `fs.ErrNotExist`), `New` treats it as unreadable: a missing `config.local.jsonc` makes the project fail with `read <file>: ...`, and a missing `home.jsonc` is taken as no home info.
- A reader that cannot parse JSONC (like the strict `encoding/json` one in the example) fails on a committed `config.jsonc` with comments; the error is the project's error, not `New`'s.
- The built-in reader is internal (`internal/jsonclite`) and not importable by your code; it is replaced by `ubgo/jsonc` when that module exists.

## Related

- [Pointer keys](../pointers/pointer-keys.md): `home:` and `db:`.
- [Profiles](../profiles/profiles.md): the dev profile and its folders.
- [Overrides](../homes/overrides.md): `--home` and `<APP>_HOME`.
- [paths --json and doctor](../reference/paths-json.md): the `ConfigFile` JSON fields.
- Example: [examples/config-files](../../examples/config-files).
- SPEC: [6.1 Config files](../SPEC.md#61-config-files), [6.3 Config reader seam](../SPEC.md#63-config-reader-seam).
