package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/appdir"
)

// envPrefix is the demo app's env prefix; every DEMO_* variable is cleared so
// the developer's shell cannot change what the test sees.
const envPrefix = "DEMO_"

// workFolder is the working folder inside the sandbox. It is not HOME itself:
// .demo/ in HOME is the global home, never a project.
const workFolder = "work"

// sandbox points HOME at a fresh temp folder, the working folder at a folder
// inside it, and clears the demo's own variables. The demo reads the process
// env (it is a real command line tool), so the test fences the env instead of
// the code. It returns the working folder.
func sandbox(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, envPrefix) {
			t.Setenv(name, "") // appdir treats set-but-empty as unset
		}
	}
	work := filepath.Join(dir, workFolder)
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	return work
}

// runDemo runs the command with args and returns stdout.
func runDemo(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := run(args, &stdout, &stderr); err != nil {
		t.Fatalf("demo %v: %v\n%s", args, err, stderr.String())
	}
	return stdout.String()
}

// TestNoProject: outside any project the text report says how to make one.
func TestNoProject(t *testing.T) {
	sandbox(t)
	out := runDemo(t, "--short")
	want := "project   none: no .demo/ here; run 'demo init' or pass --home"
	if !strings.Contains(out, want) {
		t.Errorf("output lacks %q\n%s", want, out)
	}
}

// TestInitJSON: --init creates .demo/ and the JSON report shows the project.
func TestInitJSON(t *testing.T) {
	dir := sandbox(t)
	out := runDemo(t, "--init", "--json")
	var r report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if r.Project == nil || r.Project.Marker != filepath.Join(dir, ".demo") {
		t.Fatalf("project = %+v, want marker %s", r.Project, filepath.Join(dir, ".demo"))
	}
	if r.Init == "" || len(r.Paths) == 0 || len(r.ConfigFiles) == 0 {
		t.Errorf("report misses init, paths or config files: %+v", r)
	}
}

// TestHomeFlag: --home sends every kind to one folder with rule flag.
func TestHomeFlag(t *testing.T) {
	dir := sandbox(t)
	home := filepath.Join(dir, "h")
	out := runDemo(t, "--home", home, "--json")
	var r report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if r.Global.Base != home || r.Global.Rule != appdir.RuleFlag {
		t.Errorf("global = %+v, want %s [flag]", r.Global, home)
	}
}
