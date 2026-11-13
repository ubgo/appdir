package appdir

// Cross-disk legacy move on a real second filesystem.
//
// MoveLegacy has two paths: a plain os.Rename when the old home and the new
// home's parent share a device, and copy + verify + marker + swap when they do
// not. Every other legacy test runs inside one t.TempDir(), so only the rename
// path ever runs for real; the copy path (copyTree, countTree, the
// .legacy-move marker and its crash recovery) needs two filesystems. A test
// cannot mount one without root, so the test is gated by an env var naming a
// folder that is on another device than t.TempDir(); CI provides it (a tmpfs
// on Linux, a RAM disk on macOS, D:\ on Windows). It never runs by accident
// on a laptop.
//
// Layout: the old home and the new home are both children of the user's
// home (legacy.go joins userHome with LegacyHome.Dir, and the new home is
// userHome/.<app>), so the only way to split them across disks without root
// is the one users actually do: the user home lives on the second disk and
// the old dotfolder is a symlink to a folder on the first disk (the "I moved
// ~/.old to my big drive" case). The new home is then created on the second
// disk, the old data sits on the first.

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const (
	// xdEnvSecondDisk names a folder on a filesystem other than the one
	// holding t.TempDir(). Unset means skip.
	xdEnvSecondDisk = "APPDIR_TEST_SECOND_DISK"
	// xdApp is the app under test; its new home is ~/.xdapp.
	xdApp = "xdapp"
	// xdOld is the legacy home folder name under the user's home.
	xdOld = ".xdold"
	// xdRunPrefix prefixes the per-run folder made under the second disk, so
	// parallel or leftover runs never collide and cleanup removes only ours.
	xdRunPrefix = "appdir-xdisk-"
	// xdBigSize is the size of the one large file: big enough that io.Copy
	// loops several times, small enough for a 64 MB tmpfs or RAM disk.
	xdBigSize = 3<<20 + 17
	// xdStray is a file planted in the half-copied temp folder of a crashed
	// run; the finished move must not contain it.
	xdStray = "stray-from-crashed-copy.txt"
)

// xdEntry is one item of the legacy tree: a folder (body nil, not link), a
// file, or a relative symlink (link set).
type xdEntry struct {
	rel  string // slash-separated path below the home
	mode fs.FileMode
	body []byte
	link string
}

// xdTree is the legacy home the tests move: folders with non-default modes,
// files with distinct modes (private, group-readable, executable), an empty
// folder and file, a binary file, a large file and a relative symlink.
func xdTree() []xdEntry {
	big := make([]byte, xdBigSize)
	for i := range big {
		big[i] = byte(i*7 + i>>9)
	}
	bin := []byte{0, 1, 2, 0xff, 0xfe, '\n', '\r', 0}
	return []xdEntry{
		{rel: "config.jsonc", mode: 0o600, body: []byte(`{"layout": 1}` + "\n")},
		{rel: "xdapp.db", mode: 0o600, body: big},
		{rel: "empty.txt", mode: 0o644, body: []byte{}},
		{rel: "bin/hook.sh", mode: 0o755, body: []byte("#!/bin/sh\necho hi\n")},
		{rel: "bin", mode: 0o750},
		{rel: "logs", mode: 0o700},
		{rel: "logs/2026/11/app.log", mode: 0o640, body: []byte("line 1\nline 2\n")},
		{rel: "logs/2026/11/blob.bin", mode: 0o600, body: bin},
		{rel: "run", mode: 0o700}, // default daemon lock folder, lock file absent
		{rel: "empty-dir", mode: 0o750},
		{rel: "current.log", link: "logs/2026/11/app.log"},
	}
}

// xdSecondDisk returns the second-disk folder or skips the test.
func xdSecondDisk(t *testing.T) string {
	t.Helper()
	p := os.Getenv(xdEnvSecondDisk)
	if p == "" {
		t.Skipf("set %s=<folder on another filesystem than %s> to run the cross-disk legacy move test", xdEnvSecondDisk, os.TempDir())
	}
	return p
}

// xdWorld is one test's layout: home on the second disk, old data on the
// first, ~/.xdold a symlink to it.
type xdWorld struct {
	home    string // user home, on the second disk
	oldReal string // the old home's real folder, on the first disk
	from    string // ~/.xdold, the symlink MoveLegacy sees
	to      string // ~/.xdapp, the new home
	work    string // working folder for New, holds no project
}

