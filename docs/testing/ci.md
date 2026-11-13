# Real-OS test workflow

The tests pass on a Linux laptop, but a few promises only show on the real OS: the Windows ACL, `LockFileEx` between two processes, `MoveFileEx` over an existing file, drive letters, UNC paths, `USERPROFILE` and the named pipe; macOS's `/var` -> `/private/var` temp folders and its socket fallback; and a legacy move across two real filesystems. The workflow `.github/workflows/test.yml` runs them on GitHub's Linux, macOS (Apple silicon and Intel) and Windows runners.

## Running it by hand

The workflow has one trigger, `workflow_dispatch`: it never runs on a push, a pull request or a schedule (the owner's decision; Actions minutes are a budget). Start it from a machine where `gh` can write to the repo:

```sh
gh workflow run test.yml -R ubgo/appdir --ref main
gh run list -R ubgo/appdir --workflow test.yml
gh run view <run-id> -R ubgo/appdir --log-failed
```

or with the "Run workflow" button on the Actions tab. Do not add automatic triggers.

## What each job runs

Every job (ubuntu-latest, macos-latest, macos-15-intel, windows-latest; `fail-fast: false`, Go from `go.mod`):

1. `go vet ./...`
2. `go test -count=1 ./...`: the library, `internal/flock`, `internal/jsonclite` and every example. The OS-only tests live in `platform_windows_test.go` and `platform_darwin_test.go`; `internal/flock/flock_process_test.go` re-runs the test binary as a second process that holds the lock.
3. The race set of `task test:race:concurrency`: `go test -race` on `internal/flock` and on `-run 'Legacy|Lock|Concurren'` in the root package.
4. A second filesystem, then `go test -count=1 -run XDisk -v .`. The step fails when no `XDisk` test passed or one was skipped, so the log proves the cross-disk move ran.

## The second disk: APPDIR_TEST_SECOND_DISK

The cross-disk legacy move tests (`legacy_xdisk_test.go`) run only when `APPDIR_TEST_SECOND_DISK` names a folder on another filesystem than the temp folder; unset, they skip, so they never run on a laptop by accident. When it is set but the folder is on the same device, the test fails instead of skipping. The workflow provides one per OS:

| OS | Second filesystem | `APPDIR_TEST_SECOND_DISK` |
|---|---|---|
| Linux | `sudo mount -t tmpfs -o size=64m tmpfs /mnt/appdir2` | `/mnt/appdir2` |
| macOS | a 64 MB RAM disk: `hdiutil attach -nomount ram://131072`, then `diskutil erasevolume HFS+ appdir2 <device>` | `/Volumes/appdir2` |
| Windows | the runner's `D:` drive (the temp folder is on `C:`) | `D:\appdir2` |

To run them on a Linux machine, point the variable at a tmpfs such as `/dev/shm`:

```sh
APPDIR_TEST_SECOND_DISK=/dev/shm/appdir-xdisk go test -count=1 -run XDisk -v .
```

The tests create their own folder under that path and remove it afterwards. They pass `WithTestGuard(false)`, because the second disk is outside the guard's temp root; the home and environment seams still keep the real home out.

## Related

- [Testing setup](for-test.md)
- [Legacy move](../runtime/legacy-move.md)
- [Platforms](../reference/platforms.md)
