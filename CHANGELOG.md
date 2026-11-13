# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Windows: the project walk-up no longer stops at the first existing parent when it starts in a folder that does not exist yet (a worktree-only subfolder looked up in the main checkout). A missing folder now has no volume id, as on Unix; a folder that cannot be opened gets its volume's real serial instead of a made-up one.
- Windows: `MoveLegacy` no longer replaces a file that sits where the new home goes (MoveFileEx replaced it with the old folder); like on Unix it fails and leaves both alone.
- `ForTest` also points `TMP` and `TEMP` into its root, so on Windows the temp fallback no longer lands in the real `%TEMP%`. On macOS its root is under `/tmp`, so a daemon test's socket fits in `run/` instead of falling back to the real `/tmp` and tripping the guard.
- `MoveLegacy` across disks now follows an old home that is itself a symlink (`~/.<old>` linked to another disk) and copies its contents. Before, the copy took the link alone: the new home became a link back to the old disk, nothing was moved, and the result still said `copied (other disk)`, so deleting the old data afterwards lost it.

### Added

- A manual GitHub Actions workflow (`.github/workflows/test.yml`) that runs vet, the tests, the race tests and a cross-disk legacy move on Linux, macOS (Apple silicon and Intel) and Windows; tests for the Windows ACL, cross-process locks, atomic replace, drive-letter and UNC paths, `USERPROFILE`, the named pipe, and the macOS `/private/var` temp folders and socket fallback.

## [0.1.0] - 2026-11-13

First release.

### Added

- `New(app, options...)` resolves every folder a Go tool needs, once, without reading or creating anything it was not asked to; a `*Dirs` is immutable and safe for concurrent use.
- Global home `~/.<app>` and project home `.<app>/`, found by walking up from the working folder; the walk stops at `$HOME`, at a repo wall (`.git`) and at `<APP>_CEILING_DIRS`; git worktrees share the main checkout's home.
- Modes for project tools, global tools and here-only lookups (`WithMode`, `WithLookup`), and overrides through `--home`, `<APP>_HOME` and `<APP>_<KIND>_DIR`, each path reported with the `Rule` that chose it.
- Kinds for the home file, config, local config, .gitignore, shared files, the database, backups, cache, run files and logs, with scopes (global, project, both, project-else-global); add, move or drop kinds with `WithKind` and `WithoutKind`.
- Prod and dev profiles: a dev build uses `~/.<app>-dev` and `profiles/dev/`, release detection from build info (`IsRelease`, `ReleaseReason`), extra profiles, and `CheckDB` so a dev build never opens the prod database.
- Pointer keys `home:` and `db:` in the project config to move a project's home or database elsewhere, with typed errors for a missing or wrong target.
- Lazy writes only when asked: `Ensure`, `InitProject`, `InitHome`, `WriteGitignore` and `EnsureSocketDir`, with folders `0700`, files `0600`, atomic writes and a cache tag.
- Run files and Unix sockets with a short per-user fallback when the path is too long (`Socket`, `RunPath`).
- One-time move of an old home to the new place (`MoveLegacy`), crash-safe across disks.
- `ForTest(t, app)` gives a test its own temp root, and a guard refuses any path outside it in a test binary.
- `Paths()` and `ConfigFiles()` for `paths --json` and `doctor` commands, a `Warnings()` list with stable codes, and 18 sentinel errors usable with `errors.Is`.
- Standard library only; macOS, Linux and Windows.
- Documentation: a fourteen-step learning path, step-by-step guides for every feature, decision guides, a feature index, reference pages, an architecture diagram, and sixteen runnable examples, each checked by its own test.
