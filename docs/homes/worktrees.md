# Worktrees and submodules

`git worktree add ../shop-fix` gives you a second checkout of the same repository. Your project data (tasks, notes, the db) belongs to the repository, not to the checkout, so appdir makes a linked worktree use the main checkout's `.<app>/` (rule `worktree`). A submodule is a different repository, so it is its own project. appdir reads two small files to tell them apart and never runs git. Read this page if your users work in worktrees.

## Before you start

- [How the project home is found](lookup.md): the walk and its stop rules.
- Import `"github.com/ubgo/appdir"`.
- No git binary is needed, for appdir or for the example.

## Step 1 — Know what git leaves on disk

A linked worktree's root holds a `.git` **file** (not a folder) with one line, `gitdir: <main>/.git/worktrees/<name>`. That admin folder holds `commondir`, whose content (`../..`) leads back to the main repo's `.git`. The example writes exactly those files:

<!-- snippet: examples/worktree/main.go#worktree -->
```go
// linkWorktree writes what `git worktree add ../<wt>` leaves on disk:
//
//	<wt>/.git                          "gitdir: <main>/.git/worktrees/<wt>"
//	<main>/.git/worktrees/<wt>/commondir  "../.." (the main .git, relative)
//	<main>/.git/worktrees/<wt>/gitdir     "<wt>/.git" (the way back)
//
// appdir reads only the first two. sub, when not empty, is created inside
// the worktree.
func linkWorktree(projects, main, wt, sub string) error {
	mainGit := filepath.Join(projects, main, gitEntry)
	wtDir := filepath.Join(projects, wt)
	admin := filepath.Join(mainGit, worktreesDir, wt)
	if err := mkdirs(projects, wt, filepath.Join(wt, sub)); err != nil {
		return err
	}
	if err := os.MkdirAll(admin, 0o700); err != nil {
		return fmt.Errorf("worktree admin folder: %w", err)
	}
	files := map[string]string{
		filepath.Join(wtDir, gitEntry):   gitdirPrefix + filepath.ToSlash(admin) + "\n",
		filepath.Join(admin, commondir):  commondirBody,
		filepath.Join(admin, gitdirFile): filepath.ToSlash(filepath.Join(wtDir, gitEntry)) + "\n",
		filepath.Join(admin, headFile):   headBody,
	}
	return writeFiles(files)
}
```

## Step 2 — Let the lookup translate the working folder

Before the normal walk, appdir looks for the first folder `R` at or above the working folder that holds an entry named `.git` ([lookupProject](ds:block?id=dirs-lookupproject-cbn3hgm5)). If `R/.git` is a file whose `gitdir:` folder holds `commondir`, and the common folder is named `.git`, this is a linked worktree whose main checkout is the common folder's parent ([linkedWorktreeMain](ds:block?id=linkedworktreemain-xwtqp3tk)). The working folder is translated to the same relative path in the main checkout, and the lookup runs there. From `shop-fix/src` that is `shop/src`, which finds `shop/.lore/` with rule `worktree` and the note `main checkout of a linked git worktree`.

<!-- snippet: examples/worktree/main.go#resolve -->
```go
// resolve runs New from one working folder and prints the project home it
// found (with its rule and note) or the error, then the warnings.
func resolve(w io.Writer, root, home, projects string, c lookupCase) error {
	opts := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(filepath.Join(projects, filepath.FromSlash(c.workDir))),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(true), // prod profile: plain folder names
	}
	if c.style != "" {
		opts = append(opts, appdir.WithLookup(c.style))
	}
	d, err := appdir.New(appName, opts...)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	fmt.Fprintf(w, "%-24s from %s\n", c.name+":", c.workDir)
	if p, err := d.Project(); err == nil {
		fmt.Fprintf(w, "  -> %s (rule %s)\n", show(root, p.Marker), p.Rule)
		if p.Note != "" {
			fmt.Fprintf(w, "     note: %s\n", p.Note)
		}
	} else {
		fmt.Fprintf(w, "  -> %s\n", err)
	}
	printWarnings(w, root, d)
	return nil
}
```

## Step 3 — Handle a main checkout with no home

If the main checkout has no `.lore/` on that path, appdir runs the normal lookup in the worktree itself. When that finds a home, it adds warning `worktree-main-missing`; when the worktree has none either (a repository that does not use your tool), there is just no project home and no warning (the `wiki-fix` case below). A tool should print warnings once on stderr ([Paths JSON](../reference/paths-json.md)). This lets you try a project home in a worktree before adding it to the main checkout.

The warning has a code, so a tool can match it with `wn.Code == appdir.WarnWorktreeMainMissing` and add advice instead of parsing the message. The example prints every warning and a hint for this one:

<!-- snippet: examples/worktree/main.go#warnings -->
```go
// printWarnings prints each warning once, the way a tool prints them on
// stderr. worktree-main-missing gets a hint: the worktree's own home is in
// use, and its data is not shared with the other checkouts until the home
// moves to the main checkout.
func printWarnings(w io.Writer, root string, d *appdir.Dirs) {
	for _, wn := range d.Warnings() {
		fmt.Fprintf(w, "  warning %s: %s\n", wn.Code, hide(root, wn.Message))
		if wn.Code == appdir.WarnWorktreeMainMissing {
			fmt.Fprintf(w, "     hint: run '%s init' in the main checkout to share one home\n", appName)
		}
	}
}
```

