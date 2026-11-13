# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- A `--home` or `<APP>_HOME` that cannot be expanded (an unset `${VAR}`, `~user`) now fails every kind with its error; the temp fallback and code defaults apply only when there is no user home and no override (F2).
- `WithDirName`, `WithRunFile` names and `LegacyHome.Dir` must be one plain path segment; `.`, `..`, separators and NUL are `ErrInvalidOption`, so a home can no longer land above the user's home (F3).
- `CheckDB` builds the prod view with only the `<APP>_*` variables removed, so a prod `db:` pointer written with `${VAR}` is recognised; a failing project view no longer skips the global db (F4).
- Warning `worktree-main-missing` is only added when the worktree has a home of its own (F5).
- An entry named `.git` of any kind, including a dangling symlink, is a repo wall for the lookup (F7).
- `EnsureSocketDir` creates `run/` on every OS and for the socket fallback, where the daemon writes `run/socket` (F8).
- A broken pointer fails every project-capable kind (`logs` and the project config files too) instead of falling back to the global home; `ConfigFiles` still lists the marker's own files (F11).
- A `WithGitIgnore` pattern equal to a committed kind is no longer also written as a plain line (F12).
- Warning `pointer-ignored-for-profile` names the path the profile's database really uses (F13).
- `InitProject` in `$HOME` is refused before `~/.<app>` exists, instead of creating the global home with `scope: project` (F14).
- `ErrLayoutTooNew` names the folder that holds `home.jsonc`, not a profile's data folder (F15).
- A walk that starts in `$HOME` stops there instead of climbing above it (F17).
- Under `--home` or `<APP>_HOME`, `Paths` lists `both` kinds once labelled `global`, like `ConfigFiles` (X1).
- A walk that starts in a `<APP>_CEILING_DIRS` folder stops there (X2).
- In a test binary, `Socket` and `EnsureSocketDir` check the socket fallback folder against the test root before it can be created (X3).
- `CheckDB` recognises the prod db when the prod home does not exist yet (BX1).
- `MoveLegacy` finishes a cross-disk move that crashed after the old folder was renamed, instead of reporting `none` forever (BX2).
- A folder named `..x` inside a home counts as inside for the symlink rule and `Banner` (BX3).
- In a test binary with no `APPDIR_TEST_ROOT` and `TMPDIR` unset, `Socket` and `EnsureSocketDir` still refuse the shared `/tmp/<app>-<uid>` socket fallback folder, which the default root `/tmp` used to contain (L1).
- `InitProject` and `InitHome` on a `home.jsonc` that cannot be read fail with the new sentinel `ErrBadHomeFile` (the reader's error stays wrapped) and name the file once instead of three times (D6).
- An `<APP>_CEILING_DIRS` entry that does not expand to an absolute path (`$VAR` without braces, an unset `${VAR}`, `~user`) is no longer dropped silently: it gets the new warning `ceiling-dropped` (`WarnCeilingDropped`) (D5).
- With `WithoutKind(KindHome)` a `home:` pointer no longer always fails with `ErrPointerNotHome`: the target is recognised by its folder name `.<app>`, and any other folder is still refused (D3).
- The internal JSONC reader rejects a top-level `null` (J1) and a backslash before a raw newline in a string (J2).
- A `WithGitIgnore` pattern that equals only a generated re-include line is written after the re-includes, so it wins (TL1).
- `InitProject` resolves a relative folder against `WithWorkDir` (TL2).
- Init creates the parent folder of a `home` kind moved to a nested path (TL3).
- Init refuses a `home.jsonc` it cannot read instead of replacing it (TL4).
- With `WithoutKind(KindHome)`, init only creates the folder (TL5).
- `ConfigFiles` leaves out a dropped `config` or `config-local` kind instead of listing the home folder (TR1).
- `ForTest` clears every `<APP>_*` variable it does not set itself, so `<APP>_DAEMON_PROFILE`, `<APP>_ALLOW_PROD`, `<APP>_CEILING_DIRS` and `<APP>_<KIND>_DIR` no longer leak from the shell into tests (T1).
- `CheckDB` resolves a relative path against `WithWorkDir`, not the process's current folder (T2).

### Changed

- `docs/SPEC.md` matches the code: every sentinel's message shape and every `ErrInvalidOption` case (3.3, 3.5), `MoveResult` `omitempty` tags, the `WithLegacyVerify` default (`nil`; the file-count and byte check always runs), here-only keeping the linked-worktree translation (4.4), the new stop rules, pointer failure, temp fallback, prod view, `EnsureSocketDir`, gitignore, init and legacy-move rules, and neutral example paths.
- Guides, examples and the reference no longer describe the fixed bugs as pitfalls; `examples/overrides`, `examples/worktree`, `examples/lookup` and `examples/daemon-paths` show the fixed behaviour.

### Documentation

- Learning path in `docs/README.md`: fourteen steps from the first call to a production setup, a "start from your goal" table, and an index of every page and example.
- Step-by-step guides under `docs/start`, `docs/homes`, `docs/pointers`, `docs/layout`, `docs/profiles`, `docs/runtime` and `docs/testing`, each with complete snippets taken from a tested example, the real output, and pitfalls traced to the code.
- Decision guides under `docs/choosing`: mode and scopes, finding and moving a home, profiles, and the options per app type (project CLI, global tool, daemon, desktop app, test suite).
- Reference pages: every option with its default (`docs/reference/options.md`), every sentinel error with a suggested exit code (`docs/reference/errors.md`), the `paths --json` and `doctor` result types, rules and warning codes (`docs/reference/paths-json.md`), and the platform table (`docs/reference/platforms.md`).
- Feature index (`docs/reference/features.md`): every feature with its guide section and the example that calls it.
- Sixteen runnable examples under `examples/`, each running in a temp folder with seams and checked by its own test (`go test ./examples/...`).
- Architecture diagram (`docs/assets/architecture.svg`), shown in the README and the learning path.
- Docs gate `task docs:check`: no hard-wrapped lines, every link and anchor resolves, every snippet matches its example region, banned words, and docsync drift between cited code and the docs.
- Deeper guides: every step of the lookup, global and project homes, overrides, worktrees, names and release-detection guides now shows its own snippet; new steps cover a relative `InitProject` folder and a broken `home.jsonc`, a broken `--home` or `<APP>_HOME`, `Ignore()` and unknown kinds, dropping the home file, `WithPerm` and `WithCacheTag(false)`, `VerifyFile`, a dropped config kind, `ConfigFiles` under a broken pointer, an empty local `home:`, and `CheckDB` with a relative path or a prod home that does not exist yet.
- New example cases for those steps in `examples/quickstart`, `project-init`, `lookup`, `worktree`, `overrides`, `custom-layout`, `config-files`, `pointers`, `profiles` and `prod-db-guard`; `examples/doctor` adds a table of option sets `New` refuses, `ErrUnknownKind`, and a `--home` run that lists a `both` kind once. The feature index links each of these features to its step and example, and the learning path, goal table and choosing guides point at the new steps.
