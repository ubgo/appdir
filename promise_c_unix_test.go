//go:build unix

package appdir

// promise_c_unix_test.go: promises of SPEC 5.6 and 7 and of the layout and
// choosing guides that need Unix symlinks, modes or uids. Helpers start
// with "pc".

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestPCFolderKindSymlinkFollowed: docs/layout/files-and-permissions.md.
// The symlink check covers file kinds only: a folder kind that is a symlink
// to another disk (cache/ linked elsewhere) is followed without a check.
func TestPCFolderKindSymlinkFollowed(t *testing.T) {
	fx := tlNewFix(t)
	other := filepath.Join(fx.root, "fast-disk", "cache")
	tlMkdir(t, other)
	tlSymlink(t, other, filepath.Join(fx.tlGlobal(), "cache"))
	p, err := fx.tlNew(t).Ensure(KindCache)
	if err != nil || p != filepath.Join(fx.tlGlobal(), "cache") {
		t.Fatalf("Ensure(cache) = %q, %v", p, err)
	}
	if !tlExists(filepath.Join(other, cacheTagFile)) {
		t.Fatal("the linked cache folder was not used")
	}
}

// TestPCVerifyFileMessages: docs/layout/files-and-permissions.md. The
// ErrSymlinkEscape messages: "<path> links to <real path>, outside the home
// <home>" and "<path> is a broken symlink"; both stop Ensure and CheckDB.
func TestPCVerifyFileMessages(t *testing.T) {
	fx := tlNewFix(t)
	m := fx.tlProjectMarker(t)
	outside := filepath.Join(fx.root, "outside.db")
	tlWrite(t, outside, "", 0o600)
	db := filepath.Join(m, tlApp+".db")
	tlSymlink(t, outside, db)
	d := fx.tlNew(t)
	want := db + " links to " + outside + ", outside the home " + m
	if err := d.VerifyFile(db); !errors.Is(err, ErrSymlinkEscape) || err.Error() != want {
		t.Fatalf("VerifyFile = %v; want %q", err, want)
	}
	if _, err := d.Ensure(KindDB); !errors.Is(err, ErrSymlinkEscape) {
		t.Errorf("Ensure(db) = %v", err)
	}
	if _, err := d.CheckDB(db); !errors.Is(err, ErrSymlinkEscape) {
		t.Errorf("CheckDB = %v", err)
	}
	if err := os.Remove(db); err != nil {
		t.Fatal(err)
	}
	tlSymlink(t, filepath.Join(m, "gone.db"), db)
	want = db + " is a broken symlink"
	if err := d.VerifyFile(db); !errors.Is(err, ErrSymlinkEscape) || err.Error() != want {
		t.Fatalf("VerifyFile broken = %v; want %q", err, want)
	}
	if _, err := d.CheckDB(db); !errors.Is(err, ErrSymlinkEscape) {
		t.Errorf("CheckDB broken = %v", err)
	}
}

// TestPCDevDBLinkedIntoMarkerEscapes: docs/layout/files-and-permissions.md.
// In the dev profile the db's home is .<app>/profiles/dev/ (the longest home
// holding it): a dev db linked to the prod db in .<app>/ itself escapes.
func TestPCDevDBLinkedIntoMarkerEscapes(t *testing.T) {
	fx := tlNewFix(t)
	m := fx.tlProjectMarker(t)
	prod := filepath.Join(m, tlApp+".db")
	tlWrite(t, prod, "", 0o600)
	dev := filepath.Join(m, profilesDir, string(ProfileDev), tlApp+".db")
	tlSymlink(t, prod, dev)
	d := fx.tlNew(t, WithRelease(false))
	if err := d.VerifyFile(dev); !errors.Is(err, ErrSymlinkEscape) {
		t.Fatalf("VerifyFile(dev db -> prod db) = %v; want ErrSymlinkEscape", err)
	}
	if _, err := d.Ensure(KindDB); !errors.Is(err, ErrSymlinkEscape) {
		t.Fatalf("Ensure(db) = %v; want ErrSymlinkEscape", err)
	}
}

