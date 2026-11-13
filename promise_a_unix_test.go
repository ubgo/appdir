//go:build unix

package appdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC 3.3 WithSocketMaxLen default 100 (options.md "100 bytes"): with no
// option, a run/<app>.sock path of exactly 100 bytes stays under run/ and
// one of 101 bytes falls back. Socket only computes, so the guard is off for
// the short root under the literal /tmp.
func TestPromiseA_SocketMaxLenDefault(t *testing.T) {
	base, err := os.MkdirTemp("/tmp", "pa")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) }) // our own test folder
	tail := len(string(filepath.Separator) + filepath.Join("run", paApp+".sock"))
	for _, c := range []struct {
		n        int
		fallback bool
	}{{100, false}, {101, true}} {
		h := filepath.Join(base, strings.Repeat("x", c.n-tail-len(base)-1))
		d, err := New(paApp, WithHomeDir(func() (string, error) { return base, nil }), WithWorkDir(base), paEnv(nil),
			WithRelease(true), WithHome(h), WithTestGuard(false))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(h, "run", paApp+".sock")
		if len(want) != c.n {
			t.Fatalf("fixture: path length %d, want %d", len(want), c.n)
		}
		s, err := d.Socket()
		if err != nil {
			t.Fatal(err)
		}
		if s.Fallback != c.fallback || (!c.fallback && s.Path != want) {
			t.Errorf("length %d: Socket() = %+v, want fallback %v", c.n, s, c.fallback)
		}
	}
}
