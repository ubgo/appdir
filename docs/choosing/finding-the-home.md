# Choosing how the home is found and moved

appdir finds the project home by walking up from the working folder, and lets users and teams move homes in five ways. This page helps you pick the lookup style, tell users when to set ceilings, and pick the right way to move a home: who sets each one, whether it is committed, and which one wins.

## Decision table: the lookup style

| Your tool is run... | Lookup | Why |
|---|---|---|
| from anywhere inside a repo (a shell in `src/api/`) | `WalkUp` (the default) | finds the nearest `.<app>/` above, like git finds `.git` |
| by a script or a launcher that always starts in the project root | `WithLookup(appdir.HereOnly)` | a stray `.<app>/` higher up can never be picked; a missing one is `ErrNoProject` right away |
| as a global tool that never uses a project | either; `ModeGlobal` makes data global | the lookup still runs for config files, but nothing fails without a project ([Mode and scopes](mode-and-scopes.md)) |

The walk visits the working folder (its real path) and each parent, and stops at the first `.<app>/`, at a folder holding an entry named `.git` of any kind (a repo root is a wall), at or below the user's home folder (a walk that starts in `~` never climbs above it), at the filesystem root, at a change of disk, and at any `<APP>_CEILING_DIRS` entry. The global home is never taken as a project home ([walk](ds:block?id=dirs-walk-5dgdy46j), [Lookup: the walk](../homes/lookup.md#step-2--know-the-walk), [examples/lookup](../../examples/lookup)).

With either style, a linked git worktree is first translated to its main checkout; `HereOnly` does not turn that off, by design: a worktree shares its main checkout's home (SPEC 4.4 step 4, [Worktrees](../homes/worktrees.md), [examples/worktree](../../examples/worktree)).

## When to set ceiling dirs

| Projects live... | Set `<APP>_CEILING_DIRS`? |
|---|---|
| under the user's home folder | no: the walk already stops below `$HOME` |
| outside it (`/srv/repos`, `/data/work`, a second disk mount point) | yes, to the folder that holds the repos, so a stray `.<app>/` in `/srv` is never found |
| inside one big repo | no: the repo root is a wall |

