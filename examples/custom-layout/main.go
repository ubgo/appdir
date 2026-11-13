// Command custom-layout changes the standard layout of a home and shows what
// follows from it: the kinds in effect and their env names, the gitignore
// block, writing it into a .gitignore that also has hand-written lines,
// Ignore() and unknown kinds, init without a home file, Ensure with
// owner-only permissions and CACHEDIR.TAG, WithPerm and WithCacheTag(false),
// the symlink rule, VerifyFile and the layout-version guard.
//
// Everything happens inside one temp root: the user home is root/userhome (via
// WithHomeDir) and the project is root/shop, a git work tree (a .git folder
// is enough: appdir never runs git). Paths are printed with the root as $TMP.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example.
const (
	appName    = "lore"
	userHome   = "userhome"
	projectDir = "shop"
	gitDir     = ".git"
	outsideDir = "outside" // a folder that is not part of any home
	tmpLabel   = "$TMP"    // printed in place of the temp root

	// Kinds this tool adds to the standard layout.
	kindTemplates appdir.Kind = "templates" // committed prompt templates
	kindIdentity  appdir.Kind = "identity"  // login data: always global
	kindSchema    appdir.Kind = "schema"    // part of what makes a home
	kindThumbs    appdir.Kind = "thumbs"    // thumbnails: safe to delete
	kindPreviews  appdir.Kind = "previews"  // added only by the WithPerm step

	cacheTag       = "CACHEDIR.TAG"
	gitignoreFile  = ".gitignore"
	homeInfoFile   = "home.jsonc"
	dirPerm        = 0o700
	filePerm       = 0o600
	looserDirPerm  = 0o750 // set by hand to show Ensure leaves it alone
	newerLayout    = 2     // a home written by a newer binary
	handHeader     = "# hand-written lines stay"
	handFooterLine = "notes.txt"
	plainDir       = "plain"     // a project folder outside any git work tree
	teamHomeDir    = "team-home" // a home shared with a group
	groupDirPerm   = 0o750       // WithPerm step: group may read
	groupFilePerm  = 0o640
	noteFile       = "note.md" // a file the tool opens itself, not a kind
)

func main() {
	root, err := os.MkdirTemp("", "appdir-custom-layout-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "custom-layout:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp folder
	if err != nil {
		fmt.Fprintln(os.Stderr, "custom-layout:", err)
		os.Exit(1)
	}
}

// example holds the folders of one run, so every step reads the same names.
type example struct {
	w    io.Writer
	root string
	home string // the user's home folder
	shop string // the project checkout, a git work tree
}

// run walks through every step in order inside root. Steps share one
// project, so the order matters: each step starts from the files the
// previous one left.
func run(w io.Writer, root string) error {
	// Resolve the temp root once: appdir compares real paths, and on macOS
	// the temp folder sits behind the /var -> /private/var link.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	ex := &example{w: w, root: root, home: filepath.Join(root, userHome), shop: filepath.Join(root, projectDir)}
	for _, dir := range []string{ex.home, filepath.Join(ex.shop, gitDir)} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	steps := []func() error{
		ex.showKinds,
		ex.showModes,
		ex.badKind,
		ex.kindEdits,
		ex.noHomeFile,
		ex.showBlock,
		ex.writeGitignore,
		ex.brokenMarkers,
		ex.ensure,
		ex.customPerm,
		ex.symlink,
		ex.verifyFile,
		ex.layoutGuard,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// snippet:layout
// layout is the tool's own layout: every change is an option of New, so
// paths --json, the gitignore block and Ensure all follow it.
func layout() []appdir.Option {
	return []appdir.Option{
		// Rename: shared/ becomes data/. Same kind, new path; it stays
		// committed and in the data scope.
		appdir.WithKind(appdir.KindShared, appdir.Folder("data")),
		// A new committed folder in the data scope.
		appdir.WithKind(kindTemplates, appdir.Commit()),
		// Login data belongs to the user, not to a project.
		appdir.WithKind(kindIdentity, appdir.File("identity.json"), appdir.Scoped(appdir.ScopeGlobal)),
		// A file next to home.jsonc in both profiles, with no env override.
		appdir.WithKind(kindSchema, appdir.File("schema.jsonc"), appdir.Commit(), appdir.Anchored()),
		// Safe to delete: gets CACHEDIR.TAG when Ensure creates it.
		appdir.WithKind(kindThumbs, appdir.Disposable()),
		// This tool keeps no backups.
		appdir.WithoutKind(appdir.KindBackups),
		// Commit prompt files, but never data/private/.
		appdir.WithGitCommit("prompts/*.md"),
		appdir.WithGitIgnore("data/private/"),
	}
}

// snippet:end

// newDirs resolves lore from the project folder with a fixed home and an
// empty environment, so nothing on this machine changes the result.
func (ex *example) newDirs(extra ...appdir.Option) (*appdir.Dirs, error) {
	noEnv := func(string) (string, bool) { return "", false }
	opts := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return ex.home, nil }),
		appdir.WithWorkDir(ex.shop),
		appdir.WithEnv(noEnv),
		appdir.WithRelease(true), // prod profile: data stays in .lore/ itself
	}
	opts = append(opts, layout()...)
	d, err := appdir.New(appName, append(opts, extra...)...)
	if err != nil {
		// Not wrapped: badKind prints this error, and the guide quotes it.
		return nil, err
	}
	return d, nil
}

// markerDir is the project home folder (shop/.lore) that init creates.
func (ex *example) markerDir() string {
	return filepath.Join(ex.shop, "."+appName)
}

