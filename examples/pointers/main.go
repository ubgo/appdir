// Command pointers shows the home: and db: pointer keys of a project home:
// a committed config.jsonc (with comments and trailing commas) that moves the
// project home into a private folder next to the repo, the errors a wrong
// pointer gives, the one-hop rule, what the dev profile does with both keys,
// the config files listed under a broken pointer, and an empty local home:
// that does not clear the committed one.
//
// Everything happens inside one temp root: the user home is root/userhome (via
// WithHomeDir), the project is root/work/shop, the private folder is
// root/work/shop-private. Paths are printed relative to the root.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example. The app is "lore"; the project is "shop".
const (
	appName    = "lore"
	userHome   = "userhome"
	workDir    = "work"
	projectDir = "shop"
	privateDir = "shop-private" // a second repo next to the project
	dbDir      = "shop-db"      // a fast disk for the database, say

	configFile      = "config.jsonc"       // committed
	localConfigFile = "config.local.jsonc" // this machine only
	tmpLabel        = "$TMP"               // printed in place of the temp root
	dirPerm         = 0o700
	filePerm        = 0o600
	homeKey         = "home"                 // the pointer key that moves the whole project home
	goneTarget      = "../../shop-gone/lore" // a home: target nobody created
)

func main() {
	root, err := os.MkdirTemp("", "appdir-pointers-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "pointers:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp folder
	if err != nil {
		fmt.Fprintln(os.Stderr, "pointers:", err)
		os.Exit(1)
	}
}

// example holds the folders of one run, so every step reads the same names.
type example struct {
	w       io.Writer
	root    string
	home    string // the user's home folder
	shop    string // the project checkout
	marker  string // shop/.lore
	private string // shop-private/lore: the home: target
}

// run walks through every step in order inside root. Steps share one
// project, so the order matters: each step starts from the files the
// previous one left.
func run(w io.Writer, root string) error {
	// Resolve the temp root once: appdir compares real paths, and on macOS
	// the temp folder sits behind the /var -> /private/var link.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	ex := &example{
		w:       w,
		root:    root,
		home:    filepath.Join(root, userHome),
		shop:    filepath.Join(root, workDir, projectDir),
		private: filepath.Join(root, workDir, privateDir, appName),
	}
	for _, dir := range []string{ex.home, ex.shop} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	steps := []func() error{
		ex.initProject,
		ex.missingTarget,
		ex.createTarget,
		ex.absoluteCommitted,
		ex.absoluteLocal,
		ex.dbPointer,
		ex.oneHop,
		ex.notAString,
		ex.devProfile,
		ex.brokenConfigFiles,
		ex.emptyLocalHome,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// newDirs resolves lore from the project folder with a fixed home and an
// empty environment, so nothing on this machine changes the result.
func (ex *example) newDirs(extra ...appdir.Option) (*appdir.Dirs, error) {
	noEnv := func(string) (string, bool) { return "", false }
	opts := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return ex.home, nil }),
		appdir.WithWorkDir(ex.shop),
		appdir.WithEnv(noEnv),
		// A release build: the prod profile, so data stays in the home
		// itself. The dev step passes WithProfile on top.
		appdir.WithRelease(true),
	}
	d, err := appdir.New(appName, append(opts, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("appdir: %w", err)
	}
	return d, nil
}

// initProject creates shop/.lore with home.jsonc, like `lore init`.
func (ex *example) initProject() error {
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	marker, err := d.InitProject(ex.shop)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	ex.marker = marker
	fmt.Fprintf(ex.w, "== init\ncreated %s\n", ex.rel(marker))
	return nil
}

// snippet:committed-pointer
// committedPointer is a team-shared config.jsonc: JSONC, so it may hold
// comments and trailing commas. The home: value is relative to the folder
// holding this file (shop/.lore), so ../../shop-private/lore is the
// private repo next to shop on every machine that clones both.
const committedPointer = `{
  // Our lore data lives in the private repo next to this one.
  /* A relative path works on every machine. */
  "home": "../../shop-private/lore",
  "note": "a // inside a string is not a comment",
}
`

// snippet:end

// missingTarget writes the pointer before the target exists.
func (ex *example) missingTarget() error {
	fmt.Fprintln(ex.w, "\n== home: pointer, target missing")
	if err := ex.write(ex.marker, configFile, committedPointer); err != nil {
		return err
	}
	return ex.show()
}

