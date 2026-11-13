# Pointer keys: `home:` and `db:`

A project home `.<app>/` lives in the repo, but its data does not have to. Two keys in the marker's config files move it: `home:` moves the whole project home to another folder (say a private repo next to the public one), and `db:` moves only the database (say to a faster disk). You want them when the data must not be committed with the code, or must live on another disk, while the tool still finds it from inside the checkout.

## Before you start

- A project home made by `InitProject` (see [Initialize a project](../start/init.md)) and the two homes explained in [Global and project homes](../homes/global-and-project.md).
- Import `github.com/ubgo/appdir`. Nothing else: appdir reads the pointer keys itself in `New`, with its built-in JSONC reader (see [Config files](../layout/config-files.md) to replace it).
- The keys are read from `.<app>/config.jsonc` (committed) and then `.<app>/config.local.jsonc` (this machine only); for each key the local file wins ([applyPointers](ds:block?id=dirs-applypointers-hqmm53fn)).

The example below runs every case in one temp folder: the user home is `$TMP/userhome`, the project is `$TMP/work/shop`, the private folder is `$TMP/work/shop-private/lore`. Every step calls one helper that resolves `lore` and prints the result or the error:

<!-- snippet: examples/pointers/main.go#show -->
```go
// show resolves lore and prints the project home, the db and backups paths
// and any warnings, or the error with the sentinel it wraps.
func (ex *example) show(extra ...appdir.Option) error {
	d, err := ex.newDirs(extra...)
	if err != nil {
		return err
	}
	h, err := d.Project()
	if err != nil {
		fmt.Fprintf(ex.w, "error: %s\n", ex.rel(err.Error()))
		fmt.Fprintf(ex.w, "  sentinel: %s\n", sentinelName(err))
		return nil
	}
	db, err := d.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	backups, err := d.Path(appdir.KindBackups)
	if err != nil {
		return err
	}
	fmt.Fprintf(ex.w, "profile: %s\n", d.Profile())
	fmt.Fprintf(ex.w, "project: %s (rule %s)\n", ex.rel(h.Base), h.Rule)
	fmt.Fprintf(ex.w, "data:    %s\n", ex.rel(h.Data))
	fmt.Fprintf(ex.w, "db:      %s\n", ex.rel(db))
	fmt.Fprintf(ex.w, "backups: %s\n", ex.rel(backups))
	for _, wn := range d.Warnings() {
		fmt.Fprintf(ex.w, "warning %s: %s\n", wn.Code, ex.rel(wn.Message))
	}
	return nil
}
```

## Step 1 — Write a relative `home:` in the committed file

A committed pointer must work on every clone, so it is relative (or starts with `~/`). It is resolved against the folder that holds the config file, here `shop/.lore`, so `../../shop-private/lore` is the private repo next to `shop`. The file is JSONC: comments, `/* */` blocks and trailing commas are fine, and `//` inside a string stays text.

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

Until the target exists, every project path fails with `ErrPointerMissing`, and the message tells the user how to create it:

```text
error: pointer points to $TMP/work/shop-private/lore, which does not exist; run 'lore init --home $TMP/work/shop-private/lore'
  sentinel: ErrPointerMissing
```

## Step 2 — Create the target with `InitHome`

The target must be a home of this app: a folder with a `home.jsonc` whose `app` is `lore`. `InitHome` writes it (it is what `lore init --home X` calls). A folder that exists but has no such file fails with `ErrPointerNotHome`. A tool that dropped the home kind (`WithoutKind(KindHome)`) writes no `home.jsonc`; its targets must be folders named `.lore` instead.

<!-- snippet: examples/pointers/main.go#create-target -->
```go
// createTarget makes the pointer target a lore home, like
// `lore init --home ../shop-private/lore`: InitHome writes its home.jsonc.
func (ex *example) createTarget() error {
	fmt.Fprintln(ex.w, "\n== home: pointer, target created with InitHome")
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	if err := d.InitHome(ex.private, appdir.ScopeProject); err != nil {
		return fmt.Errorf("init home: %w", err)
	}
	return ex.show()
}
```

Now the project home's `Base` is the target and its rule is `pointer`; the db, backups and every other project kind live there.

## Step 3 — Keep absolute paths in `config.local.jsonc`

