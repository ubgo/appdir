package appdir

// Runtime tests (tag tt): profiles and release detection, the prod-db guard,
// child env, run paths and sockets, the legacy move and ForTest. Each case
// reads as "who runs what, and what they get": a user with an installed
// binary, a developer with a dev build on the same machine, a tool author
// with custom options. Every test uses the seams (WithHomeDir, WithWorkDir,
// WithEnv, WithRelease/WithBuildInfo) and a temp root; only the ForTest
// tests touch the process env, and only through t.Setenv.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ubgo/appdir/internal/flock"
)

// ttApp is the app every runtime test resolves; ttPrefix is its env prefix.
const (
	ttApp    = "ttapp"
	ttPrefix = "TTAPP"
	ttOld    = ".ttold" // a legacy home name
)

// ttPseudo is the release reason of every pseudo-version.
const ttPseudo = "dev: pseudo-version"

// ttEnvOpt turns a map into a WithEnv lookup: tests never read the process env.
func ttEnvOpt(m map[string]string) Option {
	return WithEnv(func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	})
}

// ttFix is one test's world: a temp root, a user home and a project folder
// holding .ttapp/ (created), all real paths.
type ttFix struct {
	root, home, proj string
}

// ttWorld makes the temp world. The project marker exists; ~/.ttapp does not.
func ttWorld(t *testing.T) ttFix {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	f := ttFix{root: root, home: filepath.Join(root, "home"), proj: filepath.Join(root, "proj")}
	ttMkdir(t, f.home)
	ttMkdir(t, filepath.Join(f.proj, "."+ttApp))
	return f
}

// base returns the seam options for this world: home, work dir, env.
func (f ttFix) base(env map[string]string) []Option {
	home := f.home
	return []Option{
		WithHomeDir(func() (string, error) { return home, nil }),
		WithWorkDir(f.proj),
		ttEnvOpt(env),
	}
}

// ttNew resolves ttapp in world f with env and extra options.
func ttNew(t *testing.T, f ttFix, env map[string]string, extra ...Option) (*Dirs, error) {
	t.Helper()
	return New(ttApp, append(f.base(env), extra...)...)
}

