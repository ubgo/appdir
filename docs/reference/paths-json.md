# paths --json and doctor

Every tool on the standard has a `paths` command (`--json` gives kind, path, rule, fallback) and a `doctor` command that shows each folder and the rule that picked it, the config files read, and anything odd. appdir gives you the data for both as plain Go values with JSON tags: `Paths()`, `ConfigFiles()`, `Warnings()`, `Kinds()` and `GitignoreBlock()`. You add the envelope, the text layout and your own checks (database, locks, backup size). This page builds both from the [doctor example](../../examples/doctor/main.go), then lists every field, rule and warning code.

## Before you start

- `import "github.com/ubgo/appdir"` and a `*appdir.Dirs` from `appdir.New` (see [First use](../start/first-use.md)).
- `encoding/json` from the standard library; the result types already carry JSON tags.
- The two homes and the rules that pick them: [Global and project homes](../homes/global-and-project.md), [Overrides](../homes/overrides.md).

## Step 1 — resolve with the tool's inputs

`paths` and `doctor` report what the tool would use, so build the `Dirs` exactly as the tool's other commands do: the same flags, the same kinds. The example passes the user home, start folder and environment through seams so it runs in a temp folder; a real tool passes its flags (`WithHome`, `WithProfile`) instead.

<!-- snippet: examples/doctor/main.go#resolve -->
```go
// resolve builds the Dirs of the example tool. Every outside input goes
// through a seam: the user home, the start folder of the lookup, the
// environment and the release flag.
func resolve(root, work string, extra ...appdir.Option) (*appdir.Dirs, error) {
	opts := append(seams(root, work),
		appdir.WithRelease(true), // the tool's -ldflags -X variable in a real build
		appdir.WithKind(kindTemplates, appdir.Folder(string(kindTemplates)), appdir.Commit()),
	)
	d, err := appdir.New(appName, append(opts, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}
	return d, nil
}

// seams hands appdir the temp user home, the start folder and the example's
// environment, so nothing is read from the real machine.
func seams(root, work string) []appdir.Option {
	home := filepath.Join(root, homeFolder)
	env := exampleEnv()
	return []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(work),
		appdir.WithEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }),
	}
}
```

## Step 2 — print paths --json

`Paths()` returns one `Entry` per kind in layout order, and two for a `both` kind (global first, then project when there is one, or when a broken project shows its error; once, labelled global, under `--home`/`<APP>_HOME`) ([Paths](ds:block?id=dirs-paths-6kdzcjzg)). It never fails: a kind whose home is unusable carries the message in `Error` and the rule that failed in `Rule`. `Entry` has JSON tags, so `paths --json` is the standard's envelope around the slice.

<!-- snippet: examples/doctor/main.go#paths-json -->
```go
// pathsEnvelope is the standard's --json envelope around Paths().
type pathsEnvelope struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	Count         int            `json:"count"`
	Profile       appdir.Profile `json:"profile"`
	Data          []appdir.Entry `json:"data"`
}

// writePathsJSON prints `paths --json`. Entry already carries JSON tags, so
// the tool only wraps the slice in its envelope.
func writePathsJSON(w io.Writer, d *appdir.Dirs, root string) error {
	entries := d.Paths()
	for i := range entries {
		// Only for this example's stable output; a real tool prints the path.
		entries[i].Path = hideRoot(entries[i].Path, root)
		entries[i].Error = hideRoot(entries[i].Error, root)
	}
	env := pathsEnvelope{SchemaVersion: schemaVersion, Kind: envelopeKind, Count: len(entries), Profile: d.Profile(), Data: entries}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(env); err != nil {
		return fmt.Errorf("paths --json: %w", err)
	}
	return nil
}
```

Under `--home` or `<APP>_HOME` there is only one home, so a `both` kind is listed once, with `home: "global"`, and every kind has the same folder and the rule `flag` (or `env-home`). Run from inside the repo, the example's 14 lines become 11: `home`, `config` and `config-local` lose their project line. A reader that counts on two lines per `both` kind must not; group by `kind` instead.

