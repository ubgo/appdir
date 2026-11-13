package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ubgo/appdir"
)

// TestRun runs the whole example in a test root and checks the lines the
// guide quotes.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`"kind": "paths"`,
		`"path": "$ROOT/repo/.doctor/doctor.db"`,
		`"path": "$ROOT/user/.doctor/cache"`,
		"profile prod (release=true, flag)",
		"profile-env-ignored: DOCTOR_PROFILE=dev ignored by a release build; pass --profile dev",
		`ceiling-relative: DOCTOR_CEILING_DIRS entry "relative/dir" is relative; ignored`,
		"  !templates/**",
		"ErrNoProject       exit 3  no .doctor/ here; run 'doctor init' or pass --home",
		"ErrUnknownProfile  exit 2",
		"ErrPointerAbsolute exit 1",
		"ErrLayoutTooNew    exit 5  home $ROOT/repo-new/.doctor has layout 2, this binary knows 1; upgrade doctor",
		`ErrUnknownKind     exit 1  appdir: unknown kind "template"`,
		"  config        global   flag  $ROOT/team/config.jsonc",
		"  db            project  flag  $ROOT/team/doctor.db",
		`app name         exit 2  appdir: app name "Doctor" must match ^[a-z0-9][a-z0-9._-]*$`,
		`dir name         exit 2  appdir: dir name ".." must be one plain path segment`,
		`kind path        exit 2  appdir: kind "tpl": path "../x" has an empty, . or .. segment`,
		"release profile  exit 2  appdir: profile \"staging\" is not in the list [prod dev]",
		"nil seam         exit 2  appdir: a seam option was given nil",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}

// TestPathsJSONParses checks that the envelope is valid JSON with one entry
// per kind (two for the kinds that live in both homes).
func TestPathsJSONParses(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := run(&out, root); err != nil {
		t.Fatal(err)
	}
	body := strings.SplitN(strings.SplitN(out.String(), "== doctor", 2)[0], "\n", 2)[1]
	var env pathsEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("paths --json is not JSON: %v", err)
	}
	if env.Count != len(env.Data) || env.Count != 14 {
		t.Errorf("count = %d, data = %d, want 14", env.Count, len(env.Data))
	}
}

// TestExitCode covers every sentinel, wrapped the way callers see them.
func TestExitCode(t *testing.T) {
	cases := map[error]int{
		nil:                        exitOK,
		appdir.ErrInvalidOption:    exitUsage,
		appdir.ErrUnknownProfile:   exitUsage,
		appdir.ErrEnvVar:           exitUsage,
		appdir.ErrNoProject:        exitNotFound,
		appdir.ErrNoHome:           exitNotFound,
		appdir.ErrPointerMissing:   exitNotFound,
		appdir.ErrPointerNotHome:   exitNotFound,
		appdir.ErrBusy:             exitBusy,
		appdir.ErrLayoutTooNew:     exitTooNew,
		appdir.ErrNotHome:          exitConflict,
		appdir.ErrGitignoreMarkers: exitConflict,
		appdir.ErrBadHomeFile:      exitConflict,
		appdir.ErrUnknownKind:      exitError,
		appdir.ErrPointerAbsolute:  exitError,
		appdir.ErrRootOwned:        exitError,
		appdir.ErrSymlinkEscape:    exitError,
		appdir.ErrUnsafeTemp:       exitError,
		appdir.ErrProdDB:           exitError,
		errors.New("disk full"):    exitError,
	}
	for err, want := range cases {
		wrapped := err
		if err != nil {
			wrapped = fmt.Errorf("doctor: %w", err)
		}
		if got := exitCode(wrapped); got != want {
			t.Errorf("exitCode(%v) = %d, want %d", err, got, want)
		}
	}
}

// TestOverrideListsBothKindsOnce checks the --home run: one line per kind
// (11), not 14, and each kind that lives in both homes appears once,
// labelled global (X1).
func TestOverrideListsBothKindsOnce(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := run(&out, root); err != nil {
		t.Fatal(err)
	}
	section := strings.SplitN(strings.SplitN(out.String(), "== paths --home team\n", 2)[1], "== errors", 2)[0]
	lines := strings.Split(strings.TrimSuffix(section, "\n"), "\n")
	if len(lines) != 11 {
		t.Fatalf("got %d lines, want 11:\n%s", len(lines), section)
	}
	for _, kind := range []appdir.Kind{appdir.KindHome, appdir.KindConfig, appdir.KindConfigLocal} {
		n := 0
		for _, l := range lines {
			if f := strings.Fields(l); f[0] == string(kind) {
				n++
				if f[1] != string(appdir.ScopeGlobal) {
					t.Errorf("%s labelled %s, want global", kind, f[1])
				}
			}
		}
		if n != 1 {
			t.Errorf("%s listed %d times, want 1", kind, n)
		}
	}
}
