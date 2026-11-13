package jsonclite

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// TestStandardizeAccepted covers inputs that Standardize must turn into JSON
// that encoding/json accepts, and checks the decoded value against want.
func TestStandardizeAccepted(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want any
	}{
		{"plain object", `{"a":1}`, map[string]any{"a": 1.0}},
		{"line comment before key", "{\n// c\n\"a\":1}", map[string]any{"a": 1.0}},
		{"line comment after key", "{\"a\" // c\n:1}", map[string]any{"a": 1.0}},
		{"line comment after value", "{\"a\":1 // c\n}", map[string]any{"a": 1.0}},
		{"line comment after comma", "{\"a\":1, // c\n\"b\":2}", map[string]any{"a": 1.0, "b": 2.0}},
		{"line comment at start of file", "// head\n{\"a\":1}", map[string]any{"a": 1.0}},
		{"line comment at EOF no newline", "{\"a\":1} // tail", map[string]any{"a": 1.0}},
		{"block comment before key", `{/* c */"a":1}`, map[string]any{"a": 1.0}},
		{"block comment between key and colon", `{"a"/* c */:1}`, map[string]any{"a": 1.0}},
		{"block comment between colon and value", `{"a":/* c */1}`, map[string]any{"a": 1.0}},
		{"block comment after value", `{"a":1/* c */}`, map[string]any{"a": 1.0}},
		{"block comment at EOF", `{"a":1}/* c */`, map[string]any{"a": 1.0}},
		{"multi-line block comment", "{\n/* a\n b\n*/\"a\":1}", map[string]any{"a": 1.0}},
		{"empty block comment", `{/**/"a":1}`, map[string]any{"a": 1.0}},
		{"block comment with stars", `{/*** x **/"a":1}`, map[string]any{"a": 1.0}},
		{"block comment holding //", "{/* // */\"a\":1}", map[string]any{"a": 1.0}},
		{"line comment holding /*", "{// /* \n\"a\":1}", map[string]any{"a": 1.0}},
		{"comments inside array", "[1, /* c */ 2, // c\n 3]", []any{1.0, 2.0, 3.0}},
		{"line slashes in string", `{"u":"http://x/y"}`, map[string]any{"u": "http://x/y"}},
		{"block opener in string", `{"u":"a/*b*/c"}`, map[string]any{"u": "a/*b*/c"}},
		{"escaped quote before close", `{"q":"say \"hi\""}`, map[string]any{"q": `say "hi"`}},
		{"escaped backslash before close", `{"p":"c:\\"}`, map[string]any{"p": `c:\`}},
		{"escaped backslash then comment", `{"p":"c:\\" // x` + "\n}", map[string]any{"p": `c:\`}},
		{"two escaped backslashes then quote", `{"p":"\\\\"}`, map[string]any{"p": `\\`}},
		{"escaped quote then slashes", `{"q":"\"//"}`, map[string]any{"q": `"//`}},
		{"unicode escape", `{"u":"\u00e9\u002f\u002f"}`, map[string]any{"u": "é//"}},
		{"raw utf8", `{"u":"日本 // no"}`, map[string]any{"u": "日本 // no"}},
		{"trailing comma object", `{"a":1,}`, map[string]any{"a": 1.0}},
		{"trailing comma array", `[1,2,]`, []any{1.0, 2.0}},
		{"trailing comma nested", `{"a":[1,{"b":2,},],}`, map[string]any{"a": []any{1.0, map[string]any{"b": 2.0}}}},
		{"trailing comma then whitespace", "{\"a\":1,\n\t \r\n}", map[string]any{"a": 1.0}},
		{"trailing comma then comment", "{\"a\":1, // c\n /* d */}", map[string]any{"a": 1.0}},
		{"comma in string not trailing", `{"a":",}"}`, map[string]any{"a": ",}"}},
		{"string ending in comma before close", `["a,"]`, []any{"a,"}},
		{"empty object", `{}`, map[string]any{}},
		{"empty array", `[]`, []any{}},
		{"scalar top level", `42`, 42.0},
		{"slash outside comment passes through", `{"a":1}`, map[string]any{"a": 1.0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Standardize([]byte(tt.in))
			if err != nil {
				t.Fatalf("Standardize(%q) error: %v", tt.in, err)
			}
			if len(out) != len(tt.in) {
				t.Errorf("length changed: in %d, out %d (%q)", len(tt.in), len(out), out)
			}
			if got, want := bytes.Count(out, []byte("\n")), bytes.Count([]byte(tt.in), []byte("\n")); got != want {
				t.Errorf("newlines: got %d, want %d", got, want)
			}
			var got any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("encoding/json rejected %q: %v", out, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("decoded %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestStandardizeLeavesInvalidForJSON covers inputs Standardize lets through
// but encoding/json must still reject: Standardize only removes comments and
// trailing commas, it does not repair other mistakes.
func TestStandardizeLeavesInvalidForJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"leading comma object", `{,"a":1}`},
		{"leading comma array", `[,1]`},
		{"lone comma array", `[,]`},
		{"lone comma object", `{,}`},
		{"double comma middle", `[1,,2]`},
		{"double comma trailing", `[1,,]`},
		{"double comma object", `{"a":1,,}`},
		{"comment-separated double comma", `[1,/* c */,]`},
		{"top-level trailing comma", `1,`},
		{"bare slash", `{"a":1/}`},
		{"empty", ``},
		{"whitespace only", " \n\t\r\n"},
		{"comment only", "// nothing\n/* here */"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Standardize([]byte(tt.in))
			if err != nil {
				// A Standardize error is also a rejection; it must be ErrSyntax.
				if !errors.Is(err, ErrSyntax) {
					t.Fatalf("error %v is not ErrSyntax", err)
				}
				return
			}
			if json.Valid(out) {
				t.Errorf("encoding/json accepted %q (from %q)", out, tt.in)
			}
		})
	}
}

// TestStandardizeSyntaxErrors covers what Standardize itself rejects; each
// error must match ErrSyntax with errors.Is.
func TestStandardizeSyntaxErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"unterminated string", `{"a":"b`},
		{"unterminated key", `{"a`},
		{"unterminated after escaped quote", `{"a":"b\"}`},
		{"unterminated ending in backslash", `{"a":"b\`},
		{"lone quote", `"`},
		{"newline in string", "{\"a\":\"b\nc\"}"},
		{"escaped newline in string", "{\"a\":\"x\\\ny\"}"},
		{"unterminated block comment", `{"a":1 /* x`},
		{"unterminated block comment one star", `{"a":1 /*/`},
		{"unterminated block comment star at EOF", `/* x *`},
		{"block opener at EOF", `/*`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Standardize([]byte(tt.in))
			if err == nil {
				t.Fatalf("Standardize(%q) = %q, want error", tt.in, out)
			}
			if !errors.Is(err, ErrSyntax) {
				t.Errorf("error %v is not ErrSyntax", err)
			}
			if out != nil {
				t.Errorf("output on error = %q, want nil", out)
			}
		})
	}
}

// TestStandardizeLinesPreserved checks that each byte keeps its line, so an
// encoding/json offset maps back to the same line of the original file.
func TestStandardizeLinesPreserved(t *testing.T) {
	in := "// head\n{\n  /* a\n     b */ \"a\": 1, // one\n  \"b\": [2,\n  ],\n}\n// tail"
	out, err := Standardize([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("length: in %d, out %d", len(in), len(out))
	}
	for i := range in {
		if (in[i] == '\n') != (out[i] == '\n') {
			t.Fatalf("byte %d: newline moved (in %q, out %q)", i, in[i], out[i])
		}
	}
	if !json.Valid(out) {
		t.Fatalf("output not valid JSON: %q", out)
	}
}

// TestStandardizeErrorLine checks the line reported for an error that comes
// after comments and strings spanning lines.
func TestStandardizeErrorLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"block comment after comments", "// a\n/* b\n*/\n{\"a\":1 /* open", "line 4"},
		// A backslash before a newline must not hide the newline from the
		// line counter, or every later error names the wrong line.
		{"escaped newline on line 2", "{\n\"a\":\"x\\\ny\"}", "line 2"},
		{"newline in string on line 3", "{\n\"a\":1,\n\"b\":\"x\ny\"}", "line 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Standardize([]byte(tt.in))
			if err == nil {
				t.Fatal("want error")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tt.want)) {
				t.Errorf("error %q does not name %q", err, tt.want)
			}
		})
	}
}

