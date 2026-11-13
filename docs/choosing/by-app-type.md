# Choosing options by app type

The defaults fit a command line tool that keeps its data in each repo. Other kinds of programs need a few options more. This page lists, per app type, the options to pass to `New`, the calls to make, and the guide and example for each. Every option not listed keeps its default ([Options reference](../reference/options.md)).

## At a glance

| App type | Mode | Must pass | Must call | Example |
|---|---|---|---|---|
| Project CLI (notes, tasks, an index per repo) | `ModeProject` (default) | `WithHome`, `WithProfile`, `WithRelease` when set | `InitProject` in `init`, `Ensure`, `CheckDB` | [quickstart](../../examples/quickstart), [project-init](../../examples/project-init) |
| Global tool (one store per user, works in any folder) | `ModeGlobal` | `WithMode`, `WithHome`, `WithProfile`, `WithRelease` when set | `Ensure`, `CheckDB` | [fallbacks](../../examples/fallbacks), [testing-setup](../../examples/testing-setup) |
| Daemon and its clients | `ModeGlobal`, or `ModeProject` with a global `run` kind | the above plus `WithDaemonProfile` | `EnsureSocketDir`, `RunPath`, `DaemonDiscovery`, `ChildEnv` | [daemon-paths](../../examples/daemon-paths) |
| Desktop app | `ModeGlobal` | `WithMode`, `WithRelease` when set, `WithLegacyHome` after a rename | `Name`/`NameWith`, `MoveLegacy`, `CheckDB` | [profiles](../../examples/profiles), [legacy-move](../../examples/legacy-move) |
| Test suite | the tool's own | the seams, or nothing after `ForTest` | `ForTest(t, app)` | [testing-setup](../../examples/testing-setup) |

## Project CLI

A tool like `lore` that keeps its database next to the code.