<!-- snippet: examples/doctor/main.go#paths-override -->
```go
// writePathsOverride prints `paths --home team` from inside the repo. Under
// --home (or <APP>_HOME) there is one home, so a kind that lives in both
// homes (home, config, config-local) is listed once, labelled global, and
// every kind gets the same folder and rule.
func writePathsOverride(w io.Writer, root, repo string) error {
	d, err := resolve(root, repo, appdir.WithHome(filepath.Join(root, teamFolder)))
	if err != nil {
		return err
	}
	for _, e := range d.Paths() {
		fmt.Fprintf(w, "  %-13s %-8s %-5s %s\n", e.Kind, e.Home, e.Rule, hideRoot(e.Path, root))
	}
	return nil
}
```

The output, from the Try it run below:

```text
== paths --home team
  home          global   flag  $ROOT/team/home.jsonc
  config        global   flag  $ROOT/team/config.jsonc
  config-local  global   flag  $ROOT/team/config.local.jsonc
  gitignore     project  flag  $ROOT/team/.gitignore
  shared        project  flag  $ROOT/team/shared
  db            project  flag  $ROOT/team/doctor.db
  backups       project  flag  $ROOT/team/backups
  cache         global   flag  $ROOT/team/cache
  run           project  flag  $ROOT/team/run
  logs          project  flag  $ROOT/team/logs
  templates     project  flag  $ROOT/team/templates
```

`db` still says `project` and `cache` still says `global`: `Home` is the kind's scope, not a second folder. `ConfigFiles()` follows the same rule and lists the two config files of the one home, labelled global.

## Step 3 — print the appdir part of doctor

`ConfigFiles()` lists the config files in read order, later wins per key, including files that do not exist yet (the loader treats them as empty) ([ConfigFiles](ds:block?id=dirs-configfiles-nzbecu5q)). `Warnings()` returns what the tool should print once. `Kinds()` gives the layout in effect, with the names and paths the author chose. `GitignoreBlock()` returns the block `WriteGitignore` keeps between the markers.

<!-- snippet: examples/doctor/main.go#doctor -->
```go
// writeDoctor prints the parts of `doctor` that come from appdir: config
// files in read order, warnings, the layout in effect and the gitignore block.
func writeDoctor(w io.Writer, d *appdir.Dirs, root string) {
	fmt.Fprintf(w, "profile %s (release=%t, %s)\n", d.Profile(), d.IsRelease(), d.ReleaseReason())
	fmt.Fprintln(w, "config files (later wins):")
	for _, c := range d.ConfigFiles() {
		state := "missing"
		if _, err := os.Stat(c.Path); err == nil {
			state = "present"
		}
		fmt.Fprintf(w, "  %-7s %-40s local=%t committed=%t %s\n", c.Home, hideRoot(c.Path, root), c.Local, c.Committed, state)
	}
	fmt.Fprintln(w, "warnings:")
	for _, wn := range d.Warnings() {
		fmt.Fprintf(w, "  %s: %s\n", wn.Code, hideRoot(wn.Message, root))
	}
	fmt.Fprintln(w, "layout:")
	for _, k := range d.Kinds() {
		fmt.Fprintf(w, "  %-13s %-20s %-6s %-6s %s\n", k.Name, k.Path, k.Type, k.Git, k.Scope)
	}
	fmt.Fprintln(w, "gitignore block:")
	for _, line := range strings.Split(strings.TrimSuffix(d.GitignoreBlock(), "\n"), "\n") {
		fmt.Fprintln(w, "  "+line)
	}
}
```

## Step 4 — map errors to exit codes

`doctor` and every other command end with an exit code from the standard's fixed set. Check sentinels with `errors.Is`; the full table is in [Errors and exit codes](errors.md#suggested-exit-codes).

