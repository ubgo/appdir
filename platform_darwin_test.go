//go:build darwin

package appdir

// platform_darwin_test.go: what only a real macOS machine can show. Its temp
// folder lives under /var, a symlink to /private/var, so the same folder has
// two spellings; the walk-up, the $HOME wall (sameFile) and the test guard
// must treat them as one. Its $TMPDIR is long, so the socket must fall back
// to the literal /tmp. Every helper here starts with "pd".

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pdApp = "pdapp"

// pdRoots returns the same fresh temp folder twice: as /var/... (the symlink
// spelling os.TempDir gives) and as /private/var/... (the real path).
func pdRoots(t *testing.T) (link, real string) {
	t.Helper()
	link = t.TempDir()
	real, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	if link == real {
		t.Skipf("temp folder %s has no symlink in it on this machine", link)
	}
	return link, real
}

// TestDarwinPrivateVarWalk: the user's home given in one spelling and the
// working folder in the other: the walk still stops at $HOME (a .<app>/
// above it is never the project) and still finds the marker below it, in
// both directions.
func TestDarwinPrivateVarWalk(t *testing.T) {
	link, real := pdRoots(t)
	for _, tc := range []struct{ name, homeRoot, workRoot string }{
		{"home-var-work-private", link, real},
		{"home-private-work-var", real, link},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := strings.ReplaceAll(tc.name, "-", "")
			rxMkdir(t, filepath.Join(real, sub, "."+pdApp)) // above $HOME
			home := filepath.Join(tc.homeRoot, sub, "user")
			work := filepath.Join(tc.workRoot, sub, "user", "src", "proj", "sub")
			rxMkdir(t, work)
			d, err := New(pdApp, rxHome(home), WithWorkDir(work), rxEnv(nil), WithRelease(true))
			if err != nil {
				t.Fatal(err)
			}
			if d.HasProject() {
				p, _ := d.Project()
				t.Fatalf("found project %s above the user's home", p.Marker)
			}
			marker := filepath.Join(real, sub, "user", "src", "proj", "."+pdApp)
			rxMkdir(t, marker)
			d, err = New(pdApp, rxHome(home), WithWorkDir(work), rxEnv(nil), WithRelease(true))
			if err != nil {
				t.Fatal(err)
			}
			p, err := d.Project()
			if err != nil || !sameFile(p.Marker, marker) {
				t.Fatalf("Project() = %+v, %v; want %s", p, err, marker)
			}
			// The guard's root is os.TempDir() (the /var spelling): a
			// /private/var path inside it is still inside.
			if _, err := d.Ensure(KindLogs); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestDarwinCeilingOtherSpelling: a ceiling given as /var/... stops a walk
// that runs through /private/var/...
func TestDarwinCeilingOtherSpelling(t *testing.T) {
	link, real := pdRoots(t)
	rxMkdir(t, filepath.Join(real, "srv", "."+pdApp), filepath.Join(real, "srv", "ceil", "proj"))
	home := filepath.Join(real, "user")
	rxMkdir(t, home)
	env := map[string]string{"PDAPP_CEILING_DIRS": filepath.Join(link, "srv", "ceil")}
	d, err := New(pdApp, rxHome(home), WithWorkDir(filepath.Join(real, "srv", "ceil", "proj")), rxEnv(env), WithRelease(true))
	if err != nil {
		t.Fatal(err)
	}
	if d.HasProject() {
		p, _ := d.Project()
		t.Fatalf("walk climbed past the ceiling to %s", p.Marker)
	}
}

// TestDarwinSocketFallbackLiteralTmp: under macOS's long $TMPDIR the socket
// falls back to the literal /tmp/<name>-<uid>/<hash>.sock, short enough for
// sun_path (104 bytes), never to $TMPDIR. Only Socket() is called (nothing is
// created in /tmp), with the guard off because /tmp is outside the test root
// by design.
func TestDarwinSocketFallbackLiteralTmp(t *testing.T) {
	_, real := pdRoots(t)
	home := filepath.Join(real, "user")
	rxMkdir(t, home)
	d, err := New(pdApp, rxHome(home), WithWorkDir(home), rxEnv(nil), WithRelease(true),
		WithMode(ModeGlobal), WithTestGuard(false))
	if err != nil {
		t.Fatal(err)
	}
	s, err := d.Socket()
	if err != nil {
		t.Fatal(err)
	}
	run, _ := d.Path(KindRun)
	if len(filepath.Join(run, pdApp+".sock")) <= 100 {
		t.Skipf("run/ path %s is short enough on this machine; no fallback to check", run)
	}
	if !s.Fallback || !strings.HasPrefix(s.Path, "/tmp/"+pdApp+"-") || s.Dir != filepath.Dir(s.Path) {
		t.Fatalf("Socket = %+v; want /tmp/%s-<uid>/<hash>.sock", s, pdApp)
	}
	if len(s.Path) > 103 {
		t.Fatalf("fallback socket path %d bytes; sun_path holds 103 plus NUL", len(s.Path))
	}
}

// TestDarwinSocketListensWhereItFits: a socket path within WithSocketMaxLen
// stays under run/ and a real listener can bind it.
func TestDarwinSocketListensWhereItFits(t *testing.T) {
	// /tmp is short on every Mac; a per-test folder under it keeps the run/
	// path well inside 100 bytes. Removed at the end.
	base, err := os.MkdirTemp("/tmp", "pd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	base, _ = filepath.EvalSymlinks(base) // /tmp -> /private/tmp
	t.Setenv(envTestRoot, base)
	home := filepath.Join(base, "u")
	rxMkdir(t, home)
	d, err := New(pdApp, rxHome(home), WithWorkDir(home), rxEnv(nil), WithRelease(true), WithMode(ModeGlobal))
	if err != nil {
		t.Fatal(err)
	}
	s, err := d.EnsureSocketDir()
	if err != nil {
		t.Fatal(err)
	}
	if s.Fallback {
		t.Fatalf("Socket = %+v; a %d-byte path should stay under run/", s, len(s.Path))
	}
	ln, err := net.Listen("unix", s.Path)
	if err != nil {
		t.Fatalf("listen on %s: %v", s.Path, err)
	}
	_ = ln.Close()
}
