package appdir

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFix2L1_GuardDefaultRootRefusesSocketFallback: with TMPDIR and
// APPDIR_TEST_ROOT both unset the guard's default root is os.TempDir(), which
// on Unix is /tmp itself and so contains the shared /tmp/<app>-<uid> socket
// fallback. That folder belongs to the real user (a running daemon's socket
// lives there), so Socket's guard must still refuse it. Only Socket() is
// called: it panics before anything is created.
func TestFix2L1_GuardDefaultRootRefusesSocketFallback(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("the /tmp socket fallback is Unix only")
	}
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	d, err := New(cxApp, rxHome(home), WithWorkDir(home), rxEnv(nil),
		WithRelease(true), WithMode(ModeGlobal), WithSocketMaxLen(1))
	if err != nil {
		t.Fatal(err)
	}
	// Unset only now: New resolved under the caller's test root, and the
	// socket fallback is resolved (and guarded) inside Socket().
	for _, k := range []string{"TMPDIR", envTestRoot} {
		t.Setenv(k, "") // registers the restore
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Socket(): no panic for the shared /tmp socket fallback under the default test root")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "outside the test root") {
			t.Fatalf("Socket(): panic %v", r)
		}
	}()
	s, _ := d.Socket()
	t.Logf("got %+v", s)
}

// TestFix2D6_InitBrokenHomeFile: InitProject over a home.jsonc that does not
// parse matches ErrBadHomeFile with errors.Is, names the file exactly once,
// and leaves it untouched. An unreadable file (permission) keeps its cause
// reachable too, so errors.Is(err, fs.ErrPermission) still works.
func TestFix2D6_InitBrokenHomeFile(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	proj := filepath.Join(root, "proj")
	marker := filepath.Join(proj, "."+cxApp)
	file := filepath.Join(marker, "home.jsonc")
	rxMkdir(t, home, marker)
	const broken = "{ \"app\": \"cxapp\", \n"
	rxWrite(t, file, broken)
	d, err := New(cxApp, rxHome(home), WithWorkDir(proj), rxEnv(nil), WithRelease(true))
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.InitProject(proj)
	if !errors.Is(err, ErrBadHomeFile) {
		t.Fatalf("InitProject err = %v; want ErrBadHomeFile", err)
	}
	if n := strings.Count(err.Error(), file); n != 1 {
		t.Fatalf("error names %s %d times, want once: %v", file, n, err)
	}
	if n := strings.Count(err.Error(), marker); n != 1 {
		t.Fatalf("error names the marker %d times, want once: %v", n, err)
	}
	if b, rerr := os.ReadFile(file); rerr != nil || string(b) != broken {
		t.Fatalf("home.jsonc changed: %q, %v", b, rerr)
	}

	// A reader error that is not a parse error keeps its cause.
	perm := WithConfigReader(func(p string) (map[string]any, error) {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrPermission}
	})
	d, err = New(cxApp, rxHome(home), WithWorkDir(proj), rxEnv(nil), WithRelease(true), perm)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.InitProject(proj)
	if !errors.Is(err, ErrBadHomeFile) || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("InitProject err = %v; want ErrBadHomeFile and fs.ErrPermission", err)
	}
	if n := strings.Count(err.Error(), file); n != 1 {
		t.Fatalf("error names %s %d times, want once: %v", file, n, err)
	}
}