<!-- snippet: examples/doctor/main.go#exit-code -->
```go
// exitCode maps an error to the standard's exit codes. appdir leaves this to
// the tool; errors.Is works through every wrap.
func exitCode(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, appdir.ErrInvalidOption), errors.Is(err, appdir.ErrUnknownProfile), errors.Is(err, appdir.ErrEnvVar):
		return exitUsage
	case errors.Is(err, appdir.ErrNoProject), errors.Is(err, appdir.ErrNoHome),
		errors.Is(err, appdir.ErrPointerMissing), errors.Is(err, appdir.ErrPointerNotHome):
		return exitNotFound
	case errors.Is(err, appdir.ErrBusy):
		return exitBusy
	case errors.Is(err, appdir.ErrLayoutTooNew):
		return exitTooNew
	case errors.Is(err, appdir.ErrNotHome), errors.Is(err, appdir.ErrGitignoreMarkers), errors.Is(err, appdir.ErrBadHomeFile):
		return exitConflict
	default:
		return exitError
	}
}
```

The example provokes five real errors and checks each maps as expected:

<!-- snippet: examples/doctor/main.go#errors -->
```go
// failures provokes five real errors, each the way a user would hit it. The
// errors are returned unwrapped on purpose: writeErrors prints them as the
// user would see them.
func failures() []failure {
	return []failure{
		{"ErrNoProject", appdir.ErrNoProject, func(root string) error {
			// A project kind (db) asked for outside any project.
			d, err := resolve(root, filepath.Join(root, elsewhere))
			if err != nil {
				return err
			}
			_, err = d.Path(appdir.KindDB)
			return err
		}},
		{"ErrUnknownProfile", appdir.ErrUnknownProfile, func(root string) error {
			// --profile staging, but the list is prod, dev.
			_, err := resolve(root, root, appdir.WithProfile("staging"))
			return err
		}},
		{"ErrPointerAbsolute", appdir.ErrPointerAbsolute, func(root string) error {
			// An absolute home: pointer in the committed config.jsonc.
			repo, err := initRepo(root, pointerRepo)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(repo, markerFolder, configFile), []byte(`{"home": "/srv/doctor"}`), 0o600); err != nil {
				return err
			}
			d, err := resolve(root, repo)
			if err != nil {
				return err
			}
			_, err = d.Project()
			return err
		}},
		{"ErrUnknownKind", appdir.ErrUnknownKind, func(root string) error {
			// A kind the layout never declared: a typo of "templates".
			d, err := resolve(root, filepath.Join(root, repoFolder))
			if err != nil {
				return err
			}
			_, err = d.Path("template")
			return err
		}},
		{"ErrLayoutTooNew", appdir.ErrLayoutTooNew, func(root string) error {
			// A newer binary wrote layout 2; this one knows 1 and refuses to write.
			repo, err := initRepo(root, tooNewRepo)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(repo, markerFolder, homeInfoFile), []byte(`{"app": "doctor", "layout": 2, "scope": "project"}`), 0o600); err != nil {
				return err
			}
			d, err := resolve(root, repo)
			if err != nil {
				return err
			}
			_, err = d.Ensure(appdir.KindDB)
			return err
		}},
	}
}

// writeErrors provokes each failure and prints its exit code and message.
func writeErrors(w io.Writer, root string) error {
	for _, f := range failures() {
		err := f.provoke(root)
		if !errors.Is(err, f.sentinel) {
			return fmt.Errorf("%s: got %v", f.name, err)
		}
		fmt.Fprintf(w, "%-18s exit %d  %s\n", f.name, exitCode(err), hideRoot(err.Error(), root))
	}
	return nil
}
```

