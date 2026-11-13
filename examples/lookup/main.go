// Command lookup shows how appdir finds the project home: the walk-up from
// the working folder, the walls that stop it (a repo root, the user's home,
// a ceiling folder), the real-path rule, nearest-wins in a monorepo, the
// here-only style, and the ErrNoProject message when nothing is found.
//
// It builds a small tree of folders inside a temp root (the user's home is
// root/home) and resolves from several working folders. git is never run: an
// entry named .git is all the lookup looks at.
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
	appName     = "lore"
	homeDir     = "home"
	ceilingEnv  = "LORE_CEILING_DIRS"
	relativeDir = "projects" // a relative ceiling entry: ignored with a warning
)

// treeFolders lists the folders the example creates, relative to root.
// Entries ending in .lore are project homes; entries ending in .git are repo
// roots. A function, not a variable: the example keeps no package state.
func treeFolders() []string {
	return []string{
		".lore",                             // a stray home above the user's home
		"work/.lore",                        // a project outside the user's home ...
		"work/a/b",                          // ... and a folder two levels down
		"home/.lore",                        // the global home
		"home/projects/shop/.git",           // a repo ...
		"home/projects/shop/.lore",          // ... with a project home at its root
		"home/projects/shop/src/api",        // a folder deep inside it
		"home/projects/notes/sub",           // no repo, no project home
		"home/projects/mono/.git",           // a monorepo ...
		"home/projects/mono/.lore",          // ... with a home at the root
		"home/projects/mono/pkg/web/.lore",  // ... and a nearer one in a package
		"home/projects/mono/pkg/web/src",    //
		"home/projects/mono/vendor/lib/src", // a nested repo (its .git is a file)
		"work/repo/src",                     // a repo whose .git is a dangling symlink
	}
}

// The nested repo's .git file, as git writes it for a submodule. A .git
// file is a wall just like a .git folder.
const (
	nestedGit     = "home/projects/mono/vendor/lib/.git"
	nestedGitBody = "gitdir: ../../.git/modules/lib\n"
)

// A .git symlink whose target is gone (a moved worktree, a half-deleted
// clone). It is still a wall: the lookup checks for the name, not the target.
const (
	danglingGit    = "work/repo/.git"
	danglingTarget = "../gone/.git"
)

// A symlink into the shop repo: the lookup walks the real path, so starting
// from the link finds shop's home, not one above the link.
const (
	linkPath   = "home/projects/api-link"
	linkTarget = "shop/src/api" // relative to the link's folder
)

// lookupCase is one run of the lookup from a working folder.
type lookupCase struct {
	name    string
	workDir string // relative to root
	env     map[string]string
	style   appdir.Lookup
}

func main() {
	root, err := os.MkdirTemp("", "appdir-lookup-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "lookup:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "lookup:", err)
		os.Exit(1)
	}
}

// run builds the tree and resolves each case, printing one line per case.
func run(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	if err := build(root); err != nil {
		return err
	}
	home := filepath.Join(root, homeDir)
	var cases []lookupCase
	cases = append(cases, walkCases()...)
	cases = append(cases, ceilingCases(root)...)
	cases = append(cases, hereOnlyCases()...)
	for _, c := range cases {
		if err := resolve(w, root, home, c); err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
	}
	return nil
}

// snippet:walk
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

// snippet:end

// snippet:ceilings
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

// snippet:end

// snippet:hereonly
// hereOnlyCases use WithLookup(HereOnly) (Step 4): only the working folder
// itself is checked, so a deep folder finds nothing and the project root
// finds its own .lore/.
func hereOnlyCases() []lookupCase {
	return []lookupCase{
		{name: "here-only, deep folder", workDir: "home/projects/shop/src/api", style: appdir.HereOnly},
		{name: "here-only, project root", workDir: "home/projects/shop", style: appdir.HereOnly},
	}
}

// snippet:end

// build creates the folders, the nested repo's .git file and the symlink.
func build(root string) error {
	for _, p := range treeFolders() {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(p)), 0o700); err != nil {
			return fmt.Errorf("build tree: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(nestedGit)), []byte(nestedGitBody), 0o600); err != nil {
		return fmt.Errorf("write .git file: %w", err)
	}
	if err := os.Symlink(linkTarget, filepath.Join(root, filepath.FromSlash(linkPath))); err != nil {
		return fmt.Errorf("symlink: %w", err)
	}
	if err := os.Symlink(danglingTarget, filepath.Join(root, filepath.FromSlash(danglingGit))); err != nil {
		return fmt.Errorf("dangling .git symlink: %w", err)
	}
	return nil
}

// snippet:resolve
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

// snippet:end

// snippet:noproject
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

// snippet:end

// show prints a path relative to root, so the output is the same on every run.
func show(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}
