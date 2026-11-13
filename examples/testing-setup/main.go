// Command testing-setup is a tiny tool written so it can be tested without
// touching the developer's real homes. Its main.go shows the seams (WithEnv,
// WithHomeDir, WithWorkDir, WithBuildInfo) that replace every outside input;
// its main_test.go shows appdir.ForTest and the test-binary guard.
//
// Everything happens inside a temp root: the user's home is root/home and
// the environment is an empty map.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example.
const (
	appName     = "demo"
	homeDir     = "home"
	noteText    = "remember the milk\n"
	fakeVersion = "v1.0.0" // what `go install module@v1.0.0` reports
)

func main() {
	root, err := os.MkdirTemp("", "appdir-testing-setup-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testing-setup:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp root
	if err != nil {
		fmt.Fprintln(os.Stderr, "testing-setup:", err)
		os.Exit(1)
	}
}

// snippet:tool
// openDirs is the tool's one place that resolves its paths. The real binary
// calls it with no options (process env, real home, real build info); tests
// and this example pass seams. WithMode(ModeGlobal): the db lives in the
// global home, so the tool works outside a project.
func openDirs(opts ...appdir.Option) (*appdir.Dirs, error) {
	base := []appdir.Option{appdir.WithMode(appdir.ModeGlobal)}
	return appdir.New(appName, append(base, opts...)...)
}

// saveNote is the tool's real work: it creates the db folder on first use
// and writes a file there.
func saveNote(d *appdir.Dirs, text string) (string, error) {
	p, err := d.Ensure(appdir.KindDB)
	if err != nil {
		return "", fmt.Errorf("ensure db: %w", err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	return p, nil
}

// snippet:end

// snippet:seams
// run drives the tool with every outside input replaced: the user's home
// (WithHomeDir), the working folder (WithWorkDir), the environment (WithEnv)
// and the build info that decides release or dev (WithBuildInfo).
func run(w io.Writer, root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	home := filepath.Join(root, homeDir)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create home: %w", err)
	}
	d, err := openDirs(
		appdir.WithHomeDir(func() (string, error) { return home, nil }),
		appdir.WithWorkDir(home),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithBuildInfo(func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: fakeVersion}}, true
		}),
	)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	fmt.Fprintf(w, "profile %s (%s)\n", d.Profile(), d.ReleaseReason())
	p, err := saveNote(d, noteText)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "saved %s\n", show(root, p))
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
