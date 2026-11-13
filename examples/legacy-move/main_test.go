package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// TestRun runs the walk-through in a test folder and checks the lines the
// guide relies on: the same-disk move and MOVED.txt, "already" on the second
// run, both-exist, the skipped cases, and ErrBusy while the old daemon's lock
// is held (Unix only: the example holds it with flock).
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`status=moved from=home-move/.demo-old to=home-move/.demo note="renamed (same disk)"`,
		"status=already from=home-move/.demo-old to=home-move/.demo",
		".demo-old/MOVED.txt: moved to home-move/.demo",
		"  .demo.moving.lock",
		"  .demo/notes/today.md",
		`status=both-exist from=home-both/.demo-old to=home-both/.demo note="both exist; using home-both/.demo"`,
		`dev build         status=skipped note="profile dev never runs a legacy move"`,
		`--home elsewhere  status=skipped note="home set by --home or DEMO_HOME"`,
		"no old folder     status=none",
	}
	if runtime.GOOS != "windows" {
		want = append(want,
			"while the old daemon runs: old home home-busy/.demo-old is in use by a running daemon; will retry next run",
			`next start: status=moved from=home-busy/.demo-old to=home-busy/.demo`)
	}
	for _, w := range want {
		if !strings.Contains(out.String(), w) {
			t.Errorf("output lacks %q\n%s", w, out.String())
		}
	}
}