The user sets it, in their shell or service unit, as a path list (`:` on macOS and Linux, `;` on Windows). Entries may use `~` and `${VAR}`; a relative entry is ignored with warning `ceiling-relative` ([ceilings](ds:block?id=dirs-ceilings-7ypck9dn), [Lookup: ceilings](../homes/lookup.md#step-3--set-ceilings-for-projects-outside-your-home)). An entry that does not expand to an absolute path (`$VAR` without braces, an unset `${VAR}`, `~user`) is dropped with warning `ceiling-dropped`, so tell users to write `${VAR}` and show warnings in `doctor`.

## Decision table: moving a home

| Way | Who sets it | Moves | Committed? | Rule | Guide | Example |
|---|---|---|---|---|---|---|
| `--home X` (your flag, passed with `WithHome`) | the user, for one run | both homes become `X` | no | `flag` | [Overrides: --home](../homes/overrides.md#step-1--pass---home) | [overrides](../../examples/overrides) |
| `<APP>_HOME=X` | the user's shell, a service unit, a CI job | both homes become `X` | no | `env-home` | [Overrides: APP_HOME](../homes/overrides.md#step-2--honour-app_home) | [overrides](../../examples/overrides) |
| `<APP>_<KIND>_DIR=X` | the user, for one kind (a fast disk for the cache, a big disk for the db) | one non-anchored kind | no | `env-kind` | [Overrides: APP_KIND_DIR](../homes/overrides.md#step-3--move-one-kind-with-app_kind_dir) | [overrides](../../examples/overrides) |
| `home:` in `.<app>/config.jsonc` | the team, for everyone who clones | the project home, to a relative or `~/` path | yes | `pointer` | [Pointer keys: relative home:](../pointers/pointer-keys.md#step-1--write-a-relative-home-in-the-committed-file) | [pointers](../../examples/pointers) |
| `home:` in `.<app>/config.local.jsonc` | one machine | the project home, to any path, absolute allowed; an empty value does not clear a committed `home:` | no (gitignored) | `pointer` | [Pointer keys: absolute paths](../pointers/pointer-keys.md#step-3--keep-absolute-paths-in-configlocaljsonc), [empty local home:](../pointers/pointer-keys.md#step-9--an-empty-local-home-does-not-clear-the-committed-one) | [pointers](../../examples/pointers) |
| `db:` in either config file | the team or one machine | only the database file; `backups/` stays | per file, as above | `pointer` | [Pointer keys: db:](../pointers/pointer-keys.md#step-4--move-only-the-database-with-db) | [pointers](../../examples/pointers) |

## Which one wins

For one kind, appdir checks in this order and takes the first that applies ([entryIn](ds:block?id=dirs-entryin-zhkmadk3), [resolveHomes](ds:block?id=dirs-resolvehomes-2zu5rbrz)):

1. `<APP>_<KIND>_DIR` for that kind, if the kind is not anchored. It wins even over `--home`.
2. A `db:` pointer, for the `db` kind in the project home, release profile only.
3. `--home`, then `<APP>_HOME`: one home for every scope; the lookup and the pointer keys are not read.
4. Otherwise the global home `~/.<app>` and the project home from the lookup, moved by a `home:` pointer when the marker's config files have one (`config.local.jsonc` wins per key).

## Rules

- If a teammate's data lives in a sibling folder of the repo (`../shop-data`), commit `home: "../shop-data"` in `config.jsonc`; it is relative to the folder holding the file, so it works on every clone.
- If one machine keeps data on another disk, put the absolute path in `config.local.jsonc`; the committed file refuses absolute paths with `ErrPointerAbsolute`.
- If the database is big but the rest is small, use `db:` rather than `home:`.
- If the user wants a throwaway sandbox for one run, tell them `--home ./sandbox`; to sandbox a shell, `<APP>_HOME`.
- If only the cache or the logs should move, use `<APP>_<KIND>_DIR`; print the names with `KindEnv` in your `doctor` output ([Overrides: print the variable names](../homes/overrides.md#step-5--print-the-variable-names)).
- If you are a tool author, never write pointer keys yourself except through `init --home`: create pointer targets with `InitHome` so they hold `home.jsonc` ([Init](../start/init.md#step-4--create-homes-at-other-paths-with-inithome), [examples/project-init](../../examples/project-init)).

## Pitfalls

- **`<APP>_<KIND>_DIR` beats `--home`.** A leftover `LORE_DB_DIR` in the shell still moves the db when the user passes `--home ./sandbox`. Show `Paths()` with its `Rule` column in `doctor`; the db line says `env-kind`.
- **A dev build ignores env paths into the prod home.** In a non-release profile, `<APP>_HOME` or `<APP>_<KIND>_DIR` inside `~/.<app>` is ignored with warning `env-points-to-prod`; `--home` is not filtered, because the user typed it ([Child processes](../profiles/child-processes.md#step-1--let-the-env-leak-filter-ignore-prod-paths)).
- **The dev profile ignores `db:`.** A `db:` pointer is release-profile only; a dev build keeps its database under `.<app>/profiles/dev/` and adds warning `pointer-ignored-for-profile`, so dev data never lands where the pointer names. This is SPEC section 14 item 2, current behaviour ([Pointer keys: the dev profile](../pointers/pointer-keys.md#step-7--the-dev-profile)).
- **A broken pointer stops every project command.** A missing target (`ErrPointerMissing`), a folder without this app's `home.jsonc` (`ErrPointerNotHome`) or an absolute committed path (`ErrPointerAbsolute`) fails every kind that can live in the project: `db`, `backups`, `run`, `shared`, your project data kinds, `logs` and the project side of the config files. Nothing falls back to the marker or the global home; only global kinds such as the cache keep working. `ConfigFiles()` still lists the marker's own two files, so `doctor` can show the file that holds the bad key ([Pointer keys: ConfigFiles under a broken pointer](../pointers/pointer-keys.md#step-8--configfiles-under-a-broken-pointer), [examples/pointers](../../examples/pointers)).
- **Pointers go one hop.** A `home:` in the target's own config is ignored with warning `pointer-chain-ignored`.
- **A `--home` or `<APP>_HOME` with an unset `${VAR}` fails everything.** `New` succeeds, then every kind returns `ErrEnvVar` naming the variable, the cache included; a typo in the override never sends data to the temp folder. Check `Path` of one kind at start-up and print its error ([Overrides: report a broken override](../homes/overrides.md#step-6--report-a-broken-override), [examples/overrides](../../examples/overrides)).
- **An empty local `home:` is not a reset.** `"home": ""` in `config.local.jsonc` is skipped, so the committed `home:` still applies; to undo a committed pointer on one machine, point the local file at another folder that holds this app's `home.jsonc` ([Pointer keys: step 9](../pointers/pointer-keys.md#step-9--an-empty-local-home-does-not-clear-the-committed-one)).
- **Without the home kind, a `home:` target must be named `.<app>`.** The target is normally accepted when its `home.jsonc` names this app. With `WithoutKind(appdir.KindHome)` there is no `home.jsonc` to read, so the target must be a folder named `.<app>` (`../shop-private/.lore`); any other folder fails with `ErrPointerNotHome`. Keep the home kind if teams want to name pointer targets freely ([Kinds: drop the home file](../layout/kinds.md#step-6--drop-the-home-file)).
- **The `.<app>/` must be at or below the repo root.** The walk stops at the first folder holding `.git`, so a home in the repo's parent is never found from inside the repo; use a `home:` pointer from a `.<app>/` inside the repo instead.

## Related

- [How the project home is found](../homes/lookup.md), [Worktrees](../homes/worktrees.md), [Overrides](../homes/overrides.md), [Pointer keys](../pointers/pointer-keys.md).
- Design: [SPEC 4.4 lookup](../SPEC.md#44-project-home-lookup), [SPEC 4.5 pointer keys](../SPEC.md#45-project-home-pointer-keys).
