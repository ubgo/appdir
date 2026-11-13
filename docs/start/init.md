# Create a home with init

A project home is never made by accident: only `InitProject` creates `.<app>/`, and it is what your `<app> init` command calls. `InitHome` creates a home at any path, for `<app> init --home X`, for an up-front global home, or for the target of a `home:` pointer. Both write `home.jsonc`, the small file that marks a folder as a home of your app. Use this page when you write the `init` command.

## Before you start

- [First use](first-use.md): `New`, `Path` and `Ensure`.
- Import `"github.com/ubgo/appdir"`.
- `init` runs inside the project folder; the example passes it with `WithWorkDir` and gives the user's home with `WithHomeDir`. The project has a `.git` folder, so it is a git work tree.

## Step 1 — See what a project command gets before init

Without `.lore/`, `New` still succeeds, `HasProject()` is false, and every project kind fails with `ErrNoProject`. Its message tells the user what to do, so a tool can print it as is.

<!-- snippet: examples/project-init/main.go#before -->
```go
// beforeInit shows a project with no .lore/ yet: project kinds fail with
// ErrNoProject, and the message tells the user what to run.
func beforeInit(w io.Writer, root, home, project string) error {
	d, err := open(appName, home, project)
	if err != nil {
		return err
	}
	_, err = d.Path(appdir.KindDB)
	if !errors.Is(err, appdir.ErrNoProject) {
		return fmt.Errorf("want ErrNoProject, got %v", err)
	}
	fmt.Fprintf(w, "HasProject: %v\n", d.HasProject())
	fmt.Fprintf(w, "Path(db): %s\n", hide(root, err.Error()))
	return nil
}
```

## Step 2 — Create the project home with InitProject

`InitProject(dir)` creates `dir/.lore/` with mode `0700`, writes `home.jsonc`, and, when the folder is inside a git work tree, writes the `.gitignore` block that keeps local files out of git ([InitProject](ds:block?id=dirs-initproject-wpmefq6e)). It returns the new marker folder. The block and its markers are covered in [The .gitignore block](../layout/gitignore.md).

<!-- snippet: examples/project-init/main.go#init -->
```go
// initProject is the `lore init` command: it creates .lore/ with home.jsonc
// and, because the folder is in a git work tree, the .gitignore block.
func initProject(w io.Writer, root, home, project string) error {
	d, err := open(appName, home, project)
	if err != nil {
		return err
	}
	marker, err := d.InitProject(project)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	fmt.Fprintf(w, "created: %s\n", show(root, marker))
	b, err := os.ReadFile(filepath.Join(marker, homeFile))
	if err != nil {
		return fmt.Errorf("read %s: %w", homeFile, err)
	}
	fmt.Fprintf(w, "%s:\n%s", homeFile, b)
	fmt.Fprintf(w, "%s written: %v\n", ignoreFile, exists(filepath.Join(marker, ignoreFile)))
	return nil
}
```

`home.jsonc` holds three keys: `app` (whose home this is), `layout` (the layout version, `WithLayoutVersion`, default 1) and `scope` (`project` or `global`). It is written atomically: a temp file in the same folder, fsync, then a rename over the target (Windows retries a sharing violation a few times), so a crash never leaves half a file. New files get mode `0600`.

## Step 3 — Resolve again

The next `New` finds `.lore/` by the [lookup](../homes/lookup.md) and parses `home.jsonc` into `Home.Info`.

<!-- snippet: examples/project-init/main.go#after -->
```go
// afterInit resolves again: the lookup now finds .lore/, and Home.Info holds
// the parsed home.jsonc.
func afterInit(w io.Writer, root, home, project string) error {
	d, err := open(appName, home, project)
	if err != nil {
		return err
	}
	p, err := d.Project()
	if err != nil {
		return fmt.Errorf("project: %w", err)
	}
	fmt.Fprintf(w, "project home: %s (rule %s)\n", show(root, p.Base), p.Rule)
	fmt.Fprintf(w, "info: app=%s layout=%d scope=%s\n", p.Info.App, p.Info.Layout, p.Info.Scope)
	return nil
}
```

## Step 4 — Create homes at other paths with InitHome

`InitHome(path, scope)` is `<app> init --home X`. The path may use `~`, `~/…` and `${VAR}`, and a relative path resolves against the working folder ([InitHome](ds:block?id=dirs-inithome-934qhf2j)). Use `ScopeGlobal` for a global home and `ScopeProject` for a project home such as a [pointer target](../pointers/pointer-keys.md). On a folder that already holds this app's `home.jsonc` it is a no-op; on a folder holding another app's `home.jsonc` it fails with `ErrNotHome`.

