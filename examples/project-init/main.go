// Command project-init shows the two write calls that make a home:
// InitProject (the `lore init` command) and InitHome (`lore init --home X`),
// what they write into home.jsonc, and what they refuse.
//
// Everything happens inside a temp root: the user's home is root/home (passed
// with WithHomeDir) and the tool runs in a git project under it.
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
	appName      = "lore"
	otherAppName = "notes" // a second app whose home lore must not take over
	homeDir      = "home"
	projectDir   = "home/projects/shop"
	sharedHome   = "home/team/lore"  // a home created with InitHome (pointer target)
	otherHome    = "home/team/notes" // a home that belongs to otherAppName
	gitDir       = ".git"
	homeFile     = "home.jsonc"
	ignoreFile   = ".gitignore"
	globalHome   = "~/.lore"                 // a user path: InitHome expands ~ against the user's home
	relativeDir  = "packages/api"            // InitProject joins it to the working folder
	brokenDir    = "home/projects/old"       // a project whose home.jsonc was hand-edited badly
	brokenJSON   = "{ \"app\": \"lore\", \n" // cut off mid-object: not parsable
	markerDir    = "." + appName             // the project home folder: .lore
)

func main() {
	root, err := os.MkdirTemp("", "appdir-project-init-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "project-init:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "project-init:", err)
		os.Exit(1)
	}
}

// run does the whole walk-through inside root and prints paths relative to it.
func run(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	home := filepath.Join(root, filepath.FromSlash(homeDir))
	project := filepath.Join(root, filepath.FromSlash(projectDir))
	// A .git folder makes the project a git work tree, so InitProject also
	// writes the .gitignore block. git itself is never run.
	if err := os.MkdirAll(filepath.Join(project, gitDir), 0o700); err != nil {
		return fmt.Errorf("create project: %w", err)
	}

	fmt.Fprintln(w, "== before init")
	if err := beforeInit(w, root, home, project); err != nil {
		return err
	}
	fmt.Fprintln(w, "== InitProject")
	if err := initProject(w, root, home, project); err != nil {
		return err
	}
	fmt.Fprintln(w, "== after init")
	if err := afterInit(w, root, home, project); err != nil {
		return err
	}
	fmt.Fprintln(w, "== InitHome")
	if err := initHomes(w, root, home, project); err != nil {
		return err
	}
	fmt.Fprintln(w, "== InitProject on the global home")
	if err := refuseGlobal(w, root, home, project); err != nil {
		return err
	}
	fmt.Fprintln(w, "== InitProject with a relative folder")
	if err := initRelative(w, root, home, project); err != nil {
		return err
	}
	fmt.Fprintln(w, "== InitProject over a broken home.jsonc")
	return initBroken(w, root, home, project)
}

// open resolves the homes for app with the user's home and working folder
// given by the caller and an empty environment.
func open(app, home, workDir string) (*appdir.Dirs, error) {
	d, err := appdir.New(app,
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(workDir),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(true), // prod profile: plain folder names
	)
	if err != nil {
		return nil, fmt.Errorf("appdir: %w", err)
	}
	return d, nil
}

// snippet:before
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

// snippet:end

// snippet:init
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

// snippet:end

// snippet:after
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

// snippet:end

// snippet:inithome
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

// snippet:end

// snippet:refuse
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

// snippet:end

// snippet:edges
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

// snippet:end

// show prints a path relative to root, so the output is the same on every run.
func show(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}

// hide removes the temp root from an error message for stable output.
func hide(root, msg string) string {
	return strings.ReplaceAll(msg, root+string(filepath.Separator), "")
}

// exists reports whether a path is on disk.
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
