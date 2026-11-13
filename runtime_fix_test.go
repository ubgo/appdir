package appdir

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bxApp is the app name every runtime fix test resolves.
const bxApp = "bxapp"

// bxEnv turns a map into a WithEnv lookup: the test never reads the process env.
func bxEnv(m map[string]string) Option {
	return WithEnv(func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	})
}

// bxSetup makes a temp root with a user home and a project folder holding
// .bxapp/, and returns root, home and project.
func bxSetup(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	home := filepath.Join(root, "home")
	proj := filepath.Join(root, "proj")
	for _, dir := range []string{home, filepath.Join(proj, "."+bxApp)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root, home, proj
}

// bxNew resolves bxapp as a dev build in proj with home and env.
func bxNew(t *testing.T, home, proj string, env map[string]string, extra ...Option) *Dirs {
	t.Helper()
	opts := append([]Option{
		WithHomeDir(func() (string, error) { return home, nil }),
		WithWorkDir(proj),
		bxEnv(env),
		WithRelease(false),
	}, extra...)
	d, err := New(bxApp, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

// bxWrite writes a file, creating its folder.
func bxWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// bxProdHome creates the prod global home so its db's folder exists.
func bxProdHome(t *testing.T, home string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "."+bxApp), 0o700); err != nil {
		t.Fatal(err)
	}
}

// F4: a prod db: pointer in config.local.jsonc written with ${VAR} must be
// recognised by the dev build's guard: the prod view strips only <APP>_*.
func TestFixF4_ProdDBPointerWithEnvVar(t *testing.T) {
	root, home, proj := bxSetup(t)
	dbDir := filepath.Join(root, "dbs")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		t.Fatal(err)
	}
	bxWrite(t, filepath.Join(proj, "."+bxApp, "config.local.jsonc"), `{"db": "${BX_DBROOT}/prod.db"}`)
	env := map[string]string{"BX_DBROOT": dbDir}
	d := bxNew(t, home, proj, env)
	_, err := d.CheckDB(filepath.Join(dbDir, "prod.db"))
	if !errors.Is(err, ErrProdDB) {
		t.Fatalf("CheckDB(prod pointer db) = %v, want ErrProdDB", err)
	}
	// With ALLOW_PROD the same db opens read-only.
	env["BXAPP_ALLOW_PROD"] = "1"
	d = bxNew(t, home, proj, env)
	acc, err := d.CheckDB(filepath.Join(dbDir, "prod.db"))
	if err != nil || acc != AccessReadOnly {
		t.Fatalf("CheckDB with ALLOW_PROD = %v, %v; want read-only, nil", acc, err)
	}
}

// F4: <APP>_* variables are still stripped from the prod view: BXAPP_HOME
// pointing elsewhere does not make that folder's db the "prod" db.
func TestFixF4_ProdViewStripsAppVars(t *testing.T) {
	root, home, proj := bxSetup(t)
	bxProdHome(t, home)
	other := filepath.Join(root, "other")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	d := bxNew(t, home, proj, map[string]string{"BXAPP_HOME": other})
	if _, err := d.CheckDB(filepath.Join(other, bxApp+".db")); err != nil {
		t.Fatalf("CheckDB(db under BXAPP_HOME) = %v, want nil (not prod)", err)
	}
	if _, err := d.CheckDB(filepath.Join(home, "."+bxApp, bxApp+".db")); !errors.Is(err, ErrProdDB) {
		t.Fatalf("CheckDB(global prod db) = %v, want ErrProdDB", err)
	}
}

