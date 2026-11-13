package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ubgo/appdir"
)

// outsideHome is a home folder outside every test root. Nothing is ever
// created there: New only resolves, and the guard stops it first.
const outsideHome = "/appdir-guard-example/user"

// TestRun runs the example program with its seams in a test folder.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"profile prod (go-install)",
		"saved home/.demo/demo.db",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q\n%s", want, out.String())
		}
	}
}

// snippet:fortest
// TestForTest runs the tool exactly as the real binary does (no seams):
// ForTest has already pointed HOME, DEMO_HOME, the XDG folders, TMPDIR and
// the agent config folders into a temp root, and removed DEMO_PROFILE and the
// GIT_* variables, so every path lands in that root. ForTest uses t.Setenv,
// so this test cannot call t.Parallel.
func TestForTest(t *testing.T) {
	root := appdir.ForTest(t, appName)

	d, err := openDirs()
	if err != nil {
		t.Fatal(err)
	}
	p, err := saveNote(d, noteText)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "apphome", appName+".db"); p != want {
		t.Errorf("db = %s, want %s (DEMO_HOME from ForTest)", p, want)
	}
	if home := os.Getenv("HOME"); home != filepath.Join(root, "home") {
		t.Errorf("HOME = %s, want it inside the test root", home)
	}
}

// snippet:end

// snippet:guard
// TestGuardPanics forgets ForTest and hands New a home outside the test
// root. In a test binary New panics before anything can be written there.
func TestGuardPanics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("outsideHome is a Unix path")
	}
	msg := func() (msg string) {
		defer func() { msg = fmt.Sprint(recover()) }()
		_, _ = openDirs(
			appdir.WithHomeDir(func() (string, error) { return outsideHome, nil }),
			appdir.WithWorkDir(t.TempDir()),
			appdir.WithEnv(func(string) (string, bool) { return "", false }),
			appdir.WithRelease(false),
		)
		return ""
	}()
	want := "appdir: test binary resolved " + outsideHome + "/.demo-dev outside the test root"
	if !strings.HasPrefix(msg, want) {
		t.Fatalf("panic = %q, want prefix %q", msg, want)
	}
	t.Log(msg)
}

// snippet:end

// snippet:guardoff
// TestGuardOff turns the guard off for a test that must resolve a real-looking
// path (and never writes there): WithTestGuard(false).
func TestGuardOff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("outsideHome is a Unix path")
	}
	d, err := openDirs(
		appdir.WithHomeDir(func() (string, error) { return outsideHome, nil }),
		appdir.WithWorkDir(t.TempDir()),
		appdir.WithEnv(func(string) (string, bool) { return "", false }),
		appdir.WithRelease(false),
		appdir.WithTestGuard(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	p, err := d.Path(appdir.KindDB)
	if err != nil {
		t.Fatal(err)
	}
	if want := outsideHome + "/.demo-dev/demo.db"; p != want {
		t.Errorf("db = %s, want %s", p, want)
	}
}

// snippet:end