In the output, `blog-fix` shows the warning and the hint; `wiki-fix` shows neither.

## Step 4 — Know the cases that are not shared

- **Submodule.** A submodule's `.git` file points into `<main>/.git/modules/<name>`, which has no `commondir`. It is not a linked worktree, and its `.git` file is a wall, so a submodule finds its own `.lore/` or none.
- **Worktree of a bare repo.** The common folder of a bare repo is the repo folder itself (`tool.git`), not a folder named `.git`, so there is no checkout to share: the worktree uses its own `.lore/`.

<!-- snippet: examples/worktree/main.go#submodule -->
```go
// addSubmodule writes what `git submodule add` leaves on disk: a .git file
// pointing into <main>/.git/modules/<name>, a git dir with no commondir.
// The relative gitdir path is how git writes it.
func addSubmodule(projects, main, path string) error {
	sub := filepath.Join(projects, main, filepath.FromSlash(path))
	modGit := filepath.Join(projects, main, gitEntry, modulesDir, filepath.Base(path))
	if err := mkdirs(projects, filepath.Join(main, path, marker)); err != nil {
		return err
	}
	if err := os.MkdirAll(modGit, 0o700); err != nil {
		return fmt.Errorf("submodule git dir: %w", err)
	}
	rel, err := filepath.Rel(sub, modGit)
	if err != nil {
		return fmt.Errorf("submodule gitdir path: %w", err)
	}
	return writeFiles(map[string]string{
		filepath.Join(sub, gitEntry):    gitdirPrefix + filepath.ToSlash(rel) + "\n",
		filepath.Join(modGit, headFile): headBody,
	})
}
```

## Try it

```sh
go run ./examples/worktree
```

Paths are relative to the example's temp root:

```text
main checkout:           from shop/src
  -> home/projects/shop/.lore (rule project)
linked worktree:         from shop-fix/src
  -> home/projects/shop/.lore (rule worktree)
     note: main checkout of a linked git worktree
main has no .lore:       from blog-fix
  -> home/projects/blog-fix/.lore (rule project)
  warning worktree-main-missing: linked worktree of home/projects/blog, but the main checkout has no .lore; using the worktree
     hint: run 'lore init' in the main checkout to share one home
neither has .lore:       from wiki-fix
  -> no .lore/ here; run 'lore init' or pass --home
submodule:               from shop/vendor/lib
  -> home/projects/shop/vendor/lib/.lore (rule project)
worktree of a bare repo: from tool-wt
  -> home/projects/tool-wt/.lore (rule project)
here-only in a worktree: from shop-fix
  -> home/projects/shop/.lore (rule worktree)
     note: main checkout of a linked git worktree
home only in worktree:   from shop-fix/pkg
  -> home/projects/shop/.lore (rule worktree)
     note: main checkout of a linked git worktree
```

## Pitfalls

- **`HereOnly` still uses the main checkout.** The worktree translation runs before the lookup style is applied, so `WithLookup(appdir.HereOnly)` in `shop-fix` checks `shop/.lore` and finds it with rule `worktree`. That is by design (SPEC 4.4 step 4): a linked worktree always shares the main checkout's home. A tool that wants a worktree's own folder passes `--home` instead. The `here-only in a worktree` case in Try it shows it.
- **The main checkout must be on this machine.** The `gitdir:` path is absolute when git writes it. A worktree copied to another machine, or a main checkout that was moved, points to a missing folder; appdir then treats it as a normal folder and walks from the worktree.
- **Worktree data is shared, including the db.** Two worktrees running a daemon at the same time use the same `run/` folder and db. That is the point, but a tool that holds a lock per checkout must use the lock files in `run/` ([Run files and sockets](../runtime/run-files-and-sockets.md)).
- **A home made only in a worktree is ignored while the main checkout has one.** The lookup runs on the matching path in the main checkout, so `shop-fix/pkg/.lore/`, created on the worktree's branch, loses to `shop/.lore/` (the `home only in worktree` case). Nearest-wins is judged in the main checkout's tree. Merge the branch, or create the nested home in the main checkout too.
- **A folder that exists only in the worktree still works.** The translated path may not exist in the main checkout; the walk goes up from it anyway and finds the main checkout's home.
- **The warning is not an error.** Under `worktree-main-missing` everything resolves and writes go to the worktree's own home, which other checkouts never see. A tool that ignores `Warnings()` gives the user no sign that the data is not shared.
- **Files are read, git is not run.** A broken `.git` file (no `gitdir:` line) is treated as a plain repo root, not as an error.

## Related

- [How the project home is found](lookup.md)
- [Global and project homes](global-and-project.md)
- Example: [examples/worktree](../../examples/worktree/README.md)
- SPEC: [4.4 Project home: lookup](../SPEC.md#44-project-home-lookup) (step 2)