// ttMustNew is ttNew that fails the test on error.
func ttMustNew(t *testing.T, f ttFix, env map[string]string, extra ...Option) *Dirs {
	t.Helper()
	d, err := ttNew(t, f, env, extra...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

func ttMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func ttWrite(t *testing.T, path, body string) {
	t.Helper()
	ttMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func ttRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func ttSymlink(t *testing.T, target, link string) {
	t.Helper()
	ttMkdir(t, filepath.Dir(link))
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func ttExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// ttBI fakes debug.ReadBuildInfo with a main module version and
// "key=value" settings.
func ttBI(version string, settings ...string) Option {
	return WithBuildInfo(func() (*debug.BuildInfo, bool) {
		bi := &debug.BuildInfo{Main: debug.Module{Path: "example.com/ttapp", Version: version}}
		for _, s := range settings {
			k, v, _ := strings.Cut(s, "=")
			bi.Settings = append(bi.Settings, debug.BuildSetting{Key: k, Value: v})
		}
		return bi, true
	})
}

// ttPanic runs fn and returns the panic message, "" when it did not panic.
func ttPanic(fn func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprint(r)
		}
	}()
	fn()
	return ""
}

// ttWarnCodes lists the warning codes of d.
func ttWarnCodes(d *Dirs) []WarningCode {
	var out []WarningCode
	for _, w := range d.Warnings() {
		out = append(out, w.Code)
	}
	return out
}

// ttWantErr checks err against a sentinel (nil = no error expected).
func ttWantErr(t *testing.T, err, want error) {
	t.Helper()
	switch {
	case want == nil && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case want != nil && !errors.Is(err, want):
		t.Fatalf("error = %v, want %v", err, want)
	}
}

// ---------------------------------------------------------------------------
// Build x profile matrix
// ---------------------------------------------------------------------------

// ttBuild is one way a binary was built.
type ttBuild struct {
	name    string
	opts    []Option
	release bool
	reason  string
}

func ttBuilds() []ttBuild {
	noInfo := WithBuildInfo(func() (*debug.BuildInfo, bool) { return nil, false })
	nilInfo := WithBuildInfo(func() (*debug.BuildInfo, bool) { return nil, true })
	b := func(name string, release bool, reason string, opts ...Option) ttBuild {
		return ttBuild{name: name, opts: opts, release: release, reason: reason}
	}
	return []ttBuild{
		b("release-script-flag", true, "flag", WithRelease(true)),
		b("dev-flag", false, "dev: flag", WithRelease(false)),
		b("dev-flag-beats-go-install", false, "dev: flag", WithRelease(false), ttBI("v1.4.2")),
		b("release-flag-beats-checkout", true, "flag", WithRelease(true), ttBI("(devel)", "vcs.revision=abc")),
		b("go-install-tag", true, "go-install", ttBI("v1.4.2")),
		b("go-install-other-settings", true, "go-install", ttBI("v1.4.2", "GOOS=linux", "-ldflags=-s -w")),
		b("go-install-prerelease-tag", true, "go-install", ttBI("v1.5.0-rc.1")),
		b("go-install-incompatible", true, "go-install", ttBI("v2.3.0+incompatible")),
		b("tagged-checkout", false, "dev: vcs.revision", ttBI("v1.4.2", "vcs.revision=0123abcd", "vcs.modified=false")),
		b("devel", false, "dev: (devel)", ttBI("(devel)", "vcs.revision=abc")),
		b("empty-version", false, "dev: (devel)", ttBI("")),
		b("dirty-checkout", false, "dev: +dirty", ttBI("v1.4.2+dirty", "vcs.revision=abc", "vcs.modified=true")),
		b("dirty-without-vcs", false, "dev: +dirty", ttBI("v1.4.2+dirty")),
		b("pseudo-no-tag", false, ttPseudo, ttBI("v0.0.0-20261110120000-abcdef123456")),
		b("pseudo-no-tag-major-v2", false, ttPseudo, ttBI("v2.0.0-20261110120000-abcdef123456")),
		b("pseudo-after-prerelease", false, ttPseudo, ttBI("v1.5.0-rc.1.0.20261110120000-abcdef123456")),
		b("pseudo-after-release", false, ttPseudo, ttBI("v1.4.3-0.20261110120000-abcdef123456")),
		b("pseudo-incompatible", false, ttPseudo, ttBI("v2.3.1-0.20261110120000-abcdef123456+incompatible")),
		b("pseudo-dirty", false, "dev: +dirty", ttBI("v0.0.0-20261110120000-abcdef123456+dirty")),
		b("no-v-prefix", false, "dev: version without v prefix", ttBI("1.4.2")),
		b("no-build-info", false, "dev: no build info", noInfo),
		b("nil-build-info", false, "dev: no build info", nilInfo),
	}
}

// ttSource is one way the user asked for a profile.
type ttSource struct {
	name   string
	opts   []Option
	env    map[string]string
	err    error   // expected New error (both builds)
	rel    Profile // profile a release build picks
	dev    Profile // profile a dev build picks
	relRel Profile // release profile of the option set ("" = prod)
	warnR  bool    // release build warns profile-env-ignored
	warnD  bool    // dev build warns profile-env-ignored
}

func ttSources() []ttSource {
	pe := func(kv ...string) map[string]string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	const pk, ak = ttPrefix + "_PROFILE", ttPrefix + "_ALLOW_PROD"
	custom := WithProfiles(ProfileProd, ProfileDev, "staging")
	renamed := []Option{WithProfiles("stable", "edge"), WithReleaseProfile("stable"), WithDevProfile("edge")}
	return []ttSource{
		{name: "none", rel: ProfileProd, dev: ProfileDev},
		{name: "flag-prod", opts: []Option{WithProfile(string(ProfileProd))}, rel: ProfileProd, dev: ProfileProd},
		{name: "flag-dev", opts: []Option{WithProfile(string(ProfileDev))}, rel: ProfileDev, dev: ProfileDev},
		{name: "flag-unknown", opts: []Option{WithProfile("qa")}, err: ErrUnknownProfile},
		{name: "env-prod", env: pe(pk, string(ProfileProd)), rel: ProfileProd, dev: ProfileDev, warnD: true},
		{name: "env-prod-allow-prod", env: pe(pk, string(ProfileProd), ak, "1"), rel: ProfileProd, dev: ProfileProd},
		{name: "env-prod-allow-prod-not-1", env: pe(pk, string(ProfileProd), ak, "true"), rel: ProfileProd, dev: ProfileDev, warnD: true},
		{name: "env-dev", env: pe(pk, string(ProfileDev)), rel: ProfileProd, dev: ProfileDev, warnR: true},
		{name: "env-dev-allow-prod", env: pe(pk, string(ProfileDev), ak, "1"), rel: ProfileProd, dev: ProfileDev, warnR: true},
		{name: "env-empty-is-unset", env: pe(pk, ""), rel: ProfileProd, dev: ProfileDev},
		{name: "env-unknown", env: pe(pk, "qa"), err: ErrUnknownProfile},
		{name: "env-unknown-allow-prod", env: pe(pk, "qa", ak, "1"), err: ErrUnknownProfile},
		{name: "env-wrong-case", env: pe(pk, "PROD"), err: ErrUnknownProfile},
		{name: "flag-beats-env", opts: []Option{WithProfile(string(ProfileDev))}, env: pe(pk, string(ProfileProd)), rel: ProfileDev, dev: ProfileDev},
		{name: "flag-prod-beats-env-dev", opts: []Option{WithProfile(string(ProfileProd))}, env: pe(pk, string(ProfileDev)), rel: ProfileProd, dev: ProfileProd},
		{name: "custom-list-flag-staging", opts: []Option{custom, WithProfile("staging")}, rel: "staging", dev: "staging"},
		{name: "custom-list-env-staging", opts: []Option{custom}, env: pe(pk, "staging"), rel: ProfileProd, dev: "staging", warnR: true},
		{name: "custom-list-unknown", opts: []Option{custom, WithProfile("test")}, err: ErrUnknownProfile},
		{name: "renamed-profiles-none", opts: renamed, rel: "stable", dev: "edge", relRel: "stable"},
		{name: "renamed-profiles-env-stable", opts: renamed, env: pe(pk, "stable"), rel: "stable", dev: "edge", relRel: "stable", warnD: true},
		{name: "renamed-profiles-flag-prod-unknown", opts: append(append([]Option(nil), renamed...), WithProfile(string(ProfileProd))), err: ErrUnknownProfile},
	}
}

// TestRuntimeProfileMatrix: every way a binary can be built, crossed with
// every way the user can ask for a profile. Who: a user of the installed
// binary and a developer of the dev build on one machine. Expected: the
// profile, the release verdict with its exact reason, the names, the banner,
// the global home and the profile-env-ignored warning.
func TestRuntimeProfileMatrix(t *testing.T) {
	f := ttWorld(t)
	for _, b := range ttBuilds() {
		for _, s := range ttSources() {
			t.Run(b.name+"/"+s.name, func(t *testing.T) {
				opts := append(append([]Option(nil), b.opts...), s.opts...)
				d, err := ttNew(t, f, s.env, opts...)
				if s.err != nil {
					ttWantErr(t, err, s.err)
					if !strings.Contains(err.Error(), "unknown profile") || !strings.Contains(err.Error(), "known:") {
						t.Fatalf("error should name the known profiles: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("New: %v", err)
				}
				relP := s.relRel
				if relP == "" {
					relP = ProfileProd
				}
				want, warn := s.dev, s.warnD
				if b.release {
					want, warn = s.rel, s.warnR
				}
				if d.IsRelease() != b.release || d.ReleaseReason() != b.reason {
					t.Fatalf("IsRelease=%v reason=%q, want %v %q", d.IsRelease(), d.ReleaseReason(), b.release, b.reason)
				}
				if d.Profile() != want {
					t.Fatalf("Profile = %q, want %q", d.Profile(), want)
				}
				if got := slices.Contains(ttWarnCodes(d), WarnProfileEnvIgnored); got != warn {
					t.Fatalf("profile-env-ignored warning = %v, want %v (%v)", got, warn, d.Warnings())
				}
				wantName, wantDot, wantHome := "svc", "com.org.svc", filepath.Join(f.home, "."+ttApp)
				wantBanner := ""
				if want != relP {
					wantName, wantDot = "svc-"+string(want), "com.org.svc."+string(want)
					wantHome += "-" + string(want)
					wantBanner = fmt.Sprintf("[%s] profile: %s → %s", ttApp, want, filepath.Join("~", "."+ttApp+"-"+string(want)))
				}
				if got := d.Name("svc"); got != wantName {
					t.Errorf("Name = %q, want %q", got, wantName)
				}
				if got := d.NameWith("com.org.svc", "."); got != wantDot {
					t.Errorf("NameWith = %q, want %q", got, wantDot)
				}
				if got := d.Banner(); got != wantBanner {
					t.Errorf("Banner = %q, want %q", got, wantBanner)
				}
				if got := d.Global().Base; got != wantHome {
					t.Errorf("global home = %q, want %q", got, wantHome)
				}
				wantData := filepath.Join(f.proj, "."+ttApp)
				if want != relP {
					wantData = filepath.Join(wantData, "profiles", string(want))
				}
				if p, _ := d.Project(); p.Data != wantData {
					t.Errorf("project data = %q, want %q", p.Data, wantData)
				}
			})
		}
	}
}

// TestRuntimeProfileWarningText: the warning tells the user exactly what to
// do instead. Who: a developer whose shell has TTAPP_PROFILE=prod, and a
// user whose shell has TTAPP_PROFILE=dev.
func TestRuntimeProfileWarningText(t *testing.T) {
	f := ttWorld(t)
	cases := []struct {
		name    string
		release bool
		env     string
		want    []string
	}{
		{"dev-build-env-prod", false, "prod", []string{"TTAPP_PROFILE=prod ignored by a dev build", "TTAPP_ALLOW_PROD=1", "--profile"}},
		{"release-build-env-dev", true, "dev", []string{"TTAPP_PROFILE=dev ignored by a release build", "--profile dev"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := ttMustNew(t, f, map[string]string{ttPrefix + "_PROFILE": c.env}, WithRelease(c.release))
			ws := d.Warnings()
			if len(ws) != 1 || ws[0].Code != WarnProfileEnvIgnored {
				t.Fatalf("warnings = %v", ws)
			}
			for _, w := range c.want {
				if !strings.Contains(ws[0].Message, w) {
					t.Errorf("message %q lacks %q", ws[0].Message, w)
				}
			}
		})
	}
}

// TestRuntimeProfileOptionsInvalid: a tool author's broken profile options
// fail New with ErrInvalidOption, never a silent default.
func TestRuntimeProfileOptionsInvalid(t *testing.T) {
	f := ttWorld(t)
	cases := []struct {
		name string
		opts []Option
	}{
		{"empty-list", []Option{WithProfiles()}},
		{"release-profile-not-listed", []Option{WithReleaseProfile("stable")}},
		{"dev-profile-not-listed", []Option{WithDevProfile("edge")}},
		{"list-drops-dev", []Option{WithProfiles(ProfileProd)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ttNew(t, f, nil, append([]Option{WithRelease(false)}, c.opts...)...)
			ttWantErr(t, err, ErrInvalidOption)
		})
	}
}

// TestRuntimeBanner covers the banner outside the plain ~/.<app>-dev case.
func TestRuntimeBanner(t *testing.T) {
	f := ttWorld(t)
	outside := filepath.Join(f.root, "elsewhere")
	noHome := WithHomeDir(func() (string, error) { return "", errors.New("no home") })
	cases := []struct {
		name string
		env  map[string]string
		opts []Option
		want string
	}{
		{"home-flag-inside-user-home", nil, []Option{WithHome("~/work/h")}, "[ttapp] profile: dev → " + filepath.Join("~", "work", "h")},
		{"home-flag-outside-user-home", nil, []Option{WithHome(outside)}, "[ttapp] profile: dev → " + outside},
		{"home-env-outside", map[string]string{ttPrefix + "_HOME": outside}, nil, "[ttapp] profile: dev → " + outside},
		{"no-home-folder", nil, []Option{noHome}, "[ttapp] profile: dev → (no home folder)"},
		{"no-home-folder-with-home-flag", nil, []Option{noHome, WithHome(outside)}, "[ttapp] profile: dev → " + outside},
		{"custom-profile", nil, []Option{WithProfiles(ProfileProd, ProfileDev, "staging"), WithProfile("staging")}, "[ttapp] profile: staging → " + filepath.Join("~", ".ttapp-staging")},
		{"release-profile-by-flag", nil, []Option{WithProfile(string(ProfileProd))}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := ttMustNew(t, f, c.env, append([]Option{WithRelease(false)}, c.opts...)...)
			if got := d.Banner(); got != c.want {
				t.Fatalf("Banner = %q, want %q", got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Prod-db guard
// ---------------------------------------------------------------------------

// TestRuntimeCheckDB: before opening or migrating, the tool asks CheckDB.
// Who: a developer running the dev build next to an installed binary, in a
// project and outside one. Expected: the prod db (global, project, db:
// pointer, by any spelling) is refused, or read-only with ALLOW_PROD=1; the
// dev build's own db and a release build are read-write.
func TestRuntimeCheckDB(t *testing.T) {
	type ttDBCase struct {
		name    string
		release bool
		env     map[string]string
		opts    []Option
		setup   func(t *testing.T, f ttFix) string // returns the db path
		access  DBAccess
		err     error
		wd      string // work dir relative to root; "" = proj
	}
	prodGlobal := func(t *testing.T, f ttFix) string { return filepath.Join(f.home, "."+ttApp, ttApp+".db") }
	prodGlobalExisting := func(t *testing.T, f ttFix) string {
		p := prodGlobal(t, f)
		ttWrite(t, p, "sqlite")
		return p
	}
	prodGlobalHomeOnly := func(t *testing.T, f ttFix) string {
		ttMkdir(t, filepath.Join(f.home, "."+ttApp))
		return prodGlobal(t, f)
	}
	prodProject := func(t *testing.T, f ttFix) string { return filepath.Join(f.proj, "."+ttApp, ttApp+".db") }
	pointer := func(file, body string) func(t *testing.T, f ttFix) string {
		return func(t *testing.T, f ttFix) string {
			ttMkdir(t, filepath.Join(f.root, "dbs"))
			ttWrite(t, filepath.Join(f.proj, "."+ttApp, file), body)
			return filepath.Join(f.root, "dbs", ttApp+".db")
		}
	}
	allow := map[string]string{ttPrefix + "_ALLOW_PROD": "1"}
	cases := []ttDBCase{
		{name: "release-prod-global", release: true, setup: prodGlobalExisting},
		{name: "release-prod-project", release: true, setup: prodProject},
		{name: "release-any-path", release: true, setup: func(t *testing.T, f ttFix) string { return filepath.Join(f.root, "x", "y.db") }},
		{name: "dev-prod-global-existing", setup: prodGlobalExisting, err: ErrProdDB},
		{name: "dev-prod-global-file-not-yet", setup: prodGlobalHomeOnly, err: ErrProdDB},
		{name: "dev-prod-global-home-not-yet", setup: prodGlobal, err: ErrProdDB},
		{name: "dev-prod-global-through-symlinked-folder", setup: func(t *testing.T, f ttFix) string {
			ttMkdir(t, filepath.Join(f.home, "."+ttApp))
			ttSymlink(t, filepath.Join(f.home, "."+ttApp), filepath.Join(f.root, "alias"))
			return filepath.Join(f.root, "alias", ttApp+".db")
		}, err: ErrProdDB},
		{name: "dev-prod-global-unclean-spelling", setup: func(t *testing.T, f ttFix) string {
			prodGlobalExisting(t, f)
			return filepath.Join(f.home, "x", "..", "."+ttApp, ".", ttApp+".db")
		}, err: ErrProdDB},
		{name: "dev-prod-global-outside-project", wd: "elsewhere", setup: prodGlobalExisting, err: ErrProdDB},
		{name: "dev-prod-project-not-yet", setup: prodProject, err: ErrProdDB},
		{name: "dev-prod-home-pointer", setup: func(t *testing.T, f ttFix) string {
			target := filepath.Join(f.root, "shared-home")
			ttWrite(t, filepath.Join(target, "home.jsonc"), `{"app": "ttapp", "layout": 1, "scope": "project"}`)
			ttWrite(t, filepath.Join(f.proj, "."+ttApp, "config.local.jsonc"), `{"home": "../../shared-home"}`)
			return filepath.Join(target, ttApp+".db")
		}, err: ErrProdDB},
		{name: "dev-own-db-under-home-pointer", setup: func(t *testing.T, f ttFix) string {
			target := filepath.Join(f.root, "shared-home")
			ttWrite(t, filepath.Join(target, "home.jsonc"), `{"app": "ttapp", "layout": 1, "scope": "project"}`)
			ttWrite(t, filepath.Join(f.proj, "."+ttApp, "config.local.jsonc"), `{"home": "../../shared-home"}`)
			return filepath.Join(target, profilesDir, string(ProfileDev), ttApp+".db")
		}},
		{name: "dev-prod-global-home-is-symlink", setup: func(t *testing.T, f ttFix) string {
			ttMkdir(t, filepath.Join(f.root, "prod-data"))
			ttSymlink(t, filepath.Join(f.root, "prod-data"), filepath.Join(f.home, "."+ttApp))
			return filepath.Join(f.root, "prod-data", ttApp+".db")
		}, err: ErrProdDB},
		{name: "dev-project-symlink-out-of-profile-data", setup: func(t *testing.T, f ttFix) string {
			// The dev project db is inside .ttapp/profiles/dev/; a link from
			// there to the prod project db escapes that home.
			ttWrite(t, filepath.Join(f.proj, "."+ttApp, ttApp+".db"), "sqlite")
			p := filepath.Join(f.proj, "."+ttApp, profilesDir, string(ProfileDev), ttApp+".db")
			ttSymlink(t, filepath.Join(f.proj, "."+ttApp, ttApp+".db"), p)
			return p
		}, err: ErrSymlinkEscape},
		{name: "symlink-outside-every-home-same-folder", setup: func(t *testing.T, f ttFix) string {
			ttWrite(t, filepath.Join(f.root, "loose", "real.db"), "sqlite")
			p := filepath.Join(f.root, "loose", "x.db")
			ttSymlink(t, "real.db", p)
			return p
		}},
		{name: "symlink-outside-every-home-other-folder", setup: func(t *testing.T, f ttFix) string {
			ttWrite(t, filepath.Join(f.root, "elsewhere", "real.db"), "sqlite")
			p := filepath.Join(f.root, "loose", "x.db")
			ttSymlink(t, filepath.Join(f.root, "elsewhere", "real.db"), p)
			return p
		}, err: ErrSymlinkEscape},
		{name: "dev-prod-project-existing", setup: func(t *testing.T, f ttFix) string {
			p := prodProject(t, f)
			ttWrite(t, p, "sqlite")
			return p
		}, err: ErrProdDB},
		{name: "dev-prod-db-pointer-relative-local", setup: pointer("config.local.jsonc", `{"db": "../../dbs/ttapp.db"}`), err: ErrProdDB},
		{name: "dev-prod-db-pointer-relative-committed", setup: pointer("config.jsonc", `{"db": "../../dbs/ttapp.db"}`), err: ErrProdDB},
		{name: "dev-prod-db-pointer-envvar", env: map[string]string{"TT_DBROOT": "@root/dbs"}, setup: pointer("config.local.jsonc", `{"db": "${TT_DBROOT}/ttapp.db"}`), err: ErrProdDB},
		{name: "dev-prod-db-pointer-envvar-unset", setup: pointer("config.local.jsonc", `{"db": "${TT_DBROOT}/ttapp.db"}`)},
		{name: "dev-prod-global-when-prod-project-broken", setup: func(t *testing.T, f ttFix) string {
			ttWrite(t, filepath.Join(f.proj, "."+ttApp, "config.local.jsonc"), `{"db": "../../missing/ttapp.db"}`)
			return prodGlobalExisting(t, f)
		}, err: ErrProdDB},
		{name: "dev-own-global-db", opts: []Option{WithMode(ModeGlobal)}, setup: func(t *testing.T, f ttFix) string {
			p := filepath.Join(f.home, ".ttapp-dev", ttApp+".db")
			ttWrite(t, p, "sqlite")
			return p
		}},
		{name: "dev-own-project-db", setup: func(t *testing.T, f ttFix) string {
			return filepath.Join(f.proj, "."+ttApp, profilesDir, string(ProfileDev), ttApp+".db")
		}},
		{name: "dev-same-name-other-folder", setup: func(t *testing.T, f ttFix) string {
			ttMkdir(t, filepath.Join(f.root, "other"))
			ttMkdir(t, filepath.Join(f.home, "."+ttApp))
			return filepath.Join(f.root, "other", ttApp+".db")
		}},
		{name: "dev-other-name-prod-folder", setup: func(t *testing.T, f ttFix) string {
			ttMkdir(t, filepath.Join(f.home, "."+ttApp))
			return filepath.Join(f.home, "."+ttApp, "scratch.db")
		}},
		{name: "allow-prod-global", env: allow, setup: prodGlobalExisting, access: AccessReadOnly},
		{name: "allow-prod-global-not-yet", env: allow, setup: prodGlobal, access: AccessReadOnly},
		{name: "allow-prod-project", env: allow, setup: prodProject, access: AccessReadOnly},
		{name: "allow-prod-own-db", env: allow, setup: func(t *testing.T, f ttFix) string {
			return filepath.Join(f.proj, "."+ttApp, profilesDir, string(ProfileDev), ttApp+".db")
		}},
		{name: "allow-prod-not-1", env: map[string]string{ttPrefix + "_ALLOW_PROD": "true"}, setup: prodGlobalExisting, err: ErrProdDB},
		{name: "dev-home-env-points-to-prod", env: map[string]string{ttPrefix + "_HOME": "@home/.ttapp"}, opts: []Option{WithMode(ModeGlobal)}, setup: prodGlobalExisting, err: ErrProdDB},
		{name: "dev-home-flag-on-prod-home", opts: []Option{WithHome("~/.ttapp")}, setup: prodGlobalExisting, err: ErrProdDB},
		{name: "dev-custom-profile-on-prod", opts: []Option{WithProfiles(ProfileProd, ProfileDev, "staging"), WithProfile("staging")}, setup: prodGlobalExisting, err: ErrProdDB},
		{name: "dev-profile-env-prod-ignored-still-guarded", env: map[string]string{ttPrefix + "_PROFILE": string(ProfileProd)}, setup: prodGlobalExisting, err: ErrProdDB},
		{name: "symlink-escaping-home", opts: []Option{WithMode(ModeGlobal)}, setup: func(t *testing.T, f ttFix) string {
			ttWrite(t, filepath.Join(f.root, "outside", "real.db"), "sqlite")
			p := filepath.Join(f.home, ".ttapp-dev", ttApp+".db")
			ttSymlink(t, filepath.Join(f.root, "outside", "real.db"), p)
			return p
		}, err: ErrSymlinkEscape},
		{name: "symlink-escaping-home-release", release: true, setup: func(t *testing.T, f ttFix) string {
			ttWrite(t, filepath.Join(f.root, "outside", "real.db"), "sqlite")
			p := filepath.Join(f.home, "."+ttApp, ttApp+".db")
			ttSymlink(t, filepath.Join(f.root, "outside", "real.db"), p)
			return p
		}, err: ErrSymlinkEscape},
		{name: "symlink-broken", setup: func(t *testing.T, f ttFix) string {
			p := filepath.Join(f.home, ".ttapp-dev", ttApp+".db")
			ttSymlink(t, filepath.Join(f.root, "nothing.db"), p)
			return p
		}, err: ErrSymlinkEscape},
		{name: "symlink-inside-home", setup: func(t *testing.T, f ttFix) string {
			ttWrite(t, filepath.Join(f.home, ".ttapp-dev", "real.db"), "sqlite")
			p := filepath.Join(f.home, ".ttapp-dev", ttApp+".db")
			ttSymlink(t, "real.db", p)
			return p
		}},
		{name: "symlink-to-prod-db", setup: func(t *testing.T, f ttFix) string {
			// A link inside the dev home that leads to the prod db escapes
			// the dev home first: refused before the prod check runs.
			prodGlobalExisting(t, f)
			p := filepath.Join(f.home, ".ttapp-dev", ttApp+".db")
			ttSymlink(t, prodGlobal(t, f), p)
			return p
		}, err: ErrSymlinkEscape},
	}
	// Case-folding: only macOS and Windows disks match a different case.
	foldErr := error(nil)
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		foldErr = ErrProdDB
	}
	cases = append(cases,
		ttDBCase{name: "case-differs-file-not-yet", setup: func(t *testing.T, f ttFix) string {
			ttMkdir(t, filepath.Join(f.home, "."+ttApp))
			return filepath.Join(f.home, "."+ttApp, "TTAPP.DB")
		}, err: foldErr},
		ttDBCase{name: "case-differs-home-not-yet", setup: func(t *testing.T, f ttFix) string {
			return filepath.Join(f.home, ".TTAPP", ttApp+".db")
		}, err: foldErr},
	)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := ttWorld(t)
			env := map[string]string{}
			for k, v := range c.env {
				v = strings.ReplaceAll(v, "@root", f.root)
				env[k] = strings.ReplaceAll(v, "@home", f.home)
			}
			path := c.setup(t, f)
			opts := append([]Option{WithRelease(c.release)}, c.opts...)
			if c.wd != "" {
				ttMkdir(t, filepath.Join(f.root, c.wd))
				opts = append(opts, WithWorkDir(filepath.Join(f.root, c.wd)))
			}
			d := ttMustNew(t, f, env, opts...)
			acc, err := d.CheckDB(path)
			ttWantErr(t, err, c.err)
			if acc != c.access {
				t.Fatalf("access = %v, want %v", acc, c.access)
			}
			if errors.Is(err, ErrProdDB) && !strings.Contains(err.Error(), ttPrefix+"_ALLOW_PROD=1") {
				t.Fatalf("message should name the way out: %v", err)
			}
		})
	}
}

// TestRuntimeCheckDBPointerWarnsDev: the dev build ignores a db: pointer and
// says where its own database stays. Who: a developer in a project whose
// config.local.jsonc points the prod db elsewhere.
func TestRuntimeCheckDBPointerWarnsDev(t *testing.T) {
	f := ttWorld(t)
	ttMkdir(t, filepath.Join(f.root, "dbs"))
	ttWrite(t, filepath.Join(f.proj, "."+ttApp, "config.local.jsonc"), `{"db": "../../dbs/ttapp.db"}`)
	d := ttMustNew(t, f, nil, WithRelease(false))
	own := filepath.Join(f.proj, "."+ttApp, profilesDir, string(ProfileDev), ttApp+".db")
	if p, err := d.Path(KindDB); err != nil || p != own {
		t.Fatalf("dev db = %q, %v; want %q", p, err, own)
	}
	ws := d.Warnings()
	if len(ws) != 1 || ws[0].Code != WarnPointerProfile || !strings.Contains(ws[0].Message, own) {
		t.Fatalf("warnings = %v", ws)
	}
	if acc, err := d.CheckDB(own); err != nil || acc != AccessReadWrite {
		t.Fatalf("own db: %v %v", acc, err)
	}
	rel := ttMustNew(t, f, nil, WithRelease(true))
	if p, _ := rel.Path(KindDB); p != filepath.Join(f.root, "dbs", ttApp+".db") {
		t.Fatalf("release db = %q", p)
	}
}

// TestRuntimeDBAccessString: the access level prints as a word.
func TestRuntimeDBAccessString(t *testing.T) {
	if AccessReadWrite.String() != "read-write" || AccessReadOnly.String() != "read-only" {
		t.Fatalf("got %q %q", AccessReadWrite, AccessReadOnly)
	}
}

// ---------------------------------------------------------------------------
// Child env, CleanEnv, ForProfile, daemon profile
// ---------------------------------------------------------------------------

// TestRuntimeChildEnv: every child process gets TTAPP_PROFILE set to the
// tool's profile, replacing any inherited value, other variables kept in
// order. Who: a dev build that starts a worker from a shell set up for prod.
func TestRuntimeChildEnv(t *testing.T) {
	f := ttWorld(t)
	dev := ttMustNew(t, f, nil, WithRelease(false))
	prod := ttMustNew(t, f, nil, WithRelease(true))
	cases := []struct {
		name string
		d    *Dirs
		in   []string
		want []string
	}{
		{"nil", dev, nil, []string{"TTAPP_PROFILE=dev"}},
		{"replaces-inherited", dev, []string{"A=1", "TTAPP_PROFILE=prod", "B=2"}, []string{"A=1", "B=2", "TTAPP_PROFILE=dev"}},
		{"replaces-duplicates", dev, []string{"TTAPP_PROFILE=prod", "X=1", "TTAPP_PROFILE=staging"}, []string{"X=1", "TTAPP_PROFILE=dev"}},
		{"keeps-similar-names", dev, []string{"TTAPP_PROFILE_X=1", "TTAPP_PROFILES=2", "TTAPP_HOME=/h", "ttapp_profile=prod"}, []string{"TTAPP_PROFILE_X=1", "TTAPP_PROFILES=2", "TTAPP_HOME=/h", "ttapp_profile=prod", "TTAPP_PROFILE=dev"}},
		{"replaces-empty-value", dev, []string{"TTAPP_PROFILE="}, []string{"TTAPP_PROFILE=dev"}},
		{"release-sets-prod", prod, []string{"TTAPP_PROFILE=dev"}, []string{"TTAPP_PROFILE=prod"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := slices.Clone(c.in)
			got := c.d.ChildEnv(in)
			if !slices.Equal(got, c.want) {
				t.Fatalf("ChildEnv = %q, want %q", got, c.want)
			}
			if !slices.Equal(in, c.in) {
				t.Fatalf("input changed: %q", in)
			}
		})
	}
}

// TestRuntimeCleanEnv: a dev task wrapper drops every TTAPP_* variable and
// nothing else (the Go twin of dev-env.sh).
func TestRuntimeCleanEnv(t *testing.T) {
	cases := []struct {
		name   string
		in     []string
		prefix string
		want   []string
	}{
		{"nil", nil, ttPrefix, []string{}},
		{"strips-prefix-only", []string{"TTAPP_HOME=/h", "PATH=/bin", "TTAPPX_HOME=/x", "TTAPP_PROFILE=prod", "TTAPP=1", "ttapp_home=/l", "TTAPP_=e", "HOME=/u"}, ttPrefix, []string{"PATH=/bin", "TTAPPX_HOME=/x", "TTAPP=1", "ttapp_home=/l", "HOME=/u"}},
		{"other-prefix", []string{"LORE_HOME=/h", "TTAPP_HOME=/t"}, "LORE", []string{"TTAPP_HOME=/t"}},
		{"nothing-to-strip", []string{"A=1", "B=2"}, ttPrefix, []string{"A=1", "B=2"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := slices.Clone(c.in)
			got := CleanEnv(in, c.prefix)
			if !slices.Equal(got, c.want) {
				t.Fatalf("CleanEnv = %q, want %q", got, c.want)
			}
			if !slices.Equal(in, c.in) {
				t.Fatalf("input changed: %q", in)
			}
		})
	}
}

// TestRuntimeForProfile: a tool computes another profile's paths with the
// same options (doctor, discovery). Who: a dev build comparing with prod.
func TestRuntimeForProfile(t *testing.T) {
	f := ttWorld(t)
	opts := []Option{WithRelease(false), WithDirName(".tt-home"), WithMode(ModeGlobal), WithRunFile(RunPID, "app.pid")}
	ttMkdir(t, filepath.Join(f.proj, ".tt-home"))
	d := ttMustNew(t, f, nil, opts...)
	cases := []struct {
		name    string
		p       Profile
		home    string
		release bool
		err     error
	}{
		{"prod", ProfileProd, ".tt-home", false, nil},
		{"dev-same", ProfileDev, ".tt-home-dev", false, nil},
		{"unknown", "qa", "", false, ErrUnknownProfile},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o, err := d.ForProfile(c.p)
			ttWantErr(t, err, c.err)
			if err != nil {
				return
			}
			if o.Profile() != c.p || o.IsRelease() != c.release || o.Mode() != ModeGlobal {
				t.Fatalf("profile %q release %v mode %q", o.Profile(), o.IsRelease(), o.Mode())
			}
			want := filepath.Join(f.home, c.home, "run", "app.pid")
			if p, err := o.RunPath(RunPID); err != nil || p != want {
				t.Fatalf("RunPath = %q, %v; want %q", p, err, want)
			}
			if d.Profile() != ProfileDev {
				t.Fatalf("ForProfile changed the original: %q", d.Profile())
			}
		})
	}
	t.Run("env-kept", func(t *testing.T) {
		env := map[string]string{ttPrefix + "_HOME": filepath.Join(f.root, "custom")}
		d := ttMustNew(t, f, env, WithRelease(true))
		o, err := d.ForProfile(ProfileDev)
		if err != nil {
			t.Fatal(err)
		}
		if o.Global().Base != filepath.Join(f.root, "custom") {
			t.Fatalf("other profile home = %q", o.Global().Base)
		}
	})
}

// TestRuntimeDaemonProfile: a client finds the daemon it talks to. Who: a
// developer running the dev client against the installed prod daemon.
func TestRuntimeDaemonProfile(t *testing.T) {
	const dk = ttPrefix + "_DAEMON_PROFILE"
	cases := []struct {
		name    string
		release bool
		env     map[string]string
		opts    []Option
		want    Profile
		home    string // global home folder name holding run/discovery.json
		newErr  error
		discErr error
		noProj  bool // work dir outside any project, ModeProject
	}{
		{name: "own-dev", want: ProfileDev, home: ".ttapp-dev"},
		{name: "own-prod", release: true, want: ProfileProd, home: "." + ttApp},
		{name: "flag-prod", opts: []Option{WithDaemonProfile(string(ProfileProd))}, want: ProfileProd, home: "." + ttApp},
		{name: "env-prod", env: map[string]string{dk: string(ProfileProd)}, want: ProfileProd, home: "." + ttApp},
		{name: "flag-beats-env", opts: []Option{WithDaemonProfile(string(ProfileDev))}, env: map[string]string{dk: string(ProfileProd)}, want: ProfileDev, home: ".ttapp-dev"},
		{name: "release-to-dev-daemon", release: true, opts: []Option{WithDaemonProfile(string(ProfileDev))}, want: ProfileDev, home: ".ttapp-dev"},
		{name: "custom-profile", opts: []Option{WithProfiles(ProfileProd, ProfileDev, "staging"), WithDaemonProfile("staging")}, want: "staging", home: ".ttapp-staging"},
		{name: "flag-unknown", opts: []Option{WithDaemonProfile("qa")}, newErr: ErrUnknownProfile},
		{name: "env-unknown", env: map[string]string{dk: "qa"}, newErr: ErrUnknownProfile},
		{name: "no-project-other-profile", noProj: true, opts: []Option{WithDaemonProfile(string(ProfileProd))}, want: ProfileProd, discErr: ErrNoProject},
		{name: "no-project-own-profile", noProj: true, want: ProfileDev, discErr: ErrNoProject},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := ttWorld(t)
			ttMkdir(t, filepath.Join(f.root, "elsewhere"))
			opts := append([]Option{WithRelease(c.release), WithMode(ModeGlobal)}, c.opts...)
			if c.noProj {
				opts = append(opts, WithMode(ModeProject), WithWorkDir(filepath.Join(f.root, "elsewhere")))
			}
			d, err := ttNew(t, f, c.env, opts...)
			ttWantErr(t, err, c.newErr)
			if err != nil {
				return
			}
			if d.DaemonProfile() != c.want {
				t.Fatalf("DaemonProfile = %q, want %q", d.DaemonProfile(), c.want)
			}
			path, p, err := d.DaemonDiscovery()
			ttWantErr(t, err, c.discErr)
			if p != c.want {
				t.Fatalf("discovery profile = %q, want %q", p, c.want)
			}
			if err != nil {
				return
			}
			if want := filepath.Join(f.home, c.home, "run", "discovery.json"); path != want {
				t.Fatalf("discovery = %q, want %q", path, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Run paths and sockets
// ---------------------------------------------------------------------------

// TestRuntimeRunPaths: a daemon and its clients find the lock, pid,
// discovery and socket files in run/ of the right home.
func TestRuntimeRunPaths(t *testing.T) {
	f := ttWorld(t)
	devRun := filepath.Join(f.home, ".ttapp-dev", "run")
	prodProjRun := filepath.Join(f.proj, "."+ttApp, "run")
	cases := []struct {
		name    string
		opts    []Option
		file    RunFile
		want    string
		newErr  error
		pathErr error
	}{
		{name: "lock", file: RunLock, want: filepath.Join(devRun, "lock")},
		{name: "daemon-lock", file: RunDaemonLock, want: filepath.Join(devRun, "daemon.lock")},
		{name: "pid", file: RunPID, want: filepath.Join(devRun, "ttapp.pid")},
		{name: "discovery", file: RunDiscovery, want: filepath.Join(devRun, "discovery.json")},
		{name: "socket", file: RunSocket, want: filepath.Join(devRun, "ttapp.sock")},
		{name: "socket-file", file: RunSocketFile, want: filepath.Join(devRun, "socket")},
		{name: "renamed-pid", opts: []Option{WithRunFile(RunPID, "daemon.pid")}, file: RunPID, want: filepath.Join(devRun, "daemon.pid")},
		{name: "renamed-socket", opts: []Option{WithRunFile(RunSocket, "s.sock")}, file: RunSocket, want: filepath.Join(devRun, "s.sock")},
		{name: "renamed-kind-run", opts: []Option{WithKind(KindRun, Folder("var/run"))}, file: RunLock, want: filepath.Join(f.home, ".ttapp-dev", "var", "run", "lock")},
		{name: "prod-project", opts: []Option{WithRelease(true), WithMode(ModeProject)}, file: RunLock, want: filepath.Join(prodProjRun, "lock")},
		{name: "dev-project", opts: []Option{WithMode(ModeProject)}, file: RunLock, want: filepath.Join(prodProjRun[:len(prodProjRun)-len("run")], profilesDir, string(ProfileDev), "run", "lock")},
		{name: "unknown-run-file", file: "nope", pathErr: ErrInvalidOption},
		{name: "run-file-with-separator", opts: []Option{WithRunFile(RunPID, "a/b.pid")}, newErr: ErrInvalidOption},
		{name: "run-file-dotdot", opts: []Option{WithRunFile(RunLock, "..")}, newErr: ErrInvalidOption},
		{name: "run-file-empty", opts: []Option{WithRunFile(RunLock, "")}, newErr: ErrInvalidOption},
		{name: "no-project", opts: []Option{WithMode(ModeProject), WithWorkDir(f.root)}, file: RunLock, pathErr: ErrNoProject},
		{name: "run-removed", opts: []Option{WithoutKind(KindRun)}, file: RunLock, pathErr: ErrUnknownKind},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := ttNew(t, f, nil, append([]Option{WithRelease(false), WithMode(ModeGlobal)}, c.opts...)...)
			ttWantErr(t, err, c.newErr)
			if err != nil {
				return
			}
			p, err := d.RunPath(c.file)
			ttWantErr(t, err, c.pathErr)
			if p != c.want {
				t.Fatalf("RunPath = %q, want %q", p, c.want)
			}
			if c.pathErr == nil && ttExists(filepath.Dir(p)) {
				t.Fatalf("RunPath created %s", filepath.Dir(p))
			}
		})
	}
}

// ttSocketHash is the expected fallback name: SHA-256 of the real run/.
func ttSocketHash(run string) string {
	sum := sha256.Sum256([]byte(realPath(run)))
	return hex.EncodeToString(sum[:])[:12]
}

// TestRuntimeSocket: a daemon listens where a client will find it. Who: a
// user whose project path is so long the socket would not fit run/.
func TestRuntimeSocket(t *testing.T) {
	t.Run("short-under-run", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the socket is always a named pipe on Windows (TestWinNamedPipeShape)")
		}
		f := ttWorld(t)
		// The temp root is deep: raise the limit so run/ fits.
		d := ttMustNew(t, f, nil, WithRelease(false), WithMode(ModeGlobal), WithSocketMaxLen(4096))
		s, err := d.Socket()
		if err != nil {
			t.Fatal(err)
		}
		run := filepath.Join(f.home, ".ttapp-dev", "run")
		want := Socket{Path: filepath.Join(run, "ttapp.sock"), PointerFile: filepath.Join(run, "socket"), Dir: run}
		if s != want {
			t.Fatalf("Socket = %+v, want %+v", s, want)
		}
		if ttExists(run) {
			t.Fatal("Socket created run/")
		}
	})
	t.Run("exact-max-len-stays", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the socket is always a named pipe on Windows (TestWinNamedPipeShape)")
		}
		f := ttWorld(t)
		p := filepath.Join(f.home, ".ttapp-dev", "run", "ttapp.sock")
		d := ttMustNew(t, f, nil, WithRelease(false), WithMode(ModeGlobal), WithSocketMaxLen(len(p)))
		if s, _ := d.Socket(); s.Fallback || s.Path != p {
			t.Fatalf("Socket = %+v", s)
		}
	})
	t.Run("no-project", func(t *testing.T) {
		f := ttWorld(t)
		d := ttMustNew(t, f, nil, WithRelease(false), WithWorkDir(f.root))
		if _, err := d.Socket(); !errors.Is(err, ErrNoProject) {
			t.Fatalf("Socket err = %v", err)
		}
		if _, err := d.EnsureSocketDir(); !errors.Is(err, ErrNoProject) {
			t.Fatalf("EnsureSocketDir err = %v", err)
		}
	})
	t.Run("invalid-max-len", func(t *testing.T) {
		f := ttWorld(t)
		if _, err := ttNew(t, f, nil, WithSocketMaxLen(0)); !errors.Is(err, ErrInvalidOption) {
			t.Fatalf("err = %v", err)
		}
	})
	if runtime.GOOS != "linux" {
		t.Skip("the XDG_RUNTIME_DIR fallback is Linux only")
	}
	// fallback resolves a socket for profile flag p in home with run/ longer
	// than the limit, XDG_RUNTIME_DIR pointing into the test root.
	fallback := func(t *testing.T, f ttFix, xdg, profile string, extra ...Option) (*Dirs, Socket) {
		t.Helper()
		env := map[string]string{"XDG_RUNTIME_DIR": xdg}
		opts := append([]Option{WithRelease(false), WithMode(ModeGlobal), WithSocketMaxLen(10), WithProfile(profile)}, extra...)
		d := ttMustNew(t, f, env, opts...)
		s, err := d.Socket()
		if err != nil {
			t.Fatal(err)
		}
		return d, s
	}
	t.Run("fallback-xdg", func(t *testing.T) {
		f := ttWorld(t)
		xdg := filepath.Join(f.root, "xdg")
		ttMkdir(t, xdg)
		d, s := fallback(t, f, xdg, string(ProfileDev))
		run, _ := d.Path(KindRun)
		dir := filepath.Join(xdg, "ttapp-dev")
		want := Socket{Path: filepath.Join(dir, ttSocketHash(run)+".sock"), Fallback: true, PointerFile: filepath.Join(run, "socket"), Dir: dir}
		if s != want {
			t.Fatalf("Socket = %+v, want %+v", s, want)
		}
		hash := strings.TrimSuffix(filepath.Base(s.Path), ".sock")
		if len(hash) != 12 || strings.Trim(hash, "0123456789abcdef") != "" {
			t.Fatalf("hash %q is not 12 hex", hash)
		}
		if ttExists(dir) {
			t.Fatal("Socket created the fallback folder")
		}
	})
	t.Run("fallback-prod-name", func(t *testing.T) {
		f := ttWorld(t)
		xdg := filepath.Join(f.root, "xdg")
		ttMkdir(t, xdg)
		_, s := fallback(t, f, xdg, string(ProfileProd))
		if s.Dir != filepath.Join(xdg, "ttapp") {
			t.Fatalf("prod fallback dir = %q", s.Dir)
		}
	})
	t.Run("fallback-hash-stable-and-unique", func(t *testing.T) {
		f := ttWorld(t)
		xdg := filepath.Join(f.root, "xdg")
		ttMkdir(t, xdg)
		_, a := fallback(t, f, xdg, string(ProfileDev))
		_, b := fallback(t, f, xdg, string(ProfileDev))
		if a.Path != b.Path {
			t.Fatalf("same home, two paths: %q %q", a.Path, b.Path)
		}
		_, prod := fallback(t, f, xdg, string(ProfileProd))
		if filepath.Base(prod.Path) == filepath.Base(a.Path) {
			t.Fatalf("dev and prod share a hash: %q", a.Path)
		}
		g := f
		g.home = filepath.Join(f.root, "home2")
		ttMkdir(t, g.home)
		_, other := fallback(t, g, xdg, string(ProfileDev))
		if filepath.Base(other.Path) == filepath.Base(a.Path) {
			t.Fatalf("two homes share a hash: %q", a.Path)
		}
		// A symlinked spelling of the same home gives the same hash: the
		// hash is of the real path of run/.
		h := f
		h.home = filepath.Join(f.root, "homelink")
		ttSymlink(t, f.home, h.home)
		_, linked := fallback(t, h, xdg, string(ProfileDev))
		if filepath.Base(linked.Path) != filepath.Base(a.Path) {
			t.Fatalf("symlinked home changed the hash: %q vs %q", linked.Path, a.Path)
		}
	})
	if strings.HasPrefix(os.TempDir(), "/tmp") {
		t.Log("os.TempDir is under /tmp: the /tmp fallback panic cases cannot be told apart from the test root")
		return
	}
	for _, c := range []struct{ name, xdg string }{
		{"no-xdg-goes-to-tmp-guard-panics", ""},
		{"relative-xdg-ignored-guard-panics", "xdg"},
		{"missing-xdg-ignored-guard-panics", "@root/no-such"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := ttWorld(t)
			env := map[string]string{}
			if c.xdg != "" {
				env["XDG_RUNTIME_DIR"] = strings.ReplaceAll(c.xdg, "@root", f.root)
			}
			d := ttMustNew(t, f, env, WithRelease(false), WithMode(ModeGlobal), WithSocketMaxLen(10))
			msg := ttPanic(func() { _, _ = d.Socket() })
			if !strings.Contains(msg, "outside the test root") || !strings.Contains(msg, "/tmp/ttapp-dev-") {
				t.Fatalf("panic = %q", msg)
			}
			msg = ttPanic(func() { _, _ = d.EnsureSocketDir() })
			if !strings.Contains(msg, "outside the test root") {
				t.Fatalf("EnsureSocketDir panic = %q", msg)
			}
		})
	}
}