An absolute path, a drive or UNC path, `~` alone, `~user` or a value starting with `$` in the committed file is `ErrPointerAbsolute`: it would point somewhere else (or nowhere) on a teammate's machine ([portablePointer](ds:block?id=portablepointer-cuc25d5h)). The same value in `config.local.jsonc` is fine, because that file is gitignored and belongs to this machine; there any user path works (`~`, `${VAR}`, absolute, relative).

<!-- snippet: examples/pointers/main.go#absolute -->
```go
// absoluteCommitted puts an absolute path in the committed file: it would
// break every other clone, so appdir refuses it.
func (ex *example) absoluteCommitted() error {
	fmt.Fprintln(ex.w, "\n== absolute home: in config.jsonc")
	body := fmt.Sprintf("{ %q: %q }\n", homeKey, ex.private)
	if err := ex.write(ex.marker, configFile, body); err != nil {
		return err
	}
	return ex.show()
}

// absoluteLocal moves the same value to config.local.jsonc, which is this
// machine's own file: any user path is fine there.
func (ex *example) absoluteLocal() error {
	fmt.Fprintln(ex.w, "\n== absolute home: in config.local.jsonc")
	if err := ex.write(ex.marker, configFile, "{}\n"); err != nil {
		return err
	}
	body := fmt.Sprintf("{ %q: %q }\n", homeKey, ex.private)
	if err := ex.write(ex.marker, localConfigFile, body); err != nil {
		return err
	}
	return ex.show()
}
```

## Step 4 — Move only the database with `db:`

`db:` names the database file. Its folder must exist (else `ErrPointerMissing`); the file itself may not exist yet, the first run creates it. Only the `db` kind moves: `backups/` stays in the home that owns the marker.

<!-- snippet: examples/pointers/main.go#db-pointer -->
```go
// dbPointer moves only the database. Its folder must exist; the file
// itself may not (the first run creates it). backups/ stays in the home.
func (ex *example) dbPointer() error {
	fmt.Fprintln(ex.w, "\n== db: pointer")
	if err := os.Remove(filepath.Join(ex.marker, localConfigFile)); err != nil {
		return fmt.Errorf("remove local config: %w", err)
	}
	body := `{ "db": "../../shop-db/lore.db" }` + "\n"
	if err := ex.write(ex.marker, configFile, body); err != nil {
		return err
	}
	if err := ex.show(); err != nil { // shop-db/ is not there yet
		return err
	}
	if err := os.MkdirAll(filepath.Join(ex.root, workDir, dbDir), dirPerm); err != nil {
		return fmt.Errorf("create db folder: %w", err)
	}
	return ex.show()
}
```

## Step 5 — Pointers go one hop only

If the target has its own `home:` or `db:` key, appdir does not follow it: the target is used as it is and the tool gets warning `pointer-chain-ignored` to print once. Chains would make "where is my data" depend on files the user is not looking at.

<!-- snippet: examples/pointers/main.go#one-hop -->
```go
// oneHop gives the target its own home: key. appdir does not follow it:
// pointers go one hop only, and the tool gets a warning to print.
func (ex *example) oneHop() error {
	fmt.Fprintln(ex.w, "\n== one hop only")
	if err := ex.write(ex.marker, configFile, committedPointer); err != nil {
		return err
	}
	if err := ex.write(ex.private, configFile, `{ "home": "../elsewhere" }`+"\n"); err != nil {
		return err
	}
	return ex.show()
}
```

## Step 6 — A value must be a string

A pointer value that is not a string (a number, an object, `true`) fails with `ErrInvalidOption`, naming the file and key. `null` and `""` count as not set.

<!-- snippet: examples/pointers/main.go#not-a-string -->
```go
// notAString shows that a pointer value must be a string path.
func (ex *example) notAString() error {
	fmt.Fprintln(ex.w, "\n== home: is not a string")
	if err := ex.write(ex.marker, localConfigFile, `{ "home": 42 }`+"\n"); err != nil {
		return err
	}
	if err := ex.show(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(ex.marker, localConfigFile)); err != nil {
		return fmt.Errorf("remove local config: %w", err)
	}
	return nil
}
```

## Step 7 — The dev profile

A non-release profile keeps its data apart from prod (see [Profiles](../profiles/profiles.md)). With pointers this means: `home:` is followed, but the project data goes to `<target>/profiles/<profile>/`; `db:` is ignored with warning `pointer-ignored-for-profile`, so the dev database stays at `<target>/profiles/dev/lore.db` and dev data never lands next to the prod database the pointer names. This is SPEC section 14 item 2, chosen by the coordinator and to be reviewed with the owner; it is the current behaviour.

