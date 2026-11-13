# How the project home is found

When a tool runs, appdir looks for the nearest `.<app>/` from the working folder upward, the way git finds `.git`. It stops at a repository root, below the user's home, at the filesystem root, at a change of disk and at folders the user lists in `<APP>_CEILING_DIRS`, so a tool never picks up a stray home from far above the project. Read this page when a tool finds the wrong project home, or none.

## Before you start

- [First use](../start/first-use.md) and [Create a home with init](../start/init.md).
- Import `"github.com/ubgo/appdir"`.
- The lookup is skipped entirely under `--home` or `<APP>_HOME` ([Overrides](overrides.md)). Linked git worktrees have their own page: [Worktrees](worktrees.md).

## Step 1 — Resolve from a working folder

`New` starts from the working folder (`os.Getwd`, or `WithWorkDir` in tests and examples) and reports the result as `Project()`: the `Marker` it found and rule `project`, or an error wrapping `ErrNoProject`. The example runs one `New` per case:

<!-- snippet: examples/lookup/main.go#resolve -->
```go
// resolve runs New from one working folder, then report prints the project
// home it found or handles ErrNoProject, and the warnings follow.
func resolve(w io.Writer, root, home string, c lookupCase) error {
	opts := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(filepath.Join(root, filepath.FromSlash(c.workDir))),
		appdir.WithEnv(func(k string) (string, bool) { v, ok := c.env[k]; return v, ok }),
		appdir.WithRelease(true), // prod profile: plain folder names
	}
	if c.style != "" {
		opts = append(opts, appdir.WithLookup(c.style))
	}
	d, err := appdir.New(appName, opts...)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	fmt.Fprintf(w, "%-31s from %s\n", c.name+":", c.workDir)
	if err := report(w, root, d); err != nil {
		return err
	}
	for _, wn := range d.Warnings() {
		fmt.Fprintf(w, "  warning %s: %s\n", wn.Code, wn.Message)
	}
	return nil
}
```

## Step 2 — Know the walk

For each folder `D` from the start upward ([findMarker](ds:block?id=dirs-findmarker-s5yjcjcd)):

1. If `D/.lore` is a folder and it is not the global home, it is the project home. Stop. **Nearest wins**: in a monorepo, `pkg/web/.lore/` beats the one at the repo root.
2. If `D` holds an entry named `.git` of any kind (file, folder, or a symlink, even a dangling one), stop: a repository root is a wall. A nested repository (a submodule or a vendored clone) is therefore its own world; its parent's `.lore/` is not used.
3. Move to the parent, unless a stop rule hits ([walk](ds:block?id=dirs-walk-5dgdy46j)): `D` is the user's home or a ceiling (a walk that starts there never climbs above it), `D` is the filesystem root, the parent is the user's home, the parent is in `<APP>_CEILING_DIRS`, or the parent is on another disk (device id; volume serial on Windows).

Two more rules make the walk predictable:

- **Real paths.** The start is the working folder with symlinks resolved, so running from a symlink into `shop/src/api` finds `shop/.lore/`, not something next to the link.
- **Identity, not strings.** Folders are compared with `os.SameFile`, so case-insensitive disks and a symlinked home work. The global home (`~/.lore`, and `~/.lore-dev` in a dev build) is never taken as a project home, even when the walk starts in the user's home.

The example builds a tree with each of these cases and names the working folder for each; your tool needs no code for any of them, they all happen inside `New`:

<!-- snippet: examples/lookup/main.go#walk -->
```go
// walkCases are the working folders that show the walk (Step 2): the nearest
// .lore/ upward wins, an entry named .git is a wall, the walk stops below the
// user's home, the global home is never a project home, and a symlinked
// working folder is walked by its real path.
func walkCases() []lookupCase {
	return []lookupCase{
		{name: "walk-up", workDir: "home/projects/shop/src/api"},
		{name: "repo root is a wall", workDir: "home/projects/mono/vendor/lib/src"},
		{name: "dangling .git is a wall", workDir: "work/repo/src"},
		{name: "stops below $HOME", workDir: "home/projects/notes/sub"},
		// The walk checks home itself (its .lore is the global home, so it is
		// skipped) and stops there: the stray .lore above it is never reached.
		{name: "global home is never a project", workDir: "home"},
		{name: "no ceiling", workDir: "work/a/b"},
		{name: "real path of a symlink", workDir: linkPath},
		{name: "nearest wins", workDir: "home/projects/mono/pkg/web/src"},
	}
}
```

