# Errors and exit codes

Every refusal appdir decides itself wraps one of 18 sentinel errors, so a tool tells them apart with `errors.Is`, and every message is a full sentence naming the path, key or variable it is about, so a user can act on it without the source. Mapping errors to exit codes is the tool's job; this page lists each sentinel, when it happens, its real message, and the exit code we suggest.

Errors that come from the OS or a parser (a folder that cannot be created, a `config.jsonc` that does not parse) and `MoveLegacy`'s copy and move failures wrap no sentinel; they name the paths and wrap the underlying error when there is one, so check them with `errors.Is(err, fs.ErrPermission)` and the like. The sentinels are the only package-level variables in appdir ([errors.go](ds:block?id=sentinel-errors-mwtzt4d9)). The message of a returned error does not repeat the sentinel's own text: `errors.Is(err, appdir.ErrNoProject)` is the check, `err.Error()` is what the user reads.

## Check for an error

The [doctor example](../../examples/doctor/main.go) maps every sentinel to an exit code in one function:

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

`errors.Is` works through any wrapping the tool adds (`fmt.Errorf("open db: %w", err)`). Some errors also come wrapped by appdir with the source in front, for example `X_CACHE_DIR: ${FAST} is not set` or `--home: ${X} is not set`.

## Suggested exit codes

The standard (section 11) fixes the set: 0 ok, 1 error, 2 usage, 3 not found, 4 locked or busy, 5 schema too new, 6 conflict. `exitCode` above implements this table.