// TestRuntimeEnsureSocketDir: before listening, the daemon creates run/ and
// the private fallback folder; a planted symlink is refused.
func TestRuntimeEnsureSocketDir(t *testing.T) {
	perm := func(t *testing.T, dir string) os.FileMode {
		t.Helper()
		fi, err := os.Lstat(dir)
		if err != nil {
			t.Fatal(err)
		}
		return fi.Mode().Perm()
	}
	t.Run("under-run", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the socket is always a named pipe on Windows (TestWinNamedPipeShape)")
		}
		f := ttWorld(t)
		d := ttMustNew(t, f, nil, WithRelease(false), WithMode(ModeGlobal), WithSocketMaxLen(4096))
		s, err := d.EnsureSocketDir()
		if err != nil {
			t.Fatal(err)
		}
		if s.Fallback || !isDir(s.Dir) || s.Dir != filepath.Join(f.home, ".ttapp-dev", "run") {
			t.Fatalf("Socket = %+v", s)
		}
		if runtime.GOOS != "windows" && perm(t, s.Dir) != 0o700 {
			t.Fatalf("run/ mode = %v", perm(t, s.Dir))
		}
		// Twice is fine.
		if _, err := d.EnsureSocketDir(); err != nil {
			t.Fatal(err)
		}
	})
	if runtime.GOOS != "linux" {
		t.Skip("the XDG_RUNTIME_DIR fallback is Linux only")
	}
	newFallback := func(t *testing.T) (ttFix, *Dirs, string) {
		f := ttWorld(t)
		xdg := filepath.Join(f.root, "xdg")
		ttMkdir(t, xdg)
		d := ttMustNew(t, f, map[string]string{"XDG_RUNTIME_DIR": xdg}, WithRelease(false), WithMode(ModeGlobal), WithSocketMaxLen(10))
		return f, d, filepath.Join(xdg, "ttapp-dev")
	}
	t.Run("fallback-creates-both", func(t *testing.T) {
		f, d, dir := newFallback(t)
		s, err := d.EnsureSocketDir()
		if err != nil {
			t.Fatal(err)
		}
		if !s.Fallback || s.Dir != dir || !isDir(dir) || perm(t, dir) != 0o700 {
			t.Fatalf("Socket = %+v", s)
		}
		run := filepath.Join(f.home, ".ttapp-dev", "run")
		if !isDir(run) || perm(t, run) != 0o700 || filepath.Dir(s.PointerFile) != run {
			t.Fatalf("run/ not created: %+v", s)
		}
		// A listener fits at the fallback path (the reason it exists); the
		// test root itself may be too deep for that, so only when it fits.
		if len(s.Path) >= 100 {
			return
		}
		if l, err := net.Listen("unix", s.Path); err == nil {
			_ = l.Close()
		} else {
			t.Fatalf("listen on %s: %v", s.Path, err)
		}
	})
	t.Run("fallback-existing-own-folder", func(t *testing.T) {
		// An existing folder we own is accepted as it is: its mode is not
		// tightened (only we could have created it).
		_, d, dir := newFallback(t)
		ttMkdir(t, dir)
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := d.EnsureSocketDir(); err != nil {
			t.Fatalf("existing own folder: %v", err)
		}
		if perm(t, dir) != 0o755 {
			t.Fatalf("mode changed to %v", perm(t, dir))
		}
	})
	t.Run("fallback-hash-stable-across-creation", func(t *testing.T) {
		_, d, _ := newFallback(t)
		before, err := d.Socket()
		if err != nil {
			t.Fatal(err)
		}
		after, err := d.EnsureSocketDir()
		if err != nil || after != before {
			t.Fatalf("before %+v, after %+v, %v", before, after, err)
		}
	})
	t.Run("run-blocked-by-a-file", func(t *testing.T) {
		f, d, dir := newFallback(t)
		ttWrite(t, filepath.Join(f.home, ".ttapp-dev"), "a file where the home should be")
		if _, err := d.EnsureSocketDir(); err == nil {
			t.Fatal("want an error")
		}
		if ttExists(dir) {
			t.Fatal("fallback folder created though run/ failed")
		}
	})
	t.Run("fallback-symlink-refused", func(t *testing.T) {
		f, d, dir := newFallback(t)
		ttMkdir(t, filepath.Join(f.root, "attacker"))
		ttSymlink(t, filepath.Join(f.root, "attacker"), dir)
		_, err := d.EnsureSocketDir()
		ttWantErr(t, err, ErrUnsafeTemp)
	})
	t.Run("fallback-is-a-file-refused", func(t *testing.T) {
		_, d, dir := newFallback(t)
		ttWrite(t, dir, "x")
		_, err := d.EnsureSocketDir()
		ttWantErr(t, err, ErrUnsafeTemp)
	})
}