## Step 3 — Set ceilings for projects outside your home

The stop below the user's home protects projects under `~`. Projects on another path, such as `/srv/work`, walk up to the filesystem root unless a disk change stops them. `LORE_CEILING_DIRS` lists folders the walk may not climb into, separated by the OS list separator (`:`, or `;` on Windows). Entries may use `~` and `${VAR}`. A relative entry is ignored with warning `ceiling-relative`, as git does, and an entry that does not expand to an absolute path (`$VAR` without braces, an unset `${VAR}`, `~user`) with warning `ceiling-dropped` ([ceilings](ds:block?id=dirs-ceilings-7ypck9dn)). The start folder itself is always checked, even when it is a ceiling, and then the walk stops there.

A real tool reads the variable from the process environment and needs no code. The example passes it through `WithEnv` to show three cases: a ceiling that hides `work/.lore`, a walk that starts at a ceiling, and an entry written `$ROOT/...` without braces, which is not expanded, is not absolute, and is dropped without a warning:

<!-- snippet: examples/lookup/main.go#ceilings -->
```go
// ceilingCases set LORE_CEILING_DIRS for a project outside the user's home
// (Step 3). The ceiling work/a stops the walk before work/.lore; a walk that
// starts at a ceiling checks it and climbs no further; a relative entry is
// ignored with warning ceiling-relative; $VAR without braces is not
// expanded, so that entry is ignored with warning ceiling-dropped.
func ceilingCases(root string) []lookupCase {
	sep := string(filepath.ListSeparator)
	workA := filepath.Join(root, "work", "a")
	return []lookupCase{
		{name: "ceiling dirs", workDir: "work/a/b", env: map[string]string{ceilingEnv: workA + sep + relativeDir}},
		{name: "start at a ceiling", workDir: "work/a/b", env: map[string]string{ceilingEnv: filepath.Join(workA, "b")}},
		{name: "$VAR ceiling is ignored", workDir: "work/a/b", env: map[string]string{ceilingEnv: "$ROOT/work/a", "ROOT": root}},
	}
}
```

## Step 4 — Look only here with HereOnly

`WithLookup(appdir.HereOnly)` checks only the working folder itself: no walk. Use it for a tool that should act only on the folder it was started in, such as a per-directory runner. From `shop/src/api` it finds nothing; from `shop` it finds `shop/.lore/`.

<!-- snippet: examples/lookup/main.go#hereonly -->
```go
// hereOnlyCases use WithLookup(HereOnly) (Step 4): only the working folder
// itself is checked, so a deep folder finds nothing and the project root
// finds its own .lore/.
func hereOnlyCases() []lookupCase {
	return []lookupCase{
		{name: "here-only, deep folder", workDir: "home/projects/shop/src/api", style: appdir.HereOnly},
		{name: "here-only, project root", workDir: "home/projects/shop", style: appdir.HereOnly},
	}
}
```

## Step 5 — Handle no project home

Project kinds then fail with `ErrNoProject` and the message `no .lore/ here; run 'lore init' or pass --home`. Print it as is: it names both ways out. Kinds that need no project (the cache, a `project-else-global` kind such as `logs`) still work. A tool can map `ErrNoProject` to its own exit code ([Errors reference](../reference/errors.md)).

`New` still succeeds; the error is stored and returned by `Project()` and by `Path`, `PathIn` and `Ensure` for project kinds. Test it with `errors.Is`, never by comparing the text:

