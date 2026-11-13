// Command worktree shows how appdir treats git worktrees and submodules
// without running git: a linked worktree uses the main checkout's .lore/, a
// main checkout without one makes the worktree look for its own (with a
// warning when the worktree has one; silent when neither has), a submodule
// is its own project, a worktree of a bare repo uses itself, and here-only
// lookup still uses the main checkout's home.
//
// The example writes the files `git worktree add` and `git submodule add`
// leave behind (the .git file, and gitdir / commondir in the repo's .git),
// so it needs no git binary.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example.
const (
	appName      = "lore"
	marker       = ".lore"
	gitEntry     = ".git"
	homeDir      = "home"
	projectsDir  = "home/projects"
	worktreesDir = "worktrees" // inside the main repo's git dir
	modulesDir   = "modules"   // inside the main repo's git dir
	gitdirPrefix = "gitdir: "
	commondir    = "commondir"
	gitdirFile   = "gitdir"
	headFile     = "HEAD"
	headBody     = "ref: refs/heads/main\n"
	// commondirBody is what git writes in a worktree's admin folder: the
	// main git dir, relative to <git>/worktrees/<wt>.
	commondirBody = "../..\n"
)

// lookupCase is one run of the lookup from a working folder.
type lookupCase struct {
	name    string
	workDir string // relative to projectsDir
	style   appdir.Lookup
}

func main() {
	root, err := os.MkdirTemp("", "appdir-worktree-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "worktree:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "worktree:", err)
		os.Exit(1)
	}
}

// run builds the repos and resolves from each working folder.
func run(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	projects := filepath.Join(root, filepath.FromSlash(projectsDir))
	if err := build(projects); err != nil {
		return err
	}
	home := filepath.Join(root, homeDir)
	cases := []lookupCase{
		{name: "main checkout", workDir: "shop/src"},
		{name: "linked worktree", workDir: "shop-fix/src"},
		{name: "main has no .lore", workDir: "blog-fix"},
		{name: "neither has .lore", workDir: "wiki-fix"},
		{name: "submodule", workDir: "shop/vendor/lib"},
		{name: "worktree of a bare repo", workDir: "tool-wt"},
		{name: "here-only in a worktree", workDir: "shop-fix", style: appdir.HereOnly},
		{name: "home only in worktree", workDir: "shop-fix/pkg"},
	}
	for _, c := range cases {
		if err := resolve(w, root, home, projects, c); err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
	}
	return nil
}

// build lays out the repos under projects.
func build(projects string) error {
	steps := []func() error{
		// shop: a main checkout with a project home, a linked worktree
		// shop-fix, and a submodule vendor/lib with its own home.
		func() error { return mkdirs(projects, "shop/.git", "shop/.lore", "shop/src") },
		func() error { return linkWorktree(projects, "shop", "shop-fix", "src") },
		func() error { return addSubmodule(projects, "shop", "vendor/lib") },
		// A nested home made on the worktree's branch only: the lookup runs
		// in the main checkout, which has none on that path, so shop's root
		// home wins over it.
		func() error { return mkdirs(projects, "shop-fix/pkg/.lore") },
		// blog: the main checkout has no home, the worktree has one.
		func() error { return mkdirs(projects, "blog/.git") },
		func() error { return linkWorktree(projects, "blog", "blog-fix", marker) },
		// wiki: no home anywhere.
		func() error { return mkdirs(projects, "wiki/.git") },
		func() error { return linkWorktree(projects, "wiki", "wiki-fix", "") },
		// tool.git: a bare repo (its git dir is not named .git) with a
		// worktree that has its own home.
		func() error { return mkdirs(projects, "tool.git") },
		func() error { return bareWorktree(projects, "tool.git", "tool-wt") },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return fmt.Errorf("build repos: %w", err)
		}
	}
	return nil
}

// snippet:worktree
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

// snippet:end

// snippet:submodule
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

// snippet:end

// bareWorktree writes a worktree of a bare repo: its commondir points at the
// bare repo folder itself, which is not named .git, so there is no main
// checkout to share.
func bareWorktree(projects, bare, wt string) error {
	admin := filepath.Join(projects, bare, worktreesDir, wt)
	if err := mkdirs(projects, filepath.Join(wt, marker)); err != nil {
		return err
	}
	if err := os.MkdirAll(admin, 0o700); err != nil {
		return fmt.Errorf("bare worktree admin folder: %w", err)
	}
	return writeFiles(map[string]string{
		filepath.Join(projects, wt, gitEntry): gitdirPrefix + filepath.ToSlash(admin) + "\n",
		filepath.Join(admin, commondir):       commondirBody,
		filepath.Join(admin, headFile):        headBody,
	})
}

// snippet:resolve
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

// snippet:end

// snippet:warnings
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

// snippet:end

// mkdirs creates folders relative to base.
func mkdirs(base string, rels ...string) error {
	for _, r := range rels {
		if err := os.MkdirAll(filepath.Join(base, filepath.FromSlash(r)), 0o700); err != nil {
			return fmt.Errorf("create %s: %w", r, err)
		}
	}
	return nil
}

// writeFiles writes each file with mode 0600.
func writeFiles(files map[string]string) error {
	for p, body := range files {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", filepath.Base(p), err)
		}
	}
	return nil
}

// show prints a path relative to root, so the output is the same on every run.
func show(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}

// hide removes the temp root from a message for stable output.
func hide(root, msg string) string {
	return filepath.ToSlash(strings.ReplaceAll(msg, root+string(filepath.Separator), ""))
}