// xdSetup builds the world and checks it really spans two devices. A missing
// second device is a fatal error, not a skip: the env var promised one.
func xdSetup(t *testing.T) xdWorld {
	t.Helper()
	disk := xdSecondDisk(t)
	if err := os.MkdirAll(disk, 0o700); err != nil {
		t.Fatalf("%s=%s: %v", xdEnvSecondDisk, disk, err)
	}
	run, err := os.MkdirTemp(disk, xdRunPrefix+"*")
	if err != nil {
		t.Fatalf("%s=%s: %v", xdEnvSecondDisk, disk, err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(run); err != nil {
			t.Errorf("cleanup %s: %v", run, err)
		}
	})
	first := rxRoot(t)
	w := xdWorld{
		home:    filepath.Join(run, "home"),
		oldReal: filepath.Join(first, "olddisk", "old-home"),
		work:    filepath.Join(first, "work"),
	}
	w.from = filepath.Join(w.home, xdOld)
	w.to = filepath.Join(w.home, "."+xdApp)
	rxMkdir(t, w.home, w.oldReal, w.work)

	d1, ok1 := deviceID(w.oldReal)
	d2, ok2 := deviceID(w.home)
	if !ok1 || !ok2 || d1 == d2 {
		t.Fatalf("%s=%s is not on another filesystem than %s: device %d (ok %v) vs %d (ok %v)", xdEnvSecondDisk, disk, first, d2, ok2, d1, ok1)
	}
	t.Logf("first disk %s device %d; second disk %s device %d", first, d1, run, d2)

	xdBuild(t, w.oldReal)
	if err := os.Symlink(w.oldReal, w.from); err != nil {
		// Windows without the symlink privilege; GitHub runners have it.
		t.Fatalf("symlink %s -> %s: %v", w.from, w.oldReal, err)
	}
	if sameDevice(w.from, w.home) {
		t.Fatalf("sameDevice(%s, %s) = true; MoveLegacy would rename, not copy", w.from, w.home)
	}
	return w
}

