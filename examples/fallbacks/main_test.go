package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/ubgo/appdir"
)

// TestRun checks the lines the fallbacks guide quotes.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`global home: rule=defaults fallback=true note="no home folder"`,
		"config  rule=defaults fallback=true (none: code defaults)",
		"db      rule=global   error: no home folder; set LORE_HOME",
		"cache   rule=global   error: no home folder; set LORE_HOME",
	}
	// The <uid> placeholder and a read-only folder mode exist only on Unix.
	if runtime.GOOS != "windows" {
		want = append(want, "cache   rule=temp     fallback=true $TMPDIR/lore-<uid>/cache")
	}
	if os.Geteuid() != 0 && runtime.GOOS != "windows" {
		want = append(want,
			"Ensure(db): create ro-home/.lore: mkdir ro-home/.lore: permission denied (set LORE_HOME or LORE_DB_DIR)",
			"Ensure(cache): create ro-home/.lore/cache: mkdir ro-home/.lore: permission denied (set LORE_HOME or LORE_CACHE_DIR)")
	}
	for _, w := range want {
		if !strings.Contains(out.String(), w) {
			t.Errorf("output lacks %q\n%s", w, out.String())
		}
	}
}

// privateTemp points the system temp folder at a fresh test folder, so the
// temp fallback is created there and nowhere else. It returns that folder
// and the fallback root <tmp>/lore-<uid>.
func privateTemp(t *testing.T) (tmp, fallback string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fallback folder is named by uid and checked by mode, both Unix; the library's own tests cover Windows")
	}
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	return tmp, filepath.Join(tmp, appName+"-"+strconv.Itoa(os.Getuid()))
}

// TestEnsureTempFallback creates the cache with no home: it lands in the
// private temp folder, which is created with mode 0700.
func TestEnsureTempFallback(t *testing.T) {
	tmp, fallback := privateTemp(t)
	d, err := open(func() (string, error) { return "", errors.New("no home") }, tmp)
	if err != nil {
		t.Fatal(err)
	}
	p, err := d.Ensure(appdir.KindCache)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(fallback, "cache"); p != want {
		t.Fatalf("Ensure(cache) = %s, want %s", p, want)
	}
	fi, err := os.Stat(fallback)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("temp root mode = %v, want 0700", fi.Mode().Perm())
	}
}

// TestEnsureReadOnlyFallsBack shows the second temp fallback: the home
// exists but cannot be written, so Ensure(cache) uses the temp folder
// while Ensure(db) still fails.
func TestEnsureReadOnlyFallsBack(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores folder permissions")
	}
	tmp, fallback := privateTemp(t)
	home := filepath.Join(tmp, readOnlyDir)
	if err := os.Mkdir(home, readOnly); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, writable) })
	d, err := open(func() (string, error) { return home, nil }, tmp)
	if err != nil {
		t.Fatal(err)
	}
	p, err := d.Ensure(appdir.KindCache)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(fallback, "cache"); p != want {
		t.Fatalf("Ensure(cache) = %s, want %s", p, want)
	}
	if _, err := d.Ensure(appdir.KindDB); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Ensure(db) = %v, want a permission error", err)
	}
}

// TestUnsafeTemp plants a symlink where the temp fallback goes: appdir
// refuses it with ErrUnsafeTemp instead of writing through it.
func TestUnsafeTemp(t *testing.T) {
	tmp, fallback := privateTemp(t)
	elsewhere := filepath.Join(tmp, "elsewhere")
	if err := os.Mkdir(elsewhere, writable); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, fallback); err != nil {
		t.Fatal(err)
	}
	d, err := open(func() (string, error) { return "", errors.New("no home") }, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Ensure(appdir.KindCache); !errors.Is(err, appdir.ErrUnsafeTemp) {
		t.Fatalf("Ensure(cache) = %v, want ErrUnsafeTemp", err)
	}
}