// snippet:create-target
// createTarget makes the pointer target a lore home, like
// `lore init --home ../shop-private/lore`: InitHome writes its home.jsonc.
func (ex *example) createTarget() error {
	fmt.Fprintln(ex.w, "\n== home: pointer, target created with InitHome")
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	if err := d.InitHome(ex.private, appdir.ScopeProject); err != nil {
		return fmt.Errorf("init home: %w", err)
	}
	return ex.show()
}

// snippet:end

// snippet:absolute
// absoluteCommitted puts an absolute path in the committed file: it would
// break every other clone, so appdir refuses it.
func (ex *example) absoluteCommitted() error {
	fmt.Fprintln(ex.w, "\n== absolute home: in config.jsonc")
	body := fmt.Sprintf("{ %q: %q }\n", homeKey, ex.private)
	if err := ex.write(ex.marker, configFile, body); err != nil {
		return err
	}
	return ex.show()
}

// absoluteLocal moves the same value to config.local.jsonc, which is this
// machine's own file: any user path is fine there.
func (ex *example) absoluteLocal() error {
	fmt.Fprintln(ex.w, "\n== absolute home: in config.local.jsonc")
	if err := ex.write(ex.marker, configFile, "{}\n"); err != nil {
		return err
	}
	body := fmt.Sprintf("{ %q: %q }\n", homeKey, ex.private)
	if err := ex.write(ex.marker, localConfigFile, body); err != nil {
		return err
	}
	return ex.show()
}

// snippet:end

// snippet:db-pointer
// dbPointer moves only the database. Its folder must exist; the file
// itself may not (the first run creates it). backups/ stays in the home.
func (ex *example) dbPointer() error {
	fmt.Fprintln(ex.w, "\n== db: pointer")
	if err := os.Remove(filepath.Join(ex.marker, localConfigFile)); err != nil {
		return fmt.Errorf("remove local config: %w", err)
	}
	body := `{ "db": "../../shop-db/lore.db" }` + "\n"
	if err := ex.write(ex.marker, configFile, body); err != nil {
		return err
	}
	if err := ex.show(); err != nil { // shop-db/ is not there yet
		return err
	}
	if err := os.MkdirAll(filepath.Join(ex.root, workDir, dbDir), dirPerm); err != nil {
		return fmt.Errorf("create db folder: %w", err)
	}
	return ex.show()
}

// snippet:end

// snippet:one-hop
// oneHop gives the target its own home: key. appdir does not follow it:
// pointers go one hop only, and the tool gets a warning to print.
func (ex *example) oneHop() error {
	fmt.Fprintln(ex.w, "\n== one hop only")
	if err := ex.write(ex.marker, configFile, committedPointer); err != nil {
		return err
	}
	if err := ex.write(ex.private, configFile, `{ "home": "../elsewhere" }`+"\n"); err != nil {
		return err
	}
	return ex.show()
}

// snippet:end

// snippet:not-a-string
// notAString shows that a pointer value must be a string path.
func (ex *example) notAString() error {
	fmt.Fprintln(ex.w, "\n== home: is not a string")
	if err := ex.write(ex.marker, localConfigFile, `{ "home": 42 }`+"\n"); err != nil {
		return err
	}
	if err := ex.show(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(ex.marker, localConfigFile)); err != nil {
		return fmt.Errorf("remove local config: %w", err)
	}
	return nil
}

// snippet:end

// snippet:broken-files
// brokenConfigFiles points home: at a folder that does not exist. Project
// fails, but ConfigFiles still lists the marker's own two files: they hold
// the pointer the user has to fix, so a config command can show them.
func (ex *example) brokenConfigFiles() error {
	fmt.Fprintln(ex.w, "\n== ConfigFiles under a broken pointer")
	body := fmt.Sprintf("{ %q: %q }\n", homeKey, goneTarget)
	if err := ex.write(ex.marker, configFile, body); err != nil {
		return err
	}
	d, err := ex.newDirs()
	if err != nil {
		return err
	}
	if _, err := d.Project(); err != nil {
		fmt.Fprintf(ex.w, "Project: %s\n", sentinelName(err))
	}
	for _, f := range d.ConfigFiles() {
		fmt.Fprintf(ex.w, "%-7s %s\n", f.Home, ex.rel(f.Path))
	}
	return ex.write(ex.marker, configFile, committedPointer) // fix it again
}

