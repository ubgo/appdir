//go:build windows

package appdir

// links_windows_test.go: directory junctions count as links (SPEC 5.6 and
// the symlink row of SPEC 12). Go 1.23+ EvalSymlinks no longer follows
// junctions, so appdir resolves them itself; these tests make real
// junctions with mklink /J (no admin rights needed).

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// winJunction makes a directory junction link -> target.
func winJunction(t *testing.T, link, target string) {
	t.Helper()
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J %s %s: %v\n%s", link, target, err, out)
	}
}

// TestWinJunctionEscape: a junction inside the home that leads outside it
// is refused by VerifyFile with ErrSymlinkEscape, one that stays inside is
// accepted, and realPath follows the junction to its target.
func TestWinJunctionEscape(t *testing.T) {
	root := trRoot(t)
	home := filepath.Join(root, "home")
	outside := filepath.Join(root, "outside")
	global := filepath.Join(home, trMarker)
	inside := filepath.Join(global, "sub")
	trMkdir(t, outside, inside)
	trWrite(t, filepath.Join(outside, "f.txt"), "x")
	d := trNew(t, trHome(home), root, nil, WithMode(ModeGlobal))

	out := filepath.Join(global, "out")
	winJunction(t, out, outside)
	if err := d.VerifyFile(out); !errors.Is(err, ErrSymlinkEscape) {
		t.Errorf("VerifyFile(junction to outside) = %v, want ErrSymlinkEscape", err)
	}
	in := filepath.Join(global, "in")
	winJunction(t, in, inside)
	if err := d.VerifyFile(in); err != nil {
		t.Errorf("VerifyFile(junction inside the home) = %v, want nil", err)
	}

	got := realPath(filepath.Join(out, "f.txt"))
	if want := filepath.Join(outside, "f.txt"); !sameFile(got, want) || !within(got, outside) {
		t.Errorf("realPath through junction = %s, want %s", got, want)
	}
	if within(filepath.Join(out, "f.txt"), global) {
		t.Errorf("a file behind a junction to outside counts as inside the home")
	}
	if _, err := os.Stat(filepath.Join(out, "f.txt")); err != nil {
		t.Fatal(err)
	}
}