// snippet:kinds
// showKinds prints the layout in effect, in the order Kinds returns it.
func (ex *example) showKinds() error {
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	fmt.Fprintf(ex.w, "== kinds (mode %s)\n", d.Mode())
	for _, k := range d.Kinds() {
		var flags []string
		if k.Anchored {
			flags = append(flags, "anchored")
		}
		if k.Disposable {
			flags = append(flags, "disposable")
		}
		env := d.KindEnv(k.Name)
		if env == "" {
			env = "-"
		}
		line := fmt.Sprintf("%-12s %-18s %-6s %-6s %-19s %-18s %s",
			k.Name, k.Path, k.Type, k.Git, k.Scope, env, strings.Join(flags, ","))
		fmt.Fprintln(ex.w, strings.TrimRight(line, " "))
	}
	return nil
}

// snippet:end

// snippet:modes
// showModes compares the data scope of the two modes. A global tool passes
// WithMode(ModeGlobal); the default ModeProject fails safe outside projects.
func (ex *example) showModes() error {
	fmt.Fprintln(ex.w, "\n== mode and scope")
	for _, m := range []appdir.Mode{appdir.ModeProject, appdir.ModeGlobal} {
		d, err := ex.newDirs(appdir.WithMode(m))
		if err != nil {
			return err
		}
		scopes := map[appdir.Kind]appdir.Scope{}
		for _, k := range d.Kinds() {
			scopes[k.Name] = k.Scope
		}
		fmt.Fprintf(ex.w, "%-7s db=%s logs=%s cache=%s\n",
			d.Mode(), scopes[appdir.KindDB], scopes[appdir.KindLogs], scopes[appdir.KindCache])
	}
	return nil
}

// snippet:end

// snippet:bad-kind
// badKind shows that a kind path may not leave the home.
func (ex *example) badKind() error {
	fmt.Fprintln(ex.w, "\n== invalid kind")
	_, err := ex.newDirs(appdir.WithKind("escape", appdir.Folder("../outside")))
	fmt.Fprintf(ex.w, "error: %v\n", err)
	fmt.Fprintf(ex.w, "is ErrInvalidOption: %t\n", errors.Is(err, appdir.ErrInvalidOption))
	return nil
}

// snippet:end

// snippet:kind-edits
// kindEdits turns the committed shared kind into an ignored one with
// Ignore(), then asks for backups, which layout() dropped: every call that
// takes a kind answers ErrUnknownKind instead of guessing a path.
func (ex *example) kindEdits() error {
	fmt.Fprintln(ex.w, "\n== Ignore() and unknown kinds")
	d, err := ex.newDirs(appdir.WithKind(appdir.KindShared, appdir.Ignore()))
	if err != nil {
		return err
	}
	spec, _ := kindSpec(d, appdir.KindShared)
	fmt.Fprintf(ex.w, "shared git rule: %s; block re-includes data/: %t\n",
		spec.Git, strings.Contains(d.GitignoreBlock(), "!data/\n"))
	_, err = d.Path(appdir.KindBackups)
	fmt.Fprintf(ex.w, "Path(backups): %v\n", err)
	_, err = d.Ensure(appdir.KindBackups)
	fmt.Fprintf(ex.w, "Ensure(backups): %v\n", err)
	fmt.Fprintf(ex.w, "is ErrUnknownKind: %t\n", errors.Is(err, appdir.ErrUnknownKind))
	return nil
}

// snippet:end

// snippet:no-home-file
// noHomeFile drops the home kind: init then creates only the .lore folder
// and writes no home.jsonc (and, outside a git work tree, no .gitignore).
func (ex *example) noHomeFile() error {
	fmt.Fprintln(ex.w, "\n== WithoutKind(home) and init")
	d, err := ex.newDirs(appdir.WithoutKind(appdir.KindHome))
	if err != nil {
		return err
	}
	marker, err := d.InitProject(filepath.Join(ex.root, plainDir))
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	entries, err := os.ReadDir(marker)
	if err != nil {
		return fmt.Errorf("read %s: %w", marker, err)
	}
	fmt.Fprintf(ex.w, "init %s: folder %s, %d files, home.jsonc %s\n",
		ex.rel(marker), mode(marker), len(entries), mode(filepath.Join(marker, homeInfoFile)))
	return nil
}

// snippet:end

// snippet:block
// showBlock prints the block appdir writes between the markers.
func (ex *example) showBlock() error {
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	fmt.Fprintf(ex.w, "\n== gitignore block\n%s", d.GitignoreBlock())
	return nil
}

// snippet:end

// snippet:write-gitignore
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

// snippet:end

// snippet:broken-markers
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

// snippet:end

// snippet:ensure
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

// snippet:end

// snippet:perm
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

// snippet:end

// snippet:symlink
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

// snippet:end

// snippet:verify-file
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

// snippet:end

// snippet:layout-guard
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

// snippet:end

// kindSpec finds one kind of the layout in effect.
func kindSpec(d *appdir.Dirs, k appdir.Kind) (appdir.KindSpec, bool) {
	for _, s := range d.Kinds() {
		if s.Name == k {
			return s, true
		}
	}
	return appdir.KindSpec{}, false
}

// mode prints a file's permission bits, or "missing".
func mode(p string) string {
	fi, err := os.Stat(p)
	if err != nil {
		return "missing"
	}
	return fmt.Sprintf("%04o", fi.Mode().Perm())
}

// exists reports whether p exists.
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// rel replaces the temp root in s with $TMP, so the output reads the same on
// every run.
func (ex *example) rel(s string) string {
	return strings.ReplaceAll(s, ex.root, tmpLabel)
}