<!-- snippet: examples/project-init/main.go#inithome -->
```go
// initHomes creates homes at chosen paths. "~" expands against the user's
// home. Running InitHome again on lore's own home is a no-op; on a folder
// that holds another app's home.jsonc it fails with ErrNotHome.
func initHomes(w io.Writer, root, home, project string) error {
	d, err := open(appName, home, project)
	if err != nil {
		return err
	}
	// The global home, created up front (a tool's `lore init --global`).
	if err := d.InitHome(globalHome, appdir.ScopeGlobal); err != nil {
		return fmt.Errorf("init global home: %w", err)
	}
	fmt.Fprintf(w, "InitHome(%s, global): ok\n", globalHome)
	// A project home outside any project: a pointer target (see pointer keys).
	target := filepath.Join(root, filepath.FromSlash(sharedHome))
	for range 2 { // the second call finds lore's own home.jsonc: no-op
		if err := d.InitHome(target, appdir.ScopeProject); err != nil {
			return fmt.Errorf("init shared home: %w", err)
		}
		fmt.Fprintf(w, "InitHome(%s, project): ok\n", show(root, target))
	}
	// A folder that already belongs to another app.
	notes, err := open(otherAppName, home, project)
	if err != nil {
		return err
	}
	theirs := filepath.Join(root, filepath.FromSlash(otherHome))
	if err := notes.InitHome(theirs, appdir.ScopeProject); err != nil {
		return fmt.Errorf("init %s home: %w", otherAppName, err)
	}
	err = d.InitHome(theirs, appdir.ScopeProject)
	if !errors.Is(err, appdir.ErrNotHome) {
		return fmt.Errorf("want ErrNotHome, got %v", err)
	}
	fmt.Fprintf(w, "InitHome(%s, project): %s\n", show(root, theirs), hide(root, err.Error()))
	return nil
}
```

## Step 5 — Let init refuse the global home

Run in the user's home folder, `lore init` would make `~/.lore` a project home, and the global and project homes would be one folder. `InitProject` refuses when the target is the global home, compared by identity, not by string.

<!-- snippet: examples/project-init/main.go#refuse -->
```go
// refuseGlobal tries to turn the global home into a project home, as
// `lore init` run in the user's home folder would. InitProject recognises
// the global home and refuses; it does the same before ~/.lore exists,
// comparing the parent folder and the name.
func refuseGlobal(w io.Writer, root, home, project string) error {
	d, err := open(appName, home, project)
	if err != nil {
		return err
	}
	_, err = d.InitProject(home)
	if !errors.Is(err, appdir.ErrInvalidOption) {
		return fmt.Errorf("want ErrInvalidOption, got %v", err)
	}
	fmt.Fprintf(w, "InitProject(~): %s\n", hide(root, err.Error()))
	return nil
}
```

## Step 6 — Handle a relative folder and a broken home.jsonc

