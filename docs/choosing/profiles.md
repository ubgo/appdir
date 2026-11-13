# Choosing how profiles work for your tool

A profile is a full set of homes. The release profile (`prod` by default) uses `~/.<app>` and `.<app>/`; every other profile uses `~/.<app>-<profile>` and `.<app>/profiles/<profile>/`, so a dev build never touches the installed tool's data. This page helps you decide how a binary becomes a release, whether you need your own profiles, and which profile calls (`ForProfile`, `DaemonProfile`, `CheckDB`) your tool should make.

## How the profile is picked

In order, first match wins ([pickProfile](ds:block?id=dirs-pickprofile-g7fmfemq), [Profiles: let appdir pick](../profiles/profiles.md#step-2--let-appdir-pick-the-profile)):

1. `--profile` (your flag, passed with `WithProfile`). Any known profile, in any build.
2. `<APP>_PROFILE`, with two limits: a dev build ignores the release profile unless `<APP>_ALLOW_PROD=1`, and a release build ignores every other profile. Both add warning `profile-env-ignored`.
3. The build: release build → the release profile; anything else → the dev profile.

An unknown name from the flag or the variable is `ErrUnknownProfile`, never a quiet fallback.

## Decision table: release flag or the go-install rule

Without `WithRelease`, appdir reads Go's build info: only a clean tagged version with no `vcs.revision` is a release, which only `go install module@vX` produces ([detectRelease](ds:block?id=detectrelease-7fwv9uc3), [Release detection](../profiles/release-detection.md)).

| How the binary is built | No `WithRelease` | `WithRelease(true)` from your `-ldflags -X` variable |
|---|---|---|
| `go install module@v1.4.2` by a user | release (`go-install`) | release (`flag`) |
| `go build` or `go install ./cmd/x` in a checkout, even on a tag | dev (`dev: vcs.revision` or `dev: (devel)`) | release (`flag`) |
| CI or goreleaser `go build` | dev (`dev: vcs.revision`) | release (`flag`) |
| `go run`, tests | dev | release (`flag`) |

## Rules

- If users install only with `go install module@vX`, you need no flag: the go-install rule decides.
- If you ship binaries from a script (CI, goreleaser, a Makefile), add a string variable set by `-ldflags "-X main.release=true"` and pass `appdir.WithRelease(release == "true")` only when the variable is not empty, the way [examples/demo](../../examples/demo) does. Passing `WithRelease(false)` always would turn a user's `go install module@vX` into a dev build: the flag always wins.
- If you have a staging or a second dev setup, add profiles with `WithProfiles(appdir.ProfileProd, appdir.ProfileDev, "staging")`; users pick them with `--profile staging`. `WithReleaseProfile` and `WithDevProfile` rename the two defaults; both must be in the list, or `New` fails with `ErrInvalidOption` ([Profiles: add your own](../profiles/profiles.md#step-3--add-your-own-profiles), [examples/profiles](../../examples/profiles)).
- If your tool names anything outside its folders (a service label, a keychain item, a pipe, a lock name), pass every name through `Name` or `NameWith`, and print `Banner()` to stderr at start-up ([Names and banner](../profiles/names.md)).
- If your tool starts child processes that use appdir, give them `ChildEnv(os.Environ())` so they run in the same profile ([Child processes](../profiles/child-processes.md#step-2--pass-the-profile-to-child-processes)).

## Decision table: which profile call

| You want to... | Call | Notes | Guide | Example |
|---|---|---|---|---|
| open or migrate a database | `CheckDB(path)` first | a dev build refuses the prod database with `ErrProdDB`; with `<APP>_ALLOW_PROD=1` it returns `AccessReadOnly`, so never migrate then; a release build is never refused | [Prod-db guard](../profiles/prod-db-guard.md) | [prod-db-guard](../../examples/prod-db-guard) |
| show another profile's paths (`doctor --profile prod`, a "where is prod" line) | `ForProfile(p)` | resolves again with the same options plus `--profile p`; reads only, opens nothing | [Child processes](../profiles/child-processes.md#step-2--pass-the-profile-to-child-processes) | [profiles](../../examples/profiles) |
| let a dev client talk to the prod daemon | `WithDaemonProfile(flag)`, then `DaemonDiscovery()` | `--daemon-profile`, then `<APP>_DAEMON_PROFILE`, then the own profile; the client reads only the discovery file and the socket it names, never the other profile's db | [Run files: another profile's daemon](../runtime/run-files-and-sockets.md#step-5--find-the-daemon-of-another-profile) | [daemon-paths](../../examples/daemon-paths) |
| run a child in a clean environment (a test helper, a sandbox) | `CleanEnv(environ, d.EnvPrefix())` | drops every `<PREFIX>_*` variable; pure function | [Child processes](../profiles/child-processes.md#step-2--pass-the-profile-to-child-processes) | [profiles](../../examples/profiles) |
| say why the binary is prod or dev | `IsRelease()`, `ReleaseReason()` | put both in `version` and `doctor` | [Release detection: report](../profiles/release-detection.md#step-2--report-the-decision) | [profiles](../../examples/profiles) |

## Pitfalls

- **The suffix follows the profile, not the build.** A release binary run with `--profile dev` uses `~/.<app>-dev` and `Name` gives `<app>-dev`; a dev build run with `--profile prod` uses the real homes. `CheckDB` still guards the second case, because it looks at the build.
- **`ForProfile` is not a way to open prod data.** It returns paths; a dev build that opens the prod database it names is stopped only by `CheckDB`. Call `CheckDB` on every database path, whatever produced it.
- **`CheckDB` compares against what an installed binary would open here.** It resolves the release profile with no `--home` and with every `<APP>_*` variable removed (other variables stay, so a prod `db:` pointer written with `${VAR}` is still recognised), then compares by file identity; when the file or its folder does not exist yet it compares the real path instead, so a dev build on a machine where the installed tool never ran is still refused. A prod database the installed binary would not find from this folder, such as a copy elsewhere or one the installed tool only finds through `<APP>_HOME`, is not refused ([Prod-db guard: relative path, no folders yet](../profiles/prod-db-guard.md#step-5--pass-a-relative-path-or-none-of-the-folders-yet), [examples/prod-db-guard](../../examples/prod-db-guard)).
- **Anchored files are shared by every profile.** `home.jsonc` and the project's `config.jsonc` and `config.local.jsonc` stay in `.<app>/`; a dev profile can add its own `profiles/<profile>/config.local.jsonc`, read last ([Config files: the dev profile's own file](../layout/config-files.md#step-2--the-dev-profiles-own-file)).
- **A relative path is checked against `WithWorkDir`, not the process.** `CheckDB(".demo/demo.db")` joins the name to the folder `New` resolved from, so pass the same `WithWorkDir` your tool opens files from; if the tool `chdir`s after `New`, pass `CheckDB` an absolute path.
- **Skipping `CheckDB` on a fresh machine is the dangerous case.** "No prod home yet, nothing to protect" is wrong: that first dev run would create the prod database with the dev schema. Call it every time.
- **`CheckDB` also runs the symlink rule.** A database path that is a symlink leading out of its home fails with `ErrSymlinkEscape` before any profile check, in a release build too.
- **`MoveLegacy` runs only in the release profile.** A dev build reports `skipped` ([Legacy move](../runtime/legacy-move.md#step-4--handle-the-cases-that-do-not-move)).

## Related

- [Profiles: prod, dev and your own](../profiles/profiles.md), [Release detection](../profiles/release-detection.md), [Prod-db guard](../profiles/prod-db-guard.md).
- Design: [SPEC 8 Profiles](../SPEC.md#8-profiles).