| Sentinel | Exit | Why that code |
|---|---|---|
| [`ErrInvalidOption`](#errinvalidoption) | 2 | A bad value, usually from a flag, a variable or a config file. A bad option set in code is an author bug, but it is reported the same way. |
| [`ErrUnknownProfile`](#errunknownprofile) | 2 | `--profile` or `<APP>_PROFILE` names a profile not in the list. |
| [`ErrUnknownKind`](#errunknownkind) | 1 | A programming error: the tool asked for a kind it never declared. |
| [`ErrNoProject`](#errnoproject) | 3 | No project home was found. |
| [`ErrNoHome`](#errnohome) | 3 | No user home folder was found. |
| [`ErrEnvVar`](#errenvvar) | 2 | A `${VAR}` in a user path is not set. |
| [`ErrPointerAbsolute`](#errpointerabsolute) | 1 | A config file breaks the committed-file rule. |
| [`ErrPointerMissing`](#errpointermissing) | 3 | The pointer target does not exist. |
| [`ErrPointerNotHome`](#errpointernothome) | 3 | The pointer target is not a home of this app. |
| [`ErrLayoutTooNew`](#errlayouttoonew) | 5 | The home was written by a newer binary. |
| [`ErrRootOwned`](#errrootowned) | 1 | Refused for safety; the user passes `--allow-root`. |
| [`ErrSymlinkEscape`](#errsymlinkescape) | 1 | Refused for safety. |
| [`ErrUnsafeTemp`](#errunsafetemp) | 1 | Refused for safety. |
| [`ErrProdDB`](#errproddb) | 1 | Refused for safety; the user sets `<APP>_ALLOW_PROD=1`. |
| [`ErrBusy`](#errbusy) | 4 | A lock is held. |
| [`ErrGitignoreMarkers`](#errgitignoremarkers) | 6 | The file conflicts with what appdir would write. |
| [`ErrNotHome`](#errnothome) | 6 | The folder belongs to another app. |
| [`ErrBadHomeFile`](#errbadhomefile) | 6 | The existing `home.jsonc` is the user's and does not parse; appdir will not replace it. |

The messages below were captured from real runs. `$TMP` stands for the temp folder the run used, and `x` or `doctor` is the app name of that run.

## ErrInvalidOption

When: `New` validates the option set; a few methods check their arguments; a user path or pointer value cannot be read.

- `New`: bad app name, a `WithDirName` that is not one plain segment (empty, `.`, `..`, a separator or NUL), empty `WithEnvPrefix`, unknown mode or lookup, layout version below 1, socket max length below 1, a seam given `nil`, an empty profile list, a release or dev profile not in the list, a run file or legacy dir that is not one path segment, a kind name not matching `^[a-z][a-z0-9-]*$`, a kind path that could leave the home, an unknown scope.
- `PathIn` or `InitHome` with a scope other than global or project; `RunPath` with an unknown `RunFile`; `InitProject` on the global home.
- A user path (`--home`, `<APP>_HOME`, `<APP>_<KIND>_DIR`, a pointer, `InitHome`) in the `~user` form or with an unclosed `${`.
- A `home:` or `db:` pointer value that is not a string (`Project()` and every project kind).

Messages:

```text
appdir: app name "Bad" must match ^[a-z0-9][a-z0-9._-]*$
appdir: dir name ".." must be one plain path segment
appdir: kind "tpl": path "../x" has an empty, . or .. segment
appdir: kind "tpl": unknown scope "nope"
init: <marker> is the global home, not a project
"~bob/x": the ~user form is not supported
$TMP/proj/.x/config.jsonc: home must be a string path
```

The [doctor example](../../examples/doctor/main.go) calls `New` with five bad option sets and prints each refusal with its exit code. `New` checks the app name and the option set before it reads the home, the working folder or the environment, so a refused set has no side effects:

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

Its output (`go run ./examples/doctor`, last section):

```text
== invalid options
app name         exit 2  appdir: app name "Doctor" must match ^[a-z0-9][a-z0-9._-]*$
dir name         exit 2  appdir: dir name ".." must be one plain path segment
kind path        exit 2  appdir: kind "tpl": path "../x" has an empty, . or .. segment
release profile  exit 2  appdir: profile "staging" is not in the list [prod dev]
nil seam         exit 2  appdir: a seam option was given nil
```

## ErrUnknownProfile

When: `New` (and so `ForProfile`) gets a profile name not in the `WithProfiles` list from `--profile`, `<APP>_PROFILE`, `--daemon-profile` or `<APP>_DAEMON_PROFILE` ([pickProfile](ds:block?id=dirs-pickprofile-g7fmfemq)). The message names where the name came from.

```text
appdir: --profile: unknown profile "devv" (known: prod, dev)
appdir: X_PROFILE: unknown profile "devv" (known: prod, dev)
```

## ErrUnknownKind

When: `Path`, `PathIn` or `Ensure` is given a kind that is not in the layout (never declared, or dropped with `WithoutKind`). `New` does not fail for it, and `Paths()` and `Kinds()` simply do not list the kind.

The doctor example calls `d.Path("template")`, a typo of its own `templates` kind (the `ErrUnknownKind` case of `failures`, quoted in full in [paths --json and doctor, step 4](paths-json.md#step-4--map-errors-to-exit-codes)):

```text
ErrUnknownKind     exit 1  appdir: unknown kind "template"
```

Declare a `Kind` constant for every kind the tool adds (the example's `kindTemplates`) and pass the constant, never a string literal: the compiler then catches the typo this case shows.

## ErrNoProject

When: a project home is needed and the lookup found none: `Project()`, `Path`/`PathIn`/`Ensure` of a project-scoped kind, `RunPath`, `Socket` and `DaemonDiscovery` in project mode, `WriteGitignore`. `New` itself never fails for this.

```text
no .doctor/ here; run 'doctor init' or pass --home
```

## ErrNoHome

When: the user has no home folder (`os.UserHomeDir` fails, or `WithHomeDir` returns an error or a path that is not absolute) and something needs it: a global data kind, `Ensure` of an anchored global kind, or a `~` in a user path. Disposable kinds use the temp fallback instead; global config files use code defaults (rule `defaults`). Both apply only with no `--home`/`<APP>_HOME`: a broken override fails every kind with its own error.

```text
no home folder; set X_HOME
appdir: config has no path (no home folder; code defaults)
X_HOME: "~/data" needs a home folder
```

## ErrEnvVar

When: a `${VAR}` in a user path is unset or empty: `--home`, `<APP>_HOME`, `<APP>_<KIND>_DIR`, a pointer value, an `InitHome` path. The message names the variable, and appdir prefixes the source of the path ([expandUserPath](ds:block?id=config-expanduserpath-67kdzfc9)).

```text
X_CACHE_DIR: ${FAST} is not set
```

`--home` and `<APP>_HOME` give `--home: ${X} is not set` and `X_HOME: ${X} is not set`; a pointer gives `home in <file>: ${X} is not set`.

## ErrPointerAbsolute

When: the committed `config.jsonc` of a project home has a `home:` or `db:` value that is not portable: absolute, a drive or UNC path, `~user`, or starting with `${VAR}`. Only relative paths and `~/` are allowed there; anything else belongs in `config.local.jsonc`. Returned by `Project()` and every project kind.

```text
home: "/srv/doctor" in $ROOT/repo-ptr/.doctor/config.jsonc is absolute; put it in config.local.jsonc
```

## ErrPointerMissing

When: a `home:` pointer names a folder that does not exist, or a `db:` pointer's parent folder does not exist (release profile only).

```text
pointer points to $TMP/proj/missing, which does not exist; run 'x init --home $TMP/proj/missing'
pointer points to /abs/nodir, which does not exist
```

## ErrPointerNotHome

When: a `home:` pointer names a folder with no `home.jsonc`, or one whose `app` is another app. With `WithoutKind(KindHome)` there is no `home.jsonc`, and the target must be a folder named `.<app>`; any other name gives `pointer target $TMP/random is not a x home (without home.jsonc, a home is a folder named .x)`.

```text
pointer target $TMP/nothome is not a x home (no home.jsonc with app: x)
```

## ErrLayoutTooNew

When: before any write (`Ensure`, `InitProject`, `InitHome`), the home's `home.jsonc` says a layout above `WithLayoutVersion` ([checkWrite](ds:block?id=dirs-checkwrite-n23fhubq)). Reads still work, so `paths` and `doctor` can show the problem.

```text
home $ROOT/repo-new/.doctor has layout 2, this binary knows 1; upgrade doctor
```

The folder named is always the one holding `home.jsonc` (the home's `Base`), also when the write goes to a non-release profile's data folder (`.x/profiles/dev`) or a `db:` pointer folder.

## ErrRootOwned

When: before any write, the process runs as root (effective uid 0), `WithAllowRoot` is not set, and the nearest existing folder of the target belongs to another user: the `sudo lore ...` case. Unix only; on Windows the check does not apply.

```text
running as root but <folder> belongs to uid 1000; pass --allow-root
```

## ErrSymlinkEscape

When: `VerifyFile` finds a symlink whose real path is outside the real path of the home it belongs to, or a broken symlink. `Ensure` of a file kind and `CheckDB` call it. Inside is decided by whole path segments, so a file in a folder named `..x` inside the home counts as inside.

```text
$TMP/proj/.x/config.local.jsonc links to $TMP/outside.db, outside the home $TMP/proj/.x
$TMP/proj/.x/config.local.jsonc is a broken symlink
```

## ErrUnsafeTemp

When: a per-user folder in a shared place (the temp fallback `<tmp>/<app>-<uid>/`, or the socket fallback folder) is a symlink, not a folder, or not owned by the current user.

```text
$TMP/x-dev-1001 is a symlink or not a folder
<folder> is not owned by you
```

## ErrProdDB

When: `CheckDB` in a non-release build is given the path the release profile would open on this machine, and `<APP>_ALLOW_PROD=1` is not set. With it set, `CheckDB` returns `AccessReadOnly` and no error.

```text
dev build refuses the prod database $TMP/user/.x/x.db; set X_ALLOW_PROD=1 to open it read-only
```

## ErrBusy

When: `MoveLegacy` finds another process holding the move lock, or a daemon holding a lock in the old home. Retrying on the next run is the intended handling.

```text
another process is moving <old home>
old home $TMP/user/.old is in use by a running daemon; will retry next run
```

## ErrGitignoreMarkers

When: `WriteGitignore` (and `InitProject` inside a git work tree) finds the `# <app>:begin` / `# <app>:end` markers missing one side, doubled or out of order in `.<app>/.gitignore`. appdir never guesses; the file is left as is.

```text
$TMP/proj/.x/.gitignore: # x:begin / # x:end missing, doubled or out of order; fix it by hand
```

## ErrNotHome

When: `InitHome` or `InitProject` targets a folder whose `home.jsonc` names another app.

```text
$TMP/other holds a home of "y"
```

## ErrBadHomeFile

When: `InitProject` or `InitHome` finds a `home.jsonc` that exists but cannot be read or parsed (a bad hand edit, a merge conflict, no read permission). The file is the user's, so init refuses and leaves it untouched. The message names the file once; the reader's own error is wrapped too, so `errors.Is(err, fs.ErrPermission)` still works for an unreadable file.

```text
init: cannot read $TMP/proj/.x/home.jsonc; fix or remove it: jsonc: unexpected end of JSON input
```

## Try it

```sh
go run ./examples/doctor
go test ./examples/doctor/
```

The `== errors` section of the output provokes five sentinels the way a user hits them, and `== invalid options` the five `New` refusals above:

```text
== errors
ErrNoProject       exit 3  no .doctor/ here; run 'doctor init' or pass --home
ErrUnknownProfile  exit 2  resolve: appdir: --profile: unknown profile "staging" (known: prod, dev)
ErrPointerAbsolute exit 1  home: "/srv/doctor" in $ROOT/repo-ptr/.doctor/config.jsonc is absolute; put it in config.local.jsonc
ErrUnknownKind     exit 1  appdir: unknown kind "template"
ErrLayoutTooNew    exit 5  home $ROOT/repo-new/.doctor has layout 2, this binary knows 1; upgrade doctor
```

The test checks these lines and the exit code of every sentinel, each wrapped once with `%w` as a caller would.

## Pitfalls

- Compare with `errors.Is`, never with `==` or on the message: every error is wrapped at least once (by appdir with its source, often again by the tool, as `resolve:` above).
- The message does not contain the sentinel's own text (`invalid option`, `no project`). Print `err.Error()` to the user; log the sentinel name if you need it.
- `New` fails only for a bad option set (`ErrInvalidOption`) or an unknown profile (`ErrUnknownProfile`). A missing project, a broken pointer or a too-new layout surface later, from the method that needs the home, so map errors where those methods are called, not only after `New`.
- `Paths()` never returns an error; a broken kind has an empty `path` and the message in `Error`. A `paths` command exits 0 with broken lines unless the tool checks each `Entry.Error` itself.
- `ErrLayoutTooNew` is returned only before a write. A read-only command on a too-new home works; do not expect `Path` to report it.
- An `ErrInvalidOption` from `New` with a constant option set is an author bug and fails every run; cover it with one test that calls the tool's own constructor.

## Related

- [paths --json and doctor](paths-json.md)
- [Options reference](options.md)
- [Pointer keys](../pointers/pointer-keys.md)
- [Prod-db guard](../profiles/prod-db-guard.md)
- [The doctor example](../../examples/doctor/README.md)
- SPEC section 3.5, [docs/SPEC.md](../SPEC.md#35-errors)
