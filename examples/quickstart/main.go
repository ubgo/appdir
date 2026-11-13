// Command quickstart is the first program a tool writes with appdir: resolve
// every home with New, read paths with Path, PathIn and Paths, and create a
// folder only when it is needed with Ensure.
//
// Everything happens inside a temp root: the user's home is root/home (passed
// with WithHomeDir) and the tool runs in a project under it (WithWorkDir), so
// the program never reads or writes the real home.
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

// Names used by the example. The app name decides the home folder (.lore),
// the env prefix (LORE_) and the db file name (lore.db).
const (
	appName    = "lore"
	badAppName = "My App" // upper case and a space: refused by New
	homeDir    = "home"
	projectDir = "home/projects/shop"
)

func main() {
	root, err := os.MkdirTemp("", "appdir-quickstart-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "quickstart:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "quickstart:", err)
		os.Exit(1)
	}
}

// run does the whole walk-through inside root and prints paths relative to it.
func run(w io.Writer, root string) error {
	// Resolve the root once so printed paths match what appdir reports: the
	// lookup walks real paths (on macOS the temp folder is a symlink).
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	home := filepath.Join(root, filepath.FromSlash(homeDir))
	project := filepath.Join(root, filepath.FromSlash(projectDir))
	if err := os.MkdirAll(project, 0o700); err != nil {
		return fmt.Errorf("create project folder: %w", err)
	}
	if err := initProject(home, project); err != nil {
		return err
	}

	fmt.Fprintln(w, "== app name rule")
	if err := checkName(w); err != nil {
		return err
	}

	fmt.Fprintln(w, "== New: resolve, create nothing")
	d, err := open(home, project)
	if err != nil {
		return err
	}
	if err := printHomes(w, root, d); err != nil {
		return err
	}

	fmt.Fprintln(w, "== Path and PathIn")
	if err := readPaths(w, root, d); err != nil {
		return err
	}

	fmt.Fprintln(w, "== Paths (what paths --json lists)")
	listPaths(w, root, d)

	fmt.Fprintln(w, "== reads never create")
	fmt.Fprintf(w, "global home exists: %v\n", exists(d.Global().Base))

	fmt.Fprintln(w, "== Ensure: create on first use")
	if err := ensure(w, root, d); err != nil {
		return err
	}

	fmt.Fprintln(w, "== global home per build")
	if err := globalHome(w, root, home, project); err != nil {
		return err
	}

	fmt.Fprintln(w, "== which home a kind uses, outside a project")
	return kindHomes(w, root, home)
}

// initProject makes the project home the proper way (see docs/start/init.md):
// a throwaway Dirs runs InitProject, like `lore init` would.
func initProject(home, project string) error {
	d, err := open(home, project)
	if err != nil {
		return err
	}
	if _, err := d.InitProject(project); err != nil {
		return fmt.Errorf("init project: %w", err)
	}
	return nil
}

// snippet:name
// checkName shows the one thing New checks about the app name: it must match
// ^[a-z0-9][a-z0-9._-]*$, because it becomes a folder and env names.
func checkName(w io.Writer) error {
	_, err := appdir.New(badAppName)
	if !errors.Is(err, appdir.ErrInvalidOption) {
		return fmt.Errorf("want ErrInvalidOption for %q, got %v", badAppName, err)
	}
	fmt.Fprintln(w, "New:", err)
	return nil
}

// snippet:end

// snippet:open
// open resolves every home for the app. The three seams keep the example
// away from the real machine: the user's home, the working folder and the
// environment all come from the caller. WithRelease(true) keeps the prod
// profile, so folders have their plain names (.lore, not .lore-dev).
func open(home, workDir string) (*appdir.Dirs, error) {
	d, err := appdir.New(appName,
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(workDir),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(true),
	)
	if err != nil {
		return nil, fmt.Errorf("appdir: %w", err)
	}
	return d, nil
}

// snippet:end

// snippet:homes
// printHomes shows both homes at once. Base holds home.jsonc and the config
// files, Data every other kind (they differ only for a non-release profile).
// Info is the parsed home.jsonc, nil when the folder has none yet. App is
// the name New was given, for messages that name the tool.
func printHomes(w io.Writer, root string, d *appdir.Dirs) error {
	fmt.Fprintf(w, "app:          %s\n", d.App())
	g := d.Global()
	fmt.Fprintf(w, "global home:  %s (rule %s, fallback %v, info %v)\n", show(root, g.Base), g.Rule, g.Fallback, g.Info != nil)
	fmt.Fprintf(w, "has project:  %v\n", d.HasProject())
	p, err := d.Project()
	if err != nil {
		return fmt.Errorf("project home: %w", err)
	}
	fmt.Fprintf(w, "project home: %s (rule %s, data %s)\n", show(root, p.Base), p.Rule, show(root, p.Data))
	fmt.Fprintf(w, "project info: app=%s layout=%d scope=%s\n", p.Info.App, p.Info.Layout, p.Info.Scope)
	return nil
}

