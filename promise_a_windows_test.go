package appdir

import (
	"testing"
)

// platforms.md "What differs on Windows": there is no root guard (an
// elevated process is the same user, so ErrRootOwned is never returned and
// WithAllowRoot changes nothing), and the temp fallback has no owner check.
func TestWinNoRootGuard(t *testing.T) {
	if isRoot() {
		t.Fatal("isRoot() is true on Windows; the root guard would apply")
	}
	dir := t.TempDir()
	if !ownedByMe(dir) {
		t.Fatal("ownedByMe is false on Windows; the temp owner check would apply")
	}
	w := paNewWorld(t)
	for _, allow := range []bool{false, true} {
		if _, err := w.new(t, w.proj, nil, WithAllowRoot(allow)).Ensure(KindDB); err != nil {
			t.Errorf("WithAllowRoot(%v): Ensure = %v", allow, err)
		}
	}
}
