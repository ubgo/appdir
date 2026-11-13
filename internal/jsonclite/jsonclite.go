// Package jsonclite turns JSONC (JSON with // and /* */ comments and trailing
// commas) into plain JSON.
//
// It exists only so appdir can read home.jsonc and the home:/db: pointer keys
// with zero setup and zero dependencies. It is replaced by ubgo/jsonc when that
// module exists; nothing outside appdir may import it.
package jsonclite

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrSyntax marks input that cannot be standardized: an unterminated string or
// block comment. Everything else is left for encoding/json to judge.
var ErrSyntax = errors.New("jsonc: syntax error")

// Standardize returns src with comments replaced by spaces (newlines kept, so
// line numbers in later JSON errors still match the file) and trailing commas
// removed. It walks strings properly: "//" inside a string is not a comment.
func Standardize(src []byte) ([]byte, error) {
	out := make([]byte, 0, len(src))
	// pendingComma holds the index in out of a comma that may turn out to be
	// trailing; it is dropped if the next significant byte closes a container.
	pendingComma := -1
	// lastSig is the last significant byte written; a comma right after an
	// opener or another comma is not trailing and is left for encoding/json
	// to reject.
	var lastSig byte
	line := 1
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '"':
			start := i
			out = append(out, c)
			i++
			closed := false
			for ; i < len(src); i++ {
				out = append(out, src[i])
				// A backslash does not make a raw newline legal (JSON has no
				// line continuation), so the escaped byte is checked too;
				// skipping it would also leave the line counter one short.
				if src[i] == '\\' && i+1 < len(src) && src[i+1] != '\n' {
					i++
					out = append(out, src[i])
					continue
				}
				if src[i] == '\n' {
					return nil, fmt.Errorf("%w: line %d: newline in string", ErrSyntax, line)
				}
				if src[i] == '"' {
					closed = true
					break
				}
			}
			if !closed {
				return nil, fmt.Errorf("%w: unterminated string at byte %d", ErrSyntax, start)
			}
			pendingComma, lastSig = -1, '"'
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				out = append(out, ' ')
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
				line++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			startLine := line
			out = append(out, ' ', ' ')
			i += 2
			closed := false
			for ; i < len(src); i++ {
				if src[i] == '*' && i+1 < len(src) && src[i+1] == '/' {
					out = append(out, ' ', ' ')
					i++
					closed = true
					break
				}
				if src[i] == '\n' {
					out = append(out, '\n')
					line++
				} else {
					out = append(out, ' ')
				}
			}
			if !closed {
				return nil, fmt.Errorf("%w: line %d: unterminated block comment", ErrSyntax, startLine)
			}
		case c == ',':
			if lastSig != '{' && lastSig != '[' && lastSig != ',' {
				pendingComma = len(out)
			}
			out = append(out, c)
			lastSig = c
		case c == '}' || c == ']':
			if pendingComma >= 0 {
				out[pendingComma] = ' '
				pendingComma = -1
			}
			out = append(out, c)
			lastSig = c
		case c == ' ' || c == '\t' || c == '\r':
			out = append(out, c)
		case c == '\n':
			out = append(out, c)
			line++
		default:
			pendingComma = -1
			out = append(out, c)
			lastSig = c
		}
	}
	return out, nil
}

// Object parses src as a JSONC object.
func Object(src []byte) (map[string]any, error) {
	std, err := Standardize(src)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(std, &m); err != nil {
		return nil, fmt.Errorf("jsonc: %w", err)
	}
	// encoding/json decodes a top-level null into a nil map without error; a
	// file holding only null is not an object and must not read as empty.
	if m == nil {
		return nil, errors.New("jsonc: top-level value is null, want an object")
	}
	return m, nil
}