<!-- snippet: examples/pointers/main.go#dev-profile -->
```go
// devProfile resolves the same project as the dev profile, with both keys
// set: home: is followed but the data goes to <target>/profiles/dev, and
// db: is ignored so dev data never lands next to the prod database.
func (ex *example) devProfile() error {
	fmt.Fprintln(ex.w, "\n== dev profile with home: and db:")
	if err := ex.write(ex.private, configFile, "{}\n"); err != nil {
		return err
	}
	body := `{ "home": "../../shop-private/lore", "db": "../../shop-db/lore.db" }` + "\n"
	if err := ex.write(ex.marker, configFile, body); err != nil {
		return err
	}
	return ex.show(appdir.WithProfile(string(appdir.ProfileDev)))
}
```

## Step 8 — `ConfigFiles()` under a broken pointer

A pointer error fails `Project()` and every project-capable kind, but `ConfigFiles()` still lists the marker's own `config.jsonc` and `config.local.jsonc` (and the global ones). Those are the files that hold the bad key, so a `config` or `doctor` command can open them and show the user what to fix instead of failing blind.

<!-- snippet: examples/pointers/main.go#broken-files -->
```go
// brokenConfigFiles points home: at a folder that does not exist. Project
// fails, but ConfigFiles still lists the marker's own two files: they hold
// the pointer the user has to fix, so a config command can show them.
func (ex *example) brokenConfigFiles() error {
	fmt.Fprintln(ex.w, "\n== ConfigFiles under a broken pointer")
	body := fmt.Sprintf("{ %q: %q }\n", homeKey, goneTarget)
	if err := ex.write(ex.marker, configFile, body); err != nil {
		return err
	}
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	if _, err := d.Project(); err != nil {
		fmt.Fprintf(ex.w, "Project: %s\n", sentinelName(err))
	}
	for _, f := range d.ConfigFiles() {
		fmt.Fprintf(ex.w, "%-7s %s\n", f.Home, ex.rel(f.Path))
	}
	return ex.write(ex.marker, configFile, committedPointer) // fix it again
}
```

## Step 9 — An empty local `home:` does not clear the committed one

`"home": ""` (or `null`) counts as not set, so it is skipped and the committed `config.jsonc` value still applies; local wins per key only when it holds a path. A machine that must work without the team's pointer passes `--home`, or points the local file at another path; to drop the pointer for everyone, remove it from `config.jsonc`.

<!-- snippet: examples/pointers/main.go#empty-local -->
```go
// emptyLocalHome sets "home": "" in config.local.jsonc. An empty value
// counts as unset, so the committed home: still applies: a machine cannot
// clear the team's pointer this way, only replace it with another path.
func (ex *example) emptyLocalHome() error {
	fmt.Fprintln(ex.w, "\n== empty home: in config.local.jsonc")
	if err := ex.write(ex.marker, localConfigFile, `{ "home": "" }`+"\n"); err != nil {
		return err
	}
	if err := ex.show(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(ex.marker, localConfigFile)); err != nil {
		return fmt.Errorf("remove local config: %w", err)
	}
	return nil
}
```

## Try it

```sh
go run ./examples/pointers
```

Output (the temp root is printed as `$TMP`):

