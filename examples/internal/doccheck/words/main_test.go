package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The test words are made up, so this file never needs the real list.
func TestRunFindsWordsAnyCase(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	clean := write("clean.md", "Nothing to see.\n")
	dirty := write("dirty.go", "package x\n\n// Like Zorblax does.\nvar y = 1 // QUUXCORP\n")
	var out strings.Builder
	n, err := run(&out, parseWords(" zorblax, ,quuxcorp"), nil, []string{clean, dirty})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("hits = %d, want 2; output:\n%s", n, out.String())
	}
	for _, want := range []string{dirty + ":3: zorblax", dirty + ":4: quuxcorp"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), clean) {
		t.Errorf("clean file reported:\n%s", out.String())
	}
}

func TestRunRefusesAnEmptyCheck(t *testing.T) {
	if _, err := run(&strings.Builder{}, nil, nil, []string{"x"}); err == nil {
		t.Error("no words: want an error")
	}
	if _, err := run(&strings.Builder{}, []string{"x"}, nil, nil); err == nil {
		t.Error("no files: want an error")
	}
}

// TestAllowToken: an allowed exact token is not a hit, but the bare word on
// the same line still is.
func TestAllowToken(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.go")
	if err := os.WriteFile(p, []byte("ZORBLAX_HOME is set\nzorblax here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	n, err := run(&out, []string{"zorblax"}, []string{"ZORBLAX_HOME"}, []string{p})
	if err != nil || n != 1 || !strings.Contains(out.String(), ":2: zorblax") {
		t.Fatalf("n=%d err=%v out=%q, want one hit on line 2", n, err, out.String())
	}
}