// F4: a broken prod project view (pointer to an unset ${VAR}) must not stop
// the guard from checking the global prod db.
func TestFixF4_GlobalCheckedWhenProjectViewErrors(t *testing.T) {
	_, home, proj := bxSetup(t)
	bxProdHome(t, home)
	bxWrite(t, filepath.Join(proj, "."+bxApp, "config.local.jsonc"), `{"db": "${BX_UNSET}/prod.db"}`)
	d := bxNew(t, home, proj, map[string]string{})
	if _, err := d.CheckDB(filepath.Join(home, "."+bxApp, bxApp+".db")); !errors.Is(err, ErrProdDB) {
		t.Fatalf("CheckDB(global prod db, broken project) = %v, want ErrProdDB", err)
	}
}

// F8/F20: EnsureSocketDir creates run/ in the normal case (the daemon writes
// run/socket, the pid and discovery there).
func TestFixF8_EnsureSocketDirCreatesRun(t *testing.T) {
	_, home, proj := bxSetup(t)
	// A large max length keeps the socket under run/ even in a deep temp
	// root, so nothing is created outside it.
	d := bxNew(t, home, proj, map[string]string{}, WithSocketMaxLen(4096))
	s, err := d.EnsureSocketDir()
	if err != nil {
		t.Fatalf("EnsureSocketDir: %v", err)
	}
	if !isDir(filepath.Dir(s.PointerFile)) {
		t.Fatalf("run/ %s not created", filepath.Dir(s.PointerFile))
	}
	if s.Dir != "" && !isDir(s.Dir) {
		t.Fatalf("socket folder %s not created", s.Dir)
	}
}

// F8/F20: on a Unix fallback EnsureSocketDir creates both the private
// fallback folder and run/, where the daemon writes run/socket.
func TestFixF8_EnsureSocketDirFallbackCreatesRun(t *testing.T) {
	root, home, proj := bxSetup(t)
	xdg := filepath.Join(root, "xdg-run")
	if err := os.MkdirAll(xdg, 0o700); err != nil {
		t.Fatal(err)
	}
	d := bxNew(t, home, proj, map[string]string{"XDG_RUNTIME_DIR": xdg}, WithSocketMaxLen(10))
	if s, err := d.Socket(); err != nil || (s.Dir != "" && !strings.HasPrefix(s.Dir, root)) {
		t.Skipf("fallback folder %q is outside the temp root on this OS (%v); not creating it", s.Dir, err)
	}
	s, err := d.EnsureSocketDir()
	if err != nil {
		t.Fatalf("EnsureSocketDir: %v", err)
	}
	if !s.Fallback {
		t.Skip("socket did not fall back on this platform") // a Windows pipe is always Fallback; this guards a surprise
	}
	run := filepath.Dir(s.PointerFile)
	if !isDir(run) {
		t.Fatalf("run/ %s not created on the fallback path", run)
	}
	if s.Dir != "" && !isDir(s.Dir) {
		t.Fatalf("fallback folder %s not created", s.Dir)
	}
}

// F12: WithGitIgnore equal to a committed kind removes its re-include and is
// not also written as a plain extra line (`*` already ignores it).
func TestFixF12_GitIgnoreEqualToCommittedKind(t *testing.T) {
	_, home, proj := bxSetup(t)
	for _, pat := range []string{"shared", "shared/", "config.jsonc"} {
		d := bxNew(t, home, proj, map[string]string{}, WithGitIgnore(pat))
		block := d.GitignoreBlock()
		for _, l := range strings.Split(block, "\n") {
			if l == pat || l == strings.TrimSuffix(pat, "/") || l == "!"+strings.TrimSuffix(pat, "/") || strings.HasPrefix(l, "!"+strings.TrimSuffix(pat, "/")+"/") {
				t.Errorf("WithGitIgnore(%q): block still has line %q:\n%s", pat, l, block)
			}
		}
	}
	// A pattern that is not a commit entry is still written as a plain line.
	d := bxNew(t, home, proj, map[string]string{}, WithGitIgnore("shared/secret.txt"))
	if !strings.Contains(d.GitignoreBlock(), "\nshared/secret.txt\n") {
		t.Errorf("plain re-exclude missing:\n%s", d.GitignoreBlock())
	}
}

