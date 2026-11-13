// Command fallbacks shows what appdir does when the user's home is missing or
// cannot be written: config uses code defaults, data kinds fail with a clear
// error, and disposable kinds (the cache) use a private temp folder.
//
// It runs as a global tool (WithMode(ModeGlobal)) so the db lives in the
// global home and the failures are easy to see. The user's home comes from
// WithHomeDir: first a lookup that fails (no $HOME), then a temp folder made
// read-only with chmod 0500. Nothing is written to the system temp folder:
// the program only prints where the temp fallback would go; the test creates
// it under a private TMPDIR.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example.
const (
	appName     = "lore"
	workDir     = "work"
	readOnlyDir = "ro-home"
	readOnly    = 0o500 // owner may list and enter, not create
	writable    = 0o700
	tempLabel   = "$TMPDIR"
	uidLabel    = "<uid>"
	noHomeMsg   = "$HOME is not set" // the error a failing home lookup returns
)

func main() {
	root, err := os.MkdirTemp("", "appdir-fallbacks-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "fallbacks:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "fallbacks:", err)
		os.Exit(1)
	}
}

// run shows the no-home case, then the read-only case.
func run(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	work := filepath.Join(root, workDir)
	if err := os.MkdirAll(work, writable); err != nil {
		return fmt.Errorf("create work folder: %w", err)
	}
	fmt.Fprintln(w, "== no $HOME")
	if err := noHome(w, root, work); err != nil {
		return err
	}
	fmt.Fprintln(w, "== no $HOME, WithTempFallback(false)")
	if err := noHomeNoTemp(w, root, work); err != nil {
		return err
	}
	fmt.Fprintln(w, "== read-only home")
	return readOnlyHome(w, root, work)
}

// snippet:open
// open resolves a global tool's homes with the given home lookup. A failing
// lookup is what os.UserHomeDir returns in a container with no $HOME.
func open(homeDir func() (string, error), work string, extra ...appdir.Option) (*appdir.Dirs, error) {
	opts := append([]appdir.Option{
		appdir.WithMode(appdir.ModeGlobal),
		appdir.WithHomeDir(homeDir),
		appdir.WithWorkDir(work),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(true), // prod profile: plain folder names
	}, extra...)
	d, err := appdir.New(appName, opts...)
	if err != nil {
		return nil, fmt.Errorf("appdir: %w", err)
	}
	return d, nil
}

// snippet:end

// snippet:nohome
// noHome shows each kind when the home lookup fails. New still succeeds:
// config is "use code defaults", the db names the missing home, the cache
// goes to <tmp>/<app>-<uid>/.
func noHome(w io.Writer, root, work string) error {
	d, err := open(func() (string, error) { return "", errors.New(noHomeMsg) }, work)
	if err != nil {
		return err
	}
	g := d.Global()
	fmt.Fprintf(w, "global home: rule=%s fallback=%v note=%q\n", g.Rule, g.Fallback, g.Note)
	printKinds(w, root, d)
	return nil
}

// snippet:end

// snippet:notemp
// noHomeNoTemp turns the temp fallback off: the cache now fails like the db.
func noHomeNoTemp(w io.Writer, root, work string) error {
	d, err := open(func() (string, error) { return "", errors.New(noHomeMsg) }, work,
		appdir.WithTempFallback(false))
	if err != nil {
		return err
	}
	printKinds(w, root, d)
	return nil
}

// snippet:end

// snippet:readonly
// readOnlyHome makes a home the user cannot write to and calls Ensure.
// The error names the folder and the variables that move it.
func readOnlyHome(w io.Writer, root, work string) error {
	if os.Geteuid() == 0 {
		fmt.Fprintln(w, "skipped: running as root, folder permissions do not apply")
		return nil
	}
	if runtime.GOOS == "windows" {
		fmt.Fprintln(w, "skipped: Windows has no read-only folder mode")
		return nil
	}
	home := filepath.Join(root, readOnlyDir)
	if err := os.Mkdir(home, readOnly); err != nil {
		return fmt.Errorf("create read-only home: %w", err)
	}
	defer func() { _ = os.Chmod(home, writable) }() // let the caller remove root
	d, err := open(func() (string, error) { return home, nil }, work,
		appdir.WithTempFallback(false)) // off, so this run writes nothing to the system temp folder
	if err != nil {
		return err
	}
	for _, k := range []appdir.Kind{appdir.KindDB, appdir.KindCache} {
		if _, err := d.Ensure(k); err != nil {
			fmt.Fprintf(w, "Ensure(%s): %s\n", k, label(root, err.Error()))
			continue
		}
		return fmt.Errorf("Ensure(%s) wrote into a read-only home", k)
	}
	return nil
}

// snippet:end

// printKinds prints config, db and cache as paths --json would: path and
// rule, or the error.
func printKinds(w io.Writer, root string, d *appdir.Dirs) {
	for _, e := range d.Paths() {
		switch e.Kind {
		case appdir.KindConfig, appdir.KindDB, appdir.KindCache:
		default:
			continue
		}
		if e.Error != "" {
			fmt.Fprintf(w, "%-7s rule=%-8s error: %s\n", e.Kind, e.Rule, e.Error)
			continue
		}
		path := "(none: code defaults)"
		if e.Path != "" {
			path = label(root, e.Path)
		}
		fmt.Fprintf(w, "%-7s rule=%-8s fallback=%v %s\n", e.Kind, e.Rule, e.Fallback, path)
	}
}

// label makes paths in s stable: the root is dropped and the system temp
// folder and the uid are replaced by placeholders.
func label(root, s string) string {
	s = strings.ReplaceAll(s, root+string(filepath.Separator), "")
	// Clean: macOS's $TMPDIR ends in a slash, which would eat the separator.
	s = strings.ReplaceAll(s, filepath.Clean(os.TempDir()), tempLabel)
	if uid := os.Getuid(); uid >= 0 {
		s = strings.ReplaceAll(s, appName+"-"+strconv.Itoa(uid), appName+"-"+uidLabel)
	}
	return filepath.ToSlash(s)
}
