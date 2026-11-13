# Files and permissions

`Ensure` and the `Init*` calls are the only places appdir writes. What they create is private to the user: folders `0700`, files `0600` (a protected ACL on Windows). Disposable folders get a `CACHEDIR.TAG` so backup tools skip them. A file kind may be a symlink only when it points inside its own home, and a home written by a newer binary refuses writes. You want this page to know what lands on disk and which checks run before a write.

## Before you start

- Read [Kinds](kinds.md): `Disposable()` and file versus folder kinds matter here.
- Reads never create anything; `Ensure(kind)` is the lazy creation point ([Ensure](ds:block?id=dirs-ensure-mxbz3fmd)). It creates a folder kind's folder, or a file kind's parent folder (never the file), and returns the path. See [First use](../start/first-use.md).
- Import `github.com/ubgo/appdir`.

## Step 1 — Ensure creates private folders

Folders are created `0700` and files `0600`; `WithPerm(dir, file)` changes both. Every missing parent that `Ensure` creates gets the same mode. Existing folders and files keep their permissions: appdir only restricts what it created. A disposable folder gets `CACHEDIR.TAG` (the standard signature line `Signature: 8a477f597d28d172789f06886806bc55`) when it is created, or when the tag is missing; `WithCacheTag(false)` turns that off ([tagCache](ds:block?id=dirs-tagcache-8cz6ae4w)).

<!-- snippet: examples/custom-layout/main.go#ensure -->
```go
// ensure creates folders on first use: 0700 folders, 0600 files, and a
// CACHEDIR.TAG in disposable ones. A mode set by hand is left alone.
func (ex *example) ensure() error {
	fmt.Fprintln(ex.w, "\n== Ensure")
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	for _, k := range []appdir.Kind{kindTemplates, kindThumbs, appdir.KindCache, kindIdentity} {
		p, err := d.Ensure(k)
		if err != nil {
			return fmt.Errorf("ensure %s: %w", k, err)
		}
		folder := p
		if spec, _ := kindSpec(d, k); spec.Type == appdir.TypeFile {
			folder = filepath.Dir(p) // Ensure makes a file kind's folder only
		}
		fmt.Fprintf(ex.w, "%-9s %-34s folder %s  CACHEDIR.TAG %t\n",
			k, ex.rel(p), mode(folder), exists(filepath.Join(folder, cacheTag)))
	}
	homeInfo := filepath.Join(ex.markerDir(), homeInfoFile)
	fmt.Fprintf(ex.w, "home.jsonc file %s\n", mode(homeInfo))

	tpl, err := d.Path(kindTemplates)
	if err != nil {
		return fmt.Errorf("path %s: %w", kindTemplates, err)
	}
	if err := os.Chmod(tpl, looserDirPerm); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	if _, err := d.Ensure(kindTemplates); err != nil {
		return fmt.Errorf("ensure %s again: %w", kindTemplates, err)
	}
	fmt.Fprintf(ex.w, "templates after chmod %04o and Ensure: %s\n", looserDirPerm, mode(tpl))
	return nil
}
```

## Step 2 — Platform extras (not run here)

These run on their own OS and cannot be shown by the example on Linux:

- Windows: instead of mode bits, every folder and file appdir creates gets a protected DACL that grants full access to the current user's SID and to SYSTEM only, with inheritance from the parent turned off; folders pass the same two entries on to their children. The SDDL is `D:P(A;OICI;FA;;;<user-sid>)(A;OICI;FA;;;SY)` for folders and `D:P(A;;FA;;;<user-sid>)(A;;FA;;;SY)` for files. `WithPerm` has no effect there.
- macOS: a disposable folder also gets an empty `.metadata_never_index` (Spotlight skips it) and the Time Machine exclusion through `tmutil addexclusion <dir>`. That is the one place appdir runs a command; it is best effort with a 5 second timeout, and a missing or failing `tmutil` never blocks creating the cache.
- Windows: a disposable folder also gets the "not content indexed" attribute (`FILE_ATTRIBUTE_NOT_CONTENT_INDEXED`).
- Linux and the BSDs: `CACHEDIR.TAG` only.