// snippet:end

// snippet:read
// readPaths asks for paths. Path picks the kind's effective home (the db
// lives in the project, the cache is global); PathIn names the home.
func readPaths(w io.Writer, root string, d *appdir.Dirs) error {
	for _, k := range []appdir.Kind{appdir.KindDB, appdir.KindCache, appdir.KindConfig} {
		p, err := d.Path(k)
		if err != nil {
			return fmt.Errorf("path %s: %w", k, err)
		}
		fmt.Fprintf(w, "Path(%s): %s\n", k, show(root, p))
	}
	for _, s := range []appdir.Scope{appdir.ScopeGlobal, appdir.ScopeProject} {
		p, err := d.PathIn(appdir.KindConfig, s)
		if err != nil {
			return fmt.Errorf("path in %s: %w", s, err)
		}
		fmt.Fprintf(w, "PathIn(config, %s): %s\n", s, show(root, p))
	}
	return nil
}

// snippet:end

// snippet:paths
// listPaths prints every kind with the home and the rule that picked it.
// Kinds kept in both homes (home.jsonc, the config files) appear twice.
func listPaths(w io.Writer, root string, d *appdir.Dirs) {
	for _, e := range d.Paths() {
		fmt.Fprintf(w, "%-13s %-8s %-8s %s\n", e.Kind, e.Home, e.Rule, show(root, e.Path))
	}
}

// snippet:end

// snippet:ensure
// ensure creates folders on first use. For a folder kind it creates the
// folder; for a file kind (the db) it creates the parent folder only and
// returns the file path for the caller to open.
func ensure(w io.Writer, root string, d *appdir.Dirs) error {
	for _, k := range []appdir.Kind{appdir.KindCache, appdir.KindLogs, appdir.KindDB} {
		p, err := d.Ensure(k)
		if err != nil {
			return fmt.Errorf("ensure %s: %w", k, err)
		}
		fmt.Fprintf(w, "Ensure(%s): %s\n", k, show(root, p))
	}
	fmt.Fprintf(w, "global home exists: %v\n", exists(d.Global().Base))
	return nil
}

// snippet:end

// snippet:global
// globalHome shows where the global home is for each kind of build: a
// release build uses ~/.lore, a dev build (go run, go build without the
// tool's release flag) uses ~/.lore-dev, so trying a change never touches
// the installed tool's data.
func globalHome(w io.Writer, root, home, workDir string) error {
	for _, release := range []bool{true, false} {
		d, err := appdir.New(appName,
			appdir.WithHomeDir(func() (string, error) { return home, nil }),
			appdir.WithWorkDir(workDir),
			appdir.WithEnv(func(string) (string, bool) { return "", false }),
			appdir.WithRelease(release),
		)
		if err != nil {
			return fmt.Errorf("appdir: %w", err)
		}
		g := d.Global()
		fmt.Fprintf(w, "release=%-5v profile %-4s global home %s (rule %s)\n", release, d.Profile(), show(root, g.Base), g.Rule)
	}
	return nil
}

// snippet:end

// snippet:scopes
// kindHomes runs from the user's home, where there is no project home, and
// prints the home each kind lands in. In ModeProject (the default) the db is
// project data and fails with ErrNoProject, the logs fall back to the global
// home, and the cache is global anyway. In ModeGlobal every data kind lives
// in the global home, so nothing fails.
func kindHomes(w io.Writer, root, home string) error {
	for _, mode := range []appdir.Mode{appdir.ModeProject, appdir.ModeGlobal} {
		d, err := appdir.New(appName,
			appdir.WithHomeDir(func() (string, error) { return home, nil }),
			appdir.WithWorkDir(home), // no .lore/ here or above
			appdir.WithEnv(func(string) (string, bool) { return "", false }),
			appdir.WithRelease(true),
			appdir.WithMode(mode),
		)
		if err != nil {
			return fmt.Errorf("appdir: %w", err)
		}
		for _, e := range d.Paths() {
			switch e.Kind {
			case appdir.KindDB, appdir.KindLogs, appdir.KindCache:
			default:
				continue // the three kinds that show the three outcomes
			}
			if e.Error != "" {
				fmt.Fprintf(w, "%-7s %-6s error: %s\n", mode, e.Kind, e.Error)
				continue
			}
			fmt.Fprintf(w, "%-7s %-6s %-7s fallback=%-5v %s\n", mode, e.Kind, e.Home, e.Fallback, show(root, e.Path))
		}
	}
	return nil
}

// snippet:end

// show prints a path relative to root, so the output is the same on every run.
func show(root, p string) string {
	if p == "" {
		return "(none)"
	}
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}

// exists reports whether a path is on disk.
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