// BX1 (found on the way): CheckDB must recognise the prod global db before
// the prod home exists. sameDBPath compared parent folders by identity only,
// so with no ~/.bxapp yet a dev build could create and migrate the prod db.
func TestFixBX1_ProdDBBeforeProdHomeExists(t *testing.T) {
	_, home, proj := bxSetup(t)
	d := bxNew(t, home, proj, map[string]string{})
	if _, err := d.CheckDB(filepath.Join(home, "."+bxApp, bxApp+".db")); !errors.Is(err, ErrProdDB) {
		t.Fatalf("CheckDB(prod db, prod home missing) = %v, want ErrProdDB", err)
	}
	if _, err := d.CheckDB(filepath.Join(home, "."+bxApp+"-dev", bxApp+".db")); err != nil {
		t.Fatalf("CheckDB(dev db, homes missing) = %v, want nil", err)
	}
}

// BX2 (found on the way): a crash in a cross-disk legacy move after the old
// folder was renamed to <old>.moved-<date> but before .legacy-move was
// removed left the marker in the new home forever (MoveLegacy saw no old
// folder and returned MoveNone), and MOVED.txt was never written.
func TestFixBX2_LegacyFinishesAfterCrashPastOldRename(t *testing.T) {
	_, home, proj := bxSetup(t)
	from := filepath.Join(home, ".bxold")
	to := filepath.Join(home, "."+bxApp)
	bxWrite(t, filepath.Join(from+".moved-20261110", "data.txt"), "x")
	bxWrite(t, filepath.Join(to, "data.txt"), "x")
	bxWrite(t, filepath.Join(to, legacyMarkerFile), from+"\n")
	d, err := New(bxApp,
		WithHomeDir(func() (string, error) { return home, nil }),
		WithWorkDir(proj), bxEnv(map[string]string{}), WithRelease(true),
		WithLegacyHome(LegacyHome{Dir: ".bxold"}))
	if err != nil {
		t.Fatal(err)
	}
	res, err := d.MoveLegacy(t.Context())
	if err != nil || res.Status != MoveDone {
		t.Fatalf("MoveLegacy = %+v, %v; want %s", res, err, MoveDone)
	}
	if exists(filepath.Join(to, legacyMarkerFile)) {
		t.Errorf("marker still in %s", to)
	}
	if !onlyMovedNote(from) {
		t.Errorf("%s does not hold only %s", from, movedFile)
	}
	// A marker naming another old folder is not ours to finish.
	bxWrite(t, filepath.Join(to, legacyMarkerFile), filepath.Join(home, ".elsewhere")+"\n")
	if err := os.RemoveAll(from); err != nil {
		t.Fatal(err)
	}
	if res, err := d.MoveLegacy(t.Context()); err != nil || res.Status != MoveNone {
		t.Fatalf("MoveLegacy with a foreign marker = %+v, %v; want %s", res, err, MoveNone)
	}
}

// BX3 (found on the way): homeOf treated any relative path starting with
// ".." as outside, so a file under a folder named "..x" in the home got the
// wrong home and a valid in-home symlink was refused.
func TestFixBX3_HomeOfDotDotNamedFolder(t *testing.T) {
	_, home, proj := bxSetup(t)
	ghome := filepath.Join(home, "."+bxApp+"-dev")
	bxWrite(t, filepath.Join(ghome, "real.txt"), "x")
	link := filepath.Join(ghome, "..x", "f")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(ghome, "real.txt"), link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	d := bxNew(t, home, proj, map[string]string{})
	if got := d.homeOf(link); got != d.global.Base && got != d.global.Data {
		t.Errorf("homeOf(%s) = %s, want the global home %s", link, got, d.global.Base)
	}
	if err := d.VerifyFile(link); err != nil {
		t.Errorf("VerifyFile(in-home link under ..x) = %v, want nil", err)
	}
}
