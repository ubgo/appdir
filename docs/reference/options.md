# Options reference

Every choice appdir makes has an option, and every option has the standard's default, so `appdir.New("lore")` with no options is already a tool on the standard. This page lists every `Option` and every `KindOption`, grouped by what they change, with the default as the code sets it and the guide that explains it in depth.

Options are applied in the order given, after the defaults. A setter given twice keeps the last value. `WithKind`, `WithoutKind`, `WithGitCommit`, `WithGitIgnore` and `WithLegacyHome` add to a list, and the kind edits are applied in order. `WithProfiles` replaces the list. `New` then validates the whole set and returns an error wrapping `ErrInvalidOption` for a bad one (see [Errors and exit codes](errors.md#errinvalidoption)).

All defaults below are set in one place, [`defaultConfig`](ds:block?id=defaultconfig-4tyrf8cd), with the numbers in [the defaults block](ds:block?id=default-constants-kpphgn26).

```go
type Option func(*config)
type KindOption func(*KindSpec)
```

## Identity and naming

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithDirName(name string)` | `"." + app` | The home folder name: global `~/<name>`, project `<name>/`. Must be one plain path segment: not empty, not `.` or `..`, no `/`, `\` or NUL; else `ErrInvalidOption` from `New`. | [Global and project homes](../homes/global-and-project.md) |
| `WithEnvPrefix(prefix string)` | the app name upper-cased, `-` and `.` turned into `_` (`my-app` gives `MY_APP`) | The prefix of every variable appdir reads: `<PREFIX>_HOME`, `<PREFIX>_PROFILE`, `<PREFIX>_<KIND>_DIR`, and so on. Must not be empty. | [Overrides](../homes/overrides.md) |

The app name itself is the first argument of `New` and must match `^[a-z0-9][a-z0-9._-]*$`.

## Lookup and mode

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithMode(m Mode)` | `ModeProject` | The default scope of data kinds: `ModeProject` puts `db`, `backups`, `run`, `shared` in the project home (and `logs` in the project home, else the global one); `ModeGlobal` puts them in the global home. Anything else is `ErrInvalidOption`. | [Global and project homes](../homes/global-and-project.md), [Kinds](../layout/kinds.md) |
| `WithLookup(l Lookup)` | `WalkUp` | How the project home is found: `WalkUp` checks the start folder and its parents up to the stop rules; `HereOnly` checks only the start folder. | [Project lookup](../homes/lookup.md) |

## Layout and git

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithKind(name Kind, opts ...KindOption)` | the [standard kinds](ds:block?id=standardkinds-ptkt92ta) | Adds a kind or changes an existing one in place. A new kind starts as `Folder(name)`, `Ignore()`, the mode's data scope. The name must match `^[a-z][a-z0-9-]*$`. | [Kinds](../layout/kinds.md) |
| `WithoutKind(name Kind)` | none | Drops a kind. Dropping a name that does not exist does nothing. | [Kinds](../layout/kinds.md) |
| `WithGitCommit(patterns ...string)` | none beyond the kinds marked `Commit()` | Extra patterns, relative to the home, re-included in the `.gitignore` block. | [Gitignore block](../layout/gitignore.md) |
| `WithGitIgnore(patterns ...string)` | none | Patterns forced out of git. They win over every commit rule, including a kind's `Commit()`. | [Gitignore block](../layout/gitignore.md) |

### Kind options

Used inside `WithKind`. They edit one `KindSpec`; the [build step](ds:block?id=buildkinds-r6jj6d5s) then validates the path and fills in the scope.

| KindOption | What it sets | Notes |
|---|---|---|
| `Folder(p string)` | `Path = p`, `Type = folder` | `p` is slash-separated and relative to the home: no leading `/`, no `\`, no `:`, no empty, `.` or `..` segment, and already clean. |
| `File(p string)` | `Path = p`, `Type = file` | Same path rules. `Ensure` creates the file's folder, not the file. |
| `Commit()` | `Git = commit` | Re-included in the gitignore block, unless `WithGitIgnore` names it. |
| `Ignore()` | `Git = ignore` | The default for a new kind. |
| `Scoped(s Scope)` | `Scope = s` | `ScopeGlobal`, `ScopeProject`, `ScopeProjectElseGlobal` or `ScopeBoth`; any other value is `ErrInvalidOption`. Left empty, the kind takes the mode's data scope. |
| `Disposable()` | `Disposable = true` | Safe to delete: gets `CACHEDIR.TAG` and may use the temp fallback. |
| `Anchored()` | `Anchored = true` | Lives in the home root of both profiles and has no `<APP>_<KIND>_DIR` variable. |

## Flags passthrough

These carry the tool's own command line flags into appdir. Empty or `false` means the flag was not given.

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithHome(path string)` | `""` | The `--home` flag: one home for every scope, rule `flag`. `~` and `${VAR}` are expanded, a relative path is taken from the working folder. | [Overrides](../homes/overrides.md) |
| `WithProfile(name string)` | `""` | The `--profile` flag. A name not in the profile list fails `New` with `ErrUnknownProfile`. | [Profiles](../profiles/profiles.md) |
| `WithDaemonProfile(name string)` | `""` | The `--daemon-profile` flag: which profile's daemon a client talks to. Checked against the list too. | [Run files and sockets](../runtime/run-files-and-sockets.md) |
| `WithAllowRoot(allow bool)` | `false` | The `--allow-root` flag: lets a process running as root write into a home owned by another user. | [Fallbacks](../homes/fallbacks.md) |

## Profiles and release

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithProfiles(names ...Profile)` | `prod`, `dev` | The allowed profiles. Replaces the list; an empty list is `ErrInvalidOption`. | [Profiles](../profiles/profiles.md) |
| `WithReleaseProfile(p Profile)` | `prod` | The profile of a release build, the one that keeps the base names (`~/.lore`). Must be in the list. | [Profiles](../profiles/profiles.md) |
| `WithDevProfile(p Profile)` | `dev` | The profile of a non-release build. Must be in the list. | [Profiles](../profiles/profiles.md) |
| `WithRelease(release bool)` | not set: appdir reads Go's build info | States whether this binary is a release build, from the tool's own `-ldflags -X` variable. `ReleaseReason()` is then `flag` (or its dev twin). | [Release detection](../profiles/release-detection.md) |

## Legacy move

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithLegacyHome(l LegacyHome)` | none | Names an old global home folder (`LegacyHome{Dir: ".pm"}`) that `MoveLegacy` moves from. `Dir` must be one path segment. `DaemonLocks` defaults to `["run/daemon.lock"]`. Repeatable; the first that exists is used. | [Legacy move](../runtime/legacy-move.md) |
| `WithLegacyVerify(fn LegacyVerify)` | none: only the file count and byte total of the copy are compared | An extra check after a cross-disk copy, before the old folder is renamed away, for example the tool's database integrity check. | [Legacy move](../runtime/legacy-move.md) |

```go
type LegacyHome struct {
    Dir         string   // folder name under the user's home, e.g. ".pm"
    DaemonLocks []string // lock files inside it to probe; default ["run/daemon.lock"]
}
type LegacyVerify func(src, dst string) error
```

## Config reader

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithConfigReader(fn ConfigReader)` | the internal JSONC reader | How `home.jsonc` and the `home:` / `db:` pointer keys are read. A missing file must return an error wrapping `fs.ErrNotExist`. `nil` is `ErrInvalidOption`. | [Config files](../layout/config-files.md) |
| `WithLayoutVersion(n int)` | `1` | The layout version this binary knows. A home whose `home.jsonc` says more is refused on write with `ErrLayoutTooNew`. Below 1 is `ErrInvalidOption`. | [Files and permissions](../layout/files-and-permissions.md) |

```go
type ConfigReader func(path string) (map[string]any, error)
```

## Run files and sockets

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithRunFile(f RunFile, name string)` | `lock`, `daemon.lock`, `<app>.pid`, `discovery.json`, `<app>.sock`, `socket` | Renames one file in `run/`. The name must be one path segment. | [Run files and sockets](../runtime/run-files-and-sockets.md) |
| `WithSocketMaxLen(n int)` | `100` bytes | The longest socket path kept under `run/`; a longer one moves to the per-user fallback folder. Below 1 is `ErrInvalidOption`. | [Run files and sockets](../runtime/run-files-and-sockets.md) |

The default names by `RunFile`: `RunLock` is `lock`, `RunDaemonLock` is `daemon.lock`, `RunPID` is `<app>.pid`, `RunDiscovery` is `discovery.json`, `RunSocket` is `<app>.sock`, `RunSocketFile` is `socket`.

## Permissions and fallbacks

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithPerm(dir, file fs.FileMode)` | `0700`, `0600` | The modes of folders and files appdir creates. On Windows the mode bits are ignored and appdir sets an owner-only ACL instead. | [Files and permissions](../layout/files-and-permissions.md) |
| `WithTempFallback(on bool)` | `true` | Lets disposable kinds (such as `cache`) fall back to `<tmp>/<app>-<uid>/` when there is no home folder, or when `Ensure` cannot create them because the home is read-only. | [Fallbacks](../homes/fallbacks.md) |
| `WithCacheTag(on bool)` | `true` | Writes `CACHEDIR.TAG` into disposable folders and adds the OS backup and index exclusions. | [Files and permissions](../layout/files-and-permissions.md) |

## Test guard

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithTestGuard(on bool)` | `true` | Inside a test binary, `New` panics when a path resolves outside the test root (`APPDIR_TEST_ROOT`, else the temp folder). | [Testing with ForTest](../testing/for-test.md) |

## Seams

Seams replace what appdir would read from the process, so tests and examples can run in a temp folder. Giving any of them `nil` is `ErrInvalidOption`.

| Option | Default | What it changes | Guide |
|---|---|---|---|
| `WithEnv(fn func(string) (string, bool))` | `os.LookupEnv` | Every variable lookup. A variable that is set but empty counts as unset. | [Testing with ForTest](../testing/for-test.md) |
| `WithHomeDir(fn func() (string, error))` | `os.UserHomeDir` | The user's home folder. An error, or a path that is empty or not absolute, means "no home folder". | [Fallbacks](../homes/fallbacks.md) |
| `WithWorkDir(dir string)` | `os.Getwd` | The folder the project lookup starts from. | [Project lookup](../homes/lookup.md) |
| `WithBuildInfo(fn func() (*debug.BuildInfo, bool))` | `debug.ReadBuildInfo` | The build info used for release detection when `WithRelease` is not given. | [Release detection](../profiles/release-detection.md) |

## Dirs is immutable and safe for concurrent use

`New` builds the option set once and never changes it, and every `*Dirs` method only reads it. Methods that return slices (`Paths`, `Kinds`, `Warnings`, `ConfigFiles`) return a fresh copy, so a caller can change the result without affecting other callers. One `*Dirs` can be shared by every goroutine of a tool.

appdir keeps no package-level mutable state: the only package-level variables are the sentinel errors, which are never reassigned, and the standard layout is built fresh on each call. Two `New` calls with different options in one process do not affect each other. To see another profile's paths, call `ForProfile`, which resolves a new `*Dirs` and leaves the first one alone.

## Related

- [Errors and exit codes](errors.md)
- [paths --json and doctor](paths-json.md)
- [Platforms](platforms.md)
- [Kinds](../layout/kinds.md)
- SPEC section 3.3, [docs/SPEC.md](../SPEC.md#33-options-every-one-with-its-default)
