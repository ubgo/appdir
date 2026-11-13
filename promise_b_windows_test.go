package appdir

// Windows-only promise tests for SPEC 4.7 (<uid> is the user name) and 4.8
// (the root check is a no-op). Named TestWin... so the CI platform step runs
// them; they never skip on Windows.

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC 4.7: on Windows the temp fallback is %TEMP%\<Name(app)>-<user name>,
// the user name without its domain.
func TestWinPromiseBTempFallbackUserName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	name := u.Username
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	d := trNew(t, trNoHome(), tmp, nil)
	p, err := d.Path(KindCache)
	want := filepath.Join(os.TempDir(), trApp+"-"+name, "cache")
	if err != nil || p != want {
		t.Fatalf("Path(cache) = %q, %v; want %q", p, err, want)
	}
}

// SPEC 4.8: Windows has no root equivalent; the root check is a no-op, so a
// write without WithAllowRoot succeeds in any home, elevated or not.
func TestWinPromiseBRootGuardNoop(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	trMkdir(t, home)
	d := trNew(t, trHome(home), root, nil, WithMode(ModeGlobal), WithAllowRoot(false))
	if isRoot() {
		t.Fatal("isRoot() = true on Windows")
	}
	if err := d.checkWrite(home, nil); err != nil {
		t.Fatalf("checkWrite = %v; want nil", err)
	}
	if _, err := d.Ensure(KindDB); err != nil {
		t.Fatalf("Ensure(db) = %v", err)
	}
}
