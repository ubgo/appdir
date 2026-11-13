package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRun runs the walk-through in a test folder and checks the lines the
// guide relies on: every run file, the socket in run/, the fallback (Linux
// only: elsewhere it is the real /tmp and the example does not create it),
// and the daemon profile rules.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, shortRoot(t)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"RunPath(lock): home/.demo/run/lock",
		"RunPath(daemon-lock): home/.demo/run/daemon.lock",
		"RunPath(pid): home/.demo/run/demo-daemon.pid",
		"RunPath(discovery): home/.demo/run/discovery.json",
		"RunPath(socket): home/.demo/run/demo.sock",
		"RunPath(socket-file): home/.demo/run/socket",
		"PointerFile: home/.demo/run/socket",
		"dev client                             -> DaemonProfile dev, discovery home/.demo-dev/run/discovery.json",
		"dev client, --daemon-profile prod      -> DaemonProfile prod, discovery home/.demo/run/discovery.json",
		"[demo] dev client → prod daemon",
		"dev client, DEMO_DAEMON_PROFILE=prod   -> DaemonProfile prod, discovery home/.demo/run/discovery.json",
		`dev client, --daemon-profile qa        -> error: appdir: --daemon-profile: unknown profile "qa" (known: prod, dev)`,
	}
	if runtime.GOOS != "windows" {
		want = append(want,
			"Socket: home/.demo/run/demo.sock fallback=false",
			"created home/.demo/run mode 0700")
	}
	if runtime.GOOS == "linux" {
		want = append(want,
			"Socket: xdg-run/demo/",
			"created xdg-run/demo mode 0700",
			"client reads run/socket: xdg-run/demo/")
	}
	for _, w := range want {
		if !strings.Contains(out.String(), w) {
			t.Errorf("output lacks %q\n%s", w, out.String())
		}
	}
}

// shortRoot is the test's temp root. On macOS it is a fresh folder under
// /tmp, made the test root with APPDIR_TEST_ROOT: t.TempDir() there is under
// the long $TMPDIR, so run/<app>.sock would not fit in 100 bytes and the
// socket would fall back to the real /tmp, which the guard refuses.
func shortRoot(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		return t.TempDir()
	}
	dir, err := os.MkdirTemp("/tmp", "appdir-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDIR_TEST_ROOT", real)
	return real
}
