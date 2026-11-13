// Command legacy-move shows MoveLegacy: a tool that used to keep its global
// home in ~/.demo-old moves it to ~/.demo on the first run of the new
// version, leaves MOVED.txt behind, and reports every other outcome (already
// moved, both exist, skipped, busy) without ever deleting user data.
//
// Everything happens inside a temp root: each case has its own fake user home
// under root (passed with WithHomeDir) and an empty environment (WithEnv), so
// the program never touches the real home.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example.
const (
	appName    = "demo"
	oldDirName = ".demo-old" // where the previous version kept its home
	movedNote  = "MOVED.txt"
	newDirName = "." + appName     // the home the new version uses
	daemonLock = "run/daemon.lock" // the old daemon's lock, probed by default

	caseMove  = "home-move"
	caseBoth  = "home-both"
	caseBusy  = "home-busy"
	caseFresh = "home-fresh"
	elsewhere = "elsewhere" // a --home target
)

// oldFiles is what the previous version left in its home.
func oldFiles() map[string]string {
	return map[string]string{
		"demo.db":        "old database\n",
		"notes/today.md": "# today\n",
		"config.jsonc":   "{}\n",
	}
}

func main() {
	root, err := os.MkdirTemp("", "appdir-legacy-move-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "legacy-move:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "legacy-move:", err)
		os.Exit(1)
	}
}

// run does the whole walk-through inside root and prints paths relative to it.
func run(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	ctx := context.Background()
	steps := []struct {
		title string
		fn    func(context.Context, io.Writer, string) error
	}{
		{"first run moves, second run sees it done", firstRun},
		{"both homes exist", bothExist},
		{"skipped", skipped},
		{"old daemon still running", busy},
	}
	for _, st := range steps {
		fmt.Fprintln(w, "==", st.title)
		if err := st.fn(ctx, w, root); err != nil {
			return fmt.Errorf("%s: %w", st.title, err)
		}
	}
	return nil
}

// snippet:open
// open resolves the app for one fake user home. WithLegacyHome names the old
// folder; DaemonLocks is left empty, so run/daemon.lock is probed.
// WithRelease(true): only the release profile ever runs a legacy move.
func open(home string, opts ...appdir.Option) (*appdir.Dirs, error) {
	base := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(home),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(true),
		appdir.WithLegacyHome(appdir.LegacyHome{Dir: oldDirName}),
	}
	return appdir.New(appName, append(base, opts...)...)
}

// snippet:end

// snippet:move
// migrate is what the tool runs at startup, before it opens anything in its
// home. ErrBusy is not fatal: the old daemon still runs, so the tool keeps
// using the old layout for now and tries again on its next start.
func migrate(ctx context.Context, d *appdir.Dirs) (appdir.MoveResult, error) {
	res, err := d.MoveLegacy(ctx)
	if errors.Is(err, appdir.ErrBusy) {
		return res, err // print it and go on; nothing was moved
	}
	if err != nil {
		return res, fmt.Errorf("legacy move: %w", err)
	}
	return res, nil
}

// snippet:end

// snippet:first
// firstRun moves ~/.demo-old to ~/.demo (one rename on the same disk), then
// runs again: the old folder now holds only MOVED.txt, so the result is
// "already". The .moving.lock file next to the home stays by design.
func firstRun(ctx context.Context, w io.Writer, root string) error {
	home, err := makeHome(root, caseMove, false)
	if err != nil {
		return err
	}
	for range 2 {
		d, err := open(home)
		if err != nil {
			return err
		}
		res, err := migrate(ctx, d)
		if err != nil {
			return err
		}
		printResult(w, root, res)
	}
	note, err := os.ReadFile(filepath.Join(home, oldDirName, movedNote))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s/%s: %s", oldDirName, movedNote, clean(root, string(note)))
	return printTree(w, root, home)
}

// snippet:end

// snippet:both
// bothExist has an old folder with data and a new home already in place:
// nothing moves, the new home is used, and Note says so for doctor.
func bothExist(ctx context.Context, w io.Writer, root string) error {
	home, err := makeHome(root, caseBoth, true)
	if err != nil {
		return err
	}
	d, err := open(home)
	if err != nil {
		return err
	}
	res, err := migrate(ctx, d)
	if err != nil {
		return err
	}
	printResult(w, root, res)
	return nil
}