- Options: `WithHome(flagHome)` and `WithProfile(flagProfile)` from your flags (empty means not given); `WithRelease(release == "true")` only when your `-ldflags` variable is set ([Choosing profiles](profiles.md)). Add `WithKind` for your own folders ([Kinds](../layout/kinds.md)).
- Calls: `InitProject(dir)` in your `init` command, the only way a project home is made, which also writes the `.gitignore` block in a git work tree ([Init](../start/init.md)); `Ensure(kind)` before writing; `CheckDB(path)` before opening the database ([Prod-db guard](../profiles/prod-db-guard.md)); `Banner()` to stderr; `Warnings()` once.
- Errors: outside a project, data kinds fail with `ErrNoProject`; print it as is, it already says "run '<app> init' or pass --home" ([Errors](../reference/errors.md)).
- Ship `paths --json` and `doctor` from `Paths()`, `ConfigFiles()` and `Warnings()` ([paths --json and doctor](../reference/paths-json.md), [examples/doctor](../../examples/doctor)).
- Users with repos outside their home folder set `<APP>_CEILING_DIRS` ([Finding the home](finding-the-home.md#when-to-set-ceiling-dirs)).
- Before opening a file the user named inside a home (an import, an attachment), call `VerifyFile(path)` so a symlink out of the home is refused with `ErrSymlinkEscape` ([Files and permissions: VerifyFile](../layout/files-and-permissions.md#step-6--check-a-file-that-is-not-a-kind-with-verifyfile), [examples/custom-layout](../../examples/custom-layout)).

## Global tool

A tool like agx or a session manager: one store per user, run from any folder.

- Options: `WithMode(appdir.ModeGlobal)`, plus `WithHome`, `WithProfile` and `WithRelease` as above. The project home is still found for config files, so a project's `.<app>/config.jsonc` can override settings; nothing fails without one ([Mode and scopes](mode-and-scopes.md)).
- Calls: `Ensure`, `CheckDB`, `Banner`, `Warnings`. No `init` command is needed; `Ensure` creates `~/.<app>` on first use ([First use](../start/first-use.md)).
- Plan for no `$HOME` (containers, some CI): config kinds use code defaults (rule `defaults`), the database fails with `ErrNoHome`, the cache uses the temp folder. Pass `WithTempFallback(false)` if a cache in the temp folder is not acceptable ([Fallbacks](../homes/fallbacks.md), [examples/fallbacks](../../examples/fallbacks)).
- If the tool replaced an older one with another name, add `WithLegacyHome(appdir.LegacyHome{Dir: ".old"})` and call `MoveLegacy(ctx)` at start-up ([Legacy move](../runtime/legacy-move.md)).

## Daemon and its clients

One long-running process per profile, plus a CLI that talks to it.

- Options: `WithMode(appdir.ModeGlobal)` for a per-user daemon. A project CLI with one machine-wide daemon keeps `ModeProject` and adds `WithKind(appdir.KindRun, appdir.Scoped(appdir.ScopeGlobal))`, or `RunPath` and `Socket` fail with `ErrNoProject` outside a project. Clients pass `WithDaemonProfile(flagDaemonProfile)`. Rename run files with `WithRunFile` only if an older daemon used other names.
- Daemon calls: `EnsureSocketDir()` then listen on `Socket().Path`; when `Socket().Fallback` is true, write that path to `Socket().PointerFile` (`run/socket`) so clients can find it. Hold `RunPath(appdir.RunDaemonLock)` for the daemon's life and write `RunPath(appdir.RunDiscovery)` ([Run files and sockets](../runtime/run-files-and-sockets.md)).
- Client calls: `DaemonDiscovery()` gives the discovery file of the daemon profile (`--daemon-profile`, then `<APP>_DAEMON_PROFILE`, then its own). Read only that file and the socket it names ([Choosing profiles](profiles.md#decision-table-which-profile-call)).
- Child processes: start them with `ChildEnv(os.Environ())` so they keep the profile ([Child processes](../profiles/child-processes.md)).
- Socket paths longer than `WithSocketMaxLen` (100 bytes) move to `$XDG_RUNTIME_DIR` on Linux or `/tmp/<name>-<uid>/`; on Windows the socket is always a named pipe ([Platforms](../reference/platforms.md)).

## Desktop app

An app started from a launcher or a file manager: its working folder means nothing.

- Options: `WithMode(appdir.ModeGlobal)`. There is usually no `--home` flag; users can still set `<APP>_HOME`. Pass `WithRelease` from your build script for every packaged build: a script-built binary records `vcs.revision` and is dev otherwise ([Release detection](../profiles/release-detection.md)).
- Calls: `Name` and `NameWith` for the bundle id, keychain items, launch agents and pipe names, so a dev build can run next to the installed app ([Names and banner](../profiles/names.md)); `CheckDB` before opening the database; `MoveLegacy` after a rename.
- The working folder is still read for the project lookup; with `ModeGlobal` nothing depends on it except project config files. If you want none at all, pass `WithLookup(appdir.HereOnly)` and `WithWorkDir` set to the user's home folder: the only candidate is then `~/.<app>`, and the global home is never taken as a project home.

## Test suite

- Simplest: call `appdir.ForTest(t, app)` at the top of a test. It points `HOME`, `<APP>_HOME`, the XDG variables, the temp folder (`TMPDIR`, `TMP`, `TEMP`) and the agent config folder variables the standard lists (see SPEC section 10) at a temp root, unsets every other `<APP>_*` variable and the leaking `GIT_*` variables, and returns the root ([Testing setup](../testing/for-test.md#step-3--or-isolate-the-whole-process-with-fortest)).
- In parallel tests (`ForTest` uses `t.Setenv`, which a parallel test cannot call), or when you cannot change the process environment, pass the seams instead: `WithEnv`, `WithHomeDir`, `WithWorkDir`, and `WithBuildInfo` or `WithRelease` ([Testing: seams](../testing/for-test.md#step-2--replace-every-outside-input-with-seams)).
- Leave the guard on: in a test binary `New` panics when a path lands outside the test root. Turn it off with `WithTestGuard(false)` only for a test that checks real paths on purpose ([Testing: the guard](../testing/for-test.md#step-4--let-the-guard-catch-a-forgotten-isolation)).
- A test binary is a dev build. Pass `WithRelease(true)` when a test needs the plain folder names.

## Pitfalls

- **Forgetting `ModeGlobal` in a global tool** shows up as `ErrNoProject` the first time a user runs it outside a repo. Run your tool once from an empty folder before you ship.
- **Passing `WithRelease(false)` always** makes every `go install module@vX` a dev build; pass it only when your release variable is set.
- **`New` fails on bad input, not on a broken home.** An invalid app name, mode, profile list, kind path or run-file name is `ErrInvalidOption` from `New`, and an unknown `--profile` or `<APP>_PROFILE` is `ErrUnknownProfile`; map both to a usage exit code ([Errors: ErrInvalidOption](../reference/errors.md#errinvalidoption), [examples/doctor](../../examples/doctor)). A broken `--home` or `<APP>_HOME` (an unset `${VAR}`) does not fail `New`: every `Path` then returns `ErrEnvVar`, so check one kind at start-up and print the error before doing work ([Overrides: report a broken override](../homes/overrides.md#step-6--report-a-broken-override), [examples/overrides](../../examples/overrides)).
- **A kind you dropped is not an empty path.** After `WithoutKind`, `Path`, `PathIn` and `Ensure` return `ErrUnknownKind`; a shared helper that asks for every standard kind must skip the ones your tool dropped ([Errors: ErrUnknownKind](../reference/errors.md#errunknownkind)).
- **`init <dir>` with a relative folder is joined to `WithWorkDir`.** In a project CLI that takes a folder argument, pass the same working folder to `New` that the user's shell had, or pass an absolute path to `InitProject` ([Init: relative folder](../start/init.md#step-6--handle-a-relative-folder-and-a-broken-homejsonc), [examples/project-init](../../examples/project-init)).
- **One `Dirs` per process is enough.** `Dirs` is immutable and safe for concurrent use ([Options: Dirs is immutable](../reference/options.md#dirs-is-immutable-and-safe-for-concurrent-use)); resolve once at start-up and share it. Call `ForProfile` for another profile's paths instead of changing options.

## Related

- [Mode and scopes](mode-and-scopes.md), [Finding the home](finding-the-home.md), [Profiles](profiles.md).
- [Learning path](../README.md) and the [feature index](../reference/features.md).