// ---------------------------------------------------------------------------
// Legacy move
// ---------------------------------------------------------------------------

// ttLegacy is a world with an old home ~/.ttold holding data, and a release
// build configured to move it.
type ttLegacy struct {
	ttFix
	from, to string
}

// ttLegacyWorld makes the old home with data.txt, run/ and a nested file.
func ttLegacyWorld(t *testing.T) ttLegacy {
	t.Helper()
	f := ttWorld(t)
	l := ttLegacy{ttFix: f, from: filepath.Join(f.home, ttOld), to: filepath.Join(f.home, "."+ttApp)}
	ttWrite(t, filepath.Join(l.from, "data.txt"), "user data")
	ttWrite(t, filepath.Join(l.from, "notes", "a.md"), "note a")
	ttMkdir(t, filepath.Join(l.from, "run"))
	return l
}

// dirs resolves a release build that knows the legacy home.
func (l ttLegacy) dirs(t *testing.T, env map[string]string, extra ...Option) *Dirs {
	t.Helper()
	opts := append([]Option{WithRelease(true), WithMode(ModeGlobal), WithLegacyHome(LegacyHome{Dir: ttOld})}, extra...)
	return ttMustNew(t, l.ttFix, env, opts...)
}

// checkMoved asserts the data is in the new home and the old one holds only
// MOVED.txt naming it.
func (l ttLegacy) checkMoved(t *testing.T) {
	t.Helper()
	if ttRead(t, filepath.Join(l.to, "data.txt")) != "user data" || ttRead(t, filepath.Join(l.to, "notes", "a.md")) != "note a" {
		t.Fatal("data not intact in the new home")
	}
	if got := ttRead(t, filepath.Join(l.from, movedFile)); got != "moved to "+l.to+"\n" {
		t.Fatalf("MOVED.txt = %q", got)
	}
	ents, _ := os.ReadDir(l.from)
	if len(ents) != 1 {
		t.Fatalf("old home holds %d entries, want only MOVED.txt", len(ents))
	}
	if !ttExists(filepath.Join(l.home, "."+ttApp+".moving.lock")) {
		t.Fatal("the moving lock file was deleted")
	}
	if ttExists(filepath.Join(l.to, legacyMarkerFile)) {
		t.Fatal(".legacy-move left behind")
	}
}