<!-- snippet: examples/lookup/main.go#noproject -->
```go
// report prints the project home, or handles its absence (Step 5). The
// ErrNoProject message already names both ways out (init, --home), so it is
// printed as is. Kinds that need no project, such as the cache, still work,
// so a command that only needs them carries on.
func report(w io.Writer, root string, d *appdir.Dirs) error {
	p, err := d.Project()
	switch {
	case err == nil:
		fmt.Fprintf(w, "  -> %s (rule %s)\n", show(root, p.Marker), p.Rule)
		return nil
	case !errors.Is(err, appdir.ErrNoProject):
		return err
	}
	fmt.Fprintf(w, "  -> %s\n", err)
	cache, err := d.Path(appdir.KindCache)
	if err != nil {
		return fmt.Errorf("cache outside a project: %w", err)
	}
	fmt.Fprintf(w, "     cache still works: %s\n", show(root, cache))
	return nil
}
```

## Try it

```sh
go run ./examples/lookup
```

The example builds a tree under a temp root (`home/` is the fake user home, `work/` is outside it, and a stray `.lore/` sits in the root above both) and resolves from each folder:

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

`vendor/lib` has a `.git` file, so the walk stops there instead of reaching `mono/.lore/`. `work/repo` has a `.git` symlink to a folder that no longer exists, and it is still a wall, so `work/.lore/` is not reached. "start at a ceiling" lists `work/a/b` itself: it is checked, then the walk stops. "$VAR ceiling is ignored" finds `work/.lore/` because the entry `$ROOT/work/a` was dropped, with warning `ceiling-dropped`. `notes/sub` stops before the user's home, so the stray `.lore/` in the root is never reached. The "global home" case starts in the user's home: it checks `home/.lore`, rejects it as the global home, and stops there, so the stray `.lore/` in the root is not reached from there either.

## Pitfalls

- **A project outside the user's home has no home stop.** Without a repo root, a ceiling or a disk change, the walk reaches `/`. Put such projects in git, or set `LORE_CEILING_DIRS`.
- **The disk-change stop cannot be shown in the example.** It needs a second mounted file system. An external drive or a separate partition (for example `/srv` on a server) is a different disk from `/`, so a walk from a project on it stops at the drive root.
- **`HereOnly` still follows linked worktrees.** The worktree translation runs before the here-only check, so in a linked worktree the main checkout's matching folder is checked. This is by design: a worktree shares its main checkout's home ([Worktrees](worktrees.md#pitfalls)).
- **Only a folder counts.** A file named `.lore` is not a project home and does not stop the walk; `Path` returns `ErrNoProject` or finds a home further up. A `paths` command shows which marker was used.

- **A ceiling written `$VAR/...` is dropped.** Only `${VAR}` expands. `$ROOT/work` stays literal, is not an absolute path, and is skipped with warning `ceiling-dropped` (`ceiling-relative` is only for entries that start with neither `/`, `~` nor `$`). An entry with an unset `${VAR}`, `~user`, or a `${VAR}` that expands to a relative path gets the same warning. If a ceiling seems to have no effect, look at the warnings (`doctor`, `paths --json`); `go run ./examples/lookup` shows the case.
- **The lookup runs once, in `New`.** A `*Dirs` is a snapshot: `os.Chdir` after `New` does not move the project home. A long-running process that serves several folders calls `New` with `WithWorkDir` for each.
- **A dangling `.git` symlink is still a wall.** A worktree or clone deleted by hand can leave `.git` pointing nowhere; the walk stops there and finds no home above it. Remove the stale `.git` entry.
- **A home placed directly in the user's home is the global home.** `~/.lore` is never a project home, so a project that lives in `~` itself has none; create the project in a subfolder.

## Related

- [Worktrees and submodules](worktrees.md)
- [Overrides: --home and env](overrides.md)
- [Global and project homes](global-and-project.md)
- [Pointer keys](../pointers/pointer-keys.md) (what happens after the marker is found)
- Example: [examples/lookup](../../examples/lookup/README.md)
- SPEC: [4.4 Project home: lookup](../SPEC.md#44-project-home-lookup)
