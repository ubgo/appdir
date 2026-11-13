//go:build darwin

package appdir

// promise_c_darwin_test.go: promises of SPEC 5.5 and 5.8 that only a real
// macOS machine can show. Helpers start with "pc".

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDarwinCacheIndexMarkers: SPEC 5.5 and
// docs/layout/files-and-permissions.md. On macOS a disposable folder gets
// CACHEDIR.TAG, an empty .metadata_never_index and the Time Machine
// exclusion (tmutil addexclusion); WithCacheTag(false) adds none of them.
func TestDarwinCacheIndexMarkers(t *testing.T) {
	fx := tlNewFix(t)
	c, err := fx.tlNew(t, WithMode(ModeGlobal)).Ensure(KindCache)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(c, ".metadata_never_index"))
	if err != nil || fi.Size() != 0 {
		t.Fatalf(".metadata_never_index: %v (size %v)", err, fi)
	}
	if !tlExists(filepath.Join(c, cacheTagFile)) {
		t.Error("CACHEDIR.TAG missing")
	}
	// The Time Machine exclusion is best effort by design (SPEC 5.5): a
	// runner without Time Machine may refuse it, so it is reported, not
	// required. The two marker files above are the promise.
	out, err := exec.Command("tmutil", "isexcluded", c).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "[Excluded]") {
		t.Logf("tmutil isexcluded %s: %v %s (best effort)", c, err, out)
	}

	fx2 := tlNewFix(t)
	off, err := fx2.tlNew(t, WithMode(ModeGlobal), WithCacheTag(false)).Ensure(KindCache)
	if err != nil {
		t.Fatal(err)
	}
	if tlExists(filepath.Join(off, ".metadata_never_index")) || tlExists(filepath.Join(off, cacheTagFile)) {
		t.Error("WithCacheTag(false) still marked the cache")
	}
}

// TestDarwinCacheWithoutTmutil: SPEC 5.5 and
// docs/layout/files-and-permissions.md. The Time Machine exclusion is best
// effort: with no tmutil on PATH, Ensure still creates the cache with
// CACHEDIR.TAG and .metadata_never_index.
func TestDarwinCacheWithoutTmutil(t *testing.T) {
	fx := tlNewFix(t)
	empty := filepath.Join(fx.root, "empty-path")
	tlMkdir(t, empty)
	t.Setenv("PATH", empty)
	c, err := fx.tlNew(t, WithMode(ModeGlobal)).Ensure(KindCache)
	if err != nil {
		t.Fatalf("Ensure(cache) without tmutil: %v", err)
	}
	for _, f := range []string{cacheTagFile, ".metadata_never_index"} {
		if !tlExists(filepath.Join(c, f)) {
			t.Errorf("%s missing", f)
		}
	}
}

// TestDarwinInitProjectHomeOtherCase: SPEC 5.8. InitProject refuses the
// future global home before ~/.<app> exists, comparing case-insensitively
// on macOS: the user's home spelled in upper case is still refused.
func TestDarwinInitProjectHomeOtherCase(t *testing.T) {
	fx := tlNewFix(t)
	other := filepath.Join(filepath.Dir(fx.home), strings.ToUpper(filepath.Base(fx.home)))
	if _, err := fx.tlNew(t).InitProject(other); !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("InitProject(%s) err = %v; want ErrInvalidOption", other, err)
	}
	if tlExists(fx.tlGlobal()) {
		t.Fatal("the global home was created as a project")
	}
}
