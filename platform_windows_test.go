//go:build windows

package appdir

// platform_windows_test.go: what only a real Windows machine can show. The
// owner-only ACL appdir puts on what it creates, MoveFileEx replacing an
// existing file, drive letters in a different case and UNC paths in the
// resolver, USERPROFILE as the user's home and the named-pipe shape. Every
// helper here starts with "pw".

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

const pwApp = "pwapp"

// pwOpts are the seams every New here gets.
func pwOpts(home, work string, extra ...Option) []Option {
	base := []Option{rxHome(home), WithWorkDir(work), rxEnv(nil), WithRelease(true)}
	return append(base, extra...)
}

// pwDACL returns the DACL of path as SDDL (D:P(...)(...)).
func pwDACL(t *testing.T, path string) string {
	t.Helper()
	const (
		seFileObject            = 1
		daclSecurityInformation = 4
	)
	advapi := syscall.NewLazyDLL("advapi32.dll")
	get := advapi.NewProc("GetNamedSecurityInfoW")
	conv := advapi.NewProc("ConvertSecurityDescriptorToStringSecurityDescriptorW")
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	var dacl, sd uintptr
	if r, _, _ := get.Call(uintptr(unsafe.Pointer(p)), seFileObject, daclSecurityInformation,
		0, 0, uintptr(unsafe.Pointer(&dacl)), 0, uintptr(unsafe.Pointer(&sd))); r != 0 {
		t.Fatalf("GetNamedSecurityInfo %s: %v", path, syscall.Errno(r))
	}
	defer func() { _, _ = syscall.LocalFree(syscall.Handle(sd)) }()
	var str *uint16
	if r, _, e := conv.Call(sd, sddlRevision1, daclSecurityInformation, uintptr(unsafe.Pointer(&str)), 0); r == 0 {
		t.Fatalf("ConvertSecurityDescriptorToStringSecurityDescriptor %s: %v", path, e)
	}
	defer func() { _, _ = syscall.LocalFree(syscall.Handle(unsafe.Pointer(str))) }()
	return syscall.UTF16ToString(unsafe.Slice(str, 1<<16))
}

// pwMySID is the current user's SID string.
func pwMySID(t *testing.T) string {
	t.Helper()
	tok, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tok.Close() }()
	tu, err := tok.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid, err := tu.User.Sid.String()
	if err != nil {
		t.Fatal(err)
	}
	return sid
}

// pwAceRe matches one ACE: (type;flags;rights;object;inherit-object;sid).
var pwAceRe = regexp.MustCompile(`\(([^;]*);([^;]*);([^;]*);[^;]*;[^;]*;([^)]*)\)`)

// pwCheckOwnerOnly fails unless path has a protected DACL whose every entry
// allows full access to the current user or SYSTEM, and both are there.
func pwCheckOwnerOnly(t *testing.T, path, me string) {
	t.Helper()
	sddl := pwDACL(t, path)
	if !strings.HasPrefix(sddl, "D:P") {
		t.Errorf("%s: DACL %q is not protected (inherits from the parent)", path, sddl)
	}
	var who []string
	for _, m := range pwAceRe.FindAllStringSubmatch(sddl, -1) {
		typ, rights, sid := m[1], m[3], m[4]
		// SDDL prints well-known accounts as aliases: the built-in
		// Administrator (RID 500, which a CI runner may be) is LA.
		if sid == "LA" && strings.HasSuffix(me, "-500") {
			sid = me
		}
		if typ != "A" || rights != "FA" {
			t.Errorf("%s: unexpected entry %s in %q", path, m[0], sddl)
		}
		if sid != me && sid != "SY" {
			t.Errorf("%s: entry for %s in %q; only the user and SYSTEM may have one", path, sid, sddl)
		}
		who = append(who, sid)
	}
	sort.Strings(who)
	if got := strings.Join(compactStrings(who), ","); got != strings.Join(sortedPair(me, "SY"), ",") {
		t.Errorf("%s: DACL %q grants %s, want the user and SYSTEM", path, sddl, got)
	}
}