`lore init packages/api` passes a relative folder. `InitProject` joins it to the working folder `New` resolved from (`WithWorkDir`, or the process's working folder at the time of `New`), the same base every other user path uses, so the home lands where the user expects even if the tool changed directory since. And when `.lore/home.jsonc` already exists but does not parse (a bad hand edit, a merge conflict), `InitProject` refuses with `ErrBadHomeFile` (`cannot read …; fix or remove it`) and leaves the file untouched: the file is the user's, and replacing it with a fresh one would lose whatever they had in it. The message names the file once; match it with `errors.Is(err, appdir.ErrBadHomeFile)`.

<!-- snippet: examples/project-init/main.go#edges -->
```go
// initRelative passes a relative folder, as `lore init packages/api` would.
// It is joined to the working folder New resolved from (WithWorkDir, or the
// process cwd at New time), not to whatever the cwd is when InitProject runs.
func initRelative(w io.Writer, root, home, project string) error {
	d, err := open(appName, home, project)
	if err != nil {
		return err
	}
	marker, err := d.InitProject(relativeDir)
	if err != nil {
		return fmt.Errorf("init %s: %w", relativeDir, err)
	}
	fmt.Fprintf(w, "InitProject(%q): %s\n", relativeDir, show(root, marker))
	return nil
}

// initBroken runs init on a folder whose .lore/home.jsonc exists but does
// not parse. InitProject refuses instead of overwriting the user's file, and
// leaves it as it was.
func initBroken(w io.Writer, root, home, project string) error {
	dir := filepath.Join(root, filepath.FromSlash(brokenDir))
	file := filepath.Join(dir, markerDir, homeFile)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return fmt.Errorf("create broken home: %w", err)
	}
	if err := os.WriteFile(file, []byte(brokenJSON), 0o600); err != nil {
		return fmt.Errorf("write broken home.jsonc: %w", err)
	}
	d, err := open(appName, home, project)
	if err != nil {
		return err
	}
	_, initErr := d.InitProject(dir)
	if !errors.Is(initErr, appdir.ErrBadHomeFile) {
		return fmt.Errorf("init over a broken home.jsonc: want ErrBadHomeFile, got %v", initErr)
	}
	fmt.Fprintf(w, "InitProject(%s): %s\n", brokenDir, hide(root, initErr.Error()))
	body, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read back home.jsonc: %w", err)
	}
	fmt.Fprintf(w, "home.jsonc unchanged: %v\n", string(body) == brokenJSON)
	return nil
}
```

## Try it

```sh
go run ./examples/project-init
```

Paths are relative to the example's temp root:

```text
== before init
HasProject: false
Path(db): no .lore/ here; run 'lore init' or pass --home
== InitProject
created: home/projects/shop/.lore
home.jsonc:
{
  "app": "lore",
  "layout": 1,
  "scope": "project"
}
.gitignore written: true
== after init
project home: home/projects/shop/.lore (rule project)
info: app=lore layout=1 scope=project
== InitHome
InitHome(~/.lore, global): ok
InitHome(home/team/lore, project): ok
InitHome(home/team/lore, project): ok
InitHome(home/team/notes, project): home/team/notes holds a home of "notes"
== InitProject on the global home
InitProject(~): init: home/.lore is the global home, not a project
== InitProject with a relative folder
InitProject("packages/api"): home/projects/shop/packages/api/.lore
== InitProject over a broken home.jsonc
InitProject(home/projects/old): init: cannot read home/projects/old/.lore/home.jsonc; fix or remove it: jsonc: unexpected end of JSON input
home.jsonc unchanged: true
```

## Pitfalls

- **`init` in the user's home is refused, even on a new machine.** `InitProject("~")` fails with `ErrInvalidOption` (`init: /home/you/.lore is the global home, not a project`) whether or not `~/.lore` exists yet: a missing global home is matched by its parent folder and name. Show the error; do not retry with another folder.
- **A relative `dir` follows the working folder `New` used.** `InitProject("sub")` creates `<WithWorkDir>/sub/.lore`, not a folder under the process's current directory at the time of the call (Step 6). A tool that calls `os.Chdir` before init should call `New` again.
- **An unreadable `home.jsonc` stops `init`.** A home whose `home.jsonc` exists but cannot be parsed is refused with `ErrBadHomeFile` (`cannot read …; fix or remove it`) instead of being overwritten (Step 6). Match it with `errors.Is(err, appdir.ErrBadHomeFile)` and tell the user to fix or remove the file; the reader's own error stays wrapped (`fs.ErrPermission` for an unreadable file).
- **Outside a git work tree there is no `.gitignore`.** `InitProject` skips the block when no parent folder holds `.git`; run `WriteGitignore` after `git init` ([The .gitignore block](../layout/gitignore.md)).
- **A `.gitignore` with broken markers stops `InitProject` after the home is made.** It returns the marker path and an `ErrGitignoreMarkers` error; the home exists, only the block was not written.
- **`InitHome` on an existing home does not rewrite `home.jsonc`.** It does not change `scope` or raise `layout`. A newer `layout` than the binary knows fails with `ErrLayoutTooNew` before anything is written.
- **`InitHome` only accepts `ScopeGlobal` or `ScopeProject`.** Any other scope is `ErrInvalidOption`.
- **Running `init` as root in someone else's home is refused** with `ErrRootOwned` unless the tool passes `WithAllowRoot(true)` ([Fallbacks](../homes/fallbacks.md#step-4--know-the-root-guard)).

## Related

- [First use](first-use.md)
- [How the project home is found](../homes/lookup.md)
- [Pointer keys](../pointers/pointer-keys.md) (pointer targets made with `InitHome`)
- [The .gitignore block](../layout/gitignore.md)
- Example: [examples/project-init](../../examples/project-init/README.md)
- SPEC: [5.8 home.jsonc](../SPEC.md#58-homejsonc), [3.4 Methods](../SPEC.md#34-methods)
