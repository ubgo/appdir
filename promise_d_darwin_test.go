//go:build darwin

package appdir

// promise_d_darwin_test.go: the SPEC 12 cache-exclusions row on macOS.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDarwinPDCacheNeverIndex proves the SPEC 12 cache-exclusions row (macOS:
// CACHEDIR.TAG + .metadata_never_index): Ensure on the cache kind writes
// both into the cache folder, the Spotlight marker empty.
func TestDarwinPDCacheNeverIndex(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	d, err := New("pdapp", rxHome(home), WithWorkDir(home), rxEnv(nil), WithRelease(true), WithMode(ModeGlobal))
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
	fi, err := os.Stat(filepath.Join(dir, ".metadata_never_index"))
	if err != nil || fi.Size() != 0 {
		t.Fatalf(".metadata_never_index: %v %v", fi, err)
	}
}

// TestDarwinPDForTestRootRemoved proves SPEC 10 on macOS: the ForTest root is
// a fresh folder under /tmp (not t.TempDir()) and it is removed when the
// test ends.
func TestDarwinPDForTestRootRemoved(t *testing.T) {
	var root string
	t.Run("uses", func(t *testing.T) {
		root = ForTest(t, "pdapp")
		if !strings.HasPrefix(root, "/private/tmp/") && !strings.HasPrefix(root, "/tmp/") {
			t.Fatalf("ForTest root = %s, want one under /tmp", root)
		}
		if err := os.WriteFile(filepath.Join(root, "f"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	if root == "" {
		t.Fatal("no root")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("ForTest root %s still exists after the test: %v", root, err)
	}
}
