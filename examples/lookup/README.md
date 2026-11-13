# lookup

How the project home is found: the walk-up from the working folder, the repo-root wall (a `.git` folder, file or dangling symlink), the stop below the user's home, `LORE_CEILING_DIRS` (a walk that starts at a ceiling, the relative-entry warning, a `$VAR` entry that is ignored with warning `ceiling-dropped`), the real path of a symlinked working folder, nearest-wins in a monorepo, `WithLookup(HereOnly)`, and the `ErrNoProject` message with the cache still working. It builds a folder tree in a temp root and resolves from each folder.

## Run

```sh
go run ./examples/lookup
```

Output (paths relative to the temp root):

```text
walk-up:                        from home/projects/shop/src/api
  -> home/projects/shop/.lore (rule project)
repo root is a wall:            from home/projects/mono/vendor/lib/src
  -> no .lore/ here; run 'lore init' or pass --home
     cache still works: home/.lore/cache
dangling .git is a wall:        from work/repo/src
  -> no .lore/ here; run 'lore init' or pass --home
     cache still works: home/.lore/cache
stops below $HOME:              from home/projects/notes/sub
  -> no .lore/ here; run 'lore init' or pass --home
     cache still works: home/.lore/cache
global home is never a project: from home
  -> no .lore/ here; run 'lore init' or pass --home
     cache still works: home/.lore/cache
no ceiling:                     from work/a/b
  -> work/.lore (rule project)
real path of a symlink:         from home/projects/api-link
  -> home/projects/shop/.lore (rule project)
nearest wins:                   from home/projects/mono/pkg/web/src
  -> home/projects/mono/pkg/web/.lore (rule project)
ceiling dirs:                   from work/a/b
  -> no .lore/ here; run 'lore init' or pass --home
     cache still works: home/.lore/cache
  warning ceiling-relative: LORE_CEILING_DIRS entry "projects" is relative; ignored
start at a ceiling:             from work/a/b
  -> no .lore/ here; run 'lore init' or pass --home
     cache still works: home/.lore/cache
$VAR ceiling is ignored:        from work/a/b
  -> work/.lore (rule project)
  warning ceiling-dropped: LORE_CEILING_DIRS entry "$ROOT/work/a" is not an absolute path (only ${VAR} expands, not $VAR); ignored
here-only, deep folder:         from home/projects/shop/src/api
  -> no .lore/ here; run 'lore init' or pass --home
     cache still works: home/.lore/cache
here-only, project root:        from home/projects/shop
  -> home/projects/shop/.lore (rule project)
```

`go test ./examples/lookup` runs the same program in a test folder and checks these lines.

## Guide

- [How the project home is found](../../docs/homes/lookup.md)
