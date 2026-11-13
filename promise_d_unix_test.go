//go:build unix

package appdir

// promise_d_unix_test.go: SPEC 12 platform rows whose Unix column needs a
// Unix kernel (device ids, uids, Lstat ownership) and SPEC 10's default
// guard root. Helpers start with "pd".

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pdSecondDisk returns a fresh folder on a filesystem other than its parent
// chain's next mount (APPDIR_TEST_SECOND_DISK, else /dev/shm on Linux), or
// "" when the machine has none.
func pdSecondDisk(t *testing.T) string {
	t.Helper()
	for _, base := range []string{os.Getenv("APPDIR_TEST_SECOND_DISK"), "/dev/shm"} {
		if base == "" || !isDir(base) {
			continue
		}
		dir, err := os.MkdirTemp(base, "pdwalk-")
		if err != nil {
			continue
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			dir = r
		}
		return dir
	}
	return ""
}

// TestPDWalkStopsAtDiskChange proves the SPEC 12 "stop at a disk change"
// row (Stat_t.Dev on Unix): the walk-up never visits a folder on another
// device than the one it started on. It starts on a second filesystem and
// records every folder it visits.
func TestPDWalkStopsAtDiskChange(t *testing.T) {
	start := pdSecondDisk(t)
	if start == "" {
		t.Skip("no second filesystem (set APPDIR_TEST_SECOND_DISK)")
	}
	dev, ok := deviceID(start)
	if !ok {
		t.Fatalf("no device id for %s", start)
	}
	// Find the first ancestor on another device: the walk must stop below it.
	boundary := ""
	for p := start; ; {
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		if pd, ok := deviceID(parent); ok && pd != dev {
			boundary = parent
			break
		}
		p = parent
	}
	if boundary == "" {
		t.Skipf("%s has no device change above it", start)
	}
	home := t.TempDir()
	d, err := New("pdapp", rxHome(home), WithWorkDir(start), rxEnv(nil), WithRelease(true), WithMode(ModeGlobal))
	if err != nil {
		t.Fatal(err)
	}
	var visited []string
	d.walk(start, func(dir string) bool {
		visited = append(visited, dir)
		return false
	})
	for _, v := range visited {
		if id, ok := deviceID(v); !ok || id != dev {
			t.Errorf("walk visited %s on another device (boundary %s)", v, boundary)
		}
	}
	if last := visited[len(visited)-1]; filepath.Dir(last) != boundary {
		t.Errorf("walk stopped at %s, want just below the device change %s (visited %v)", last, boundary, visited)
	}
}

// TestPDPrivateDirRefusesOtherOwner proves the SPEC 12 "temp fallback owner
// check: uid + Lstat" row: a per-user folder in a shared place that exists
// but belongs to another user is ErrUnsafeTemp, and the owner is read with
// Lstat (a link's own owner, never its target's).
func TestPDPrivateDirRefusesOtherOwner(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root owns /; the other-owner case needs a normal user")
	}
	root := t.TempDir()
	d, err := New("pdapp", rxHome(root), WithWorkDir(root), rxEnv(nil), WithRelease(true), WithMode(ModeGlobal))
	if err != nil {
		t.Fatal(err)
	}
	// "/" exists, is a real folder and is owned by root: not ours.
	if err := d.ensurePrivateDir("/"); !errors.Is(err, ErrUnsafeTemp) || !strings.Contains(err.Error(), "not owned by you") {
		t.Fatalf("ensurePrivateDir(/) = %v, want ErrUnsafeTemp naming the owner", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink("/", link); err != nil {
		t.Fatal(err)
	}
	if uid, ok := ownerUID(link); !ok || uid != os.Geteuid() {
		t.Fatalf("ownerUID(link to /) = %d, %v; want the link's own owner %d (Lstat)", uid, ok, os.Geteuid())
	}
	if !ownedByMe(root) {
		t.Fatal("own temp folder not owned by me")
	}
}

// TestPDRootCheckUnix proves the SPEC 12 "root / sudo: euid 0 vs owner uid"
// row: isRoot is the effective uid 0, and when running as root a write into
// a home owned by another uid is ErrRootOwned unless WithAllowRoot(true).
func TestPDRootCheckUnix(t *testing.T) {
	if isRoot() != (os.Geteuid() == 0) {
		t.Fatalf("isRoot() = %v with euid %d", isRoot(), os.Geteuid())
	}
	root := t.TempDir()
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	d, err := New("pdapp", rxHome(home), WithWorkDir(home), rxEnv(nil), WithRelease(true), WithMode(ModeGlobal))
	if err != nil {
		t.Fatal(err)
	}
	if !isRoot() {
		// A normal user is never refused on a home it owns.
		if err := d.checkWrite(home, nil); err != nil {
			t.Fatalf("checkWrite as a normal user = %v", err)
		}
		if uid, ok := ownerUID(home); !ok || uid != os.Geteuid() {
			t.Fatalf("ownerUID(home) = %d, %v", uid, ok)
		}
		return
	}
	if err := os.Chown(home, 65534, 65534); err != nil {
		t.Fatal(err)
	}
	if err := d.checkWrite(filepath.Join(home, ".pdapp"), nil); !errors.Is(err, ErrRootOwned) {
		t.Fatalf("root writing into uid 65534's home = %v, want ErrRootOwned", err)
	}
	allowed, err := New("pdapp", rxHome(home), WithWorkDir(home), rxEnv(nil), WithRelease(true), WithMode(ModeGlobal), WithAllowRoot(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := allowed.checkWrite(filepath.Join(home, ".pdapp"), nil); err != nil {
		t.Fatalf("WithAllowRoot(true): %v", err)
	}
}

// TestPDGuardDefaultRootAllowsTempFallback proves SPEC 10: under the
// default guard root (no APPDIR_TEST_ROOT, no TMPDIR, so os.TempDir() is
// /tmp) the temp fallback of a disposable kind, <tmp>/<app>-<uid>/, stays
// allowed by New's guard. Nothing is created: only paths are resolved.
func TestPDGuardDefaultRootAllowsTempFallback(t *testing.T) {
	for _, k := range []string{"TMPDIR", envTestRoot} {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
	var d *Dirs
	var err error
	msg := ttPanic(func() {
		d, err = New("pdapp", rxNoHome(), WithWorkDir(os.TempDir()), rxEnv(nil), WithRelease(true), WithMode(ModeGlobal))
	})
	if msg != "" || err != nil {
		t.Fatalf("New with no home under the default root: panic %q err %v", msg, err)
	}
	p, err := d.Path(KindCache)
	want := filepath.Join(os.TempDir(), "pdapp-"+currentUser(), "cache")
	if err != nil || p != want {
		t.Fatalf("Path(cache) = %q, %v; want the temp fallback %q", p, err, want)
	}
}
