package appdir

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ForTest isolates a test from the developer's real homes (standard section
// 13): it points HOME, USERPROFILE, <APP>_HOME, XDG_*, CLAUDE_CONFIG_DIR,
// CODEX_HOME and the temp folder (TMPDIR, and TMP and TEMP, which Windows
// reads) into a temp root, unsets <APP>_PROFILE and the GIT_*
// variables that leak a parent repo, and sets APPDIR_TEST_ROOT so New panics
// on any path outside it. Returns the root.
// ds:def id=fortest-56v4yh8s owner=@khanakia stability=stable
func ForTest(t testing.TB, app string) string {
	t.Helper()
	root := testTempRoot(t)
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	prefix := envPrefixFor(app)
	set := map[string]string{
		"HOME":              filepath.Join(root, "home"),
		"USERPROFILE":       filepath.Join(root, "home"),
		prefix + "_HOME":    filepath.Join(root, "apphome"),
		"XDG_CONFIG_HOME":   filepath.Join(root, "xdg", "config"),
		"XDG_DATA_HOME":     filepath.Join(root, "xdg", "data"),
		"XDG_STATE_HOME":    filepath.Join(root, "xdg", "state"),
		"XDG_CACHE_HOME":    filepath.Join(root, "xdg", "cache"),
		"XDG_RUNTIME_DIR":   filepath.Join(root, "xdg", "run"),
		"CLAUDE_CONFIG_DIR": filepath.Join(root, "agent-config"),
		"CODEX_HOME":        filepath.Join(root, "codex"),
		"TMPDIR":            filepath.Join(root, "tmp"),
		"TMP":               filepath.Join(root, "tmp"),
		"TEMP":              filepath.Join(root, "tmp"),
		envTestRoot:         root,
	}
	for k, v := range set {
		if err := os.MkdirAll(v, 0o700); err != nil && k != envTestRoot {
			t.Fatalf("appdir.ForTest: %v", err)
		}
		t.Setenv(k, v)
	}
	unset := []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES"}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		_, ours := set[k]
		switch {
		case strings.HasPrefix(k, "GIT_CONFIG"):
			unset = append(unset, k)
		// Every other <APP>_* variable from the developer's shell
		// (<APP>_PROFILE, _ALLOW_PROD, _DAEMON_PROFILE, _CEILING_DIRS,
		// _<KIND>_DIR) would change what the test resolves, so none of
		// them reaches the test; only the ones set above remain.
		case strings.HasPrefix(k, prefix+"_") && !ours:
			unset = append(unset, k)
		}
	}
	// <APP>_PROFILE is always cleared, even when the shell does not set it,
	// so a later t.Setenv in the test starts from a known state.
	unset = append(unset, prefix+"_PROFILE")
	for _, k := range unset {
		t.Setenv(k, "") // registers the restore at cleanup
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("appdir.ForTest: %v", err)
		}
	}
	return root
}

// testTempRoot is t.TempDir(), except on macOS, where it is a fresh folder
// under /tmp removed at the end of the test. macOS's $TMPDIR is long
// (/var/folders/xx/.../T/), so under t.TempDir() run/<app>.sock rarely fits
// in WithSocketMaxLen and the socket falls back to the real /tmp, which the
// test guard refuses; a root under /tmp keeps the socket in run/.
func testTempRoot(t testing.TB) string {
	if runtime.GOOS != "darwin" {
		return t.TempDir()
	}
	dir, err := os.MkdirTemp("/tmp", "appdir-test-") // literally /tmp, as the socket fallback
	if err != nil {
		return t.TempDir()
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) }) // our own test folder
	return dir
}