## Step 3 — The symlink rule

A home folder may be a symlink (a linked private folder, for example). A file kind (the db, the config files, your own `File` kinds) may be a symlink only when its real path is inside the real path of the home it belongs to; otherwise `Ensure` fails with `ErrSymlinkEscape`, so a planted link cannot send writes elsewhere ([VerifyFile](ds:block?id=dirs-verifyfile-2nyrnmv3)). `Ensure` checks file kinds that already exist, and a tool calls `VerifyFile(path)` itself before opening a file it did not get from `Ensure`. A broken link fails too. appdir never creates links.

<!-- snippet: examples/custom-layout/main.go#symlink -->
```go
// symlink makes the db a link. A link inside the home is fine; a link to a
// file outside it fails, so a planted link cannot redirect writes.
func (ex *example) symlink() error {
	fmt.Fprintln(ex.w, "\n== symlink rule")
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	db, err := d.Path(appdir.KindDB)
	if err != nil {
		return fmt.Errorf("path db: %w", err)
	}
	inside := filepath.Join(filepath.Dir(db), "real.db")
	outside := filepath.Join(ex.root, outsideDir, "stolen.db")
	for _, target := range []string{inside, outside} {
		if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
			return fmt.Errorf("create: %w", err)
		}
		if err := os.WriteFile(target, nil, filePerm); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		_ = os.Remove(db) // replace the previous link, if any
		if err := os.Symlink(target, db); err != nil {
			return fmt.Errorf("link: %w", err)
		}
		_, err := d.Ensure(appdir.KindDB)
		fmt.Fprintf(ex.w, "db -> %s: Ensure error: %s\n", ex.rel(target), ex.rel(fmt.Sprint(err)))
		if err != nil {
			fmt.Fprintf(ex.w, "is ErrSymlinkEscape: %t\n", errors.Is(err, appdir.ErrSymlinkEscape))
		}
	}
	return os.Remove(db)
}
```

## Step 4 — The layout version guard

`home.jsonc` records the layout version of the binary that created the home. A binary built with a lower `WithLayoutVersion` (default 1) refuses to write into it with `ErrLayoutTooNew`, so an old binary never writes an old layout into a home a newer binary already migrated ([checkWrite](ds:block?id=dirs-checkwrite-n23fhubq)). Reads still work. The same check runs in `Ensure`, `InitProject` and `InitHome`.

<!-- snippet: examples/custom-layout/main.go#layout-guard -->
```go
// layoutGuard makes the home look written by a newer binary. This binary
// knows layout 1, so it refuses to write there; a binary built with
// WithLayoutVersion(2) carries on.
func (ex *example) layoutGuard() error {
	fmt.Fprintln(ex.w, "\n== layout version guard")
	homeInfo := filepath.Join(ex.markerDir(), homeInfoFile)
	body := fmt.Sprintf(`{ "app": %q, "layout": %d, "scope": %q }`+"\n", appName, newerLayout, appdir.ScopeProject)
	if err := os.WriteFile(homeInfo, []byte(body), filePerm); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	for _, extra := range [][]appdir.Option{nil, {appdir.WithLayoutVersion(newerLayout)}} {
		d, err := ex.newDirs(extra...)
		if err != nil {
			return err
		}
		_, err = d.Ensure(appdir.KindRun)
		fmt.Fprintf(ex.w, "Ensure(run): %s\n", ex.rel(fmt.Sprint(err)))
		if err != nil {
			fmt.Fprintf(ex.w, "is ErrLayoutTooNew: %t\n", errors.Is(err, appdir.ErrLayoutTooNew))
		}
	}
	return nil
}
```

## Step 5 — Change the modes with `WithPerm`, drop the cache tag