func compactStrings(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func sortedPair(a, b string) []string {
	s := []string{a, b}
	sort.Strings(s)
	return s
}

// TestWinOwnerOnlyACL: every folder Ensure creates and every file appdir
// writes (home.jsonc, .gitignore) carries a protected DACL for the current
// user and SYSTEM only; a folder the user made beforehand is left alone.
func TestWinOwnerOnlyACL(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	d, err := New(pwApp, pwOpts(home, home, WithMode(ModeGlobal))...)
	if err != nil {
		t.Fatal(err)
	}
	me := pwMySID(t)
	logs, err := d.Ensure(KindLogs)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.InitHome(d.Global().Base, ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{d.Global().Base, logs, filepath.Join(d.Global().Base, "home.jsonc")} {
		pwCheckOwnerOnly(t, p, me)
	}
	if strings.HasPrefix(pwDACL(t, home), "D:P") {
		t.Errorf("the user's own folder %s got a protected DACL; appdir must only restrict what it creates", home)
	}
}

// TestWinAtomicWriteReplacesExisting: the atomic write renames over an
// existing file (MoveFileEx with MOVEFILE_REPLACE_EXISTING), leaves no temp
// file, and keeps the target's ACL (it is not re-restricted).
func TestWinAtomicWriteReplacesExisting(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	rxMkdir(t, home)
	d, err := New(pwApp, pwOpts(home, home, WithMode(ModeGlobal))...)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "file.txt")
	rxWrite(t, target, "old")
	before := pwDACL(t, target)
	if err := d.writeFileAtomic(target, []byte("new"), 0o600); err != nil {
		t.Fatalf("writeFileAtomic over an existing file: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Fatalf("content = %q, want new", b)
	}
	if after := pwDACL(t, target); after != before {
		t.Errorf("ACL of the replaced file changed: %q -> %q", before, after)
	}
	ents, _ := os.ReadDir(root)
	for _, e := range ents {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left: %s", e.Name())
		}
	}
	// The retry path's own building block: a plain rename over a file.
	src := filepath.Join(root, "src.txt")
	rxWrite(t, src, "third")
	if err := renameReplace(src, target); err != nil {
		t.Fatalf("renameReplace over an existing file: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "third" {
		t.Fatalf("content after renameReplace = %q", b)
	}
}

// pwFlipDrive returns p with the drive letter in the other case.
func pwFlipDrive(p string) string {
	vol := filepath.VolumeName(p)
	if len(vol) != 2 || vol[1] != ':' {
		return p
	}
	l := vol[:1]
	if strings.ToUpper(l) == l {
		l = strings.ToLower(l)
	} else {
		l = strings.ToUpper(l)
	}
	return l + p[1:]
}

// TestWinDriveLetterCase: the user's home given with a lower-case drive
// letter and the working folder with an upper-case one are still the same
// folder to the walk: $HOME stays a wall, so a .<app>/ above it is never the
// project, and one between work and home is found.
func TestWinDriveLetterCase(t *testing.T) {
	root := rxRoot(t)
	home := filepath.Join(root, "user")
	work := filepath.Join(home, "src", "proj", "sub")
	rxMkdir(t, work, filepath.Join(root, "."+pwApp)) // above $HOME: must never count
	flipped := pwFlipDrive(home)
	if flipped == home {
		t.Skipf("temp root %s has no drive letter", root)
	}
	d, err := New(pwApp, pwOpts(flipped, work)...)
	if err != nil {
		t.Fatal(err)
	}
	if d.HasProject() {
		p, _ := d.Project()
		t.Fatalf("found project %s above the user's home", p.Marker)
	}
	marker := filepath.Join(home, "src", "proj", "."+pwApp)
	rxMkdir(t, marker)
	d, err = New(pwApp, pwOpts(flipped, work)...)
	if err != nil {
		t.Fatal(err)
	}
	p, err := d.Project()
	if err != nil || !sameFile(p.Marker, marker) {
		t.Fatalf("Project() = %+v, %v; want marker %s", p, err, marker)
	}
	if !strings.EqualFold(filepath.VolumeName(d.Global().Base), filepath.VolumeName(home)) {
		t.Errorf("global home %s is not on the home's drive %s", d.Global().Base, filepath.VolumeName(home))
	}
}

// pwUNC turns C:\x\y into \\localhost\C$\x\y, the administrative share of
// the same folder, or "" when the share is not reachable.
func pwUNC(p string) string {
	vol := filepath.VolumeName(p)
	if len(vol) != 2 || vol[1] != ':' {
		return ""
	}
	u := `\\localhost\` + strings.ToUpper(vol[:1]) + `$` + p[2:]
	if _, err := os.Stat(u); err != nil {
		return ""
	}
	return u
}

// TestWinUNCHome: --home given as a UNC path resolves to that UNC path,
// Ensure creates the folder through it, and the result is the same folder as
// the drive-letter path.
func TestWinUNCHome(t *testing.T) {
	root := rxRoot(t)
	user := filepath.Join(root, "user")
	target := filepath.Join(root, "unc-home")
	rxMkdir(t, user, target)
	unc := pwUNC(target)
	if unc == "" {
		t.Skip(`\\localhost\<drive>$ is not reachable on this machine`)
	}
	d, err := New(pwApp, pwOpts(user, user, WithMode(ModeGlobal), WithHome(unc), WithTestGuard(false))...)
	if err != nil {
		t.Fatalf("New with a UNC --home: %v", err)
	}
	if g := d.Global().Base; !strings.HasPrefix(g, `\\localhost\`) {
		t.Fatalf("global home %s lost the UNC form of %s", g, unc)
	}
	logs, err := d.Ensure(KindLogs)
	if err != nil {
		t.Fatalf("Ensure through UNC: %v", err)
	}
	rel, err := filepath.Rel(unc, logs)
	if err != nil {
		t.Fatal(err)
	}
	if !isDir(filepath.Join(target, rel)) {
		t.Fatalf("Ensure(logs) = %s, but %s does not exist", logs, filepath.Join(target, rel))
	}
}

// TestWinUserProfileIsHome: with no seams, the global home is
// %USERPROFILE%\.<app>, not $HOME (Go's os.UserHomeDir on Windows).
func TestWinUserProfileIsHome(t *testing.T) {
	root := ForTest(t, pwApp)
	profile := filepath.Join(root, "profile")
	rxMkdir(t, profile)
	t.Setenv("USERPROFILE", profile)
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("PWAPP_HOME", "")
	if err := os.Unsetenv("PWAPP_HOME"); err != nil {
		t.Fatal(err)
	}
	d, err := New(pwApp, WithWorkDir(profile), WithRelease(true), WithMode(ModeGlobal))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(profile, "."+pwApp); !sameFile(filepath.Dir(d.Global().Base), profile) || filepath.Base(d.Global().Base) != "."+pwApp {
		t.Fatalf("global home = %s, want %s", d.Global().Base, want)
	}
}

// TestWinNamedPipeShape: the socket is always the named pipe
// \\.\pipe\<name>-<user>-<12 hex>, with no folder; EnsureSocketDir creates
// only run/, and two homes get two pipes.
func TestWinNamedPipeShape(t *testing.T) {
	root := rxRoot(t)
	var paths []string
	for _, h := range []string{"a", "b"} {
		home := filepath.Join(root, h)
		rxMkdir(t, home)
		d, err := New(pwApp, pwOpts(home, home, WithMode(ModeGlobal))...)
		if err != nil {
			t.Fatal(err)
		}
		s, err := d.EnsureSocketDir()
		if err != nil {
			t.Fatal(err)
		}
		re := regexp.MustCompile(`^\\\\\.\\pipe\\` + pwApp + `-[A-Za-z0-9._-]+-[0-9a-f]{12}$`)
		if !re.MatchString(s.Path) || !s.Fallback || s.Dir != "" {
			t.Fatalf("Socket = %+v; want a named pipe with Fallback and no Dir", s)
		}
		run, _ := d.Path(KindRun)
		if !isDir(run) {
			t.Errorf("EnsureSocketDir did not create %s", run)
		}
		if filepath.Dir(s.PointerFile) != run {
			t.Errorf("pointer file %s not in %s", s.PointerFile, run)
		}
		paths = append(paths, s.Path)
	}
	if paths[0] == paths[1] {
		t.Errorf("two homes share the pipe %s", paths[0])
	}
}
