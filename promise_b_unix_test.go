//go:build unix

package appdir

// Unix-only promise tests for SPEC 4.4 (disk-change stop), 4.7 (<uid> in the
// temp fallback name) and 4.8 (root guard, read-only home). See
// promise_b_test.go for the shared rules; helpers carry the pb prefix.

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// pbDev is the device id of path, read straight from stat(2) so the test
// does not trust the code under test.
func pbDev(t *testing.T, path string) uint64 {
	t.Helper()
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return uint64(st.Dev) //nolint:unconvert // int32 on darwin
}

// pbWantWalk is the folders a walk from start must visit with no $HOME or
// ceiling on the way: up to the filesystem root or the last folder before a
// parent on another device. It reports whether a disk change ended it.
func pbWantWalk(t *testing.T, start string) ([]string, bool) {
	t.Helper()
	dev := pbDev(t, start)
	want := []string{start}
	for dir := start; ; {
		parent := filepath.Dir(dir)
		if parent == dir {
			return want, false
		}
		if pbDev(t, parent) != dev {
			return want, true
		}
		want = append(want, parent)
		dir = parent
	}
}

// SPEC 4.4 step 3.4: the walk stops when the parent is on another disk
// (device id). Runs from the test temp folder, or /dev/shm, whichever path
// crosses a mount point; skipped only on a machine where neither does.
func TestPromiseB_WalkStopsAtDiskChange(t *testing.T) {
	root := trRoot(t)
	starts := []string{filepath.Join(root, "a", "b")}
	trMkdir(t, starts[0])
	if shm, err := os.MkdirTemp("/dev/shm", "pb-walk-"); err == nil {
		t.Cleanup(func() { _ = os.RemoveAll(shm) })
		if r, err := filepath.EvalSymlinks(shm); err == nil {
			starts = append(starts, filepath.Join(r, "x"))
			trMkdir(t, filepath.Join(r, "x"))
		}
	}
	for _, start := range starts {
		want, crossed := pbWantWalk(t, start)
		if !crossed {
			continue
		}
		d := trNew(t, trHome(filepath.Join(root, "home")), root, nil, WithTestGuard(false))
		var got []string
		d.walk(start, func(dir string) bool { got = append(got, dir); return false })
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("walk from %s visited\n%v\nwant (stop below the disk change)\n%v", start, got, want)
		}
		return
	}
	t.Skip("no mount point on the path above the temp folder or /dev/shm")
}

// SPEC 4.7: the temp fallback is <tmp>/<Name(app)>-<uid> with the numeric
// uid on Unix.
func TestPromiseB_TempFallbackNumericUID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	d := trNew(t, trNoHome(), tmp, nil)
	p, err := d.Path(KindCache)
	want := filepath.Join(os.TempDir(), trApp+"-"+strconv.Itoa(os.Getuid()), "cache")
	if err != nil || p != want {
		t.Fatalf("Path(cache) = %q, %v; want %q", p, err, want)
	}
}

// homes/fallbacks.md Step 3: reads still work in a home the user cannot
// write; only Ensure fails.
func TestPromiseB_ReadOnlyHomeReadsWork(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores folder modes")
	}
	root := trRoot(t)
	home := filepath.Join(root, "home")
	trInitHome(t, filepath.Join(home, trMarker))
	if err := os.Chmod(filepath.Join(home, trMarker), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(home, trMarker), 0o700) })
	d := trNew(t, trHome(home), root, nil, WithMode(ModeGlobal), WithTempFallback(false))
	for _, k := range []Kind{KindDB, KindCache, KindConfig, KindRun} {
		if _, err := d.Path(k); err != nil {
			t.Errorf("Path(%s) in a read-only home = %v", k, err)
		}
	}
	if _, err := d.Ensure(KindRun); !errors.Is(err, os.ErrPermission) {
		t.Errorf("Ensure(run) = %v; want a permission error", err)
	}
}

// SPEC 4.8 root rule and homes/fallbacks.md Step 4, start/init.md pitfall:
// as euid 0, a write whose target home's nearest existing folder belongs to
// another uid fails with ErrRootOwned naming that folder and --allow-root;
// reads are unaffected; WithAllowRoot(true) lets it through; a root-owned
// target is fine. Also fallbacks.md Step 1: a temp fallback folder owned by
// another user is ErrUnsafeTemp. Runs only as root (the CI root job, or a
// root container); a normal run cannot make euid 0.
func TestPromiseB_RootGuard(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs euid 0")
	}
	const other = 12345
	root := trRoot(t)
	home := filepath.Join(root, "home")
	trMkdir(t, home)
	if err := os.Chown(home, other, other); err != nil {
		t.Skipf("chown: %v", err)
	}
	d := trNew(t, trHome(home), root, nil, WithMode(ModeGlobal))
	if _, err := d.Path(KindDB); err != nil {
		t.Errorf("Path(db) as root = %v; reads must not check root", err)
	}
	_, err := d.Ensure(KindDB)
	want := "running as root but " + home + " belongs to uid " + strconv.Itoa(other) + "; pass --allow-root"
	if !errors.Is(err, ErrRootOwned) || !strings.Contains(err.Error(), want) {
		t.Fatalf("Ensure(db) as root = %v; want %q", err, want)
	}
	if err := d.InitHome(filepath.Join(home, "h"), ScopeGlobal); !errors.Is(err, ErrRootOwned) {
		t.Errorf("InitHome as root = %v; want ErrRootOwned", err)
	}
	if _, err := os.Stat(filepath.Join(home, trMarker)); err == nil {
		t.Error("a refused write created the home")
	}
	allowed := trNew(t, trHome(home), root, nil, WithMode(ModeGlobal), WithAllowRoot(true))
	if _, err := allowed.Ensure(KindDB); err != nil {
		t.Errorf("Ensure(db) with WithAllowRoot(true) = %v", err)
	}
	mine := filepath.Join(root, "roothome")
	trMkdir(t, mine)
	if _, err := trNew(t, trHome(mine), root, nil, WithMode(ModeGlobal)).Ensure(KindDB); err != nil {
		t.Errorf("Ensure(db) in a root-owned home = %v", err)
	}

	tmp := filepath.Join(root, "tmp")
	trMkdir(t, filepath.Join(tmp, trApp+"-0"))
	if err := os.Chown(filepath.Join(tmp, trApp+"-0"), other, other); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	// WithAllowRoot(true): without it the root guard refuses first.
	if _, err := trNew(t, trNoHome(), root, nil, WithAllowRoot(true)).Ensure(KindCache); !errors.Is(err, ErrUnsafeTemp) {
		t.Errorf("Ensure(cache) into another user's temp folder = %v; want ErrUnsafeTemp", err)
	}
}
