package appdir

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// Sentinel errors. They are the only package-level variables in appdir and are
// never reassigned. Every refusal appdir decides itself wraps one of them with
// the path, key or variable it is about, so callers use errors.Is and users
// read a full sentence. OS and parse errors, and MoveLegacy's copy and move
// failures, wrap the underlying error instead (SPEC 3.5). Mapping to exit codes is the tool's job (standard section 11).
// ds:def id=sentinel-errors-mwtzt4d9 owner=@khanakia stability=stable
var (
	ErrInvalidOption    = errors.New("invalid option")
	ErrUnknownProfile   = errors.New("unknown profile")
	ErrUnknownKind      = errors.New("unknown kind")
	ErrNoProject        = errors.New("no project home")
	ErrNoHome           = errors.New("no home folder")
	ErrEnvVar           = errors.New("variable not set")
	ErrPointerAbsolute  = errors.New("absolute pointer in a committed file")
	ErrPointerMissing   = errors.New("pointer target does not exist")
	ErrPointerNotHome   = errors.New("pointer target is not a home of this app")
	ErrLayoutTooNew     = errors.New("home layout is newer than this binary")
	ErrRootOwned        = errors.New("running as root in another user's home")
	ErrSymlinkEscape    = errors.New("symlink leads outside the home")
	ErrUnsafeTemp       = errors.New("unsafe temp folder")
	ErrProdDB           = errors.New("dev build refuses the prod database")
	ErrBusy             = errors.New("busy")
	ErrGitignoreMarkers = errors.New("gitignore markers are broken")
	ErrNotHome          = errors.New("folder belongs to another app")
	// ErrBadHomeFile: init found a home.jsonc that exists but cannot be read
	// or parsed. It is the user's file, so init refuses rather than replace
	// it; the reader's own error is wrapped too (fs.ErrPermission, ...).
	ErrBadHomeFile = errors.New("home file cannot be read")
)

// appError carries the full sentence a user reads and the sentinel callers
// check with errors.Is. Its message does not repeat the sentinel's text.
// cause, when set, is the underlying error (a reader's fs.ErrPermission, a
// parse error) and stays reachable through errors.Is/As; its text is already
// folded into msg by the caller.
type appError struct {
	msg   string
	kind  error
	cause error
}

func (e *appError) Error() string { return e.msg }

// Unwrap returns the sentinel and, when set, the cause (Go 1.20 multi-wrap).
func (e *appError) Unwrap() []error {
	if e.cause == nil {
		return []error{e.kind}
	}
	return []error{e.kind, e.cause}
}

// newErr builds an appError: the message is fmt.Sprintf(format, args...).
func newErr(kind error, format string, args ...any) error {
	return &appError{msg: fmt.Sprintf(format, args...), kind: kind}
}

// wrapErr is newErr that also wraps cause, for an error whose own cause a
// caller may still need to match (fs.ErrPermission under ErrBadHomeFile).
func wrapErr(kind, cause error, format string, args ...any) error {
	return &appError{msg: fmt.Sprintf(format, args...), kind: kind, cause: cause}
}

// detailWithoutPath is err's text without a leading mention of path, so a
// message that already names path does not repeat it: an *fs.PathError for
// path gives just its Err ("permission denied"), and the default config
// reader's "<path>: ..." prefix is dropped.
func detailWithoutPath(err error, path string) string {
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Path == path {
		return pe.Err.Error()
	}
	return strings.TrimPrefix(err.Error(), path+": ")
}
