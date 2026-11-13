// Command config-files lists the config files appdir hands to a config
// loader, in read order, for the prod and dev profiles, with a home: pointer,
// with --home, with no $HOME and with config.local.jsonc dropped. A tiny
// loader merges one key ("port") to show that later files win. The last step
// replaces the JSONC reader appdir uses for home.jsonc and the pointer keys.
//
// Everything happens inside one temp root: the user home is root/userhome (via
// WithHomeDir) and the project is root/shop, a git work tree (a .git folder
// is enough: appdir never runs git). Paths are printed with the root as $TMP.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/appdir"
)

// Names used by the example.
const (
	appName    = "lore"
	userHome   = "userhome"
	projectDir = "shop"
	privateDir = "shop-private" // a second repo next to the project
	otherHome  = "elsewhere"    // a folder passed as --home
	gitDir     = ".git"
	tmpLabel   = "$TMP" // printed in place of the temp root
	portKey    = "port" // the one setting the toy loader merges

	configFile      = "config.jsonc"
	localConfigFile = "config.local.jsonc"
	profilesDir     = "profiles"
	dirPerm         = 0o700
	filePerm        = 0o600
	homeFolder      = "." + appName      // the global and project home folder name
	noHomeMsg       = "$HOME is not set" // the error a failing home lookup returns
)

func main() {
	root, err := os.MkdirTemp("", "appdir-config-files-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "config-files:", err)
		os.Exit(1)
	}
	err = run(os.Stdout, root)
	_ = os.RemoveAll(root) // our own temp folder
	if err != nil {
		fmt.Fprintln(os.Stderr, "config-files:", err)
		os.Exit(1)
	}
}