// TestObject covers the object wrapper: a JSONC object decodes, anything
// that is not an object is an error.
func TestObject(t *testing.T) {
	m, err := Object([]byte("{\n // c\n \"home\": \"~/x\", /* d */\n \"n\": [1,],\n}"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"home": "~/x", "n": []any{1.0}}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got %#v, want %#v", m, want)
	}

	m, err = Object([]byte(`{}`))
	if err != nil || m == nil || len(m) != 0 {
		t.Errorf("Object({}) = %#v, %v; want empty non-nil map", m, err)
	}

	bad := []struct {
		name   string
		in     string
		syntax bool // true when the error must match ErrSyntax
	}{
		{"array", `[1]`, false},
		{"string", `"x"`, false},
		{"number", `1`, false},
		{"bool", `true`, false},
		// encoding/json turns null into a nil map without error; a config file
		// holding only null is not an object and must not read as empty.
		{"null", `null`, false},
		{"null with comment", "// c\nnull", false},
		{"empty", ``, false},
		{"whitespace", " \n ", false},
		{"comment only", "// c", false},
		{"two objects", `{}{}`, false},
		{"leading comma", `{,"a":1}`, false},
		{"unterminated string", `{"a":"b`, true},
		{"unterminated comment", `{"a":1 /*`, true},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Object([]byte(tt.in))
			if err == nil {
				t.Fatalf("Object(%q) = %#v, want error", tt.in, m)
			}
			if m != nil {
				t.Errorf("map on error = %#v, want nil", m)
			}
			if tt.syntax && !errors.Is(err, ErrSyntax) {
				t.Errorf("error %v is not ErrSyntax", err)
			}
		})
	}
}