// snippet:end

// snippet:skipped
// skipped shows the cases that never move: a dev build (its own ~/.demo-dev
// home), a home given by --home (or DEMO_HOME), and no old folder at all.
func skipped(ctx context.Context, w io.Writer, root string) error {
	home, err := makeHome(root, caseFresh, false)
	if err != nil {
		return err
	}
	cases := []struct {
		label string
		home  string
		opts  []appdir.Option
	}{
		{"dev build", home, []appdir.Option{appdir.WithRelease(false)}},
		{"--home elsewhere", home, []appdir.Option{appdir.WithHome(filepath.Join(root, elsewhere))}},
		{"no old folder", filepath.Join(root, elsewhere), nil},
	}
	for _, c := range cases {
		d, err := open(c.home, c.opts...)
		if err != nil {
			return err
		}
		res, err := migrate(ctx, d)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-17s ", c.label)
		printResult(w, root, res)
	}
	return nil
}

// snippet:end

// snippet:busy
// busy holds the old daemon's lock while the new version starts: MoveLegacy
// probes run/daemon.lock (it never creates it), sees it held and returns
// ErrBusy. Once the old daemon exits, the next start moves the home.
func busy(ctx context.Context, w io.Writer, root string) error {
	home, err := makeHome(root, caseBusy, false)
	if err != nil {
		return err
	}
	release, err := holdLock(filepath.Join(home, oldDirName, filepath.FromSlash(daemonLock)))
	if err != nil {
		fmt.Fprintf(w, "skipped: %v\n", err)
		return nil
	}
	d, err := open(home)
	if err != nil {
		release()
		return err
	}
	_, err = migrate(ctx, d)
	release() // the old daemon exits
	if !errors.Is(err, appdir.ErrBusy) {
		return fmt.Errorf("want ErrBusy, got %v", err)
	}
	fmt.Fprintf(w, "while the old daemon runs: %s\n", clean(root, err.Error()))
	res, err := migrate(ctx, d)
	if err != nil {
		return err
	}
	fmt.Fprint(w, "next start: ")
	printResult(w, root, res)
	return nil
}

// snippet:end

// makeHome creates a fake user home under root with the old folder and,
// when withNew is set, a new home already in place.
func makeHome(root, name string, withNew bool) (string, error) {
	home := filepath.Join(root, name)
	files := oldFiles()
	files[daemonLock] = "" // the old daemon's lock file, not held
	for rel, body := range files {
		p := filepath.Join(home, oldDirName, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return "", fmt.Errorf("create old home: %w", err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			return "", fmt.Errorf("write old %s: %w", rel, err)
		}
	}
	if withNew {
		if err := os.MkdirAll(filepath.Join(home, newDirName), 0o700); err != nil {
			return "", fmt.Errorf("create new home: %w", err)
		}
	}
	return home, nil
}

// printResult prints a MoveResult with paths relative to root.
func printResult(w io.Writer, root string, r appdir.MoveResult) {
	fmt.Fprintf(w, "status=%s", r.Status)
	if r.From != "" {
		fmt.Fprintf(w, " from=%s to=%s", clean(root, r.From), clean(root, r.To))
	}
	if r.Note != "" {
		fmt.Fprintf(w, " note=%q", clean(root, r.Note))
	}
	fmt.Fprintln(w)
}

// printTree lists every file under dir, relative to it, in a stable order.
func printTree(w io.Writer, root, dir string) error {
	var files []string
	err := filepath.WalkDir(dir, func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	fmt.Fprintf(w, "files in %s:\n", clean(root, dir))
	for _, f := range files {
		fmt.Fprintf(w, "  %s\n", f)
	}
	return nil
}

// clean drops the temp root from a path or message, so the output is the same
// on every run.
func clean(root, s string) string {
	return filepath.ToSlash(strings.ReplaceAll(s, root+string(filepath.Separator), ""))
}
