# Platforms

appdir runs on macOS, Linux and Windows and uses only the Go standard library: `go.mod` has no requirements, and the few OS calls it needs (file locks, ACLs, volume serials, file attributes) go through `syscall`. This page maps each rule of the standard to what appdir does on each OS and to the file that does it, so you know what to expect before you ship a tool on Windows.

Files ending in `_unix.go` build on macOS, Linux and the BSDs; `_windows.go` on Windows; `_darwin.go` on macOS only; `_other.go` everywhere else.

## Platform table

| Standard rule | macOS and Linux | Windows | File |
|---|---|---|---|
| Global home | `~/.<app>` from `os.UserHomeDir` (`$HOME`) | `%USERPROFILE%\.<app>`, same call | `resolve.go` |
| Owner-only folders and files | mode bits `0700` / `0600` (`WithPerm`) | a protected ACL granting full access to the current user and SYSTEM only, inherited by children of a folder; the mode bits are ignored | `perm_unix.go`, `perm_windows.go` |
| Stop the lookup at a disk change | device id from `stat`; a missing folder has none, so the walk climbs on | volume serial number of the folder; a missing folder has none, as on Unix; a folder that exists but cannot be opened gets its volume's serial from `GetVolumeInformationW` | `sys_unix.go`, `sys_windows.go` |
| Locks (legacy move, daemon probe) | `flock`, non-blocking | `LockFileEx`, fail immediately | `internal/flock` |
| Atomic write of `home.jsonc` and `.gitignore` | temp file in the same folder, fsync, rename | same, and the rename retries up to 5 times when antivirus or the indexer holds the target (sharing violation, access denied) | `atomic.go`, `rename_other.go`, `rename_windows.go` |
| Socket | `run/<app>.sock` when the path fits in `WithSocketMaxLen` (100 bytes); else `$XDG_RUNTIME_DIR/<name>/` on Linux when set, or `/tmp/<name>-<uid>/` (literally `/tmp`, because macOS's `$TMPDIR` is too long) | always a named pipe `\\.\pipe\<name>-<user>-<hash>`; no folder, `Dir` is empty | `run.go`, `socket_unix.go`, `socket_windows.go` |
| Symlinks | checked by `VerifyFile` | symlinks and directory junctions are followed in the same check (appdir resolves junctions itself, since Go 1.23+ `filepath.EvalSymlinks` no longer does); appdir never makes links of its own | `symlink.go`, `links_windows.go` |
| Long paths | not an issue | Go's `os` package adds the `\\?\` prefix to absolute paths, and appdir only passes absolute paths | `resolve.go` |
| Cache exclusions | `CACHEDIR.TAG`; on macOS also `.metadata_never_index` and `tmutil addexclusion` (best effort, 5 second limit) | `CACHEDIR.TAG` and the "not content indexed" attribute | `cachetag.go`, `cachetag_darwin.go`, `cachetag_other.go`, `cachetag_windows.go` |
| Root and sudo guard | effective uid 0 writing into a folder owned by another uid fails with `ErrRootOwned` unless `WithAllowRoot` | no check: an elevated process is the same user | `ensure.go`, `sys_unix.go`, `sys_windows.go` |
| Temp and socket fallback folder | must not be a symlink, must be a folder, must be owned by the effective uid | must not be a symlink and must be a folder; no owner check, because `%TEMP%` is per user | `ensure.go`, `sys_unix.go`, `sys_windows.go` |
| User in folder names | numeric uid (`/tmp/lore-1000`) | the user name without the domain, keeping only ASCII letters, digits, `.`, `-` and `_` (`user` when nothing is left) | `sys_unix.go`, `sys_windows.go` |
| Network or synced folder check | not done | not done | the database library, not appdir |
| Service install | not done | not done | the tool; `Name` and `NameWith` give the labels |

`<name>` is `Name(app)`: the app name for the release profile, `<app>-<profile>` for another one. `<hash>` is the first 12 hex characters of the SHA-256 of the real path of `run/`, so two homes of one app never share a socket ([platformSocket on Unix](ds:block?id=dirs-platformsocket-5kfqqv9a), [on Windows](ds:block?id=dirs-platformsocket-cj45m675)).

## Proven on real OSes

[Run 43378905306](https://github.com/ubgo/appdir/actions/runs/43378905306) of the manual [real-OS test workflow](../testing/ci.md) (commit `fc6202e`) passed on ubuntu-latest, windows-latest, macos-latest (Apple silicon) and macos-15-intel. Its proof step lists every platform test as PASS, none skipped. "All four" below means all four runners.

| Row | Proven on | Tests |
|---|---|---|
| Global home | all four; `%USERPROFILE%` on Windows | the whole suite; `TestWinUserProfileIsHome` |
| Owner-only folders and files | all four: mode bits on Linux and macOS, the protected ACL (user and SYSTEM only) on Windows | the layout tests' mode checks; `TestWinOwnerOnlyACL` |
| Stop the lookup at a disk change | all four; on Windows also a missing start folder, a drive letter in the other case, and worktree-only subfolders | `TestWinDeviceIDMissingPath`, `TestWinDriveLetterCase`, `TestResolveWorktreeMatrix`, `TestLegacyXDisk_Move` |
| Locks | all four, between two real processes | `TestTryLockAcrossProcesses` |
| Atomic write | all four, over an existing file (MoveFileEx with replace on Windows) | `TestAtomicWriteOverExisting`, `TestWinAtomicWriteReplacesExisting` |
| Socket | named pipe on Windows; literal `/tmp` fallback and a real listener in `run/` on macOS; `$XDG_RUNTIME_DIR` fallback on Linux | `TestWinNamedPipeShape`, `TestDarwinSocketFallbackLiteralTmp`, `TestDarwinSocketListensWhereItFits`, the runtime socket tests |
| Symlinks | macOS `/var` vs `/private/var` for the walk-up, the `$HOME` wall and ceilings | `TestDarwinPrivateVarWalk`, `TestDarwinCeilingOtherSpelling` |
| Temp and socket fallback folder | all four, including `ErrUnsafeTemp` for a file in the way | `TestLayoutTempFallback`, `TestForTestPointsTempIntoRoot`, `TestDarwinForTestShortRoot` |
| User in folder names | Windows user name in the pipe and temp folder names | `TestWinNamedPipeShape`, `TestLayoutTempFallback` |
| Legacy move across disks | all four, onto a second filesystem (tmpfs, RAM disk, `D:`), including crash recovery | `TestLegacyXDisk_Move`, `TestLegacyXDisk_CrashRecovery` |
| No service install (appdir starts no process) | all four | `TestNoProcessStart` |

Rows with tests added in v0.1.2 (see the [promise table](../testing/promises.md)); they run in the same workflow:

- The atomic-write retry while another handle holds the target: `TestWinAtomicWriteRetriesSharingViolation`, `TestWinPDRenameRetriesWhileTargetHeld`.
- Windows directory junctions in the symlink check, made with `mklink /J`: `TestWinJunctionEscape`.
- Cache exclusions read back: the macOS `.metadata_never_index` file (`TestDarwinCacheIndexMarkers`, `TestDarwinPDCacheNeverIndex`) and the Windows "not content indexed" attribute (`TestWinCacheIndexAttrOnlyOnTaggedCache`, `TestWinPDCacheNotContentIndexed`).
- The root guard on Linux without real root, from inside a user namespace where the test is uid 0: `TestPromiseA_RootOwned` (it skips where user namespaces are off).

Rows still unproven, and why:

- Long paths over 260 characters on Windows: no test builds one.
- The macOS `tmutil` Time Machine exclusion: best effort by design, so the test logs it but does not require it.
- The root and sudo guard on macOS and Windows: the runners do not run tests as root, so those cases skip (on Windows the guard is a no-op, which `TestWinNoRootGuard` checks).
- Network folder check and service install: appdir does not do these.

## What differs on Windows

- **Permissions are an ACL.** `WithPerm` has no effect; every folder and file appdir creates gets a protected DACL with two entries (the current user's SID and SYSTEM, full access), and inheritance from the parent is turned off. See [Files and permissions](../layout/files-and-permissions.md).
- **The socket is a named pipe.** `Socket()` always returns `Fallback: true` and an empty `Dir`. The daemon still writes the pipe name into `run/socket`, so clients find it the same way on every OS. See [Run files and sockets](../runtime/run-files-and-sockets.md).
- **The disk-change stop rule uses the volume serial number** instead of a device id.
- **A legacy move cannot rename an old home while a file in it is open.** Windows refuses to rename a folder holding an open file, so `MoveLegacy` returns the rename error (access denied) and the next run tries again; on Unix the rename succeeds. List the old daemon's lock files in `DaemonLocks` so a running daemon is reported as `ErrBusy` first.
- **There is no root guard.** `ErrRootOwned` is never returned and `WithAllowRoot` changes nothing.
- **The temp fallback has no owner check.** `ErrUnsafeTemp` is returned only for a symlink or a non-folder.

## Pitfalls

- On Windows the socket is a named pipe: `Socket().Dir` is empty and there is no folder to create for it. `EnsureSocketDir` still creates `run/`, where the daemon writes `run/socket`, the same as on Unix.
- On Linux the socket fallback goes to `$XDG_RUNTIME_DIR/<name>/` only when that variable holds an absolute path to an existing folder; otherwise to `/tmp/<name>-<uid>/`. In tests, give `XDG_RUNTIME_DIR` a temp folder with `WithEnv`.
- On macOS, `tmutil` failing or missing never blocks creating a cache folder; the exclusion is best effort.

## Related

- [Options reference](options.md)
- [Errors and exit codes](errors.md)
- [Fallbacks](../homes/fallbacks.md)
- [Files and permissions](../layout/files-and-permissions.md)
- SPEC section 12, [docs/SPEC.md](../SPEC.md#12-platform-table-mapped-to-code)