// FuzzStandardize checks the invariants over arbitrary input: no panic; on
// success the output keeps the length and every newline in place; plain
// JSON (no comments, no trailing commas) comes back byte-identical.
func FuzzStandardize(f *testing.F) {
	seeds := []string{
		`{"a":1}`, `{"a":1,}`, `[1,2,]`, "{// c\n\"a\":1}", `{/* c */"a":1}`,
		`{"u":"http://x"}`, `{"q":"\"//"}`, `{"p":"c:\\"}`, `"\u00e9"`, `/*`, `"`,
		"{\"a\":\"b\nc\"}", `[1,,]`, `{,}`, ``, " \n", `null`, `{"a":[1,{"b":2,},],}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		out, err := Standardize(in)
		if err != nil {
			if !errors.Is(err, ErrSyntax) {
				t.Fatalf("error %v is not ErrSyntax", err)
			}
			return
		}
		if len(out) != len(in) {
			t.Fatalf("length: in %d, out %d", len(in), len(out))
		}
		if json.Valid(out) && bytes.Count(out, []byte("\n")) != bytes.Count(in, []byte("\n")) {
			t.Fatalf("newline count changed: %q -> %q", in, out)
		}
		for i := range in {
			if (in[i] == '\n') != (out[i] == '\n') {
				t.Fatalf("byte %d: newline moved: %q -> %q", i, in, out)
			}
		}
		// Valid JSON cannot hold a comment or trailing comma outside a
		// string, so Standardize must have nothing to change.
		if json.Valid(in) && !bytes.Equal(in, out) {
			t.Fatalf("valid JSON changed: %q -> %q", in, out)
		}
	})
}
