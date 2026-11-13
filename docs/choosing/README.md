# Choosing

The guides under [docs/](../README.md) show how each feature works. These pages answer the question before that: which option do I want? Each one gives a decision table, short "if this, then that" rules, the traps people fall into, and for every option the guide that explains it and the example that runs it. Every rule here was checked against the code; where the design in [SPEC](../SPEC.md) and the code differ, these pages describe the code.

| Page | Read it when you are deciding |
|---|---|
| [Mode and scopes](mode-and-scopes.md) | whether your tool keeps its data per project or per user, which home one kind lives in, and what dropping a kind (`WithoutKind`) costs |
| [Finding the home](finding-the-home.md) | how far the lookup walks, and which way a user or a team should move a home: `--home`, `<APP>_HOME`, `<APP>_<KIND>_DIR`, or a `home:` / `db:` pointer |
| [Profiles](profiles.md) | how a binary becomes prod or dev, whether you need your own profiles, and when to call `ForProfile`, `DaemonProfile` and `CheckDB` |
| [By app type](by-app-type.md) | the option set for a project CLI, a daemon, a desktop app, a global tool, and a test suite |

## The short version

- Data that belongs to a repo: keep the default `ModeProject`. Data that belongs to the user: `WithMode(appdir.ModeGlobal)`.
- Always pass your `--home` and `--profile` flags with `WithHome` and `WithProfile`; an empty string means "not given".
- Pass `WithRelease` only when your release variable was set by `-ldflags`, so a plain `go install module@vX` still counts as a release.
- Call `CheckDB` before you open a database or migrate it, even when the prod home does not exist yet.
- `New` fails only on bad input (`ErrInvalidOption`, `ErrUnknownProfile`); a broken `--home` or pointer shows up as an error from `Path`, so check one kind at start-up.
- In tests, use the seams or `ForTest`; never the real home.

## Related

- [Learning path](../README.md) and the [feature index](../reference/features.md).
- [Options reference](../reference/options.md): every option with its default.