// TestPCReadOnlyProjectDisposableFallsBack: docs/choosing/mode-and-scopes.md.
// The read-only fallback of Ensure works in either home: a disposable kind
// in a read-only project home goes to the private temp folder.
func TestPCReadOnlyProjectDisposableFallsBack(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores folder modes")
	}
	fx := tlNewFix(t)
	tlSetTemp(t, fx.root)
	m := fx.tlProjectMarker(t)
	if err := os.Chmod(m, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(m, 0o700) })
	p, err := fx.tlNew(t, WithKind("thumbs", Disposable())).Ensure("thumbs")
	if want := filepath.Join(tlTempRoot(fx.root), "thumbs"); err != nil || p != want {
		t.Fatalf("Ensure(thumbs) = %q, %v; want %s", p, err, want)
	}
}

// TestPCSocketTmpFallbackWithoutXDG: SPEC 7 and
// docs/runtime/run-files-and-sockets.md (Linux; macOS has
// TestDarwinSocketFallbackLiteralTmp). XDG_RUNTIME_DIR is used only when it
// names an existing absolute folder; unset, relative or missing, the socket
// falls back to the literal /tmp/<Name(app)>-<uid>/. Only Socket() is
// called, so nothing is created in /tmp; the guard is off for that reason.
func TestPCSocketTmpFallbackWithoutXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("macOS is covered by TestDarwinSocketFallbackLiteralTmp")
	}
	for name, xdg := range map[string]string{"unset": "", "relative": "xdg", "missing": "@root/no-such"} {
		t.Run(name, func(t *testing.T) {
			fx := tlNewFix(t)
			env := map[string]string{}
			switch xdg {
			case "xdg":
				env["XDG_RUNTIME_DIR"] = xdg
			case "@root/no-such":
				env["XDG_RUNTIME_DIR"] = filepath.Join(fx.root, "no-such")
			}
			d := fx.tlNew(t, WithEnv(tlEnv(env)), WithMode(ModeGlobal), WithSocketMaxLen(10), WithRelease(false), WithTestGuard(false))
			s, err := d.Socket()
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join("/tmp", tlApp+"-dev-"+currentUser())
			if !s.Fallback || s.Dir != dir || filepath.Dir(s.Path) != dir {
				t.Fatalf("Socket = %+v; want it in %s", s, dir)
			}
		})
	}
}

// TestPCEnsureSocketDirRefusesForeignOwner: SPEC 7. EnsureSocketDir refuses
// a fallback folder owned by another uid with ErrUnsafeTemp. The app is
// named "usr" and XDG_RUNTIME_DIR is "/", so the fallback folder is /usr: an
// existing folder owned by root. run/ is created in the test root first;
// nothing outside it is created (the guard is off for /usr).
func TestPCEnsureSocketDirRefusesForeignOwner(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_RUNTIME_DIR is read on Linux only")
	}
	if os.Geteuid() == 0 {
		t.Skip("root owns /usr")
	}
	if fi, err := os.Lstat("/usr"); err != nil || !fi.IsDir() || ownedByMe("/usr") {
		t.Skip("/usr is not a root-owned folder here")
	}
	fx := tlNewFix(t)
	d, err := New("usr", WithHomeDir(func() (string, error) { return fx.home, nil }), WithWorkDir(fx.work),
		WithEnv(tlEnv(map[string]string{"XDG_RUNTIME_DIR": "/"})), WithRelease(true), WithMode(ModeGlobal),
		WithSocketMaxLen(10), WithTestGuard(false))
	if err != nil {
		t.Fatal(err)
	}
	s, err := d.Socket()
	if err != nil || s.Dir != "/usr" {
		t.Fatalf("Socket = %+v, %v; want the fallback folder /usr", s, err)
	}
	if _, err := d.EnsureSocketDir(); !errors.Is(err, ErrUnsafeTemp) {
		t.Fatalf("EnsureSocketDir err = %v; want ErrUnsafeTemp", err)
	}
}
