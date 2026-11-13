# Example: daemon paths

Shows `RunPath` for every run file (with `WithRunFile`), `Socket` and `EnsureSocketDir` in `run/`, the socket fallback when the path is longer than `WithSocketMaxLen` (with `XDG_RUNTIME_DIR` pointed at a folder in the temp root, so nothing is created in `/tmp`), and `DaemonProfile`/`DaemonDiscovery` for `--daemon-profile`.

Everything runs inside a temp root: the user's home is a folder under it (`WithHomeDir`) and the environment is a map (`WithEnv`), so the program never touches your real home or reads your shell's variables.

## Run it

```sh
go run ./examples/daemon-paths
```

## Output

On Linux, with paths relative to the temp root. The socket hash differs on every run. On macOS the fallback is `/tmp/demo-<uid>/` and is not created; on Windows the socket is a named pipe.

```text
== run files
RunPath(lock): home/.demo/run/lock
RunPath(daemon-lock): home/.demo/run/daemon.lock
RunPath(pid): home/.demo/run/demo-daemon.pid
RunPath(discovery): home/.demo/run/discovery.json
RunPath(socket): home/.demo/run/demo.sock
RunPath(socket-file): home/.demo/run/socket
== socket in run/
Socket: home/.demo/run/demo.sock fallback=false
created home/.demo/run mode 0700
== socket fallback (path too long)
Socket: xdg-run/demo/23b257995ba6.sock fallback=true
PointerFile: home/.demo/run/socket
created xdg-run/demo mode 0700
client reads run/socket: xdg-run/demo/23b257995ba6.sock
== daemon profile
dev client                             -> DaemonProfile dev, discovery home/.demo-dev/run/discovery.json
dev client, --daemon-profile prod      -> DaemonProfile prod, discovery home/.demo/run/discovery.json
                                          [demo] dev client → prod daemon
dev client, DEMO_DAEMON_PROFILE=prod   -> DaemonProfile prod, discovery home/.demo/run/discovery.json
                                          [demo] dev client → prod daemon
dev client, --daemon-profile qa        -> error: appdir: --daemon-profile: unknown profile "qa" (known: prod, dev)
```

## Guides

- [Run files and sockets](../../docs/runtime/run-files-and-sockets.md)
