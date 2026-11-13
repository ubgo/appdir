# Choosing the mode and the scope of each kind

Every kind (a file or folder in a home) lives in one of two homes: the global home `~/.<app>` or the project home `.<app>/` found from the working folder. The mode sets the default for data kinds; `Scoped` overrides it for one kind. This page helps you pick both.

## Decision table: the mode

| Your tool keeps... | Mode | What happens outside a project |
|---|---|---|
| data that belongs to a repo (notes, tasks, an index of the code) | `ModeProject` (the default) | data kinds fail with `ErrNoProject` ("no .<app>/ here; run '<app> init' or pass --home") |
| one store per user (sessions, a machine-wide daemon, credentials, history) | `WithMode(appdir.ModeGlobal)` | everything works; data lives in `~/.<app>` |
| both kinds of data | `ModeProject`, plus `Scoped(ScopeGlobal)` on the per-user kinds | the per-user kinds work, the project kinds fail with `ErrNoProject` |

`ModeProject` is the default because it fails safe: a global tool that forgets `WithMode(ModeGlobal)` gets a loud `ErrNoProject` outside a project, while the opposite mistake would quietly mix every project's data into `~/.<app>`. This is SPEC section 14 item 4, an owner decision (2026-11-13) ([Kinds: pick the mode](../layout/kinds.md#step-3--pick-the-mode), [buildKinds](ds:block?id=buildkinds-r6jj6d5s)).

## Where each standard kind lives

Read from `buildKinds` and `standardKinds`; an empty scope in the standard list means "the mode's data scope".

| Kind | `ModeProject` | `ModeGlobal` |
|---|---|---|
| `home` (`home.jsonc`), `config`, `config-local` | `both` | `both` |
| `gitignore` | `project` | `project` |
| `shared`, `db`, `backups`, `run` | `project` | `global` |
| `logs` | `project-else-global` | `global` |
| `cache` | `global` | `global` |

## Decision table: the scope of one kind

| Scope | Lives in | Pick it when |
|---|---|---|
| `ScopeGlobal` | the global home | the kind is per user even in a project tool: a cache, a machine-wide daemon's `run/`, a token store |
| `ScopeProject` | the project home; `ErrNoProject` without one | the kind only makes sense next to the code: templates committed with the repo, a per-repo index |
| `ScopeProjectElseGlobal` | the project home when there is one, else the global home (the entry is marked `Fallback`) | the tool should work anywhere but prefer the project: logs, scratch output |
| `ScopeBoth` | one copy per home; `Path` gives the project copy when there is a project, `PathIn` picks one | the kind layers like config: a global default and a project override |

## Decision table: dropping a kind or keeping it out of git

