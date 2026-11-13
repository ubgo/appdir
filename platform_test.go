package appdir

// platform_test.go: platform promises that hold on every OS and are checked
// against the source, plus the atomic write over an existing file.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoProcessStart: appdir never starts a process (no daemon, no Task
// Scheduler, no launchd or systemd unit) except tmutil for the macOS backup
// exclusion. Only cachetag_darwin.go may import os/exec, and no file names a
// service manager.
func TestNoProcessStart(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if path == "os/exec" && f != "cachetag_darwin.go" {
				t.Errorf("%s imports os/exec; appdir must not start processes", f)
			}
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range []string{"schtasks", "CreateProcess", "launchctl", "systemctl", "StartProcess"} {
			if strings.Contains(string(b), w) {
				t.Errorf("%s mentions %s", f, w)
			}
		}
	}
}

// TestAtomicWriteOverExisting: the atomic write replaces an existing file
// (rename over it: MoveFileEx with replace on Windows), keeps its mode on
// Unix and leaves no temp file.
func TestAtomicWriteOverExisting(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	d := rxNew(t, home, home, nil, WithMode(ModeGlobal))
	target := filepath.Join(root, "f.txt")
	rxWrite(t, target, "old")
	if os.PathSeparator == '/' {
		if err := os.Chmod(target, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{"new", "newer"} {
		if err := d.writeFileAtomic(target, []byte(body), 0o600); err != nil {
			t.Fatalf("write over existing: %v", err)
		}
		if b, _ := os.ReadFile(target); string(b) != body {
			t.Fatalf("content = %q, want %q", b, body)
		}
	}
	if os.PathSeparator == '/' {
		if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o640 {
			t.Errorf("mode = %o, want the existing 640", fi.Mode().Perm())
		}
	}
	ents, _ := os.ReadDir(root)
	for _, e := range ents {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left: %s", e.Name())
		}
	}
}