`WithPerm(dir, file)` sets the mode of every folder and file appdir creates from then on: the folders `Ensure` and the `Init*` calls make, and the files they write (`home.jsonc`, `.gitignore`, `CACHEDIR.TAG`). Use it for a home a group shares, say `0750` / `0640`. `WithCacheTag(false)` turns off `CACHEDIR.TAG` and the macOS / Windows index markers of Step 2, for a tool whose cache should be backed up. Both are options of `New`, so they hold for the whole `Dirs`.

<!-- snippet: examples/custom-layout/main.go#perm -->
```go
// customPerm builds a group-readable layout with no cache tags: WithPerm
// sets the mode of every folder and file appdir creates, WithCacheTag(false)
// skips CACHEDIR.TAG (and the macOS/Windows index markers).
func (ex *example) customPerm() error {
	fmt.Fprintln(ex.w, "\n== WithPerm and WithCacheTag(false)")
	d, err := ex.newDirs(
		appdir.WithKind(kindPreviews, appdir.Disposable()),
		appdir.WithPerm(groupDirPerm, groupFilePerm),
		appdir.WithCacheTag(false),
	)
	if err != nil {
		return err
	}
	p, err := d.Ensure(kindPreviews)
	if err != nil {
		return fmt.Errorf("ensure %s: %w", kindPreviews, err)
	}
	fmt.Fprintf(ex.w, "%-9s %-34s folder %s  CACHEDIR.TAG %t\n",
		kindPreviews, ex.rel(p), mode(p), exists(filepath.Join(p, cacheTag)))
	team := filepath.Join(ex.root, teamHomeDir)
	if err := d.InitHome(team, appdir.ScopeGlobal); err != nil {
		return fmt.Errorf("init home: %w", err)
	}
	fmt.Fprintf(ex.w, "InitHome %s: folder %s, home.jsonc %s\n",
		ex.rel(team), mode(team), mode(filepath.Join(team, homeInfoFile)))
	return nil
}
```

## Step 6 — Check a file that is not a kind with `VerifyFile`

`Ensure` checks the file kinds. A tool that opens other files inside a home (a note in `data/`, an attachment, a lock file it names itself) calls `VerifyFile(path)` first: it applies the same symlink rule and returns `nil` for a plain file or a file that does not exist yet, and `ErrSymlinkEscape` for a link whose real path leaves the home it sits in.

<!-- snippet: examples/custom-layout/main.go#verify-file -->
```go
// verifyFile checks a file the tool opens on its own: a note in data/ is
// not a kind, so Ensure never looks at it. VerifyFile applies the same
// symlink rule: a plain file or no file passes, a link out of the home fails.
func (ex *example) verifyFile() error {
	fmt.Fprintln(ex.w, "\n== VerifyFile")
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	shared, err := d.Ensure(appdir.KindShared)
	if err != nil {
		return fmt.Errorf("ensure %s: %w", appdir.KindShared, err)
	}
	note := filepath.Join(shared, noteFile)
	fmt.Fprintf(ex.w, "missing note: %v\n", d.VerifyFile(note))
	target := filepath.Join(ex.root, outsideDir, noteFile)
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("create: %w", err)
	}
	if err := os.WriteFile(target, nil, filePerm); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := os.Symlink(target, note); err != nil {
		return fmt.Errorf("link: %w", err)
	}
	err = d.VerifyFile(note)
	fmt.Fprintf(ex.w, "linked note: %s\n", ex.rel(fmt.Sprint(err)))
	fmt.Fprintf(ex.w, "is ErrSymlinkEscape: %t\n", errors.Is(err, appdir.ErrSymlinkEscape))
	return os.Remove(note)
}
```

## Try it

```sh
go run ./examples/custom-layout
```

The part of the output this page covers (the temp root is printed as `$TMP`):

