package flock

// Why these tests can run inside one process: flock(2) locks belong to the
// open file description, not to the process, and LockFileEx locks belong to
// the handle. Two os.OpenFile calls on the same path therefore conflict with
// each other exactly like two processes would, so a second TryLock (or the
// probe inside Held) stands in for "another process".

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// flLockName is the lock file name every test uses inside its temp dir.
const flLockName = "daemon.lock"

// flPath returns a fresh lock path in a per-test temp dir.
func flPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), flLockName)
}

// flMustLock takes the lock with mode 0600 or fails the test; the caller
// releases it.
func flMustLock(t *testing.T, path string) *Lock {
	t.Helper()
	lk, err := TryLock(path, 0o600)
	if err != nil {
		t.Fatalf("TryLock(%s): %v", path, err)
	}
	return lk
}

// TestTryLockSecondIsLocked: a second TryLock on a held lock fails with ErrLocked and returns no lock.
func TestTryLockSecondIsLocked(t *testing.T) {
	path := flPath(t)
	lk := flMustLock(t, path)
	defer func() { _ = lk.Release() }()

	second, err := TryLock(path, 0o600)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("second TryLock err = %v, want ErrLocked", err)
	}
	if second != nil {
		t.Errorf("second TryLock returned a lock alongside ErrLocked")
	}
}

// TestReleaseFreesLock: after Release the same path can be locked again.
func TestReleaseFreesLock(t *testing.T) {
	path := flPath(t)
	lk := flMustLock(t, path)
	if err := lk.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	again := flMustLock(t, path)
	if err := again.Release(); err != nil {
		t.Fatalf("Release again: %v", err)
	}
}

// TestHeld: Held reports true while a lock is held and false after release, and its probe neither steals nor keeps the lock.
func TestHeld(t *testing.T) {
	path := flPath(t)
	lk := flMustLock(t, path)

	held, err := Held(path)
	if err != nil || !held {
		t.Fatalf("Held while locked = %v, %v; want true, nil", held, err)
	}
	// Held's probe must not steal or keep the lock: the holder still holds it.
	if _, err := TryLock(path, 0o600); !errors.Is(err, ErrLocked) {
		t.Fatalf("TryLock after Held probe err = %v, want ErrLocked", err)
	}

	if err := lk.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	held, err = Held(path)
	if err != nil || held {
		t.Fatalf("Held after release = %v, %v; want false, nil", held, err)
	}
	// The probe closed its handle, so the lock is free to take.
	after := flMustLock(t, path)
	_ = after.Release()
}

// TestHeldMissingDoesNotCreate: Held on a missing file (or one in a missing folder) reports false and creates nothing.
func TestHeldMissingDoesNotCreate(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join(dir, flLockName)},
		{"missing parent", filepath.Join(dir, "gone", flLockName)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			held, err := Held(tt.path)
			if err != nil || held {
				t.Fatalf("Held = %v, %v; want false, nil", held, err)
			}
			if _, err := os.Lstat(tt.path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Held created %s (Lstat err = %v)", tt.path, err)
			}
		})
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Held left %d entries in %s", len(entries), dir)
	}
}

// TestHeldOnDirectoryIsError: Held on a folder returns an error and never reports held.
func TestHeldOnDirectoryIsError(t *testing.T) {
	held, err := Held(t.TempDir())
	if err == nil {
		t.Fatalf("Held(dir) = %v, nil; want an error", held)
	}
	if held {
		t.Errorf("Held(dir) reported held alongside error %v", err)
	}
}

// TestTryLockOpenError: TryLock in a missing folder returns an open error that is not ErrLocked.
func TestTryLockOpenError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone", flLockName)
	lk, err := TryLock(path, 0o600)
	if err == nil {
		_ = lk.Release()
		t.Fatal("TryLock in a missing folder succeeded; want an error")
	}
	if errors.Is(err, ErrLocked) {
		t.Errorf("open failure reported as ErrLocked: %v", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want it to wrap os.ErrNotExist", err)
	}
}

// TestTryLockCreatesWithPermAndKeepsFile: TryLock creates the lock file with the given mode, and Release leaves the file in place.
func TestTryLockCreatesWithPermAndKeepsFile(t *testing.T) {
	path := flPath(t)
	lk := flMustLock(t, path)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
	if !fi.Mode().IsRegular() {
		t.Errorf("lock file mode %v, want a regular file", fi.Mode())
	}
	// Windows has no Unix permission bits; os.Stat reports 0666 there.
	if runtime.GOOS != "windows" {
		if got := fi.Mode().Perm(); got != 0o600 {
			t.Errorf("lock file perm %o, want 600", got)
		}
	}

	if err := lk.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	// Unlinking a lock file races with a waiter that already opened it, so
	// Release must leave the file in place.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Release removed the lock file: %v", err)
	}
}

// TestTryLockExistingFileKeepsContent: locking an existing file neither truncates nor rewrites its content.
func TestTryLockExistingFileKeepsContent(t *testing.T) {
	path := flPath(t)
	const content = "12345\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lk := flMustLock(t, path)
	if err := lk.Release(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("content after lock = %q, want %q (TryLock must not truncate)", got, content)
	}
}

// TestTryLockRaceOneWinner: many goroutines racing TryLock on one path produce exactly one winner; every other gets ErrLocked.
func TestTryLockRaceOneWinner(t *testing.T) {
	const n = 32
	path := flPath(t)

	var (
		start   sync.WaitGroup
		done    sync.WaitGroup
		mu      sync.Mutex
		winners []*Lock
		others  []error
	)
	start.Add(1)
	done.Add(n)
	for range n {
		go func() {
			defer done.Done()
			start.Wait() // release every goroutine at once to maximise overlap
			lk, err := TryLock(path, 0o600)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				winners = append(winners, lk)
				return
			}
			others = append(others, err)
		}()
	}
	start.Done()
	done.Wait()
	// Winners are released only after every goroutine finished, so a late
	// TryLock can never find the lock free and win a second time.
	for _, lk := range winners {
		_ = lk.Release()
	}

	if len(winners) != 1 {
		t.Fatalf("winners = %d, want exactly 1", len(winners))
	}
	for _, err := range others {
		if !errors.Is(err, ErrLocked) {
			t.Errorf("loser err = %v, want ErrLocked", err)
		}
	}
}