// checkUntouched asserts nothing moved.
func (l ttLegacy) checkUntouched(t *testing.T) {
	t.Helper()
	if ttRead(t, filepath.Join(l.from, "data.txt")) != "user data" {
		t.Fatal("old data changed")
	}
	if ttExists(filepath.Join(l.from, movedFile)) {
		t.Fatal("MOVED.txt written though nothing moved")
	}
}

// TestRuntimeLegacyMove: a user upgrades to a release that renamed its home
// from ~/.ttold to ~/.ttapp. Expected: one move on first run, data intact,
// MOVED.txt left, every refusal leaves both folders as they were.
func TestRuntimeLegacyMove(t *testing.T) {
	ctx := context.Background()
	t.Run("no-legacy-option", func(t *testing.T) {
		l := ttLegacyWorld(t)
		d := ttMustNew(t, l.ttFix, nil, WithRelease(true), WithMode(ModeGlobal))
		res, err := d.MoveLegacy(ctx)
		if err != nil || res != (MoveResult{Status: MoveNone}) {
			t.Fatalf("res = %+v, %v", res, err)
		}
		l.checkUntouched(t)
	})
	t.Run("old-missing", func(t *testing.T) {
		f := ttWorld(t)
		l := ttLegacy{ttFix: f, from: filepath.Join(f.home, ttOld), to: filepath.Join(f.home, "."+ttApp)}
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveNone || res.To != l.to {
			t.Fatalf("res = %+v, %v", res, err)
		}
		if ttExists(l.to) {
			t.Fatal("new home created")
		}
	})
	t.Run("same-disk-move-then-already", func(t *testing.T) {
		l := ttLegacyWorld(t)
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone || res.From != l.from || res.To != l.to || res.Note != "renamed (same disk)" {
			t.Fatalf("res = %+v, %v", res, err)
		}
		l.checkMoved(t)
		res, err = l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveAlready || res.From != l.from {
			t.Fatalf("second run = %+v, %v", res, err)
		}
		l.checkMoved(t)
	})
	t.Run("old-is-a-file", func(t *testing.T) {
		f := ttWorld(t)
		ttWrite(t, filepath.Join(f.home, ttOld), "not a folder")
		l := ttLegacy{ttFix: f}
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveNone {
			t.Fatalf("res = %+v, %v", res, err)
		}
	})
	t.Run("new-home-path-is-a-file", func(t *testing.T) {
		l := ttLegacyWorld(t)
		ttWrite(t, l.to, "a file")
		if _, err := l.dirs(t, nil).MoveLegacy(ctx); err == nil {
			t.Fatal("want a move error")
		}
		l.checkUntouched(t)
		if ttRead(t, l.to) != "a file" {
			t.Fatal("file at the new home changed")
		}
	})
	t.Run("empty-old-home", func(t *testing.T) {
		f := ttWorld(t)
		l := ttLegacy{ttFix: f, from: filepath.Join(f.home, ttOld), to: filepath.Join(f.home, "."+ttApp)}
		ttMkdir(t, l.from)
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone || !isDir(l.to) {
			t.Fatalf("res = %+v, %v", res, err)
		}
	})
	t.Run("old-home-is-a-symlink", func(t *testing.T) {
		// ~/.ttold is a link to the real data folder: the link moves, the
		// data stays where it is and is reached through ~/.ttapp.
		f := ttWorld(t)
		real := filepath.Join(f.root, "data")
		ttWrite(t, filepath.Join(real, "data.txt"), "user data")
		l := ttLegacy{ttFix: f, from: filepath.Join(f.home, ttOld), to: filepath.Join(f.home, "."+ttApp)}
		ttSymlink(t, real, l.from)
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone {
			t.Fatalf("res = %+v, %v", res, err)
		}
		if ttRead(t, filepath.Join(l.to, "data.txt")) != "user data" || ttRead(t, filepath.Join(real, "data.txt")) != "user data" {
			t.Fatal("data not reachable through the new home")
		}
		if ttRead(t, filepath.Join(l.from, movedFile)) != "moved to "+l.to+"\n" {
			t.Fatal("MOVED.txt missing")
		}
	})
	t.Run("both-exist", func(t *testing.T) {
		l := ttLegacyWorld(t)
		ttWrite(t, filepath.Join(l.to, "new.txt"), "new data")
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveBothExist || res.Note != "both exist; using "+l.to {
			t.Fatalf("res = %+v, %v", res, err)
		}
		l.checkUntouched(t)
		if ttRead(t, filepath.Join(l.to, "new.txt")) != "new data" || ttExists(filepath.Join(l.to, "data.txt")) {
			t.Fatal("new home changed")
		}
	})
	skipped := []struct {
		name string
		env  map[string]string
		opts []Option
		flag bool // pass --home <root>/h
		note string
	}{
		{"dev-build", nil, []Option{WithRelease(false)}, false, "profile dev never runs a legacy move"},
		{"release-with-dev-flag", nil, []Option{WithProfile(string(ProfileDev))}, false, "profile dev never runs a legacy move"},
		{"home-flag", nil, nil, true, "home set by --home or TTAPP_HOME"},
		{"home-env", map[string]string{ttPrefix + "_HOME": "@root/h"}, nil, false, "home set by --home or TTAPP_HOME"},
		{"no-user-home", nil, []Option{WithHomeDir(func() (string, error) { return "", errors.New("no $HOME") })}, false, "no home folder"},
	}
	for _, c := range skipped {
		t.Run("skipped-"+c.name, func(t *testing.T) {
			l := ttLegacyWorld(t)
			env := map[string]string{}
			for k, v := range c.env {
				env[k] = strings.ReplaceAll(v, "@root", l.root)
			}
			opts := c.opts
			if c.flag {
				opts = append(opts, WithHome(filepath.Join(l.root, "h")))
			}
			res, err := l.dirs(t, env, opts...).MoveLegacy(ctx)
			if err != nil || res.Status != MoveSkipped || res.Note != c.note {
				t.Fatalf("res = %+v, %v", res, err)
			}
			l.checkUntouched(t)
			if ttExists(l.to) {
				t.Fatal("new home created")
			}
		})
	}
	t.Run("release-ignores-env-dev-and-moves", func(t *testing.T) {
		l := ttLegacyWorld(t)
		res, err := l.dirs(t, map[string]string{ttPrefix + "_PROFILE": string(ProfileDev)}).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone {
			t.Fatalf("res = %+v, %v", res, err)
		}
		l.checkMoved(t)
	})
	t.Run("several-entries-first-existing-wins", func(t *testing.T) {
		l := ttLegacyWorld(t)
		third := filepath.Join(l.home, ".third")
		ttWrite(t, filepath.Join(third, "t.txt"), "third")
		d := ttMustNew(t, l.ttFix, nil, WithRelease(true), WithMode(ModeGlobal),
			WithLegacyHome(LegacyHome{Dir: ".first"}), WithLegacyHome(LegacyHome{Dir: ttOld}), WithLegacyHome(LegacyHome{Dir: ".third"}))
		res, err := d.MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone || res.From != l.from {
			t.Fatalf("res = %+v, %v", res, err)
		}
		l.checkMoved(t)
		if ttRead(t, filepath.Join(third, "t.txt")) != "third" {
			t.Fatal("third legacy home touched")
		}
	})
	t.Run("invalid-legacy-dir", func(t *testing.T) {
		f := ttWorld(t)
		for _, dir := range []string{"", ".", "..", "a/b"} {
			if _, err := ttNew(t, f, nil, WithLegacyHome(LegacyHome{Dir: dir})); !errors.Is(err, ErrInvalidOption) {
				t.Errorf("Dir %q: err = %v", dir, err)
			}
		}
	})
	t.Run("old-daemon-lock-held", func(t *testing.T) {
		l := ttLegacyWorld(t)
		lk, err := flock.TryLock(filepath.Join(l.from, "run", "daemon.lock"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lk.Release() }()
		_, err = l.dirs(t, nil).MoveLegacy(ctx)
		ttWantErr(t, err, ErrBusy)
		if !strings.Contains(err.Error(), "in use by a running daemon") {
			t.Fatalf("message = %v", err)
		}
		l.checkUntouched(t)
		if ttExists(l.to) {
			t.Fatal("new home created")
		}
		// The daemon stops: the next run moves.
		_ = lk.Release()
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone {
			t.Fatalf("after release = %+v, %v", res, err)
		}
		l.checkMoved(t)
	})
	t.Run("old-daemon-lock-free-file-kept", func(t *testing.T) {
		l := ttLegacyWorld(t)
		ttWrite(t, filepath.Join(l.from, "run", "daemon.lock"), "")
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone {
			t.Fatalf("res = %+v, %v", res, err)
		}
		if !ttExists(filepath.Join(l.to, "run", "daemon.lock")) {
			t.Fatal("daemon.lock not moved with the data")
		}
	})
	t.Run("probe-never-creates-lock", func(t *testing.T) {
		l := ttLegacyWorld(t)
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone {
			t.Fatalf("res = %+v, %v", res, err)
		}
		if ttExists(filepath.Join(l.to, "run", "daemon.lock")) {
			t.Fatal("probing created run/daemon.lock")
		}
	})
	t.Run("custom-daemon-locks-held", func(t *testing.T) {
		l := ttLegacyWorld(t)
		ttMkdir(t, filepath.Join(l.from, "pty"))
		lk, err := flock.TryLock(filepath.Join(l.from, "pty", "pty.lock"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lk.Release() }()
		d := ttMustNew(t, l.ttFix, nil, WithRelease(true), WithMode(ModeGlobal),
			WithLegacyHome(LegacyHome{Dir: ttOld, DaemonLocks: []string{"run/daemon.lock", "pty/pty.lock"}}))
		_, err = d.MoveLegacy(ctx)
		ttWantErr(t, err, ErrBusy)
		l.checkUntouched(t)
	})
	t.Run("custom-daemon-locks-replace-default", func(t *testing.T) {
		l := ttLegacyWorld(t)
		lk, err := flock.TryLock(filepath.Join(l.from, "run", "daemon.lock"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lk.Release() }()
		d := ttMustNew(t, l.ttFix, nil, WithRelease(true), WithMode(ModeGlobal),
			WithLegacyHome(LegacyHome{Dir: ttOld, DaemonLocks: []string{"pty/pty.lock"}}))
		res, err := d.MoveLegacy(ctx)
		if runtime.GOOS == "windows" {
			// Windows cannot rename a folder while a file in it is open
			// (the lock handle above), so the move fails and nothing moves.
			if err == nil {
				t.Fatalf("res = %+v; want an error renaming a folder with an open file", res)
			}
			l.checkUntouched(t)
			return
		}
		if err != nil || res.Status != MoveDone {
			t.Fatalf("res = %+v, %v", res, err)
		}
	})
	t.Run("daemon-lock-probe-error", func(t *testing.T) {
		l := ttLegacyWorld(t)
		ttMkdir(t, filepath.Join(l.from, "run", "daemon.lock")) // a folder: cannot be opened for writing
		_, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err == nil || errors.Is(err, ErrBusy) {
			t.Fatalf("err = %v, want a probe error", err)
		}
		l.checkUntouched(t)
	})
	t.Run("moving-lock-held", func(t *testing.T) {
		l := ttLegacyWorld(t)
		lk, err := flock.TryLock(filepath.Join(l.home, ".ttapp.moving.lock"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lk.Release() }()
		_, err = l.dirs(t, nil).MoveLegacy(ctx)
		ttWantErr(t, err, ErrBusy)
		if !strings.Contains(err.Error(), "another process is moving") {
			t.Fatalf("message = %v", err)
		}
		l.checkUntouched(t)
	})
	t.Run("moving-lock-unopenable", func(t *testing.T) {
		l := ttLegacyWorld(t)
		ttMkdir(t, filepath.Join(l.home, ".ttapp.moving.lock"))
		_, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err == nil || errors.Is(err, ErrBusy) {
			t.Fatalf("err = %v, want an open error", err)
		}
		l.checkUntouched(t)
	})
	t.Run("cancelled-context", func(t *testing.T) {
		l := ttLegacyWorld(t)
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := l.dirs(t, nil).MoveLegacy(cctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
		l.checkUntouched(t)
		if ttExists(l.to) {
			t.Fatal("new home created")
		}
	})
}

// TestRuntimeLegacyCrashRecovery: the move crashed half-way on an earlier
// run. Expected: the next run finishes it, never reports both-exist for its
// own half-done move, never loses data.
func TestRuntimeLegacyCrashRecovery(t *testing.T) {
	ctx := context.Background()
	t.Run("marker-in-new-old-still-there", func(t *testing.T) {
		// Crash after <new>.moving-tmp -> new, before old was renamed.
		l := ttLegacyWorld(t)
		ttWrite(t, filepath.Join(l.to, "data.txt"), "user data")
		ttWrite(t, filepath.Join(l.to, "notes", "a.md"), "note a")
		ttWrite(t, filepath.Join(l.to, legacyMarkerFile), l.from+"\n")
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone || !strings.HasPrefix(res.Note, "copied (other disk)") {
			t.Fatalf("res = %+v, %v", res, err)
		}
		l.checkMoved(t)
		kept, _ := filepath.Glob(l.from + ".moved-*")
		if len(kept) != 1 || ttRead(t, filepath.Join(kept[0], "data.txt")) != "user data" {
			t.Fatalf("old folder not kept as .moved-<date>: %v", kept)
		}
	})
	t.Run("marker-in-new-old-already-renamed", func(t *testing.T) {
		// BX2: crash after old -> old.moved-<date>, before the marker went.
		l := ttLegacyWorld(t)
		ttWrite(t, filepath.Join(l.to, "data.txt"), "user data")
		ttWrite(t, filepath.Join(l.to, "notes", "a.md"), "note a")
		ttWrite(t, filepath.Join(l.to, legacyMarkerFile), l.from+"\n")
		if err := os.Rename(l.from, l.from+".moved-20261101"); err != nil {
			t.Fatal(err)
		}
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone {
			t.Fatalf("res = %+v, %v", res, err)
		}
		l.checkMoved(t)
		if ttRead(t, filepath.Join(l.from+".moved-20261101", "data.txt")) != "user data" {
			t.Fatal("kept old folder changed")
		}
		res, err = l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveAlready {
			t.Fatalf("third run = %+v, %v", res, err)
		}
	})
	t.Run("marker-naming-another-old-home", func(t *testing.T) {
		// The marker is from a different legacy name: this one is gone, so
		// there is nothing of ours to finish.
		f := ttWorld(t)
		l := ttLegacy{ttFix: f, from: filepath.Join(f.home, ttOld), to: filepath.Join(f.home, "."+ttApp)}
		ttWrite(t, filepath.Join(l.to, legacyMarkerFile), filepath.Join(f.home, ".other")+"\n")
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveNone {
			t.Fatalf("res = %+v, %v", res, err)
		}
	})
	t.Run("stale-moving-tmp-same-disk", func(t *testing.T) {
		// A <new>.moving-tmp from a crashed copy. On one disk the move is a
		// rename, so the copy is not ours to need; the data still moves and
		// the user's old folder is never mixed with the stale copy.
		l := ttLegacyWorld(t)
		tmp := l.to + ".moving-tmp"
		ttWrite(t, filepath.Join(tmp, "data.txt"), "half copy")
		res, err := l.dirs(t, nil).MoveLegacy(ctx)
		if err != nil || res.Status != MoveDone {
			t.Fatalf("res = %+v, %v", res, err)
		}
		l.checkMoved(t)
	})
	t.Run("copy-tree-redo-after-crash", func(t *testing.T) {
		// The cross-disk branch cannot run on one disk; drive its steps as
		// moveOne does: remove our partial copy, copy, count, compare.
		l := ttLegacyWorld(t)
		ttSymlink(t, "data.txt", filepath.Join(l.from, "link"))
		if err := os.Chmod(filepath.Join(l.from, "notes", "a.md"), 0o640); err != nil {
			t.Fatal(err)
		}
		sock := filepath.Join(l.from, "s")
		if ln, err := net.Listen("unix", sock); err == nil {
			defer func() { _ = ln.Close() }()
		}
		tmp := l.to + ".moving-tmp"
		ttWrite(t, filepath.Join(tmp, "data.txt"), "half copy")
		if _, _, err := copyTree(ctx, l.from, tmp); err == nil {
			t.Fatal("copy over a stale tmp should fail (O_EXCL): that is why moveOne removes it first")
		}
		if err := os.RemoveAll(tmp); err != nil {
			t.Fatal(err)
		}
		n, size, err := copyTree(ctx, l.from, tmp)
		if err != nil {
			t.Fatal(err)
		}
		n2, size2, err := countTree(tmp)
		if err != nil || n != n2 || size != size2 || n != 3 || size != int64(len("user data")+len("note a")) {
			t.Fatalf("copy %d/%d, count %d/%d, %v", n, size, n2, size2, err)
		}
		if target, err := os.Readlink(filepath.Join(tmp, "link")); err != nil || target != "data.txt" {
			t.Fatalf("symlink not copied as a link: %q %v", target, err)
		}
		if fi, err := os.Stat(filepath.Join(tmp, "notes", "a.md")); err != nil || runtime.GOOS != "windows" && fi.Mode().Perm() != 0o640 {
			t.Fatalf("mode not kept: %v %v", fi, err)
		}
		if ttExists(filepath.Join(tmp, "s")) {
			t.Fatal("socket copied")
		}
		// The marker does not count toward the byte total.
		ttWrite(t, filepath.Join(tmp, legacyMarkerFile), l.from+"\n")
		if _, size3, _ := countTree(tmp); size3 != size {
			t.Fatalf("marker counted: %d vs %d", size3, size)
		}
	})
	t.Run("copy-tree-cancelled", func(t *testing.T) {
		l := ttLegacyWorld(t)
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, _, err := copyTree(cctx, l.from, l.to+".moving-tmp"); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("copy-tree-missing-source", func(t *testing.T) {
		f := ttWorld(t)
		if _, _, err := copyTree(ctx, filepath.Join(f.root, "none"), filepath.Join(f.root, "dst")); err == nil {
			t.Fatal("want an error")
		}
		if _, _, err := countTree(filepath.Join(f.root, "none")); err == nil {
			t.Fatal("want an error")
		}
	})
}

// TestRuntimeLegacyConcurrent: two processes of the new release start at
// the same moment. Expected: exactly one moves; the other is busy or finds
// it done; the data is intact. Run with -race.
func TestRuntimeLegacyConcurrent(t *testing.T) {
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			l := ttLegacyWorld(t)
			const n = 4
			var (
				wg    sync.WaitGroup
				start = make(chan struct{})
				res   [n]MoveResult
				errs  [n]error
			)
			ds := make([]*Dirs, n)
			for j := range ds {
				ds[j] = l.dirs(t, nil)
			}
			for j := 0; j < n; j++ {
				wg.Add(1)
				go func(j int) {
					defer wg.Done()
					<-start
					res[j], errs[j] = ds[j].MoveLegacy(ctx)
				}(j)
			}
			close(start)
			wg.Wait()
			done := 0
			for j := 0; j < n; j++ {
				switch {
				case errs[j] == nil && res[j].Status == MoveDone:
					done++
				case errors.Is(errs[j], ErrBusy):
				case errs[j] == nil && (res[j].Status == MoveAlready || res[j].Status == MoveBothExist || res[j].Status == MoveNone):
				default:
					t.Fatalf("goroutine %d: %+v, %v", j, res[j], errs[j])
				}
			}
			if done != 1 {
				t.Fatalf("%d moves, want exactly 1 (%+v %v)", done, res, errs)
			}
			l.checkMoved(t)
		})
	}
}

// ---------------------------------------------------------------------------
// ForTest and the test guard
// ---------------------------------------------------------------------------

// TestRuntimeForTest: a tool's test isolates itself with one call. Who: a
// tool author whose shell has TTAPP_PROFILE and GIT_DIR from a parent repo.
func TestRuntimeForTest(t *testing.T) {
	leak := map[string]string{
		ttPrefix + "_PROFILE":     "prod",
		"GIT_DIR":                 "/leak/.git",
		"GIT_WORK_TREE":           "/leak",
		"GIT_INDEX_FILE":          "/leak/.git/index",
		"GIT_COMMON_DIR":          "/leak/.git",
		"GIT_CEILING_DIRECTORIES": "/leak",
		"GIT_CONFIG_GLOBAL":       "/leak/gitconfig",
		"GIT_CONFIG_COUNT":        "1",
		"GIT_AUTHOR_NAME":         "kept",
	}
	for k, v := range leak {
		t.Setenv(k, v)
	}
	var root string
	t.Run("isolates", func(t *testing.T) {
		root = ForTest(t, ttApp)
		set := map[string]string{
			"HOME":              "home",
			"USERPROFILE":       "home",
			ttPrefix + "_HOME":  "apphome",
			"XDG_CONFIG_HOME":   "xdg/config",
			"XDG_DATA_HOME":     "xdg/data",
			"XDG_STATE_HOME":    "xdg/state",
			"XDG_CACHE_HOME":    "xdg/cache",
			"XDG_RUNTIME_DIR":   "xdg/run",
			"CLAUDE_CONFIG_DIR": "agent-config",
			"CODEX_HOME":        "codex",
			"TMPDIR":            "tmp",
		}
		for k, rel := range set {
			want := filepath.Join(root, filepath.FromSlash(rel))
			if got := os.Getenv(k); got != want {
				t.Errorf("%s = %q, want %q", k, got, want)
			}
			if !isDir(want) {
				t.Errorf("%s folder %s not created", k, want)
			}
		}
		if got := os.Getenv(envTestRoot); got != root {
			t.Errorf("%s = %q", envTestRoot, got)
		}
		for k := range leak {
			_, ok := os.LookupEnv(k)
			if want := k == "GIT_AUTHOR_NAME"; ok != want {
				t.Errorf("%s set = %v, want %v", k, ok, want)
			}
		}
		if real, err := filepath.EvalSymlinks(root); err != nil || real != root {
			t.Errorf("root %q is not a real path (%q)", root, real)
		}

		// New with every default (process env, real home lookup) resolves
		// inside the root.
		d, err := New(ttApp, WithWorkDir(root), WithRelease(false))
		if err != nil {
			t.Fatal(err)
		}
		if d.Global().Base != filepath.Join(root, "apphome") || d.Profile() != ProfileDev {
			t.Fatalf("global = %q profile %q", d.Global().Base, d.Profile())
		}
		for _, e := range d.Paths() {
			if e.Path != "" && !within(e.Path, root) {
				t.Errorf("%s at %s outside the root", e.Kind, e.Path)
			}
		}
		// Without the <APP>_HOME override, the user home is the root's.
		if err := os.Unsetenv(ttPrefix + "_HOME"); err != nil {
			t.Fatal(err)
		}
		d, err = New(ttApp, WithWorkDir(root), WithRelease(false))
		if err != nil {
			t.Fatal(err)
		}
		if d.Global().Base != filepath.Join(root, "home", ".ttapp-dev") {
			t.Fatalf("global = %q", d.Global().Base)
		}

		// The guard: a home outside the root panics, naming it.
		outside := filepath.Join(filepath.Dir(root), "elsewhere")
		msg := ttPanic(func() { _, _ = New(ttApp, WithWorkDir(root), WithRelease(false), WithHome(outside)) })
		if !strings.Contains(msg, "outside the test root") || !strings.Contains(msg, outside) || !strings.Contains(msg, "appdir.ForTest") {
			t.Fatalf("panic = %q", msg)
		}
		msg = ttPanic(func() {
			_, _ = New(ttApp, WithWorkDir(root), WithRelease(false), WithHomeDir(func() (string, error) { return outside, nil }))
		})
		if !strings.Contains(msg, "outside the test root") {
			t.Fatalf("home dir panic = %q", msg)
		}
		msg = ttPanic(func() {
			_, _ = New(ttApp, WithWorkDir(outside), WithRelease(false), WithHome(filepath.Join(root, "h")))
		})
		if msg != "" {
			t.Fatalf("a work dir outside the root with every path inside panicked: %q", msg)
		}
		var off *Dirs
		msg = ttPanic(func() {
			off, err = New(ttApp, WithWorkDir(root), WithRelease(false), WithHome(outside), WithTestGuard(false))
		})
		if msg != "" || err != nil || off.Global().Base != outside {
			t.Fatalf("WithTestGuard(false): panic %q err %v", msg, err)
		}
	})
	// After the subtest, the env is restored for the caller.
	if os.Getenv(ttPrefix+"_PROFILE") != "prod" || os.Getenv("GIT_DIR") != "/leak/.git" || os.Getenv(envTestRoot) == root {
		t.Fatalf("env not restored: %q %q %q", os.Getenv(ttPrefix+"_PROFILE"), os.Getenv("GIT_DIR"), os.Getenv(envTestRoot))
	}
}

// TestRuntimeForTestLeaksAppVars: a developer's shell has other TTAPP_*
// variables (TTAPP_DAEMON_PROFILE, TTAPP_ALLOW_PROD, TTAPP_CEILING_DIRS). A
// test isolated by ForTest should not change behaviour with them.
func TestRuntimeForTestLeaksAppVars(t *testing.T) {
	t.Setenv(ttPrefix+"_DAEMON_PROFILE", "prod")
	t.Setenv(ttPrefix+"_ALLOW_PROD", "1")
	root := ForTest(t, ttApp)
	d, err := New(ttApp, WithWorkDir(root), WithRelease(false))
	if err != nil {
		t.Fatal(err)
	}
	if d.DaemonProfile() != ProfileDev {
		t.Fatalf("T1: ForTest leaked <APP>_DAEMON_PROFILE from the shell: DaemonProfile() = %s", d.DaemonProfile())
	}
	if acc, err := d.CheckDB(filepath.Join(root, "home", "."+ttApp, ttApp+".db")); !errors.Is(err, ErrProdDB) {
		t.Fatalf("ALLOW_PROD leaked: %v %v", acc, err)
	}
}

// TestRuntimeForTestPrefix: an app name with "-" and "." gets its own
// prefixed variables.
func TestRuntimeForTestPrefix(t *testing.T) {
	t.Setenv("TT_APP_X_PROFILE", "prod")
	root := ForTest(t, "tt-app.x")
	if os.Getenv("TT_APP_X_HOME") != filepath.Join(root, "apphome") {
		t.Fatalf("TT_APP_X_HOME = %q", os.Getenv("TT_APP_X_HOME"))
	}
	if _, ok := os.LookupEnv("TT_APP_X_PROFILE"); ok {
		t.Fatal("TT_APP_X_PROFILE still set")
	}
}