// example holds the folders of one run, so every step reads the same names.
type example struct {
	w       io.Writer
	root    string
	home    string // the user's home folder
	shop    string // the project checkout, a git work tree
	marker  string // shop/.lore
	private string // shop-private/lore: a home: target
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
		shop:    filepath.Join(root, projectDir),
		private: filepath.Join(root, privateDir, appName),
	}
	if err := ex.setup(); err != nil {
		return err
	}
	steps := []func() error{
		ex.prod,
		ex.dev,
		ex.pointer,
		ex.homeFlag,
		ex.noHome,
		ex.dropped,
		ex.reader,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// options resolves lore from the project folder with a fixed home and an
// empty environment, so nothing on this machine changes the result.
func (ex *example) options(extra ...appdir.Option) []appdir.Option {
	noEnv := func(string) (string, bool) { return "", false }
	opts := []appdir.Option{
		appdir.WithHomeDir(func() (string, error) { return ex.home, nil }),
		appdir.WithWorkDir(ex.shop),
		appdir.WithEnv(noEnv),
		appdir.WithRelease(true), // prod unless a step passes WithProfile
	}
	return append(opts, extra...)
}

// setup makes the project a git work tree, runs init and writes one port
// per file, so the merge shows which file won.
func (ex *example) setup() error {
	for _, dir := range []string{
		filepath.Join(ex.shop, gitDir),
		filepath.Join(ex.home, homeFolder),
	} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	d, err := appdir.New(appName, ex.options()...)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	if ex.marker, err = d.InitProject(ex.shop); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	fmt.Fprintf(ex.w, "== init\ncreated %s\n", ex.rel(ex.marker))
	files := map[string]string{
		filepath.Join(ex.home, homeFolder, configFile): `{ "port": 2000 }`,
		filepath.Join(ex.marker, configFile):           `{ "port": 3000 }`,
		filepath.Join(ex.marker, localConfigFile):      `{ "port": 4000 }`,
	}
	for p, body := range files {
		if err := os.WriteFile(p, []byte(body+"\n"), filePerm); err != nil {
			return fmt.Errorf("write %s: %w", p, err)
		}
	}
	return nil
}

// snippet:list
// list prints ConfigFiles in read order and what a loader that lets later
// files win per key makes of "port".
func (ex *example) list(title string, extra ...appdir.Option) error {
	d, err := appdir.New(appName, ex.options(extra...)...)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	fmt.Fprintf(ex.w, "\n== %s (profile %s)\n", title, d.Profile())
	files := d.ConfigFiles()
	for i, f := range files {
		var flags []string
		if f.Local {
			flags = append(flags, "local")
		}
		if f.Committed {
			flags = append(flags, "committed")
		}
		if f.Profile != "" {
			flags = append(flags, "profile="+string(f.Profile))
		}
		if !exists(f.Path) {
			flags = append(flags, "missing")
		}
		line := fmt.Sprintf("%d. %-7s %s %s", i+1, f.Home, ex.rel(f.Path), strings.Join(flags, " "))
		fmt.Fprintln(ex.w, strings.TrimRight(line, " "))
	}
	port, from, err := loadPort(files)
	if err != nil {
		return fmt.Errorf("load port: %w", err)
	}
	if from == "" {
		fmt.Fprintln(ex.w, "port unset (code default)")
		return nil
	}
	fmt.Fprintf(ex.w, "port = %d (from %s)\n", port, ex.rel(from))
	return nil
}

// loadPort is a toy config loader: it reads the files in order, skips the
// missing ones, and lets a later file win per key. A real tool hands the
// list to its config loader instead. JSON numbers decode as float64, so the
// value is checked and turned into an int here, at the boundary. from is
// the file that set the port, empty when no file did.
func loadPort(files []appdir.ConfigFile) (port int, from string, err error) {
	for _, f := range files {
		m, err := readJSON(f.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue // a missing file counts as empty
		}
		if err != nil {
			return 0, "", err
		}
		v, ok := m[portKey]
		if !ok {
			continue
		}
		n, ok := v.(float64)
		if !ok {
			return 0, "", fmt.Errorf("%s: %q is %T, want a number", f.Path, portKey, v)
		}
		port, from = int(n), f.Path
	}
	return port, from, nil
}

// snippet:end

// prod lists the files of a release build.
func (ex *example) prod() error { return ex.list("prod") }

// snippet:dev
// dev lists the files of the dev profile: the same shared files, then the
// profile's own config.local.jsonc, read last so it wins.
func (ex *example) dev() error {
	devFile := filepath.Join(ex.marker, profilesDir, string(appdir.ProfileDev), localConfigFile)
	if err := os.MkdirAll(filepath.Dir(devFile), dirPerm); err != nil {
		return fmt.Errorf("create: %w", err)
	}
	if err := os.WriteFile(devFile, []byte(`{ "port": 4100 }`+"\n"), filePerm); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return ex.list("dev", appdir.WithProfile(string(appdir.ProfileDev)))
}

// snippet:end

// snippet:pointer
// pointer moves the project home with a home: key in the local file: the
// target's two files come after the marker's, and the dev profile's own
// file moves into the target with the rest of the dev data.
func (ex *example) pointer() error {
	for _, dir := range []string{ex.private, filepath.Join(ex.root, privateDir, gitDir)} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	d, err := appdir.New(appName, ex.options()...)
	if err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	if err := d.InitHome(ex.private, appdir.ScopeProject); err != nil {
		return fmt.Errorf("init home: %w", err)
	}
	local := `{ "port": 4000, "home": "../../shop-private/lore" }` + "\n"
	if err := os.WriteFile(filepath.Join(ex.marker, localConfigFile), []byte(local), filePerm); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := os.WriteFile(filepath.Join(ex.private, configFile), []byte(`{ "port": 5000 }`+"\n"), filePerm); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := ex.list("home: pointer"); err != nil {
		return err
	}
	return ex.list("home: pointer", appdir.WithProfile(string(appdir.ProfileDev)))
}

// snippet:end

// snippet:overrides
// homeFlag lists the files of one home named by --home: two files, once.
func (ex *example) homeFlag() error {
	return ex.list("--home", appdir.WithHome(filepath.Join(ex.root, otherHome)))
}

// noHome drops the global files when there is no home folder.
func (ex *example) noHome() error {
	noHomeDir := func() (string, error) { return "", errors.New(noHomeMsg) }
	return ex.list("no $HOME", appdir.WithHomeDir(noHomeDir))
}

// snippet:end

// snippet:dropped
// dropped lists the files of a tool that keeps no machine-only settings:
// WithoutKind(KindConfigLocal) leaves config.local.jsonc out of every home,
// so the loader never reads it (and a home: key in it is not read either).
func (ex *example) dropped() error {
	return ex.list("WithoutKind(config-local)", appdir.WithoutKind(appdir.KindConfigLocal))
}

// snippet:end

// snippet:reader
// reader passes the tool's own reader to appdir. appdir calls it for
// home.jsonc and for the files that may hold home:/db:, never for the
// settings themselves. A missing file must give an error that wraps
// fs.ErrNotExist, as os.ReadFile does.
func (ex *example) reader() error {
	fmt.Fprintln(ex.w, "\n== WithConfigReader")
	var read []string
	recording := func(path string) (map[string]any, error) {
		read = append(read, ex.rel(path))
		return readJSON(path)
	}
	if _, err := appdir.New(appName, ex.options(appdir.WithConfigReader(recording))...); err != nil {
		return fmt.Errorf("appdir: %w", err)
	}
	for _, p := range read {
		fmt.Fprintf(ex.w, "read %s\n", p)
	}
	return nil
}

// readJSON is a strict JSON reader: the tool's choice, here the stdlib.
func readJSON(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err // keeps fs.ErrNotExist for a missing file
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// snippet:end

// exists reports whether p exists.
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// rel replaces the temp root in s with $TMP, so the output reads the same on
// every run.
func (ex *example) rel(s string) string {
	return strings.ReplaceAll(s, ex.root, tmpLabel)
}