// snippet:end

// snippet:empty-local
// emptyLocalHome sets "home": "" in config.local.jsonc. An empty value
// counts as unset, so the committed home: still applies: a machine cannot
// clear the team's pointer this way, only replace it with another path.
func (ex *example) emptyLocalHome() error {
	fmt.Fprintln(ex.w, "\n== empty home: in config.local.jsonc")
	if err := ex.write(ex.marker, localConfigFile, `{ "home": "" }`+"\n"); err != nil {
		return err
	}
	if err := ex.show(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(ex.marker, localConfigFile)); err != nil {
		return fmt.Errorf("remove local config: %w", err)
	}
	return nil
}

// snippet:end

// snippet:dev-profile
// devProfile resolves the same project as the dev profile, with both keys
// set: home: is followed but the data goes to <target>/profiles/dev, and
// db: is ignored so dev data never lands next to the prod database.
func (ex *example) devProfile() error {
	fmt.Fprintln(ex.w, "\n== dev profile with home: and db:")
	if err := ex.write(ex.private, configFile, "{}\n"); err != nil {
		return err
	}
	body := `{ "home": "../../shop-private/lore", "db": "../../shop-db/lore.db" }` + "\n"
	if err := ex.write(ex.marker, configFile, body); err != nil {
		return err
	}
	return ex.show(appdir.WithProfile(string(appdir.ProfileDev)))
}

// snippet:end

// snippet:show
// show resolves lore and prints the project home, the db and backups paths
// and any warnings, or the error with the sentinel it wraps.
func (ex *example) show(extra ...appdir.Option) error {
	d, err := ex.newDirs(extra...)
	if err != nil {
		return err
	}
	h, err := d.Project()
	if err != nil {
		fmt.Fprintf(ex.w, "error: %s\n", ex.rel(err.Error()))
		fmt.Fprintf(ex.w, "  sentinel: %s\n", sentinelName(err))
		return nil
	}
	db, err := d.Path(appdir.KindDB)
	if err != nil {
		return err
	}
	backups, err := d.Path(appdir.KindBackups)
	if err != nil {
		return err
	}
	fmt.Fprintf(ex.w, "profile: %s\n", d.Profile())
	fmt.Fprintf(ex.w, "project: %s (rule %s)\n", ex.rel(h.Base), h.Rule)
	fmt.Fprintf(ex.w, "data:    %s\n", ex.rel(h.Data))
	fmt.Fprintf(ex.w, "db:      %s\n", ex.rel(db))
	fmt.Fprintf(ex.w, "backups: %s\n", ex.rel(backups))
	for _, wn := range d.Warnings() {
		fmt.Fprintf(ex.w, "warning %s: %s\n", wn.Code, ex.rel(wn.Message))
	}
	return nil
}

// snippet:end

// sentinelName names the appdir sentinel an error wraps, the way a tool
// would pick an exit code with errors.Is.
func sentinelName(err error) string {
	for _, s := range []struct {
		err  error
		name string
	}{
		{appdir.ErrPointerMissing, "ErrPointerMissing"},
		{appdir.ErrPointerNotHome, "ErrPointerNotHome"},
		{appdir.ErrPointerAbsolute, "ErrPointerAbsolute"},
		{appdir.ErrInvalidOption, "ErrInvalidOption"},
	} {
		if errors.Is(err, s.err) {
			return s.name
		}
	}
	return "none"
}

// write writes name in dir with owner-only permissions.
func (ex *example) write(dir, name, body string) error {
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), filePerm); err != nil {
		return fmt.Errorf("write %s: %w", ex.rel(p), err)
	}
	return nil
}

// rel replaces the temp root in s with $TMP, so the output reads the same on
// every run.
func (ex *example) rel(s string) string {
	if os.PathSeparator == '\\' {
		// Windows: undo the %q escaping of a quoted path ("C:\\x") first.
		s = strings.ReplaceAll(s, `\\`, `\`)
	}
	return filepath.ToSlash(strings.ReplaceAll(s, ex.root, tmpLabel))
}