```text
== init
created $TMP/work/shop/.lore

== home: pointer, target missing
error: pointer points to $TMP/work/shop-private/lore, which does not exist; run 'lore init --home $TMP/work/shop-private/lore'
  sentinel: ErrPointerMissing

== home: pointer, target created with InitHome
profile: prod
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore
db:      $TMP/work/shop-private/lore/lore.db
backups: $TMP/work/shop-private/lore/backups

== absolute home: in config.jsonc
error: home: "$TMP/work/shop-private/lore" in $TMP/work/shop/.lore/config.jsonc is absolute; put it in config.local.jsonc
  sentinel: ErrPointerAbsolute

== absolute home: in config.local.jsonc
profile: prod
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore
db:      $TMP/work/shop-private/lore/lore.db
backups: $TMP/work/shop-private/lore/backups

== db: pointer
error: pointer points to $TMP/work/shop-db, which does not exist
  sentinel: ErrPointerMissing
profile: prod
project: $TMP/work/shop/.lore (rule project)
data:    $TMP/work/shop/.lore
db:      $TMP/work/shop-db/lore.db
backups: $TMP/work/shop/.lore/backups

== one hop only
profile: prod
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore
db:      $TMP/work/shop-private/lore/lore.db
backups: $TMP/work/shop-private/lore/backups
warning pointer-chain-ignored: home in $TMP/work/shop-private/lore/config.jsonc not followed: pointers go one hop only

== home: is not a string
error: $TMP/work/shop/.lore/config.local.jsonc: home must be a string path
  sentinel: ErrInvalidOption

== dev profile with home: and db:
profile: dev
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore/profiles/dev
db:      $TMP/work/shop-private/lore/profiles/dev/lore.db
backups: $TMP/work/shop-private/lore/profiles/dev/backups
warning pointer-ignored-for-profile: db: pointer in $TMP/work/shop/.lore/config.jsonc ignored for profile dev; the dev database stays at $TMP/work/shop-private/lore/profiles/dev/lore.db

== ConfigFiles under a broken pointer
Project: ErrPointerMissing
global  $TMP/userhome/.lore/config.jsonc
global  $TMP/userhome/.lore/config.local.jsonc
project $TMP/work/shop/.lore/config.jsonc
project $TMP/work/shop/.lore/config.local.jsonc

== empty home: in config.local.jsonc
profile: prod
project: $TMP/work/shop-private/lore (rule pointer)
data:    $TMP/work/shop-private/lore
db:      $TMP/work/shop-private/lore/lore.db
backups: $TMP/work/shop-private/lore/backups
```

## Pitfalls

- `New` does not fail on a pointer error. It is stored and returned by `Project()`, `Path`, `Ensure` and the other calls that need the project home, so check those errors (or `HasProject()`) rather than the one from `New`.
- A pointer error fails every kind that can live in the project: the project-only kinds (`db`, `backups`, `run`, `shared` and your own data kinds), `project-else-global` kinds (`logs`) and the project side of `both` kinds (`config`, `config-local`, `home`). It never falls back to the marker or to the global home (SPEC 4.5); only global kinds such as the cache keep working. `ConfigFiles()` still lists the marker's own `config.jsonc` and `config.local.jsonc`, so a `doctor` command can point at the file that holds the bad key (Step 8).
- A relative pointer is resolved against the folder holding the file it is in (`.<app>/`), not against the repo root and not against the working folder. `../target` is a sibling of `.<app>/` inside the repo.
- The `ErrPointerAbsolute` message is `home: "<value>" in <file> is absolute; put it in config.local.jsonc`. `${VAR}` in the committed file is refused even when the variable holds a relative path, because the value starts with `$`.
- A non-string value fails with `ErrInvalidOption` (`<file>: home must be a string path`) and, like any pointer error, fails every project-capable kind.
- An empty value counts as absent (Step 9). A tool whose own `config set home ""` command writes an empty string to "turn the pointer off" changes nothing; have it write another path or tell the user to pass `--home`.
- The `home:` target is checked for `home.jsonc` with `"app": "<app>"`; a target made by hand with `mkdir` fails with `ErrPointerNotHome`. Use `InitHome`. Without the home kind the check is the folder name `.<app>` instead.
- `db:` points at a file, not a folder: `"db": "../fast/lore.db"`. Only its folder has to exist.
- In the dev profile a `db:` key is ignored, but a malformed one is still read: a non-string `db:` fails before the profile check.
- With a `home:` pointer, the marker's `.<app>/` still has to exist (it is how the project is found); only `config.jsonc` and `config.local.jsonc` there are read, plus the target's own files (see [Config files](../layout/config-files.md)).

## Related

- [Config files](../layout/config-files.md): the target's config files come after the marker's in `ConfigFiles()`.
- [Initialize a project](../start/init.md): `InitProject` and `InitHome`.
- [Profiles](../profiles/profiles.md): why the dev profile keeps its data under `profiles/`.
- [Errors](../reference/errors.md): every sentinel with its message.
- Example: [examples/pointers](../../examples/pointers).
- SPEC: [4.5 Project home: pointer keys](../SPEC.md#45-project-home-pointer-keys), [4.6 Profile folders](../SPEC.md#46-profile-folders).
