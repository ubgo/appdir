//go:build windows

package appdir

// promise_d_windows_test.go: SPEC 12 platform rows whose Windows column only
// a real Windows machine can show: the rename retry on a sharing violation,
// the not-content-indexed cache attribute and the root checks being no-ops.

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestWinPDRenameRetriesWhileTargetHeld proves the SPEC 12 atomic-write row
// (Windows: "rename with retry on sharing violation"): while another handle
// holds the target with no sharing, MoveFileEx fails; renameReplace retries
// and succeeds once the handle is closed.
func TestWinPDRenameRetriesWhileTargetHeld(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	src := filepath.Join(root, "src.txt")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	// Share mode 0: no other open, delete or rename while this is held.
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, target); err == nil {
		_ = syscall.CloseHandle(h)
		t.Fatal("a plain rename over a held file succeeded; the test cannot show the retry")
	}
	done := make(chan struct{})
	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = syscall.CloseHandle(h)
		close(done)
	}()
	err = renameReplace(src, target)
	<-done
	if err != nil {
		t.Fatalf("renameReplace while the target was held briefly: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Fatalf("content = %q, want new", b)
	}
}

// TestWinPDCacheNotContentIndexed proves the SPEC 12 cache-exclusions row
// (Windows: CACHEDIR.TAG + not-content-indexed): Ensure on the cache kind
// tags the folder and sets FILE_ATTRIBUTE_NOT_CONTENT_INDEXED on it.
func TestWinPDCacheNotContentIndexed(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	d, err := New(pwApp, pwOpts(home, home, WithMode(ModeGlobal))...)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := d.Ensure(KindCache)
	if err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, cacheTagFile)) {
		t.Fatal("CACHEDIR.TAG missing")
	}
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		t.Fatal(err)
	}
	if attrs&fileAttributeNotContentIndexed == 0 {
		t.Fatalf("attributes %#x lack NOT_CONTENT_INDEXED", attrs)
	}
}

// TestWinPDRootChecksAreNoOps proves the SPEC 12 rows "root / sudo: no-op"
// and "temp fallback owner check: %TEMP% is per user; Lstat only" on
// Windows: no process counts as root, no owner uid is read, every folder
// counts as ours, and a write is never refused for ownership.
func TestWinPDRootChecksAreNoOps(t *testing.T) {
	root := rxRoot(t)
	if isRoot() {
		t.Fatal("isRoot() is true on Windows")
	}
	if _, ok := ownerUID(root); ok {
		t.Fatal("ownerUID reported an owner on Windows")
	}
	if !ownedByMe(root) {
		t.Fatal("ownedByMe is false on Windows")
	}
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	d, err := New(pwApp, pwOpts(home, home, WithMode(ModeGlobal))...)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.checkWrite(home, nil); err != nil {
		t.Fatalf("checkWrite = %v", err)
	}
	// Lstat still guards the per-user folder: a file there is refused.
	f := filepath.Join(root, "notadir")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.ensurePrivateDir(f); err == nil {
		t.Fatal("ensurePrivateDir accepted a file")
	}
}