```text
== Ensure
templates $TMP/shop/.lore/templates          folder 0700  CACHEDIR.TAG false
thumbs    $TMP/shop/.lore/thumbs             folder 0700  CACHEDIR.TAG true
cache     $TMP/userhome/.lore/cache          folder 0700  CACHEDIR.TAG true
identity  $TMP/userhome/.lore/identity.json  folder 0700  CACHEDIR.TAG false
home.jsonc file 0600
templates after chmod 0750 and Ensure: 0750

== WithPerm and WithCacheTag(false)
previews  $TMP/shop/.lore/previews           folder 0750  CACHEDIR.TAG false
InitHome $TMP/team-home: folder 0750, home.jsonc 0640

== symlink rule
db -> $TMP/shop/.lore/real.db: Ensure error: <nil>
db -> $TMP/outside/stolen.db: Ensure error: $TMP/shop/.lore/lore.db links to $TMP/outside/stolen.db, outside the home $TMP/shop/.lore
is ErrSymlinkEscape: true

== VerifyFile
missing note: <nil>
linked note: $TMP/shop/.lore/data/note.md links to $TMP/outside/note.md, outside the home $TMP/shop/.lore
is ErrSymlinkEscape: true

== layout version guard
Ensure(run): home $TMP/shop/.lore has layout 2, this binary knows 1; upgrade lore
is ErrLayoutTooNew: true
Ensure(run): <nil>
```

## Pitfalls

- `Ensure` on a file kind creates its folder, not the file. Your code creates the file; open it with mode `0600` yourself, since appdir only sets modes on what it writes (`home.jsonc`, `.gitignore`, `CACHEDIR.TAG`).
- Existing permissions are never tightened. A `.lore/` made by hand with `mkdir` keeps `0755`; only folders `Ensure` creates are `0700`.
- `WithPerm` is not checked: `WithPerm(0o755, 0o644)` makes a world-readable home. It also changes the mode of files appdir writes into an existing home only when it creates them: an existing `home.jsonc` or `.gitignore` keeps its mode on rewrite.
- `WithCacheTag(false)` does not remove a `CACHEDIR.TAG` an earlier run wrote; delete it yourself if the cache should now be backed up.
- `VerifyFile` checks only links. A missing file and a plain file both return `nil`, so it does not prove the file exists; and it checks the path at the time of the call, so open the file right after.
- `ErrSymlinkEscape` also covers a broken link: the message is `<path> links to <real path>, outside the home <home>`, or `<path> is a broken symlink` when the target is gone. Both stop `Ensure` and `CheckDB`.
- The symlink check covers file kinds only. A folder kind that is a symlink (say `cache/` linked to another disk) is followed without a check.
- The home a file belongs to is the longest known home folder that holds it by path. A file kind moved by `<APP>_<KIND>_DIR` outside every home is checked against its own folder.
- In the dev profile the db's home is `.<app>/profiles/dev/`, the longest home folder that holds it. A dev db linked to a file in `.<app>/` itself (the prod data) is an escape and fails.
- `ErrLayoutTooNew` blocks writes only; a tool that also opens its database read-write without `Ensure` must call `Ensure` (or check `Home.Info.Layout` itself) first.
- On macOS, a sandboxed or restricted environment without `tmutil` silently skips the Time Machine exclusion; `CACHEDIR.TAG` is still written.

## Related

- [Kinds](kinds.md): `Disposable()`, `File` and `Folder`.
- [Fallbacks](../homes/fallbacks.md): the temp fallback of disposable kinds and running as root.
- [Prod-db guard](../profiles/prod-db-guard.md): `CheckDB` also runs `VerifyFile`.
- [Errors](../reference/errors.md): every sentinel with its message.
- Example: [examples/custom-layout](../../examples/custom-layout).
- SPEC: [5.4 Permissions](../SPEC.md#54-permissions), [5.5 Cache tagging](../SPEC.md#55-cache-tagging), [5.6 Symlinks](../SPEC.md#56-symlinks), [5.8 home.jsonc](../SPEC.md#58-homejsonc).