A bad option set fails `New` itself with `ErrInvalidOption` (exit 2), before anything is read. The example calls `New` with five such sets; the full list of checks is in [Errors and exit codes](errors.md#errinvalidoption).

<!-- snippet: examples/doctor/main.go#invalid-options -->
```go
// invalidOptions lists option sets New refuses. Each is an author bug or a
// bad value passed through from a flag; New checks them before it reads
// anything, and the error wraps ErrInvalidOption.
func invalidOptions() []invalidOption {
	return []invalidOption{
		{"app name", "Doctor", nil}, // upper case is not allowed
		{"dir name", appName, []appdir.Option{appdir.WithDirName("..")}},
		{"kind path", appName, []appdir.Option{appdir.WithKind("tpl", appdir.Folder("../x"))}},
		{"release profile", appName, []appdir.Option{appdir.WithReleaseProfile("staging")}},
		{"nil seam", appName, []appdir.Option{appdir.WithEnv(nil)}},
	}
}

// writeInvalidOptions calls New with each bad set and prints its exit code
// and message. The seams come first so a set that New wrongly accepted
// would still read only the temp root.
func writeInvalidOptions(w io.Writer, root string) error {
	for _, c := range invalidOptions() {
		_, err := appdir.New(c.app, append(seams(root, root), c.opts...)...)
		if !errors.Is(err, appdir.ErrInvalidOption) {
			return fmt.Errorf("%s: got %v", c.name, err)
		}
		fmt.Fprintf(w, "%-16s exit %d  %s\n", c.name, exitCode(err), err)
	}
	return nil
}
```

## Try it

```sh
go run ./examples/doctor
```

The temp root is printed as `$ROOT`. A real tool prints absolute paths.

```text
== paths --json
{
  "schema_version": 1,
  "kind": "paths",
  "count": 14,
  "profile": "prod",
  "data": [
    {
      "kind": "home",
      "home": "global",
      "path": "$ROOT/user/.doctor/home.jsonc",
      "type": "file",
      "git": "commit",
      "rule": "global",
      "fallback": false
    },
    {
      "kind": "home",
      "home": "project",
      "path": "$ROOT/repo/.doctor/home.jsonc",
      "type": "file",
      "git": "commit",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "config",
      "home": "global",
      "path": "$ROOT/user/.doctor/config.jsonc",
      "type": "file",
      "git": "commit",
      "rule": "global",
      "fallback": false
    },
    {
      "kind": "config",
      "home": "project",
      "path": "$ROOT/repo/.doctor/config.jsonc",
      "type": "file",
      "git": "commit",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "config-local",
      "home": "global",
      "path": "$ROOT/user/.doctor/config.local.jsonc",
      "type": "file",
      "git": "ignore",
      "rule": "global",
      "fallback": false
    },
    {
      "kind": "config-local",
      "home": "project",
      "path": "$ROOT/repo/.doctor/config.local.jsonc",
      "type": "file",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "gitignore",
      "home": "project",
      "path": "$ROOT/repo/.doctor/.gitignore",
      "type": "file",
      "git": "commit",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "shared",
      "home": "project",
      "path": "$ROOT/repo/.doctor/shared",
      "type": "folder",
      "git": "commit",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "db",
      "home": "project",
      "path": "$ROOT/repo/.doctor/doctor.db",
      "type": "file",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "backups",
      "home": "project",
      "path": "$ROOT/repo/.doctor/backups",
      "type": "folder",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "cache",
      "home": "global",
      "path": "$ROOT/user/.doctor/cache",
      "type": "folder",
      "git": "ignore",
      "rule": "global",
      "fallback": false
    },
    {
      "kind": "run",
      "home": "project",
      "path": "$ROOT/repo/.doctor/run",
      "type": "folder",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "logs",
      "home": "project",
      "path": "$ROOT/repo/.doctor/logs",
      "type": "folder",
      "git": "ignore",
      "rule": "project",
      "fallback": false
    },
    {
      "kind": "templates",
      "home": "project",
      "path": "$ROOT/repo/.doctor/templates",
      "type": "folder",
      "git": "commit",
      "rule": "project",
      "fallback": false
    }
  ]
}
== doctor
profile prod (release=true, flag)
config files (later wins):
  global  $ROOT/user/.doctor/config.jsonc          local=false committed=false missing
  global  $ROOT/user/.doctor/config.local.jsonc    local=true committed=false missing
  project $ROOT/repo/.doctor/config.jsonc          local=false committed=true missing
  project $ROOT/repo/.doctor/config.local.jsonc    local=true committed=false missing
warnings:
  profile-env-ignored: DOCTOR_PROFILE=dev ignored by a release build; pass --profile dev
  ceiling-relative: DOCTOR_CEILING_DIRS entry "relative/dir" is relative; ignored
layout:
  home          home.jsonc           file   commit both
  config        config.jsonc         file   commit both
  config-local  config.local.jsonc   file   ignore both
  gitignore     .gitignore           file   commit project
  shared        shared               folder commit project
  db            doctor.db            file   ignore project
  backups       backups              folder ignore project
  cache         cache                folder ignore global
  run           run                  folder ignore project
  logs          logs                 folder ignore project-else-global
  templates     templates            folder commit project
gitignore block:
  # doctor:begin
  *
  !.gitignore
  !home.jsonc
  !config.jsonc
  !shared/
  !shared/**
  !templates/
  !templates/**
  # doctor:end
== paths --home team
  home          global   flag  $ROOT/team/home.jsonc
  config        global   flag  $ROOT/team/config.jsonc
  config-local  global   flag  $ROOT/team/config.local.jsonc
  gitignore     project  flag  $ROOT/team/.gitignore
  shared        project  flag  $ROOT/team/shared
  db            project  flag  $ROOT/team/doctor.db
  backups       project  flag  $ROOT/team/backups
  cache         global   flag  $ROOT/team/cache
  run           project  flag  $ROOT/team/run
  logs          project  flag  $ROOT/team/logs
  templates     project  flag  $ROOT/team/templates
== errors
ErrNoProject       exit 3  no .doctor/ here; run 'doctor init' or pass --home
ErrUnknownProfile  exit 2  resolve: appdir: --profile: unknown profile "staging" (known: prod, dev)
ErrPointerAbsolute exit 1  home: "/srv/doctor" in $ROOT/repo-ptr/.doctor/config.jsonc is absolute; put it in config.local.jsonc
ErrUnknownKind     exit 1  appdir: unknown kind "template"
ErrLayoutTooNew    exit 5  home $ROOT/repo-new/.doctor has layout 2, this binary knows 1; upgrade doctor
== invalid options
app name         exit 2  appdir: app name "Doctor" must match ^[a-z0-9][a-z0-9._-]*$
dir name         exit 2  appdir: dir name ".." must be one plain path segment
kind path        exit 2  appdir: kind "tpl": path "../x" has an empty, . or .. segment
release profile  exit 2  appdir: profile "staging" is not in the list [prod dev]
nil seam         exit 2  appdir: a seam option was given nil
```

## Result types

All JSON names are the struct tags in the code. A field marked "omitted when empty" has `omitempty`.

### Entry

One kind's resolved path, from `Paths()`.

| Field | JSON | Meaning |
|---|---|---|
| `Kind` | `kind` | The kind name (`db`, `cache`, or an author's kind). |
| `Home` | `home` | `global` or `project`: the home the path is in. |
| `Path` | `path` | Absolute path; empty when the kind has no usable home. |
| `Type` | `type` | `file` or `folder`. |
| `Git` | `git` | `commit` or `ignore`. |
| `Rule` | `rule` | The [rule](#rules) that picked the path, or the one that failed. |
| `Fallback` | `fallback` | True for a temp fallback, a `project-else-global` kind in the global home, or a home with no `$HOME`. |
| `Note` | `note` | Omitted when empty. Why this path, for example `from X_CACHE_DIR`, `db: pointer`, `temp fallback`. |
| `Error` | `error` | Omitted when empty. Why the kind has no path, for example `no .doctor/ here; run 'doctor init' or pass --home`. |

### Home and HomeInfo

`Global()` and `Project()` return a `Home`.

| Field | JSON | Meaning |
|---|---|---|
| `Scope` | `scope` | `global` or `project`. |
| `Marker` | `marker` | Omitted when empty. The `.<app>/` folder the lookup found (project only). |
| `Base` | `base` | Holds `home.jsonc` and the config files. Differs from `Marker` with a `home:` pointer. |
| `Data` | `data` | Holds every other kind: `Base`, or `Base/profiles/<profile>` for a non-release profile's project home. |
| `Rule` | `rule` | The [rule](#rules) that picked the home. |
| `Fallback` | `fallback` | True when there is no home folder (rule `defaults`). |
| `Note` | `note` | Omitted when empty, for example `from --home`, `home: pointer in <file>`. |
| `Info` | `info` | Omitted when `home.jsonc` is absent. Its parsed content, a `HomeInfo`. |

`HomeInfo` is `home.jsonc`: `App` (`app`), `Layout` (`layout`, an int) and `Scope` (`scope`).

### ConfigFile

One config file, from `ConfigFiles()`, in read order.

| Field | JSON | Meaning |
|---|---|---|
| `Path` | `path` | Absolute path; the file may not exist. |
| `Home` | `home` | `global` or `project`. |
| `Local` | `local` | True for `config.local.jsonc` (this machine only). |
| `Committed` | `committed` | True for a project `config.jsonc` inside a git work tree. |
| `Profile` | `profile` | Omitted when empty. Set on `.<app>/profiles/<profile>/config.local.jsonc`, the last file a non-release profile reads. |

From the demo, run with `--init --json` in a dev build:

```json
{
  "path": "$WORK/.demo/profiles/dev/config.local.jsonc",
  "home": "project",
  "local": true,
  "committed": false,
  "profile": "dev"
}
```

### Socket

From `Socket()` and `EnsureSocketDir()`; see [Run files and sockets](../runtime/run-files-and-sockets.md).

| Field | JSON | Meaning |
|---|---|---|
| `Path` | `path` | Where the daemon listens: `run/<app>.sock`, a fallback socket, or a Windows named pipe. |
| `Fallback` | `fallback` | True when `Path` is not under `run/`; the daemon then writes `Path` into `PointerFile`. Always true on Windows. |
| `PointerFile` | `pointer_file` | `run/socket`. |
| `Dir` | `dir` | The folder to create; empty for a Windows pipe. |

### Warning

From `Warnings()`: `Code` (`code`, a [warning code](#warning-codes)) and `Message` (`message`, a full sentence).

### MoveResult

From `MoveLegacy`; see [Legacy move](../runtime/legacy-move.md).

| Field | JSON | Meaning |
|---|---|---|
| `Status` | `status` | `none`, `moved`, `both-exist`, `skipped` or `already`. |
| `From` | `from` | Omitted when empty. The old home. |
| `To` | `to` | Omitted when empty. The new home. |
| `Note` | `note` | Omitted when empty, for example `profile dev never runs a legacy move`. |

### KindSpec

From `Kinds()`: `Name` (`name`), `Path` (`path`, slash-separated, relative to the home), `Type` (`type`), `Git` (`git`), `Scope` (`scope`), `Disposable` (`disposable`), `Anchored` (`anchored`). See [Kinds](../layout/kinds.md).

## Rules

Every `Entry` and `Home` records the rule that picked it.

| Rule | When it appears |
|---|---|
| `flag` | `--home` was given (`WithHome`): every kind is in that one folder. |
| `env-home` | `<APP>_HOME` is set (and, for a non-release profile, does not point into the release profile's home). |
| `env-kind` | `<APP>_<KIND>_DIR` is set for a kind that is not anchored. Also the rule of an entry whose variable failed to expand. |
| `project` | The lookup found `.<app>/`. Also the rule of a project kind with no project home (with `Error` set). |
| `worktree` | Inside a linked git worktree: the main checkout's `.<app>/`. |
| `pointer` | The project home's `home:` key moved the base (`Home.Rule`), or its `db:` key moved the database (the `db` entry). |
| `global` | `~/.<app>` (or `~/.<app>-<profile>`). Also the rule of a global kind with no home folder. |
| `temp` | A disposable kind fell back to `<tmp>/<app>-<uid>/` because there is no home folder. `Paths()` does not show the read-only fallback that `Ensure` may take. |
| `defaults` | No home folder: an anchored global kind (`home.jsonc`, the config files) has no path, so the global config is the code defaults. |

## Warning codes

`Warnings()` is filled once by `New`. Print each one once, to stderr (with `--json`, stdout carries only the envelope); `doctor` lists them.

| Code | When it appears | Message (real) |
|---|---|---|
| `profile-env-ignored` | `<APP>_PROFILE` names the release profile in a dev build without `<APP>_ALLOW_PROD=1`, or another profile in a release build. | `DOCTOR_PROFILE=dev ignored by a release build; pass --profile dev` |
| `env-points-to-prod` | A non-release profile got `<APP>_HOME` or `<APP>_<KIND>_DIR` pointing into the release profile's global home; the variable is ignored. | `X_CACHE_DIR points into the prod home $TMP/user/.x; ignored for profile dev` |
| `pointer-chain-ignored` | A `home:` target has its own `home:` or `db:` key; pointers go one hop only. | `<key> in <file> not followed: pointers go one hop only` |
| `pointer-ignored-for-profile` | A `db:` pointer in a non-release profile; the profile's database stays in its own data folder (or `<APP>_DB_DIR`), and the message names that path. | `db: pointer in $TMP/proj/.x/config.local.jsonc ignored for profile dev; the dev database stays at $TMP/proj/.x/profiles/dev/x.db` |
| `ceiling-relative` | An `<APP>_CEILING_DIRS` entry is relative; it is ignored, as git does. | `DOCTOR_CEILING_DIRS entry "relative/dir" is relative; ignored` |
| `ceiling-dropped` | An `<APP>_CEILING_DIRS` entry starts with `~` or `$` but does not expand to an absolute path (`$VAR` without braces, an unset `${VAR}`, `~user`); it is ignored. | `DOCTOR_CEILING_DIRS entry "$ROOT/x" is not an absolute path (only ${VAR} expands, not $VAR); ignored` |
| `worktree-main-missing` | Inside a linked git worktree whose main checkout has no `.<app>/`, when the worktree has its own, which is used. No warning when neither has one. | `linked worktree of <main>, but the main checkout has no .<app>; using the worktree` |

## Pitfalls

- `Paths()` never returns an error. A kind with no usable home has an empty `path` and an `error` field; check `Error` when you print, and use `Path`/`Ensure` (which return the error) in commands that need the path.
- Under `--home` or `<APP>_HOME`, `both` kinds (`home`, `config`, `config-local`) appear once, with `home: "global"` (as in `ConfigFiles()`), not once per home: there is only one home. Project kinds such as `db` still say `home: "project"`; every line has the same folder and rule `flag` or `env-home`.
- `ConfigFiles()` lists files that do not exist. `doctor` should say which are present (the example stats each one); `paths` does not need to.
- `Committed` is true only when the project home is inside a git work tree; a project `config.jsonc` outside git is reported as not committed.
- `Warnings()` belongs to one `New` call. `ForProfile` and a second `New` have their own list; print the warnings of the `Dirs` the command uses, once.
- `MoveResult.From` and `To` are omitted when empty (for example status `none` with no legacy home configured), so a JSON reader must treat them as optional.

## Related

- [Errors and exit codes](errors.md)
- [Options reference](options.md)
- [Profiles](../profiles/profiles.md) (where most warnings come from)
- [Child processes](../profiles/child-processes.md)
- [Gitignore block](../layout/gitignore.md)
- [Config files](../layout/config-files.md)
- [The doctor example](../../examples/doctor/README.md), [the demo](../../examples/demo/README.md)
- SPEC sections 3.4 and 11, [docs/SPEC.md](../SPEC.md#11-json-for-paths---json)
