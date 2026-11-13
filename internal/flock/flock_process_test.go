package flock

// The in-process tests stand in one handle for "another process". This file
// checks the real thing on every OS: the test binary re-runs itself as a
// second process that holds the lock (flock on Unix, LockFileEx on Windows),
// and the parent must see ErrLocked until that process lets go.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// flHelperEnv names the lock path the re-run test binary must hold.
const flHelperEnv = "FLOCK_TEST_HOLD_PATH"

// flHeldLine is what the helper prints once it holds the lock.
const flHeldLine = "held"

// TestHelperHoldLock is not a test: it runs only inside the child process
// started by TestTryLockAcrossProcesses. It takes the lock, says so on
// stdout, and keeps it until its stdin closes.
func TestHelperHoldLock(t *testing.T) {
	path := os.Getenv(flHelperEnv)
	if path == "" {
		t.Skip("helper process only")
	}
	lk, err := TryLock(path, 0o600)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(2)
	}
	fmt.Println(flHeldLine)
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n') // returns when the parent closes stdin
	_ = lk.Release()
	os.Exit(0)
}

// TestTryLockAcrossProcesses: a lock held by another process makes TryLock
// fail with ErrLocked and Held report true; once that process exits the
// kernel frees the lock and TryLock succeeds.
func TestTryLockAcrossProcesses(t *testing.T) {
	path := flPath(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperHoldLock$", "-test.count=1")
	cmd.Env = append(os.Environ(), flHelperEnv+"="+path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := false
	defer func() {
		if !done {
			_ = stdin.Close()
			_ = cmd.Wait()
		}
	}()

	line := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(stdout).ReadString('\n')
		line <- strings.TrimSpace(s)
	}()
	select {
	case s := <-line:
		if s != flHeldLine {
			t.Fatalf("helper said %q, want %q", s, flHeldLine)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("helper process never took the lock")
	}

	if lk, err := TryLock(path, 0o600); !errors.Is(err, ErrLocked) {
		if lk != nil {
			_ = lk.Release()
		}
		t.Fatalf("TryLock while another process holds it: err = %v, want ErrLocked", err)
	}
	if held, err := Held(path); err != nil || !held {
		t.Fatalf("Held while another process holds it = %v, %v; want true", held, err)
	}

	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper process: %v", err)
	}
	done = true

	if held, err := Held(path); err != nil || held {
		t.Fatalf("Held after the other process exited = %v, %v; want false", held, err)
	}
	lk, err := TryLock(path, 0o600)
	if err != nil {
		t.Fatalf("TryLock after the other process exited: %v", err)
	}
	_ = lk.Release()
}
