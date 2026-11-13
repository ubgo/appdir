# Example: testing setup

A tiny tool written to be tested: `main.go` drives it with every seam (`WithEnv`, `WithHomeDir`, `WithWorkDir`, `WithBuildInfo`); `main_test.go` shows `appdir.ForTest`, the test-binary guard panic (recovered to show its message) and `WithTestGuard(false)`. Run `go test -v ./examples/testing-setup` to see the tests.

Everything runs inside a temp root: the user's home is a folder under it (`WithHomeDir`) and the environment is a map (`WithEnv`), so the program never touches your real home or reads your shell's variables.

## Run it

```sh
go run ./examples/testing-setup
```

## Output

Paths are relative to the temp root.

```text
profile prod (go-install)
saved home/.demo/demo.db
```

## Guides

- [Testing setup](../../docs/testing/for-test.md)
