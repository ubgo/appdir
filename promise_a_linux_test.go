package appdir

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// paRootChecks proves SPEC 3.5 ErrRootOwned and SPEC 3.3 WithAllowRoot
// (default false) while the process has effective uid 0: a write into a
// folder owned by another uid is refused before anything is created, a
// write into root's own folder is not, and WithAllowRoot(true) lifts the
// refusal.
func paRootChecks(t *testing.T) {
	if !isRoot() {
		t.Fatal("not running with effective uid 0")
	}
	dir := t.TempDir()
	other := filepath.Join(dir, "other")
	paMkdir(t, other)
	if err := os.Chown(other, 65534, 65534); err != nil {
		// In a user namespace only uid 0 is mapped; folders of the real
		// root, such as /, show as the overflow uid, another user.
		other = string(filepath.Separator)
		if uid, ok := ownerUID(other); !ok || uid == 0 {
			t.Skip("no folder owned by another uid")
		}
	}
	home := filepath.Join(other, "pa-sub-"+strconv.Itoa(os.Getpid()), paMarker)
	opts := func(h string, extra ...Option) []Option {
		return append([]Option{WithHomeDir(func() (string, error) { return dir, nil }), WithWorkDir(dir), paEnv(nil),
			WithRelease(true), WithHome(h), WithTestGuard(false)}, extra...)
	}
	d, err := New(paApp, opts(home)...)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := ownerUID(other)
	_, err = d.Ensure(KindDB)
	paCheckErr(t, err, ErrRootOwned, "running as root but "+other+" belongs to uid "+strconv.Itoa(uid)+"; pass --allow-root")
	if exists(filepath.Dir(home)) {
		t.Errorf("refused write created %s", filepath.Dir(home))
	}
	if err := d.InitHome(home, ScopeGlobal); !errors.Is(err, ErrRootOwned) {
		t.Errorf("InitHome = %v, want ErrRootOwned", err)
	}
	mine, err := New(paApp, opts(filepath.Join(dir, "mine"))...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mine.Ensure(KindDB); err != nil {
		t.Errorf("root writing into its own folder: %v", err)
	}
	allowed, err := New(paApp, opts(home, WithAllowRoot(true))...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allowed.Ensure(KindDB); errors.Is(err, ErrRootOwned) {
		t.Errorf("WithAllowRoot(true): %v", err)
	}
}

// TestPromiseA_RootOwned runs paRootChecks as root: directly when the tests
// run as root, else in a child test process inside a new user namespace
// where the current user is uid 0. It skips when user namespaces are not
// available (a sandbox or an AppArmor restriction).
func TestPromiseA_RootOwned(t *testing.T) {
	if os.Getenv("PA_ROOT_CHILD") == "1" || os.Geteuid() == 0 {
		paRootChecks(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPromiseA_RootOwned$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), "PA_ROOT_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
	}
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	switch {
	case err != nil && !errors.As(err, &ee):
		t.Skipf("user namespaces unavailable: %v", err)
	case err != nil:
		t.Fatalf("child test failed: %v\n%s", err, out)
	case strings.Contains(string(out), "--- SKIP: TestPromiseA_RootOwned"):
		t.Skipf("child skipped:\n%s", out)
	case !strings.Contains(string(out), "--- PASS: TestPromiseA_RootOwned"):
		t.Fatalf("child did not run the test:\n%s", out)
	}
}

// SPEC 2 "Errors are deferred": New fails for an unreadable working folder
// (the default os.Getwd seam fails once the current folder is removed).
func TestPromiseA_UnreadableWorkDir(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	paMkdir(t, gone)
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("getcwd still answers for a removed folder")
	}
	home := filepath.Dir(gone)
	_, err := New(paApp, WithHomeDir(func() (string, error) { return home, nil }), paEnv(nil), WithRelease(true))
	if err == nil || !strings.Contains(err.Error(), "working folder") {
		t.Fatalf("New = %v, want a working folder error", err)
	}
}
