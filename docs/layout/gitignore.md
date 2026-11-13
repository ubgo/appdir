# The `.gitignore` block

A project home sits inside the repo, so git sees everything in it. appdir writes a block into `.<app>/.gitignore` that ignores everything and then re-includes only the kinds marked committed. You want this page to control what reaches git (commit templates, never commit a private subfolder) and to understand how appdir edits a `.gitignore` that also has your own lines.

## Before you start

- Read [Kinds](kinds.md): every kind has a git rule, `Commit()` or `Ignore()`.
- The project must be inside a git work tree: an entry named `.git` (folder or file) in the project folder or any parent. appdir never runs git. Outside one, init writes no `.gitignore` at all: the example's `== WithoutKind(home) and init` step inits `$TMP/plain`, which has no `.git`, and gets an empty `.lore/` (see [Kinds, Step 6](kinds.md#step-6--drop-the-home-file)).
- Import `github.com/ubgo/appdir`. `InitProject` writes the block for you (see [Initialize a project](../start/init.md)); `WriteGitignore` rewrites it later.

## Step 1 — See the block

`GitignoreBlock()` builds the text from the layout ([GitignoreBlock](ds:block?id=dirs-gitignoreblock-8bujrmug)):

- It starts with `# <app>:begin` and `*`: anything not listed is ignored, so a file the tool adds next year never reaches git by accident.
- `.gitignore` itself comes first, then each committed kind in layout order. A committed file adds `!path`; a committed folder adds `!path/` and `!path/**`. Every parent folder is re-included first (`!a/`, `!a/b/`), because git cannot re-include a file under an excluded folder.
- Each `WithGitCommit` pattern adds the parents of its fixed prefix (up to the first `*`, `?` or `[`), then `!pattern`.
- `WithGitIgnore` wins over every commit rule: a pattern equal to a committed kind's path or to a commit pattern drops that re-include and is not written again (`*` already ignores it). Every other `WithGitIgnore` pattern is written after the re-includes as a plain line, so it wins in git: it re-excludes something inside a committed folder, or a folder that is only re-included as the parent of a committed path.
- It ends with `# <app>:end`.

<!-- snippet: examples/custom-layout/main.go#block -->
```go
// showBlock prints the block appdir writes between the markers.
func (ex *example) showBlock() error {
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	fmt.Fprintf(ex.w, "\n== gitignore block\n%s", d.GitignoreBlock())
	return nil
}
```

With the layout from [Kinds](kinds.md) (`shared` renamed to `data`, a committed `templates` and `schema.jsonc`, `WithGitCommit("prompts/*.md")`, `WithGitIgnore("data/private/")`) the block is:

```text
# lore:begin
*
!.gitignore
!home.jsonc
!config.jsonc
!data/
!data/**
!templates/
!templates/**
!schema.jsonc
!prompts/
!prompts/*.md
data/private/
# lore:end
```

`config.local.jsonc`, the db, `run/`, `logs/` and `profiles/` are not listed, so `*` keeps them out of git.

## Step 2 — Write it, keeping your own lines

`WriteGitignore()` writes the block into the project home's `.gitignore` ([writeGitignoreAt](ds:block?id=dirs-writegitignoreat-6hp578mz)). It replaces only the text between the two markers and never touches other lines; with no markers in the file it appends the block. It returns `false` and writes nothing when the home is not inside a git work tree. The write is atomic (temp file, fsync, rename) and an existing file keeps its permissions.

Here `InitProject` writes the first block, the user adds a line above and below it, and a later version of the tool commits one more file:

<!-- snippet: examples/custom-layout/main.go#write-gitignore -->
```go
// writeGitignore runs init (which writes the block inside a git work tree),
// adds hand-written lines around the block, then writes the block again
// with one more commit pattern: only the text between the markers changes.
func (ex *example) writeGitignore() error {
	fmt.Fprintln(ex.w, "\n== WriteGitignore keeps hand-written lines")
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	marker, err := d.InitProject(ex.shop)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	p := filepath.Join(marker, gitignoreFile)
	old, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read %s: %w", gitignoreFile, err)
	}
	hand := handHeader + "\n" + string(old) + handFooterLine + "\n"
	if err := os.WriteFile(p, []byte(hand), filePerm); err != nil {
		return fmt.Errorf("write %s: %w", gitignoreFile, err)
	}
	// A later version of the tool commits one more file.
	d, err = ex.newDirs(appdir.WithGitCommit("glossary.md"))
	if err != nil {
		return err
	}
	written, err := d.WriteGitignore()
	if err != nil {
		return fmt.Errorf("write gitignore: %w", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read %s: %w", gitignoreFile, err)
	}
	fmt.Fprintf(ex.w, "in a git work tree: %t\n%s", written, b)
	return nil
}
```

## Step 3 — Broken markers stop the write

If a marker is missing on one side, appears twice, or `end` comes before `begin`, appdir cannot tell which text is its own, so it writes nothing and returns `ErrGitignoreMarkers`. The user fixes the file by hand.

<!-- snippet: examples/custom-layout/main.go#broken-markers -->
```go
// brokenMarkers doubles the begin marker: appdir refuses to guess which
// text is its own and leaves the file alone.
func (ex *example) brokenMarkers() error {
	fmt.Fprintln(ex.w, "\n== broken markers")
	p := filepath.Join(ex.markerDir(), gitignoreFile)
	b, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read %s: %w", gitignoreFile, err)
	}
	broken := "# " + appName + ":begin\n" + string(b)
	if err := os.WriteFile(p, []byte(broken), filePerm); err != nil {
		return fmt.Errorf("write %s: %w", gitignoreFile, err)
	}
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	_, err = d.WriteGitignore()
	fmt.Fprintf(ex.w, "error: %s\n", ex.rel(fmt.Sprint(err)))
	fmt.Fprintf(ex.w, "is ErrGitignoreMarkers: %t\n", errors.Is(err, appdir.ErrGitignoreMarkers))
	return os.WriteFile(p, b, filePerm) // put the good file back
}
```

## Try it

```sh
go run ./examples/custom-layout
```

The part of the output this page covers (the temp root is printed as `$TMP`):

```text
== WriteGitignore keeps hand-written lines
in a git work tree: true
# hand-written lines stay
# lore:begin
*
!.gitignore
!home.jsonc
!config.jsonc
!data/
!data/**
!templates/
!templates/**
!schema.jsonc
!prompts/
!prompts/*.md
!glossary.md
data/private/
# lore:end
notes.txt

== broken markers
error: $TMP/shop/.lore/.gitignore: # lore:begin / # lore:end missing, doubled or out of order; fix it by hand
is ErrGitignoreMarkers: true
```

## Pitfalls

- No git work tree, no block: `InitProject` and `WriteGitignore` skip the write silently outside a repo (`WriteGitignore` returns `false`). Run `git init` before `<app> init`, or call `WriteGitignore` again afterwards.
- The work-tree check walks up to the filesystem root, past `$HOME`. A project in a folder under any stray `.git` (a dotfiles repo in your home, for example) counts as inside a work tree.
- The `ErrGitignoreMarkers` message is `<path>: # <app>:begin / # <app>:end missing, doubled or out of order; fix it by hand`. It does not say which of the three it is; show the file to the user.
- Markers are matched as whole lines (trailing `\r` allowed). An indented or edited marker line counts as missing.
- The block lives in `.<app>/.gitignore`, so its patterns are relative to `.<app>/`. `WithGitCommit` and `WithGitIgnore` take paths relative to the home, not to the repo root.
- `WithGitIgnore("data/private/")` after `!data/**` re-excludes the subfolder, but files git already tracks stay tracked; remove them with `git rm --cached`.
- Dropping the `gitignore` kind with `WithoutKind` turns the block writing off: `WriteGitignore` returns `false`.

## Related

- [Kinds](kinds.md): `Commit()` and `Ignore()` on a kind.
- [Initialize a project](../start/init.md): `InitProject` writes the first block.
- [Files and permissions](files-and-permissions.md): the atomic write and file modes.
- Example: [examples/custom-layout](../../examples/custom-layout).
- SPEC: [5.7 .gitignore block](../SPEC.md#57-gitignore-block).