// TestFix2D5_DroppedCeilingWarns: a ceiling entry that cannot become an
// absolute path ($VAR without braces, an unset ${VAR}, ~user) used to be
// dropped with no word; it now gets warning ceiling-dropped naming the entry,
// on the same channel as ceiling-relative. A good entry next to it still
// works and gets no warning.
func TestFix2D5_DroppedCeilingWarns(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	proj := filepath.Join(root, "srv", "proj")
	deep := filepath.Join(proj, "a", "b")
	rxMkdir(t, home, deep, filepath.Join(proj, "."+cxApp))
	sep := string(os.PathListSeparator)
	for _, entry := range []string{"$ROOT/srv", "${NOPE}/srv", "~nosuchuser/srv"} {
		t.Run(entry, func(t *testing.T) {
			env := map[string]string{"CXAPP_CEILING_DIRS": entry + sep + filepath.Join(proj, "a"), "ROOT": root}
			d, err := New(cxApp, rxHome(home), WithWorkDir(deep), rxEnv(env), WithRelease(true))
			if err != nil {
				t.Fatal(err)
			}
			var got []Warning
			for _, w := range d.Warnings() {
				if w.Code == WarnCeilingDropped {
					got = append(got, w)
				}
			}
			if len(got) != 1 || !strings.Contains(got[0].Message, entry) || !strings.Contains(got[0].Message, "CXAPP_CEILING_DIRS") {
				t.Fatalf("warnings = %+v; want one %s naming %q", d.Warnings(), WarnCeilingDropped, entry)
			}
			// The good entry still applies: the walk stops at proj/a.
			if _, err := d.Project(); !errors.Is(err, ErrNoProject) {
				t.Fatalf("Project() err = %v; want ErrNoProject (the good ceiling must still apply)", err)
			}
		})
	}
}

// TestFix2D5_RelativeVarCeilingWarns: a ${VAR} that is set but relative
// makes the entry relative after expansion; it is dropped with
// ceiling-dropped and the message shows what it expanded to.
func TestFix2D5_RelativeVarCeilingWarns(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	env := map[string]string{"CXAPP_CEILING_DIRS": "${REL}/x", "REL": "rel"}
	d, err := New(cxApp, rxHome(home), WithWorkDir(root), rxEnv(env), WithRelease(true))
	if err != nil {
		t.Fatal(err)
	}
	ws := d.Warnings()
	if len(ws) != 1 || ws[0].Code != WarnCeilingDropped || !strings.Contains(ws[0].Message, `expands to "rel`) {
		t.Fatalf("warnings = %+v; want one ceiling-dropped showing the expansion", ws)
	}
}

// TestFix2D3_PointerWithoutHomeKind: with WithoutKind(KindHome) there is no
// home.jsonc to read, so a home: pointer target is recognised the way the
// lookup recognises a project home without it: by its folder name .<app>.
// A target made by InitHome under that name is followed; a random folder
// (any other name) is still refused with ErrPointerNotHome; a missing
// target is still ErrPointerMissing.
func TestFix2D3_PointerWithoutHomeKind(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	shop := filepath.Join(root, "work", "shop")
	marker := filepath.Join(shop, "."+cxApp)
	tgt := filepath.Join(root, "work", "shop-private", "."+cxApp)
	random := filepath.Join(root, "work", "random")
	rxMkdir(t, home, marker, random)
	noHome := WithoutKind(KindHome)
	open := func(t *testing.T, pointer string) (*Dirs, Home, error) {
		t.Helper()
		rxWrite(t, filepath.Join(marker, "config.local.jsonc"), `{"home": "`+pointer+`"}`)
		d, err := New(cxApp, rxHome(home), WithWorkDir(shop), rxEnv(nil), WithRelease(true), noHome)
		if err != nil {
			t.Fatal(err)
		}
		p, err := d.Project()
		return d, p, err
	}

	t.Run("target made by InitHome under .<app> is followed", func(t *testing.T) {
		d, err := New(cxApp, rxHome(home), WithWorkDir(shop), rxEnv(nil), WithRelease(true), noHome)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.InitHome(tgt, ScopeProject); err != nil {
			t.Fatal(err)
		}
		_, p, err := open(t, "../../shop-private/."+cxApp)
		if err != nil || p.Base != tgt || p.Rule != RulePointer {
			t.Fatalf("Project() = %+v, %v; want the pointer target %s", p, err, tgt)
		}
	})
	t.Run("random folder is refused", func(t *testing.T) {
		_, _, err := open(t, "../../random")
		if !errors.Is(err, ErrPointerNotHome) {
			t.Fatalf("Project() err = %v; want ErrPointerNotHome", err)
		}
	})
	t.Run("missing target is ErrPointerMissing", func(t *testing.T) {
		_, _, err := open(t, "../../nope/."+cxApp)
		if !errors.Is(err, ErrPointerMissing) {
			t.Fatalf("Project() err = %v; want ErrPointerMissing", err)
		}
	})
}