// xdBuild writes the legacy tree under root, setting modes explicitly so the
// umask cannot hide a mode the copy failed to keep.
func xdBuild(t *testing.T, root string) {
	t.Helper()
	tree := xdTree()
	for _, e := range tree {
		p := filepath.Join(root, filepath.FromSlash(e.rel))
		switch {
		case e.link != "":
			rxMkdir(t, filepath.Dir(p))
			if err := os.Symlink(filepath.FromSlash(e.link), p); err != nil {
				t.Fatalf("symlink %s: %v", p, err)
			}
		case e.body == nil:
			rxMkdir(t, p)
		default:
			rxMkdir(t, filepath.Dir(p))
			if err := os.WriteFile(p, e.body, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Modes last, deepest first is not needed: every mode keeps u+rwx on
	// folders, so later writes inside them still work.
	for _, e := range tree {
		if e.link == "" {
			if err := os.Chmod(filepath.Join(root, filepath.FromSlash(e.rel)), e.mode); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// xdDirs resolves a release build that knows the legacy home. The seams keep
// the process env and the real home out. The test guard is off because the
// second disk is outside the temp root by design: that is the point.
func xdDirs(t *testing.T, w xdWorld, extra ...Option) *Dirs {
	t.Helper()
	opts := append([]Option{
		rxHome(w.home), WithWorkDir(w.work), rxEnv(map[string]string{}),
		WithRelease(true), WithMode(ModeGlobal), WithTestGuard(false),
		WithLegacyHome(LegacyHome{Dir: xdOld}),
	}, extra...)
	d, err := New(xdApp, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if d.Global().Base != w.to {
		t.Fatalf("global home %s, want %s", d.Global().Base, w.to)
	}
	return d
}

// xdListing walks root and returns rel -> description (kind, mode where the
// OS has modes, content or link target), the marker excluded.
func xdListing(t *testing.T, root string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." || rel == legacyMarkerFile {
			return err
		}
		info, err := de.Info()
		if err != nil {
			return err
		}
		got[filepath.ToSlash(rel)] = xdDescribe(t, p, info)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return got
}

// xdWant is xdListing of the tree as built.
func xdWant(t *testing.T) map[string]string {
	t.Helper()
	want := map[string]string{}
	for _, e := range xdTree() {
		switch {
		case e.link != "":
			want[e.rel] = "link " + filepath.FromSlash(e.link)
		case e.body == nil:
			want[e.rel] = "dir" + xdMode(e.mode)
		default:
			want[e.rel] = "file" + xdMode(e.mode) + " " + xdSum(e.body)
		}
	}
	// Parents that the tree names only through children.
	for _, rel := range []string{"logs/2026", "logs/2026/11"} {
		want[rel] = "dir" + xdMode(0o700)
	}
	return want
}

// xdDescribe describes one walked entry like xdWant does.
func xdDescribe(t *testing.T, p string, info fs.FileInfo) string {
	t.Helper()
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(p)
		if err != nil {
			t.Fatal(err)
		}
		return "link " + target
	case info.IsDir():
		return "dir" + xdMode(info.Mode().Perm())
	default:
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return "file" + xdMode(info.Mode().Perm()) + " " + xdSum(b)
	}
}

// xdMode renders a mode for comparison. Windows has no Unix modes (the
// library restricts with an ACL only for folders and files it creates, not
// for a copy, and exposes no ACL check), so there only content is compared.
func xdMode(m fs.FileMode) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	return " " + m.Perm().String()
}

// xdSum is a short content fingerprint (length and FNV-1a), so a diff
// stays readable for the 3 MB file.
func xdSum(b []byte) string {
	h := fnv.New32a()
	_, _ = h.Write(b) // hash.Hash never returns an error
	return fmt.Sprintf("len=%d fnv=%08x", len(b), h.Sum32())
}

// xdSameTree fails with a sorted diff when got differs from want.
func xdSameTree(t *testing.T, where string, got, want map[string]string) {
	t.Helper()
	keys := map[string]bool{}
	for k := range got {
		keys[k] = true
	}
	for k := range want {
		keys[k] = true
	}
	var diff []string
	for k := range keys {
		if got[k] != want[k] {
			diff = append(diff, k+": got "+xdQ(got[k])+", want "+xdQ(want[k]))
		}
	}
	sort.Strings(diff)
	if len(diff) > 0 {
		t.Errorf("%s differs from the legacy tree:\n  %s", where, strings.Join(diff, "\n  "))
	}
}

// xdQ marks a missing entry clearly in a diff.
func xdQ(s string) string {
	if s == "" {
		return "<missing>"
	}
	return "[" + s + "]"
}

// xdCheckDone checks the state after a finished cross-disk move: the new
// home is a real folder on the second disk with every file, folder, mode and
// byte of the old home and no marker; ~/.xdold holds only MOVED.txt; the old
// data is still on the first disk, untouched (MoveLegacy never deletes).
func xdCheckDone(t *testing.T, w xdWorld, res MoveResult) {
	t.Helper()
	if res.Status != MoveDone || !strings.Contains(res.Note, "copied") {
		t.Errorf("MoveLegacy = %+v; want %s via the copy path", res, MoveDone)
	}
	fi, err := os.Lstat(w.to)
	if err != nil {
		t.Fatalf("new home %s: %v", w.to, err)
	}
	if !fi.IsDir() {
		// A symlink here means the copy path copied ~/.xdold as a link
		// instead of the tree behind it: the data never left the first disk.
		t.Fatalf("new home %s is not a real folder (mode %v)", w.to, fi.Mode())
	}
	if dTo, _ := deviceID(w.to); func() bool { dHome, _ := deviceID(w.home); return dTo != dHome }() {
		t.Errorf("new home %s is not on the second disk", w.to)
	}
	if exists(filepath.Join(w.to, legacyMarkerFile)) {
		t.Errorf("%s left in the new home", legacyMarkerFile)
	}
	if exists(w.to + ".moving-tmp") {
		t.Errorf("%s.moving-tmp left behind", w.to)
	}
	want := xdWant(t)
	xdSameTree(t, "new home", xdListing(t, w.to), want)
	xdSameTree(t, "old data on the first disk", xdListing(t, w.oldReal), want)
	if !onlyMovedNote(w.from) {
		t.Errorf("%s does not hold only %s", w.from, movedFile)
	}
	if b, err := os.ReadFile(filepath.Join(w.from, movedFile)); err != nil || !bytes.Contains(b, []byte(w.to)) {
		t.Errorf("%s = %q, %v; want it to name %s", movedFile, b, err, w.to)
	}
	kept, _ := filepath.Glob(w.from + ".moved-*")
	if len(kept) != 1 {
		t.Errorf("old home kept as %v; want exactly one %s.moved-<date>", kept, w.from)
	}
	// A second start is a no-op: the move is done.
	if res, err := xdDirs(t, w).MoveLegacy(context.Background()); err != nil || res.Status != MoveAlready {
		t.Errorf("MoveLegacy again = %+v, %v; want %s", res, err, MoveAlready)
	}
}

// TestLegacyXDisk_Move moves a legacy home from one filesystem to another and
// checks every file, folder, mode and byte arrived.
func TestLegacyXDisk_Move(t *testing.T) {
	w := xdSetup(t)
	res, err := xdDirs(t, w).MoveLegacy(context.Background())
	if err != nil {
		t.Fatalf("MoveLegacy: %v", err)
	}
	xdCheckDone(t, w, res)
}

// TestLegacyXDisk_CrashRecovery simulates a crash at each step of the copy
// path, by leaving exactly what legacy.go leaves at that point, and checks
// the next start finishes the move with nothing lost.
func TestLegacyXDisk_CrashRecovery(t *testing.T) {
	// The library's own copy of the old tree into dst, as moveOne leaves it
	// (the tree behind the ~/.xdold link, not the link itself).
	copyOld := func(t *testing.T, w xdWorld, dst string) {
		t.Helper()
		if _, _, err := copyTree(context.Background(), w.oldReal, dst); err != nil {
			t.Fatalf("copyTree: %v", err)
		}
	}
	writeMarker := func(t *testing.T, dir, from string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, legacyMarkerFile), []byte(from+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Crash during the copy: a half-written <new>.moving-tmp with a truncated
	// file, a missing file and a stray file, no marker. The next run must
	// throw the partial copy away and copy again from scratch.
	t.Run("during-copy", func(t *testing.T) {
		w := xdSetup(t)
		tmp := w.to + ".moving-tmp"
		rxWrite(t, filepath.Join(tmp, "config.jsonc"), `{"lay`)
		rxWrite(t, filepath.Join(tmp, xdStray), "half")
		res, err := xdDirs(t, w).MoveLegacy(context.Background())
		if err != nil {
			t.Fatalf("MoveLegacy: %v", err)
		}
		xdCheckDone(t, w, res)
	})

	// Verify failed (the LegacyVerify hook stands in for a crash right after
	// the copy, before the marker): the old home must be left as is and the
	// next run, with a passing verify, must finish.
	t.Run("after-copy-before-marker", func(t *testing.T) {
		w := xdSetup(t)
		failVerify := WithLegacyVerify(func(from, tmp string) error { return os.ErrDeadlineExceeded })
		if _, err := xdDirs(t, w, failVerify).MoveLegacy(context.Background()); err == nil {
			t.Fatal("MoveLegacy with a failing verify = nil error; want the verify error")
		}
		if isDir(w.to) {
			t.Fatalf("new home %s exists after a failed verify", w.to)
		}
		xdSameTree(t, "old home after failed verify", xdListing(t, w.from+string(filepath.Separator)), xdWant(t))
		res, err := xdDirs(t, w).MoveLegacy(context.Background())
		if err != nil {
			t.Fatalf("MoveLegacy: %v", err)
		}
		xdCheckDone(t, w, res)
	})

	// Crash after <new>.moving-tmp was renamed to the new home with its
	// marker, before the old home was renamed: the marker makes the next run
	// finish the old-home steps without copying again.
	t.Run("after-swap", func(t *testing.T) {
		w := xdSetup(t)
		copyOld(t, w, w.to)
		writeMarker(t, w.to, w.from)
		res, err := xdDirs(t, w).MoveLegacy(context.Background())
		if err != nil {
			t.Fatalf("MoveLegacy: %v", err)
		}
		xdCheckDone(t, w, res)
	})

	// Crash after the old home was renamed to <old>.moved-<date>, before the
	// marker was removed (BX2): the old path is gone, only the marker says a
	// move from it is unfinished.
	t.Run("after-old-rename", func(t *testing.T) {
		w := xdSetup(t)
		copyOld(t, w, w.to)
		writeMarker(t, w.to, w.from)
		if err := os.Rename(w.from, w.from+".moved-20261110"); err != nil {
			t.Fatal(err)
		}
		res, err := xdDirs(t, w).MoveLegacy(context.Background())
		if err != nil {
			t.Fatalf("MoveLegacy: %v", err)
		}
		xdCheckDone(t, w, res)
	})
}