| You want... | Use | What changes | Guide | Example |
|---|---|---|---|---|
| a kind your tool never uses gone from the layout | `WithoutKind(kind)` | `Path`, `PathIn` and `Ensure` answer `ErrUnknownKind`; `Paths` and `Kinds` leave it out | [Kinds: ask for a kind that is gone](../layout/kinds.md#step-5--flip-a-git-rule-and-ask-for-a-kind-that-is-gone), [Errors: ErrUnknownKind](../reference/errors.md#errunknownkind) | [custom-layout](../../examples/custom-layout), [doctor](../../examples/doctor) |
| a kind kept but out of git | `WithKind(kind, appdir.Ignore())` | only the `.gitignore` block changes; the path stays | [Kinds: flip a git rule](../layout/kinds.md#step-5--flip-a-git-rule-and-ask-for-a-kind-that-is-gone) | [custom-layout](../../examples/custom-layout) |
| no `config.jsonc` or `config.local.jsonc` | `WithoutKind(appdir.KindConfig)` or `KindConfigLocal` | `ConfigFiles` leaves that file out, and its `home:` / `db:` keys are no longer read | [Config files: drop a config kind](../layout/config-files.md#step-6--drop-a-config-kind) | [config-files](../../examples/config-files) |
| no `home.jsonc` | `WithoutKind(appdir.KindHome)` | init creates only the folder; no layout version guard; a `home:` pointer target must be a folder named `.<app>` | [Kinds: drop the home file](../layout/kinds.md#step-6--drop-the-home-file) | [custom-layout](../../examples/custom-layout) |

If a team may ever move the project home with a `home:` pointer, prefer to keep `KindHome`: the pointer target is then accepted when its `home.jsonc` names this app, whatever the folder is called. Without it the target must be named `.<app>` (`checkPointerTarget` in `pointer.go`).

## Rules

- If your tool must run in any folder (a global tool, a desktop app, a service), use `ModeGlobal`. See [examples/fallbacks](../../examples/fallbacks) and [examples/testing-setup](../../examples/testing-setup), which both pass it.
- If your tool is a project CLI but runs one daemon for the whole machine, keep `ModeProject` and move the run folder: `appdir.WithKind(appdir.KindRun, appdir.Scoped(appdir.ScopeGlobal))`. Without it, `RunPath` and `Socket` fail with `ErrNoProject` outside a project ([Run files and sockets](../runtime/run-files-and-sockets.md), [examples/daemon-paths](../../examples/daemon-paths)).
- If a kind is safe to delete, mark it `Disposable()`: it gets `CACHEDIR.TAG`, and a global disposable kind falls back to `<tmp>/<Name(app)>-<uid>/` when there is no `$HOME` ([Fallbacks](../homes/fallbacks.md), [examples/custom-layout](../../examples/custom-layout)).
- If a kind must be committed with the repo, add `Commit()` and keep it in the project home; the `.gitignore` block that re-includes it is written only inside a git work tree ([The .gitignore block](../layout/gitignore.md)).
- If a kind is part of what makes a folder a home (like `home.jsonc`), mark it `Anchored()`: it stays in the home root for every profile and has no `<APP>_<KIND>_DIR`. Most kinds should not be anchored.
- If a user moves the whole home with `--home` or `<APP>_HOME`, scopes stop mattering: both homes are that one folder ([Overrides](../homes/overrides.md)).

## Pitfalls

- **Changing the mode after release moves the data.** Switching from `ModeProject` to `ModeGlobal` makes `Path(KindDB)` point at `~/.<app>/<app>.db`; the old per-project databases stay where they were. appdir has no move for this; `MoveLegacy` only moves an old global home folder ([Legacy move](../runtime/legacy-move.md)).
- **`logs` differs by mode.** In `ModeProject` it falls back to the global home with no project; in `ModeGlobal` it is plainly global. Code that expects `Fallback` on the logs entry only sees it in project mode.
- **A project-scoped kind in a global tool still needs a project.** `Scoped(ScopeProject)` under `ModeGlobal` fails with `ErrNoProject` outside a project, the same as in project mode.
- **`gitignore` stays project-scoped in both modes.** It is the project's `.gitignore`; a global tool that never runs `init` simply never writes it.
- **An unknown scope is refused.** `Scoped("shared")` fails `New` with `ErrInvalidOption`; use the four constants.
- **Temp fallback with no `$HOME` is global only.** A disposable kind scoped to the project home gets no temp folder when there is no `$HOME`; it fails with `ErrNoProject` or uses the project home. The read-only fallback of `Ensure` works in either home ([Fallbacks: read-only home](../homes/fallbacks.md#step-3--handle-a-read-only-home)).

## Related

- [Kinds: the layout of a home](../layout/kinds.md) and [examples/custom-layout](../../examples/custom-layout): `WithKind`, `WithoutKind`, `Kinds()` and the mode comparison.
- [Global and project homes](../homes/global-and-project.md) and [examples/quickstart](../../examples/quickstart).
- Design: [SPEC 5.2 Mode and scope](../SPEC.md#52-mode-and-scope).
