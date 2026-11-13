# demo

A small command line tool named `demo` that prints everything appdir resolves for it: profile, the two homes, every kind's path and the rule that picked it, config files, socket, discovery file and warnings, as text or JSON. It is the quickest way to see how a flag or a variable changes the answer.

Unlike the other examples, `demo` reads the real process environment, `$HOME` and the working folder, because it is meant to be run by hand. It creates nothing unless you pass `--init`, `--init-home` or `--ensure`.

## Run it

```sh
go run ./examples/demo --short
```

Flags: `--json`, `--short`, `--home DIR`, `--profile NAME`, `--daemon-profile NAME`, `--init` (create `.demo/` in the current folder), `--init-home DIR`, `--ensure KIND`, `--check-db PATH`. A release build: `go build -ldflags "-X main.release=true" ./examples/demo`.

## Output

Captured with `HOME` set to a fresh temp folder (shown as `$HOME`) and run from an empty folder inside another temp folder (shown as `$WORK`). The first line goes to stderr.

```text
$ demo --short
[demo] profile: dev → ~/.demo-dev
profile   dev (release=false, dev: +dirty)
global    $HOME/.demo-dev  [global]
project   none: no .demo/ here; run 'demo init' or pass --home
paths:
  shared        project  error: no usable project home (above)                        [project]
  db            project  error: no usable project home (above)                        [project]
  cache         global   $HOME/.demo-dev/cache [global]

$ demo --init --short
[demo] profile: dev → ~/.demo-dev
profile   dev (release=false, dev: +dirty)
global    $HOME/.demo-dev  [global]
project   $WORK/.demo  [project]  data=$WORK/.demo/profiles/dev
paths:
  shared        project  $WORK/.demo/profiles/dev/shared [project]
  db            project  $WORK/.demo/profiles/dev/demo.db [project]
  cache         global   $HOME/.demo-dev/cache [global]
socket    $WORK/.demo/profiles/dev/run/demo.sock (fallback=false)
discovery $WORK/.demo/profiles/dev/run/discovery.json (daemon profile dev)
init      $WORK/.demo
```

`--json` prints the same report as one JSON object; its `paths`, `config_files`, `socket` and `warnings` fields are appdir's own types, described in [paths --json and doctor](../../docs/reference/paths-json.md).

Do not run `--init` in your home folder itself: `.demo/` there is the global home, not a project.

## Tests

`go test ./examples/demo/` runs the command with `HOME` and the working folder set to a test temp folder and every `DEMO_*` variable cleared.

## Related

- [Options reference](../../docs/reference/options.md)
- [paths --json and doctor](../../docs/reference/paths-json.md)
- [First use](../../docs/start/first-use.md)
- [The doctor example](../doctor/README.md)
