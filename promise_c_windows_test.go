//go:build windows

package appdir

// promise_c_windows_test.go: promises of SPEC 5.5 and 5.8 that only a real
// Windows machine can show. Helpers start with "pc".

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// pcAttrs returns the Windows file attributes of path.
func pcAttrs(t *testing.T, path string) uint32 {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := syscall.GetFileAttributes(p)
	if err != nil {
		t.Fatalf("GetFileAttributes %s: %v", path, err)
	}
	return a
}

// TestWinCacheIndexAttrOnlyOnTaggedCache: SPEC 5.5 and
// docs/layout/files-and-permissions.md. On Windows a disposable folder gets
// FILE_ATTRIBUTE_NOT_CONTENT_INDEXED next to CACHEDIR.TAG; a plain folder
// kind does not, and WithCacheTag(false) sets neither.
func TestWinCacheIndexAttrOnlyOnTaggedCache(t *testing.T) {
	fx := tlNewFix(t)
	d := fx.tlNew(t, WithMode(ModeGlobal))
	c, err := d.Ensure(KindCache)
	if err != nil {
		t.Fatal(err)
	}
	if pcAttrs(t, c)&fileAttributeNotContentIndexed == 0 {
		t.Errorf("%s lacks FILE_ATTRIBUTE_NOT_CONTENT_INDEXED", c)
	}
	if !tlExists(filepath.Join(c, cacheTagFile)) {
		t.Errorf("%s lacks CACHEDIR.TAG", c)
	}
	r, err := d.Ensure(KindRun)
	if err != nil {
		t.Fatal(err)
	}
	if pcAttrs(t, r)&fileAttributeNotContentIndexed != 0 {
		t.Errorf("run/ %s is marked not content indexed", r)
	}
	fx2 := tlNewFix(t)
	off, err := fx2.tlNew(t, WithMode(ModeGlobal), WithCacheTag(false)).Ensure(KindCache)
	if err != nil {
		t.Fatal(err)
	}
	if pcAttrs(t, off)&fileAttributeNotContentIndexed != 0 || tlExists(filepath.Join(off, cacheTagFile)) {
		t.Errorf("WithCacheTag(false) still marked %s", off)
	}
}

// TestWinAtomicWriteRetriesSharingViolation: SPEC 5.8. The atomic write
// retries the rename a few times while another process holds the target
// open (antivirus, the indexer): a handle released after a short while lets
// the write finish; one held for good fails after the retries, leaving the
// old file and no temp file.
func TestWinAtomicWriteRetriesSharingViolation(t *testing.T) {
	fx := tlNewFix(t)
	d := fx.tlNew(t)
	dir := filepath.Join(fx.root, "h")
	tlMkdir(t, dir)
	target := filepath.Join(dir, "home.jsonc")
	tlWrite(t, target, "old", 0o600)

	held, err := os.Open(target) // no FILE_SHARE_DELETE: a rename over it fails
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = held.Close()
		close(released)
	}()
	if err := d.writeFileAtomic(target, []byte("new"), 0o600); err != nil {
		t.Fatalf("write while the handle is released after 120ms: %v", err)
	}
	<-released
	if got := tlRead(t, target); got != "new" {
		t.Fatalf("target = %q; want new", got)
	}

	held, err = os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	start := time.Now()
	err = d.writeFileAtomic(target, []byte("never"), 0o600)
	if err == nil {
		t.Fatal("write over a file held open for good succeeded")
	}
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) && !errors.Is(err, syscall.Errno(32)) {
		t.Errorf("err = %v; want a sharing violation or access denied", err)
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Errorf("gave up after %v; want a few retries", time.Since(start))
	}
	_ = held.Close()
	if got := tlRead(t, target); got != "new" {
		t.Fatalf("target = %q after a failed write", got)
	}
	tlNoTemps(t, dir)
}

// TestWinInitProjectHomeOtherCase: SPEC 5.8. InitProject refuses the
// future global home before ~/.<app> exists, comparing case-insensitively
// on Windows: the user's home spelled in upper case is still refused.
func TestWinInitProjectHomeOtherCase(t *testing.T) {
	fx := tlNewFix(t)
	other := filepath.Join(filepath.Dir(fx.home), strings.ToUpper(filepath.Base(fx.home)))
	if _, err := fx.tlNew(t).InitProject(other); !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("InitProject(%s) err = %v; want ErrInvalidOption", other, err)
	}
	if tlExists(fx.tlGlobal()) {
		t.Fatal("the global home was created as a project")
	}
}
