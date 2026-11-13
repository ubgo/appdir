# Promise table

Every promise appdir makes in [SPEC.md](../SPEC.md) and in these docs, each with the test that proves it. A promise is a sentence with never, always, must, refuses, only or "fails with", a named error or warning, or a definite rule the code keeps (an order, a default, a mode bit). When you change a promise, change its test and its row here; when you add one, add both.

The table has 550 rows. 206 of them name a test added for this table (marked "(new)"); the tests live in the `promise_*_test.go` files, plus `links_windows_test.go` for Windows junctions. 10 rows cannot be tested and say why in one line: each is about code outside appdir, a best-effort OS call, a module that does not exist yet, or a crash no test can stage. Tests named `TestWin...` and `TestDarwin...` run only on that OS, in the [real-OS test workflow](ci.md).

The section column is the SPEC section, or the docs page (relative to `docs/`) when the promise is made only there. A test written `TestX/sub` is a subtest.

## Sections 1–3: purpose, design rules, public API, options, errors

SPEC sections 1–3 and the reference pages (`reference/*.md`), with one row per sentinel error and per warning code.

| Section | Promise | Test | File |
|---|---|---|---|
| 1 | New creates nothing; reads only stat and read files | TestPromiseA_ReadsNeverWrite (new) | promise_a_test.go |
| 1 | every path records the rule that picked it | TestResolvePathsListing, TestResolveOverrideMatrix | resolve_matrix_test.go |
| 1 | appdir starts no process (agent/service install is the tool's) | TestNoProcessStart | platform_test.go |
| 2 | rule zero: every choice is an option with the standard default | TestPromiseA_OptionDefaults (new) | promise_a_test.go |
| 2 | renamed or replaced names give no warning (D17) | TestPromiseA_OptionSemantics/renamed_names_give_no_warning_(D17) (new) | promise_a_test.go |
| 2 | no package-level mutable variables (only sentinels), no init() | TestPromiseA_NoPackageState (new) | promise_a_test.go |
| 2 | every outside input goes through a seam with a default | TestPromiseA_DefaultSeams (new), TestPromiseA_ReadsNeverWrite (new) | promise_a_test.go |
| 2 | every outside input goes through a seam (TMPDIR for the temp fallback) | TestPromiseA_TempDirThroughSeam (new) | promise_a_test.go |
| 2 | reads never write: New, Paths, Path, Socket, ConfigFiles and every other read | TestPromiseA_ReadsNeverWrite (new), TestRuntimeRunPaths | promise_a_test.go, runtime_test.go |
| 2 | only Ensure, InitProject, InitHome, EnsureSocketDir, WriteGitignore, MoveLegacy write | TestPromiseA_ReadsNeverWrite (new) | promise_a_test.go |
| 2 | New fails only for bad options, unknown profile, unreadable working folder | TestPromiseA_ErrorsDeferred (new), TestPromiseA_UnreadableWorkDir (new) | promise_a_test.go, promise_a_linux_test.go |
| 2 | missing project stored; cache still works outside a project | TestPromiseA_ErrorsDeferred/outside_a_project (new) | promise_a_test.go |
| 2 | stdlib only: no require in go.mod, no third-party import | TestPromiseA_StdlibOnly (new) | promise_a_test.go |
| 3.1 | app must match ^[a-z0-9][a-z0-9._-]*$ | TestLayoutNewValidation, TestResolveOptionValidation | layout_test.go, resolve_matrix_test.go |
| 3.1 | New never creates anything | TestPromiseA_ReadsNeverWrite (new) | promise_a_test.go |
| 3.1 | *Dirs is immutable and safe for concurrent use | TestPromiseA_DirsImmutableConcurrent (new) | promise_a_test.go |
| 3.2 | constant string values (kinds, scopes, rules, run files, moves, warnings) | TestPromiseA_ConstantValues (new) | promise_a_test.go |
| 3.2 | DBAccess: AccessReadWrite 0, AccessReadOnly 1; String read-write/read-only | TestPromiseA_ConstantValues (new), TestRuntimeDBAccessString | promise_a_test.go, runtime_test.go |
| 3.2 | run files default to lock, daemon.lock, <app>.pid, discovery.json, <app>.sock, socket | TestRuntimeRunPaths | runtime_test.go |
| 3.3 | WithDirName default "." + app | TestPromiseA_OptionDefaults/WithDirName_default_.app (new) | promise_a_test.go |
| 3.3 | WithEnvPrefix default upper-cased app, - and . become _ | TestPromiseA_OptionDefaults/WithEnvPrefix_default_upper-cased,_-_and_._to__ (new) | promise_a_test.go |
| 3.3 | WithMode default ModeProject | TestPromiseA_OptionDefaults/WithMode_default_ModeProject (new) | promise_a_test.go |
| 3.3 | WithLookup default WalkUp | TestPromiseA_OptionDefaults/WithLookup_default_WalkUp (new) | promise_a_test.go |
| 3.3 | WithKind default the standard kinds | TestPromiseA_OptionDefaults/WithKind_default_the_standard_kinds (new), TestLayoutStandardKinds | promise_a_test.go, layout_test.go |
| 3.3 | WithoutKind drops a kind | TestLayoutWithKind/WithoutKind_drops_from_Paths,_block_and_Ensure | layout_test.go |
| 3.3 | WithGitCommit adds committed patterns (default from kinds) | TestLayoutGitignoreBlock | layout_test.go |
| 3.3 | WithGitIgnore wins over commit | TestLayoutGitignoreBlock/ignore_wins, TestFixF12_GitIgnoreEqualToCommittedKind | layout_test.go, runtime_fix_test.go |
| 3.3 | WithHome, WithProfile, WithDaemonProfile default unset | TestPromiseA_OptionDefaults/WithHome_WithProfile_WithDaemonProfile_default_unset (new) | promise_a_test.go |
| 3.3 | WithProfiles default prod, dev | TestPromiseA_OptionDefaults/WithProfiles_default_prod_and_dev (new) | promise_a_test.go |
| 3.3 | WithReleaseProfile prod keeps base names; WithDevProfile dev | TestPromiseA_OptionDefaults/WithReleaseProfile_prod_keeps_base_names,_WithDevProfile_dev (new) | promise_a_test.go |
| 3.3 | WithRelease default from build info; flag wins | TestPromiseA_OptionDefaults/WithRelease_default_from_build_info (new), TestRuntimeProfileMatrix | promise_a_test.go, runtime_test.go |
| 3.3 | WithAllowRoot default false; true lifts the root refusal | TestPromiseA_RootOwned (new) | promise_a_linux_test.go |
| 3.3 | WithLegacyHome default none: MoveLegacy is MoveNone | TestPromiseA_OptionDefaults/WithLegacyHome_default_none (new) | promise_a_test.go |
| 3.3 | LegacyHome.DaemonLocks default run/daemon.lock | TestRuntimeLegacyMove/old-daemon-lock-held, TestRuntimeLegacyMove/custom-daemon-locks-held | runtime_test.go |
| 3.3 | WithLegacyVerify runs after cross-disk copy; failure leaves old home | TestLegacyXDisk_CrashRecovery/after-copy-before-marker, TestLegacyXDisk_Move | legacy_xdisk_test.go |
| 3.3 | WithConfigReader default internal JSONC reader | TestPromiseA_OptionDefaults/WithConfigReader_default_internal_JSONC_reader (new) | promise_a_test.go |
| 3.3 | WithLayoutVersion default 1 | TestPromiseA_OptionDefaults/WithLayoutVersion_default_1 (new) | promise_a_test.go |
| 3.3 | WithRunFile renames one run file | TestRuntimeRunPaths | runtime_test.go |
| 3.3 | WithSocketMaxLen default 100 (100 stays, 101 falls back) | TestPromiseA_SocketMaxLenDefault (new), TestRuntimeSocket/exact-max-len-stays | promise_a_unix_test.go, runtime_test.go |
| 3.3 | WithPerm default 0700 folders, 0600 files | TestLayoutEnsure/folder_kind_0700_with_created_parents, TestLayoutWriteGitignore/new_file_gets_the_file_mode, TestLayoutEnsure/WithPerm_custom_modes | layout_test.go |
| 3.3 | WithTempFallback default on | TestPromiseA_OptionDefaults/WithTempFallback_default_on (new), TestLayoutTempFallback | promise_a_test.go, layout_test.go |
| 3.3 | WithCacheTag default on | TestPromiseA_OptionDefaults/WithCacheTag_default_on (new), TestLayoutCacheTag/WithCacheTag(false) | promise_a_test.go, layout_test.go |
| 3.3 | WithTestGuard default on: test binary panics outside the root | TestPromiseA_OptionDefaults/WithTestGuard_default_on (new), TestResolveTestGuard | promise_a_test.go, resolve_matrix_test.go |
| 3.3 | seams default os.LookupEnv, os.UserHomeDir, os.Getwd, debug.ReadBuildInfo | TestPromiseA_DefaultSeams (new) | promise_a_test.go |
| 3.3 | KindSpec "" scope means the mode's data scope | TestPromiseA_OptionSemantics/a_new_kind_starts_as_Folder(name),_Ignore(),_the_data_scope (new) | promise_a_test.go |
| 3.3 | WithKind on an existing name keeps its git rule and scope | TestPromiseA_OptionSemantics/WithKind_on_an_existing_kind_keeps_its_git_rule_and_scope (new), TestLayoutWithKind/renamed_kind_resolves_to_its_new_path | promise_a_test.go, layout_test.go |
| 3.3 | ConfigReader: a missing file wraps fs.ErrNotExist | — | not testable: a contract on the author's reader, not on appdir |
| 3.3 | rule: bad app name is ErrInvalidOption | TestLayoutNewValidation | layout_test.go |
| 3.3 | rule: dir name, run file, legacy dir must be one plain segment | TestLayoutNewValidation, TestFixF3_DirNameMustBePlainSegment | layout_test.go, resolve_fix_test.go |
| 3.3 | rule: WithDirName("..") refused, nothing above $HOME | TestPromiseA_SentinelCatalog/ErrInvalidOption (new), TestFixF3_DirNameMustBePlainSegment | promise_a_test.go, resolve_fix_test.go |
| 3.3 | rule: empty prefix, unknown mode/lookup, layout < 1, socket len < 1 | TestLayoutNewValidation | layout_test.go |
| 3.3 | rule: nil seam (WithEnv, WithHomeDir, WithBuildInfo, WithConfigReader) | TestLayoutNewValidation | layout_test.go |
| 3.3 | rule: empty profile list, release/dev profile not in list | TestLayoutNewValidation | layout_test.go |
| 3.3 | rule: kind name, kind path, kind scope validated | TestLayoutInvalidKinds | layout_test.go |
| 3.4 | App, EnvPrefix, EnvName, Mode accessors | TestResolveCustomNames, TestPromiseA_Methods (new) | resolve_matrix_test.go, promise_a_test.go |
| 3.4 | KindEnv "" for anchored kinds | TestPromiseA_OptionSemantics/an_anchored_kind_has_no_env_variable (new), TestResolveCustomNames | promise_a_test.go, resolve_matrix_test.go |
| 3.4 | Kinds in stable order | TestPromiseA_Methods (new), TestLayoutWithKind/order:_standard_first,_new_kinds_appended_in_call_order | promise_a_test.go, layout_test.go |
| 3.4 | Project ErrNoProject when none; HasProject agrees | TestPromiseA_SentinelCatalog/ErrNoProject (new), TestPromiseA_Methods (new) | promise_a_test.go |
| 3.4 | Path: kind in its effective home (both: project, else global) | TestPromiseA_Methods (new), TestLayoutKindPaths | promise_a_test.go, layout_test.go |
| 3.4 | PathIn takes only global or project | TestPromiseA_InvalidOptionOutsideNew/PathIn_both (new) | promise_a_test.go |
| 3.4 | Ensure creates the folder, or a file kind's parent only | TestLayoutEnsure/file_kind_creates_the_parent_only | layout_test.go |
| 3.4 | InitProject refuses the global home | TestPromiseA_InvalidOptionOutsideNew/InitProject_on_the_global_home (new), TestFixF14_InitProjectRejectsGlobalHome | promise_a_test.go, resolve_fix_test.go |
| 3.4 | WriteGitignore false outside a git work tree | TestPromiseA_Methods (new), TestLayoutWriteGitignore/not_a_git_work_tree_writes_nothing | promise_a_test.go, layout_test.go |
| 3.4 | VerifyFile applies the symlink rule | TestLayoutVerifyFile | layout_test.go |
| 3.4 | ReleaseReason values flag, go-install, dev: (devel) | TestRuntimeProfileMatrix, TestPromiseA_OptionDefaults/WithRelease_default_from_build_info (new) | runtime_test.go, promise_a_test.go |
| 3.4 | Name base for release, base-dev for dev; NameWith separator | TestPromiseA_OptionDefaults/WithReleaseProfile_prod_keeps_base_names,_WithDevProfile_dev (new), TestPromiseA_Methods (new) | promise_a_test.go |
| 3.4 | Banner "" for the release profile, "[app] profile: dev → ..." | TestRuntimeBanner, TestPromiseA_Methods (new) | runtime_test.go, promise_a_test.go |
| 3.4 | ChildEnv sets <APP>_PROFILE explicitly | TestRuntimeChildEnv | runtime_test.go |
| 3.4 | CleanEnv drops every <PREFIX>_* variable | TestRuntimeCleanEnv | runtime_test.go |
| 3.4 | ForProfile resolves another profile, original unchanged | TestRuntimeForProfile | runtime_test.go |
| 3.4 | DaemonProfile flag, env, own; DaemonDiscovery of that profile | TestRuntimeDaemonProfile | runtime_test.go |
| 3.4 | RunPath, Socket, EnsureSocketDir | TestRuntimeRunPaths, TestRuntimeSocket, TestRuntimeEnsureSocketDir | runtime_test.go |
| 3.4 | ForTest returns the test root | TestRuntimeForTest | runtime_test.go |
| 3.4 | result types carry the documented JSON tags and omitempty | TestPromiseA_JSONTags (new) | promise_a_test.go |
| 3.4 | Home.Info nil when home.jsonc is absent | TestResolveCustomNames | resolve_matrix_test.go |
| 3.4 | ConfigFile.Committed only for project config.jsonc in a git work tree | TestResolveConfigFilesMatrix | resolve_matrix_test.go |
| 3.4 | Socket.Dir "" for a Windows pipe | TestWinNamedPipeShape | platform_windows_test.go |
| 3.5 | every refusal wraps a sentinel; OS and parse errors keep their cause | TestPromiseA_SentinelCatalog (new), TestPromiseA_OSErrorsWrapCause (new) | promise_a_test.go |
| 3.5 | message names the subject and does not repeat the sentinel text | TestPromiseA_SentinelCatalog (new) | promise_a_test.go |
| 3.5 | user path errors prefixed --home, <APP>_HOME, <APP>_<KIND>_DIR, home/db in <file> | TestPromiseA_ErrorSourcePrefix (new) | promise_a_test.go |
| 3.5 | ErrInvalidOption: bad option set, wraps through %w | TestPromiseA_SentinelCatalog/ErrInvalidOption (new), TestLayoutNewValidation | promise_a_test.go, layout_test.go |
| 3.5 | ErrUnknownProfile: flag/env/daemon flag/daemon env, names the source | TestPromiseA_SentinelCatalog/ErrUnknownProfile (new), TestRuntimeDaemonProfile | promise_a_test.go, runtime_test.go |
| 3.5 | ErrUnknownKind from Path, PathIn, Ensure | TestPromiseA_SentinelCatalog/ErrUnknownKind (new), TestLayoutKindPaths | promise_a_test.go, layout_test.go |
| 3.5 | ErrNoProject: project kind with no project home | TestPromiseA_SentinelCatalog/ErrNoProject (new), TestLayoutKindPaths | promise_a_test.go, layout_test.go |
| 3.5 | ErrNoHome: global data kind, ~ path, Ensure of a no-path kind | TestPromiseA_SentinelCatalog/ErrNoHome (new), TestResolveNoHomeMatrix | promise_a_test.go, resolve_matrix_test.go |
| 3.5 | ErrEnvVar: ${VAR} unset or empty in a user path | TestPromiseA_SentinelCatalog/ErrEnvVar (new), TestResolveExpandUserPath | promise_a_test.go, resolve_matrix_test.go |
| 3.5 | ErrPointerAbsolute: absolute pointer in committed config.jsonc | TestPromiseA_SentinelCatalog/ErrPointerAbsolute (new), TestResolvePointerMatrix | promise_a_test.go, resolve_matrix_test.go |
| 3.5 | ErrPointerMissing: missing home: target or db: folder | TestPromiseA_SentinelCatalog/ErrPointerMissing (new), TestResolvePointerMatrix | promise_a_test.go, resolve_matrix_test.go |
| 3.5 | ErrPointerNotHome: target without this app's home.jsonc / not .<app> | TestPromiseA_SentinelCatalog/ErrPointerNotHome (new), TestFix2D3_PointerWithoutHomeKind | promise_a_test.go, fix2_test.go |
| 3.5 | ErrLayoutTooNew on a write, names the home.jsonc folder | TestPromiseA_SentinelCatalog/ErrLayoutTooNew (new), TestFixF15_LayoutErrorNamesHomeJSONFolder | promise_a_test.go, resolve_fix_test.go |
| 3.5 | ErrRootOwned: root writing into another user's home | TestPromiseA_RootOwned (new) | promise_a_linux_test.go |
| 3.5 | ErrSymlinkEscape: link outside the home, or broken | TestPromiseA_SentinelCatalog/ErrSymlinkEscape (new), TestLayoutVerifyFile | promise_a_test.go, layout_test.go |
| 3.5 | ErrUnsafeTemp: fallback folder symlink, not a folder, or not ours | TestPromiseA_SentinelCatalog/ErrUnsafeTemp (new), TestLayoutTempFallback, TestRuntimeEnsureSocketDir | promise_a_test.go, layout_test.go, runtime_test.go |
| 3.5 | ErrProdDB: dev build on the prod db | TestPromiseA_SentinelCatalog/ErrProdDB (new), TestRuntimeCheckDB | promise_a_test.go, runtime_test.go |
| 3.5 | ErrBusy: move lock or old daemon lock held | TestPromiseA_SentinelCatalog/ErrBusy (new), TestRuntimeLegacyMove/moving-lock-held | promise_a_test.go, runtime_test.go |
| 3.5 | ErrGitignoreMarkers: markers missing, doubled or out of order | TestPromiseA_SentinelCatalog/ErrGitignoreMarkers (new), TestLayoutWriteGitignore/markers | promise_a_test.go, layout_test.go |
| 3.5 | ErrNotHome: init on another app's home | TestPromiseA_SentinelCatalog/ErrNotHome (new), TestLayoutInitHome/another_app's_home_is_ErrNotHome | promise_a_test.go, layout_test.go |
| 3.5 | ErrBadHomeFile: unparsable home.jsonc refused, untouched, cause wrapped | TestPromiseA_SentinelCatalog/ErrBadHomeFile (new), TestFix2D6_InitBrokenHomeFile | promise_a_test.go, fix2_test.go |
| 3.5 | ErrInvalidOption for ~user in a user path | TestPromiseA_InvalidOptionOutsideNew/~user (new) | promise_a_test.go |
| 3.5 | ErrInvalidOption for unclosed ${ | TestPromiseA_InvalidOptionOutsideNew/unclosed_${ (new) | promise_a_test.go |
| 3.5 | ErrInvalidOption for a non-string home:/db: on every project-capable kind | TestPromiseA_InvalidOptionOutsideNew/pointer_not_a_string (new) | promise_a_test.go |
| 3.5 | ErrInvalidOption for InitHome scope both | TestPromiseA_InvalidOptionOutsideNew/InitHome_both (new) | promise_a_test.go |
| 3.5 | ErrInvalidOption for an unknown run file | TestPromiseA_InvalidOptionOutsideNew/RunPath_unknown (new) | promise_a_test.go |
| 3.5 | exit codes stay the tool's job | — | not testable: a statement about the tool, appdir maps no exit codes |
| 3.5 | warning profile-env-ignored in its trigger case | TestRuntimeProfileMatrix, TestRuntimeProfileWarningText | runtime_test.go |
| 3.5 | warning env-points-to-prod in its trigger case | TestResolveOverrideMatrix | resolve_matrix_test.go |
| 3.5 | warning pointer-chain-ignored in its trigger case | TestResolvePointerMatrix | resolve_matrix_test.go |
| 3.5 | warning pointer-ignored-for-profile names the real path | TestFixF13_PointerProfileWarningNamesRealPath, TestRuntimeCheckDBPointerWarnsDev | resolve_fix_test.go, runtime_test.go |
| 3.5 | warning ceiling-relative in its trigger case | TestResolveCeilingMatrix | resolve_matrix_test.go |
| 3.5 | warning ceiling-dropped in its trigger case | TestFix2D5_DroppedCeilingWarns, TestResolveCeilingMatrix | fix2_test.go, resolve_matrix_test.go |
| 3.5 | warning worktree-main-missing only when the worktree has a home | TestFixF5_WorktreeWarnsOnlyWhenWorktreeHasHome, TestResolveWorktreeMatrix | resolve_fix_test.go, resolve_matrix_test.go |
| reference/errors.md | New checks options before reading home, work dir, env | TestPromiseA_NewChecksOptionsFirst (new) | promise_a_test.go |
| reference/errors.md | errors.Is works through a caller's extra %w wrap | TestPromiseA_SentinelCatalog (new) | promise_a_test.go |
| reference/errors.md | ErrBadHomeFile also wraps fs.ErrPermission of an unreadable file | TestFix2D6_InitBrokenHomeFile | fix2_test.go |
| reference/errors.md | ErrLayoutTooNew only on write; reads on a too-new home work | TestPromiseA_ErrorsDeferred/layout_too_new (new) | promise_a_test.go |
| reference/errors.md | Paths never errors; broken kind has empty path and Error | TestResolvePathsListing, TestFixF2_BrokenOverrideFailsEveryKind | resolve_matrix_test.go, resolve_fix_test.go |
| reference/errors.md | doctor example maps every sentinel to its exit code | TestExitCode | examples/doctor/main_test.go |
| reference/features.md | InitProject relative dir resolves against WithWorkDir | TestLayoutInitProject/relative_dir_resolves_against_the_working_folder | layout_test.go |
| reference/features.md | WithoutKind(KindHome): init writes no home.jsonc | TestFixTL5_InitWithoutHomeKind | coord_fix_test.go |
| reference/features.md | JSONC reader: top-level null and backslash-newline are errors | TestObject, TestStandardizeSyntaxErrors | internal/jsonclite/jsonclite_test.go |
| reference/options.md | a setter given twice keeps the last value | TestPromiseA_OptionSemantics/a_setter_given_twice_keeps_the_last_value (new) | promise_a_test.go |
| reference/options.md | WithProfiles replaces the list | TestPromiseA_OptionSemantics/WithProfiles_replaces_the_list (new) | promise_a_test.go |
| reference/options.md | kind edits apply in order; WithoutKind of unknown name is a no-op | TestPromiseA_OptionSemantics/kind_edits_apply_in_order (new), TestPromiseA_OptionSemantics/WithoutKind_of_an_unknown_name_does_nothing (new) | promise_a_test.go |
| reference/options.md | new kind starts Folder(name), Ignore(), data scope | TestPromiseA_OptionSemantics/a_new_kind_starts_as_Folder(name),_Ignore(),_the_data_scope (new) | promise_a_test.go |
| reference/options.md | Ignore() turns a committed kind back to ignored | TestPromiseA_OptionSemantics/Ignore_turns_a_committed_kind_back_to_ignored (new) | promise_a_test.go |
| reference/options.md | Anchored kind ignores <APP>_<KIND>_DIR | TestPromiseA_OptionSemantics/an_anchored_kind_has_no_env_variable (new) | promise_a_test.go |
| reference/options.md | WithHomeDir error, empty or relative means no home folder | TestPromiseA_OptionSemantics/WithHomeDir_error,_empty_or_relative_means_no_home_folder (new) | promise_a_test.go |
| reference/options.md | a set but empty variable counts as unset | TestPromiseA_OptionSemantics/a_set_but_empty_variable_counts_as_unset (new), TestPromiseA_DefaultSeams (new) | promise_a_test.go |
| reference/options.md | returned slices are fresh copies | TestPromiseA_DirsImmutableConcurrent (new) | promise_a_test.go |
| reference/options.md | WithPerm ignored on Windows; owner-only ACL instead | TestWinOwnerOnlyACL | platform_windows_test.go |
| reference/paths-json.md | Paths in layout order; both kinds global then project | TestPromiseA_Methods (new), TestPDPathsJSONOrder | promise_a_test.go, promise_d_test.go |
| reference/paths-json.md | under --home a both kind is listed once, labelled global | TestFixX1_OverrideBothKindsLabelledGlobal | resolve_fix_test.go |
| reference/paths-json.md | ConfigFiles lists files that do not exist | TestPromiseA_Methods (new) | promise_a_test.go |
| reference/paths-json.md | MoveResult from/to omitted when empty | TestPromiseA_JSONTags (new), TestPromiseA_OptionDefaults/WithLegacyHome_default_none (new) | promise_a_test.go |
| reference/paths-json.md | Home.Fallback true with no home folder (rule defaults) | TestPromiseA_OptionSemantics/WithHomeDir_error,_empty_or_relative_means_no_home_folder (new) | promise_a_test.go |
| reference/platforms.md | Windows: no root guard, WithAllowRoot changes nothing, no temp owner check | TestWinNoRootGuard (new) | promise_a_windows_test.go |
| reference/platforms.md | Windows: Socket always Fallback, Dir empty; EnsureSocketDir still creates run/ | TestWinNamedPipeShape, TestFixF8_EnsureSocketDirCreatesRun | platform_windows_test.go, runtime_fix_test.go |
| reference/platforms.md | Linux socket fallback uses XDG_RUNTIME_DIR only if absolute existing folder | TestRuntimeSocket | runtime_test.go |
| reference/platforms.md | locks work between two real processes | TestTryLockAcrossProcesses | internal/flock/flock_process_test.go |
| reference/platforms.md | atomic write over an existing file | TestAtomicWriteOverExisting, TestWinAtomicWriteReplacesExisting | platform_test.go, platform_windows_test.go |

## Section 4: resolution

SPEC section 4 and the homes, pointer and start pages.

| Section | Promise | Test | File |
|---|---|---|---|
| 4.1 | flags only as options; os.Args never parsed | TestPromiseB_NeverParsesOSArgs (new) | promise_b_test.go |
| 4.1 | env read only through WithEnv seam | TestResolveOverrideMatrix | resolve_matrix_test.go |
| 4.1 | set-but-empty <APP>_* variable counts as unset | TestPromiseB_EmptyAppVarsCountAsUnset (new), TestResolveOverrideMatrix/TRAPP_HOME_set_but_empty_counts_as_unset | promise_b_test.go, resolve_matrix_test.go |
| 4.1 | set-but-empty ${VAR} counts as unset (ErrEnvVar) | TestResolveExpandUserPath/${EMPTY}/x, TestResolveOverrideMatrix/--home_${ALT}_set_but_empty_fails_every_kind | resolve_matrix_test.go |
| 4.1 | ~ and ~/… expand to the user home | TestResolveExpandUserPath | resolve_matrix_test.go |
| 4.1 | ${VAR} expands; $VAR without braces stays literal | TestResolveExpandUserPath, TestResolveOverrideMatrix/TRAPP_HOME_$ALT_without_braces_stays_literal | resolve_matrix_test.go |
| 4.1 | ~user and unclosed ${ are ErrInvalidOption | TestResolveExpandUserPath, TestResolveOverrideMatrix | resolve_matrix_test.go |
| 4.1 | unset ${VAR} is ErrEnvVar naming the variable | TestResolveExpandUserPath/unset_variable_is_named | resolve_matrix_test.go |
| 4.1 | relative flag/env paths resolve against the working folder | TestResolveOverrideMatrix/--home_relative_resolves_against_the_working_folder, TestResolveOverrideMatrix/TRAPP_DB_DIR_relative_resolves_against_the_working_folder | resolve_matrix_test.go |
| 4.1 | relative pointers resolve against the config file's folder | TestResolvePointerMatrix/committed_home:_relative_to_the_marker_folder, TestPromiseB_RelativeLocalPointerAgainstConfigFolder (new) | resolve_matrix_test.go, promise_b_test.go |
| 4.1 | unset ${VAR} in <APP>_<KIND>_DIR: error names variable, never /path | TestPromiseB_UnsetVarErrorNamesItsSource/kind_dir (new) | promise_b_test.go |
| 4.2 | profile picked before homes; it names both homes | TestResolveProfileHomes | resolve_matrix_test.go |
| 4.3 | WithHome is rule flag, one home for everything | TestResolveOverrideMatrix/--home_absolute:_one_home_for_every_kind | resolve_matrix_test.go |
| 4.3 | <APP>_HOME is rule env-home; --home beats it | TestResolveOverrideMatrix/TRAPP_HOME_absolute, TestResolveOverrideMatrix/--home_beats_TRAPP_HOME | resolve_matrix_test.go |
| 4.3 | non-release <APP>_HOME into prod home ignored, env-points-to-prod | TestResolveOverrideMatrix/dev:_TRAPP_HOME_=_prod_home_is_ignored | resolve_matrix_test.go |
| 4.3 | global home ~/.<app> prod, ~/.<app>-dev dev, rule global | TestResolveProfileHomes, TestResolveLookupMatrix | resolve_matrix_test.go |
| 4.3 | XDG_CONFIG/DATA/STATE_HOME never read | TestPromiseB_IgnoresXDGBaseDirs (new) | promise_b_test.go |
| 4.3 | no $HOME: global config is code defaults (rule defaults) | TestResolveNoHomeMatrix | resolve_matrix_test.go |
| 4.3 | no $HOME: global data kinds fail ErrNoHome naming <APP>_HOME | TestResolveNoHomeMatrix, TestPromiseB_NoHomeMessage (new) | resolve_matrix_test.go, promise_b_test.go |
| 4.3 | no $HOME: disposable kinds use the temp fallback | TestResolveNoHomeMatrix | resolve_matrix_test.go |
| 4.3 | unexpandable --home/<APP>_HOME fails every kind, cache included | TestFixF2_BrokenOverrideFailsEveryKind, TestResolveOverrideMatrix/--home_${NOPE}_unset_fails_every_kind | resolve_fix_test.go, resolve_matrix_test.go |
| 4.3 | broken override never uses temp fallback or code defaults | TestFixF2_BrokenOverrideFailsEveryKind, TestResolveNoHomeMatrix/UserHomeDir_fails/TRAPP_HOME=~/x_fails_every_kind_with_ErrNoHome | resolve_fix_test.go, resolve_matrix_test.go |
| 4.3 | broken override error starts with --home: / <APP>_HOME: | TestPromiseB_UnsetVarErrorNamesItsSource/flag, TestPromiseB_UnsetVarErrorNamesItsSource/env (new) | promise_b_test.go |
| 4.3 | override: project home is the same folder (no lookup) | TestResolveOverrideOneHome, TestResolveOverrideMatrix/--home_absolute:_one_home_for_every_kind | resolve_matrix_test.go |
| 4.4 | lookup skipped under --home / <APP>_HOME | TestResolveOverrideOneHome | resolve_matrix_test.go |
| 4.4 | start is EvalSymlinks(workdir); the real path is walked | TestResolveLookupMatrix/project/walk-up/symlink_into_a_project_subfolder, TestResolveSymlinkedHome | resolve_matrix_test.go |
| 4.4 | unresolvable workdir: lookup runs from cleaned absolute path | TestPromiseB_LookupFromMissingWorkDir (new) | promise_b_test.go |
| 4.4 | linked worktree translated to the main checkout (rule worktree) | TestResolveWorktreeMatrix, TestResolveLookupMatrix/project/walk-up/linked_worktree_subfolder | resolve_matrix_test.go |
| 4.4 | relative gitdir and absolute commondir both followed | TestResolveWorktreeMatrix/relative_gitdir_line, TestResolveWorktreeMatrix/absolute_commondir | resolve_matrix_test.go |
| 4.4 | main lacks home: worktree's own home plus worktree-main-missing | TestResolveWorktreeMatrix/main_has_none,_worktree_has_one:_worktree_with_a_warning | resolve_matrix_test.go |
| 4.4 | neither has a home: no project, no warning | TestFixF5_WorktreeWarnsOnlyWhenWorktreeHasHome | resolve_fix_test.go |
| 4.4 | bare main repo: worktree is its own project | TestResolveWorktreeMatrix/bare_main_repo:_worktree_is_its_own_project | resolve_matrix_test.go |
| 4.4 | git is never run | TestNoProcessStart | platform_test.go |
| 4.4 | submodule (no commondir) is its own project | TestResolveLookupMatrix/project/walk-up/submodule_with_its_own_home | resolve_matrix_test.go |
| 4.4 | worktree check walks with the same stop rules | TestResolveWorktreeMatrix/worktree-only_subfolder:_lookup_climbs_from_the_missing_main_path, TestResolveLookupMatrix/project/walk-up/plain_folder_under_$HOME | resolve_matrix_test.go |
| 4.4 | first D/.<app> folder is the project home | TestResolveLookupMatrix/project/walk-up/deep_subfolder | resolve_matrix_test.go |
| 4.4 | global home of release and current profile never a project home | TestResolveLookupHomeRejectsBothProfiles, TestResolveLookupMatrix/project/walk-up/$HOME_itself | resolve_matrix_test.go |
| 4.4 | any .git entry is a wall, even a dangling symlink | TestFixF7_DanglingGitSymlinkIsWall, TestResolveLookupMatrix/project/walk-up/nested_repo_without_a_home, TestResolveWorktreeMatrix/dangling_.git_symlink_is_a_wall | resolve_fix_test.go, resolve_matrix_test.go |
| 4.4 | walk starting at $HOME checks it and never climbs above | TestFixF17_WalkStopsAtHome | resolve_fix_test.go |
| 4.4 | walk starting at a ceiling checks it and never climbs above | TestFixX2_WalkStopsAtCeilingStart, TestResolveCeilingMatrix/start_in_the_ceiling_folder_holding_the_project:_found | coord_fix_test.go, resolve_matrix_test.go |
| 4.4 | walk stops before entering $HOME | TestResolveLookupMatrix/project/walk-up/plain_folder_under_$HOME | resolve_matrix_test.go |
| 4.4 | walk stops before entering a ceiling | TestResolveCeilingMatrix/ceiling_at_the_project_folder,_start_below:_not_found | resolve_matrix_test.go |
| 4.4 | walk stops at the filesystem root | TestPromiseB_WalkStopsAtDiskChange (new), TestResolveLookupMatrix/project/walk-up/outside_any_project | promise_b_unix_test.go, resolve_matrix_test.go |
| 4.4 | walk stops at a disk change (device id) | TestPromiseB_WalkStopsAtDiskChange (new) | promise_b_unix_test.go |
| 4.4 | Windows disk change uses the volume serial | TestWinDeviceIDMissingPath | platform_windows_test.go |
| 4.4 | HereOnly: worktree translated first, then only the start | TestFixF6_HereOnlyTranslatesWorktree, TestResolveLookupMatrix/project/here-only/deep_subfolder | resolve_fix_test.go, resolve_matrix_test.go |
| 4.4 | nothing found: project kinds ErrNoProject | TestResolveLookupMatrix | resolve_matrix_test.go |
| 4.4 | nothing found: project-else-global uses global, Fallback true | TestResolveLookupMatrix | resolve_matrix_test.go |
| 4.4 | ErrNoProject text names init and --home | TestResolveLookupMatrix | resolve_matrix_test.go |
| 4.4 | ceiling list uses OS separator; empty entries skipped | TestResolveCeilingMatrix/empty_list_entries_are_skipped | resolve_matrix_test.go |
| 4.4 | ceiling entries get ~ and ${VAR} expansion | TestResolveCeilingMatrix/${VAR}_entry_expands | resolve_matrix_test.go |
| 4.4 | relative ceiling entry ignored with ceiling-relative | TestResolveCeilingMatrix/relative_entry_ignored_with_a_warning | resolve_matrix_test.go |
| 4.4 | unexpandable ceiling entry ignored with ceiling-dropped naming it | TestFix2D5_DroppedCeilingWarns, TestFix2D5_RelativeVarCeilingWarns | fix2_test.go |
| 4.4 | folders compared by os.SameFile, not strings | TestResolveSymlinkedHome, TestDarwinCeilingOtherSpelling, TestWinDriveLetterCase | resolve_matrix_test.go, platform_darwin_test.go, platform_windows_test.go |
| 4.4 | nearest .<app> wins; nested repo is a wall | TestResolveLookupMatrix/project/walk-up/nested_repo_with_its_own_home, TestResolveLookupMatrix/project/walk-up/nested_repo_without_a_home | resolve_matrix_test.go |
| 4.5 | pointer keys read from config.jsonc then config.local.jsonc, local wins per key | TestResolvePointerMatrix/local_wins_over_committed_per_key | resolve_matrix_test.go |
| 4.5 | absent, null or "" pointer counts as not set | TestResolvePointerMatrix/null_value_counts_as_absent, TestResolvePointerMatrix/local_empty_string_does_not_clear_committed | resolve_matrix_test.go |
| 4.5 | committed pointer must be relative or ~/ | TestResolvePointerMatrix/committed_home:_~/_path, TestResolvePointerMatrix/committed_home:_relative_to_the_marker_folder | resolve_matrix_test.go |
| 4.5 | committed absolute, drive, UNC or $ value is ErrPointerAbsolute | TestResolvePointerMatrix/committed_home:_absolute_is_refused, TestResolvePointerMatrix/committed_home:_drive_path_is_refused, TestResolvePointerMatrix/committed_home:_UNC_path_is_refused, TestResolvePointerMatrix/committed_home:_${VAR}_is_refused_even_when_set | resolve_matrix_test.go |
| 4.5 | committed ~ alone, ~user, relative-valued ${VAR} refused | TestPromiseB_PointerRules/committed_~_alone_is_refused (new), TestPromiseB_PointerRules/committed_${VAR}_holding_a_relative_path_is_refused (new), TestResolvePointerMatrix/committed_home:_~bob_is_refused | promise_b_test.go, resolve_matrix_test.go |
| 4.5 | local file accepts any user path | TestResolvePointerMatrix/local_home:_absolute_is_fine, TestResolvePointerMatrix/local_home:_${VAR}_is_fine | resolve_matrix_test.go |
| 4.5 | home: target missing is ErrPointerMissing | TestResolvePointerMatrix/missing_target_is_ErrPointerMissing | resolve_matrix_test.go |
| 4.5 | home: target without this app's home.jsonc is ErrPointerNotHome | TestResolvePointerMatrix/target_without_home.jsonc_is_ErrPointerNotHome, TestResolvePointerMatrix/target_is_another_app's_home:_ErrPointerNotHome | resolve_matrix_test.go |
| 4.5 | without home kind, target must be named .<app> | TestFix2D3_PointerWithoutHomeKind | fix2_test.go |
| 4.5 | home: target becomes Base, rule pointer | TestResolvePointerMatrix/committed_home:_relative_to_the_marker_folder | resolve_matrix_test.go |
| 4.5 | one hop only; pointer-chain-ignored warning | TestResolvePointerMatrix/home:_target_pointer_is_not_followed_(one_hop) | resolve_matrix_test.go |
| 4.5 | db: folder must exist, file need not | TestResolvePointerMatrix/db:_pointer_in_local,_file_need_not_exist, TestResolvePointerMatrix/db:_pointer_folder_missing_is_ErrPointerMissing | resolve_matrix_test.go |
| 4.5 | db: moves only db; backups stays with the marker home | TestResolvePointerMatrix/db:_pointer_committed_relative | resolve_matrix_test.go |
| 4.5 | non-string home:/db: is ErrInvalidOption | TestResolvePointerMatrix/home:_non-string_value_is_ErrInvalidOption, TestResolvePointerMatrix/db:_non-string_value_is_ErrInvalidOption | resolve_matrix_test.go |
| 4.5 | pointer error fails every project-capable kind, no fallback | TestResolvePointerMatrix, TestFixF11_BrokenPointerFailsProjectCapableKinds | resolve_matrix_test.go, resolve_fix_test.go |
| 4.5 | pointer error: Project() errors, HasProject false | TestPromiseB_PointerRules/broken_pointer:_every_project-capable_line_fails,_global_side_works (new) | promise_b_test.go |
| 4.5 | pointer error shown on every project line of Paths | TestPromiseB_PointerRules/broken_pointer:_every_project-capable_line_fails,_global_side_works (new), TestResolvePathsListing/broken_pointer:_project_line_carries_the_error | promise_b_test.go, resolve_matrix_test.go |
| 4.5 | pointer error: ConfigFiles still lists marker's two files | TestFixF11_ConfigFilesListsMarkerFiles, TestResolvePointerMatrix | resolve_fix_test.go, resolve_matrix_test.go |
| 4.5 | pointer error: global kinds and global side of both unaffected | TestPromiseB_PointerRules/broken_pointer:_every_project-capable_line_fails,_global_side_works (new), TestResolvePointerMatrix | promise_b_test.go, resolve_matrix_test.go |
| 4.5 | pointer error writes nothing to the global home | TestFixF11_BrokenPointerFailsProjectCapableKinds | resolve_fix_test.go |
| 4.5 | non-release: home: resolves to X/profiles/<profile> | TestResolvePointerMatrix/dev:_home:_resolves_to_target/profiles/dev | resolve_matrix_test.go |
| 4.5 | non-release: db: ignored, warning names the real db path | TestResolvePointerMatrix/dev:_db:_pointer_is_ignored_with_a_warning_naming_the_real_path, TestFixF13_PointerProfileWarningNamesRealPath | resolve_matrix_test.go, resolve_fix_test.go |
| 4.5 | ErrPointerAbsolute and ErrPointerMissing message text | TestResolvePointerMatrix/committed_home:_absolute_is_refused, TestResolvePointerMatrix/missing_target_is_ErrPointerMissing | resolve_matrix_test.go |
| 4.6 | non-release global home is ~/<Name(dirName)> | TestResolveProfileHomes | resolve_matrix_test.go |
| 4.6 | non-release project: Base stays marker, Data profiles/<p> | TestResolveProfileHomes | resolve_matrix_test.go |
| 4.6 | non-anchored kinds under Data; anchored stay in Base | TestResolveProfileHomes | resolve_matrix_test.go |
| 4.6 | profiles/ ignored by the gitignore block | TestPromiseB_GitignoreBlockIgnoresProfiles (new) | promise_b_test.go |
| 4.6 | profile's own config.local.jsonc read last | TestResolveConfigFilesMatrix/dev_in_a_git_project:_profile_file_last | resolve_matrix_test.go |
| 4.6 | --home/<APP>_HOME used as given for every profile | TestResolveOverrideMatrix/dev:_--home_into_the_prod_home_is_used_(the_user_named_it), TestResolveOverrideMatrix/dev:_TRAPP_HOME_=_~/.trapp-dev_is_used | resolve_matrix_test.go |
| 4.7 | <APP>_<KIND>_DIR first, rule env-kind | TestResolveOverrideMatrix/TRAPP_CACHE_DIR_moves_only_the_cache | resolve_matrix_test.go |
| 4.7 | <APP>_<KIND>_DIR beats --home | TestResolveOverrideMatrix/TRAPP_LOGS_DIR_wins_over_--home_for_logs | resolve_matrix_test.go |
| 4.7 | no <APP>_<KIND>_DIR for anchored kinds | TestResolveOverrideMatrix/TRAPP_CONFIG_DIR_is_ignored:_config_is_anchored, TestLayoutKindEnv | resolve_matrix_test.go, layout_test.go |
| 4.7 | file kind keeps its file name in the kind dir | TestResolveOverrideMatrix/TRAPP_DB_DIR_is_a_folder;_the_db_keeps_its_file_name | resolve_matrix_test.go |
| 4.7 | kind dir into prod home ignored by non-release | TestResolveOverrideMatrix/dev:_TRAPP_DB_DIR_=_prod_home_is_ignored | resolve_matrix_test.go |
| 4.7 | db: pointer is step 2, rule pointer | TestResolvePointerMatrix/db:_pointer_in_local,_file_need_not_exist | resolve_matrix_test.go |
| 4.7 | scope picks the home: project, global, project-else-global, both | TestResolveLookupMatrix, TestLayoutKindPaths | resolve_matrix_test.go, layout_test.go |
| 4.7 | broken project fails project-else-global and both kinds | TestFixF11_BrokenPointerFailsProjectCapableKinds | resolve_fix_test.go |
| 4.7 | override: Paths lists a both kind once, labelled global | TestFixX1_OverrideBothKindsLabelledGlobal, TestResolveOverrideOneHome, TestPromiseB_OverrideLineLabels (new) | resolve_fix_test.go, resolve_matrix_test.go, promise_b_test.go |
| 4.7 | disposable with no home: <tmp>/<Name(app)>-<uid>, rule temp, Fallback | TestResolveNoHomeMatrix | resolve_matrix_test.go |
| 4.7 | broken override never reaches the temp fallback | TestFixF2_BrokenOverrideFailsEveryKind | resolve_fix_test.go |
| 4.7 | WithTempFallback(true): Ensure falls back on read-only error | TestLayoutTempFallback/read-only_home:_disposable_kind_falls_back_to_temp | layout_test.go |
| 4.7 | <uid> is the numeric uid on Unix | TestPromiseB_TempFallbackNumericUID (new) | promise_b_unix_test.go |
| 4.7 | <uid> is the user name on Windows | TestWinPromiseBTempFallbackUserName (new) | promise_b_windows_test.go |
| 4.8 | write checks never run in reads | TestPromiseB_ReadsNeverWriteOrCheck (new), TestPromiseB_RootGuard (new) | promise_b_test.go, promise_b_unix_test.go |
| 4.8 | euid 0 in another uid's home: ErrRootOwned naming folder, --allow-root | TestPromiseB_RootGuard (new) | promise_b_unix_test.go |
| 4.8 | WithAllowRoot(true) lets root write | TestPromiseB_RootGuard (new) | promise_b_unix_test.go |
| 4.8 | root check is a no-op on Windows | TestWinPromiseBRootGuardNoop (new) | promise_b_windows_test.go |
| 4.8 | layout above WithLayoutVersion refused with ErrLayoutTooNew | TestLayoutLayoutVersion | layout_test.go |
| 4.8 | layout error names the home.jsonc folder (profile data, db: pointer) | TestFixF15_LayoutErrorNamesHomeJSONFolder | resolve_fix_test.go |
| 4.8 | checks run in every write method (WriteGitignore too) | TestPromiseB_WriteGitignoreChecksWrite (new) | promise_b_test.go |
| 4.8 | failing mkdir: OS error wrapped with path and <APP>_HOME/<APP>_<KIND>_DIR hint | TestLayoutTempFallback/read-only_home:_disposable_kind_falls_back_to_temp, TestLayoutTempFallback/WithTempFallback(false)_names_the_folder_and_the_env_vars | layout_test.go |
| 4.8 | disposable kinds fall back to temp before failing | TestLayoutTempFallback | layout_test.go |
| homes/global-and-project.md | New creates nothing; Info nil until home exists | TestPromiseB_ReadsNeverWriteOrCheck (new), TestLayoutInitProject/creates_marker_and_home.jsonc | promise_b_test.go, layout_test.go |
| homes/global-and-project.md | Data equals Base for the release profile | TestResolveLookupMatrix | resolve_matrix_test.go |
| homes/global-and-project.md | Paths never fails; errors go in Entry.Error | TestResolvePathsListing, TestFixF11_BrokenPointerFailsProjectCapableKinds | resolve_matrix_test.go, resolve_fix_test.go |
| homes/global-and-project.md | no home: Global() rule defaults, empty Base/Data | TestResolveNoHomeMatrix | resolve_matrix_test.go |
| homes/global-and-project.md | WithDirName renames both homes; env names unchanged | TestPromiseB_DirNameKeepsEnvNames (new), TestResolveCustomNames | promise_b_test.go, resolve_matrix_test.go |
| homes/global-and-project.md | WithDirName empty, ., .., separator or NUL is ErrInvalidOption | TestFixF3_DirNameMustBePlainSegment | resolve_fix_test.go |
| homes/lookup.md | a file named .<app> is not a home and not a wall | TestPromiseB_MarkerFileIsNotAHome (new) | promise_b_test.go |
| homes/lookup.md | lookup runs once in New; later mkdir/chdir ignored | TestPromiseB_LookupIsASnapshot (new) | promise_b_test.go |
| homes/lookup.md | $VAR ceiling entry dropped with ceiling-dropped | TestResolveCeilingMatrix/$VAR_entry_without_braces_is_skipped_with_a_warning | resolve_matrix_test.go |
| homes/lookup.md | New succeeds with no project; cache still works | TestResolveLookupMatrix, TestPromiseB_NewFailsOnUnreadableWorkDir (new) | resolve_matrix_test.go, promise_b_test.go |
| homes/worktrees.md | gitdir pointing nowhere: plain folder, own home, no warning | TestPromiseB_WorktreeWithMissingMain (new) | promise_b_test.go |
| homes/worktrees.md | worktree-only home loses to the main checkout's | TestPromiseB_WorktreeOnlyHomeLosesToMain (new) | promise_b_test.go |
| homes/worktrees.md | .git file without gitdir: line is a plain repo root | TestResolveWorktreeMatrix/.git_file_without_a_gitdir_line_is_a_plain_repo_root | resolve_matrix_test.go |
| homes/overrides.md | WithHome("") means not given | TestPromiseB_EmptyHomeFlagIsNotGiven (new) | promise_b_test.go |
| homes/overrides.md | default env prefix: upper case, - and . become _ | TestPromiseB_DefaultEnvPrefix (new) | promise_b_test.go |
| homes/overrides.md | author kinds get <APP>_<KIND>_DIR; WithEnvPrefix honoured | TestLayoutKindEnv, TestResolveCustomNames | layout_test.go, resolve_matrix_test.go |
| homes/fallbacks.md | Name(app) in the temp root carries the dev profile | TestResolveNoHomeMatrix/UserHomeDir_fails/dev_build:_temp_root_carries_the_profile | resolve_matrix_test.go |
| homes/fallbacks.md | temp root created 0700 | TestLayoutTempFallback/read-only_home:_disposable_kind_falls_back_to_temp | layout_test.go |
| homes/fallbacks.md | temp root that is a symlink or not ours is ErrUnsafeTemp | TestLayoutTempFallback/temp_fallback_folder_that_is_a_symlink_is_ErrUnsafeTemp, TestPromiseB_RootGuard (new) | layout_test.go, promise_b_unix_test.go |
| homes/fallbacks.md | WithTempFallback(false) turns off both fallbacks | TestResolveNoHomeMatrix/UserHomeDir_fails/temp_fallback_off:_cache_fails, TestLayoutTempFallback/WithTempFallback(false)_names_the_folder_and_the_env_vars | resolve_matrix_test.go, layout_test.go |
| homes/fallbacks.md | reads still work in a read-only home | TestPromiseB_ReadOnlyHomeReadsWork (new) | promise_b_unix_test.go |
| homes/fallbacks.md | ~ in an override with no home is ErrNoHome | TestResolveNoHomeMatrix/UserHomeDir_fails/TRAPP_HOME=~/x_fails_every_kind_with_ErrNoHome | resolve_matrix_test.go |
| homes/fallbacks.md | ModeProject db with no home is ErrNoProject | TestResolveNoHomeMatrix/UserHomeDir_fails/project_mode_outside_a_project | resolve_matrix_test.go |
| pointers/pointer-keys.md | pointer config is JSONC (comments, trailing comma, // in strings) | TestResolvePointerMatrix/committed_home:_with_JSONC_comments_and_a_trailing_comma, TestStandardizeAccepted | resolve_matrix_test.go, internal/jsonclite/jsonclite_test.go |
| pointers/pointer-keys.md | dev profile: non-string db: still fails | TestPromiseB_PointerRules/dev:_non-string_db:_still_fails (new) | promise_b_test.go |
| pointers/pointer-keys.md | malformed config.jsonc fails the project | TestResolvePointerMatrix/malformed_config.jsonc_fails_the_project | resolve_matrix_test.go |
| start/first-use.md | app name must match ^[a-z0-9][a-z0-9._-]*$ | TestResolveOptionValidation, TestLayoutNewValidation | resolve_matrix_test.go, layout_test.go |
| start/first-use.md | New fails on an unreadable working folder | TestPromiseB_NewFailsOnUnreadableWorkDir (new) | promise_b_test.go |
| start/first-use.md | reads (New, Path, Paths) never create anything | TestPromiseB_ReadsNeverWriteOrCheck (new) | promise_b_test.go |
| start/first-use.md | Ensure creates folder 0700; file kind only its parent | TestLayoutEnsure | layout_test.go |
| start/first-use.md | Paths lists both-homes kinds once per home | TestResolvePathsListing/in_a_project:_both_kinds_once_per_home | resolve_matrix_test.go |
| start/first-use.md | unknown kind is ErrUnknownKind | TestResolvePathsListing/bad_arguments | resolve_matrix_test.go |
| start/init.md | InitProject makes .<app>/ 0700 with home.jsonc 0600 (app, layout, scope) | TestLayoutInitProject/creates_marker_and_home.jsonc | layout_test.go |
| start/init.md | .gitignore block only inside a git work tree | TestLayoutInitProject/inside_a_git_work_tree_writes_.gitignore, TestLayoutInitProject/creates_marker_and_home.jsonc | layout_test.go |
| start/init.md | home.jsonc written atomically, no temp left | TestAtomicWriteOverExisting, TestWinAtomicWriteReplacesExisting, TestLayoutInitHome | platform_test.go, platform_windows_test.go, layout_test.go |
| start/init.md | InitHome expands ~/${VAR}; relative against working folder | TestLayoutInitHome | layout_test.go |
| start/init.md | InitHome on this app's home is a no-op; another app's is ErrNotHome | TestLayoutInitHome/existing_home_of_this_app_is_kept, TestLayoutInitHome/another_app's_home_is_ErrNotHome | layout_test.go |
| start/init.md | InitProject refuses the global home by identity, existing or not | TestLayoutInitProject/on_the_global_home_exists=false, TestLayoutInitProject/on_the_global_home_through_a_symlinked_path, TestFixF14_InitProjectRejectsGlobalHome | layout_test.go, resolve_fix_test.go |
| start/init.md | relative InitProject dir joins WithWorkDir, not process cwd | TestLayoutInitProject/relative_dir_resolves_against_the_working_folder | layout_test.go |
| start/init.md | unparsable home.jsonc is ErrBadHomeFile, untouched, named once | TestFix2D6_InitBrokenHomeFile, TestFixTL4_InitHomeKeepsUnreadableHomeFile | fix2_test.go, coord_fix_test.go |
| start/init.md | broken gitignore markers: marker returned with ErrGitignoreMarkers | TestLayoutInitProject/broken_markers_fail_after_home.jsonc | layout_test.go |
| start/init.md | layout too new fails before anything is written | TestLayoutInitProject/layout_too_new_is_refused, TestLayoutLayoutVersion | layout_test.go |
| start/init.md | InitHome only accepts ScopeGlobal or ScopeProject | TestLayoutInitHome/scope_both | layout_test.go |
| start/init.md | init as root in another user's home is ErrRootOwned | TestPromiseB_RootGuard (new) | promise_b_unix_test.go |

## Sections 5–7: layout, config files, run paths

SPEC sections 5–7 and the layout, run files and choosing pages.

| Section | Promise | Test | File |
|---|---|---|---|
| 5.1 | standard kinds: names, paths, types, git rule, scope, flags | TestLayoutStandardKinds | layout_test.go |
| 5.1 | each standard kind lands in the right home per mode | TestLayoutKindPaths | layout_test.go |
| 5.1 | logs: project-else-global in project mode, global in global mode | TestLayoutStandardKinds, TestLayoutKindPaths | layout_test.go |
| 5.1 | profile data under profiles/<p>/, anchored files stay in .<app>/ | TestResolveProfileHomes | resolve_matrix_test.go |
| 5.2 | data scope: project in ModeProject, global in ModeGlobal | TestLayoutStandardKinds, TestLayoutWithKind | layout_test.go |
| 5.2 | ModeProject is the default: data kinds fail ErrNoProject outside a project | TestLayoutEnsure/project_kind_outside_a_project | layout_test.go |
| 5.2 | ErrNoProject says "no .<app>/ here; run '<app> init' or pass --home" | TestResolveLookupMatrix, TestRun | resolve_matrix_test.go, examples/project-init/main_test.go |
| 5.2 | ModeGlobal: db is ~/.<app>/<app>.db wherever you run | TestLayoutKindPaths/global_mode_inside_a_project, TestLayoutKindPaths/global_mode_outside_a_project | layout_test.go |
| 5.2 | Scoped(ScopeGlobal) identity kind is global, in or out of a project | TestPCIdentityKindGlobalInProjectMode (new) | promise_c_test.go |
| choosing/mode-and-scopes.md | Scoped(ScopeProject) under ModeGlobal still needs a project | TestPCScopedProjectUnderGlobalMode (new) | promise_c_test.go |
| choosing/mode-and-scopes.md | global run kind in ModeProject: RunPath/Socket work outside a project | TestPCGlobalRunKindInProjectMode (new) | promise_c_test.go |
| choosing/mode-and-scopes.md | logs entry is marked Fallback only in project mode | TestLayoutKindPaths | layout_test.go |
| choosing/mode-and-scopes.md | no-$HOME temp fallback is for global kinds only | TestPCDisposableProjectKindNoTempWithoutHome (new) | promise_c_test.go |
| choosing/mode-and-scopes.md | read-only fallback of Ensure works in the project home too | TestPCReadOnlyProjectDisposableFallsBack (new) | promise_c_unix_test.go |
| choosing/mode-and-scopes.md | unknown scope fails New with ErrInvalidOption | TestLayoutInvalidKinds/unknown_scope, TestLayoutInvalidKinds/unknown_scope_on_existing | layout_test.go |
| choosing/mode-and-scopes.md | Ignore() on a kind changes only the block, path stays | TestLayoutWithKind/Ignore_on_committed_shared, TestLayoutGitignoreBlock/shared_made_ignored | layout_test.go |
| 5.3 | new kind starts as Folder(name), Ignore(), data scope | TestLayoutWithKind | layout_test.go |
| 5.3 | WithKind on an existing name changes it in place | TestLayoutWithKind | layout_test.go |
| 5.3 | kind names must match ^[a-z][a-z0-9-]*$ (ErrInvalidOption) | TestLayoutInvalidKinds | layout_test.go |
| 5.3 | kind paths relative, slash-separated, no .. (ErrInvalidOption) | TestLayoutInvalidKinds | layout_test.go |
| 5.3 | WithoutKind drops a kind from Paths, block and Ensure | TestLayoutWithKind/WithoutKind_drops_from_Paths,_block_and_Ensure | layout_test.go |
| 5.3 | non-anchored kinds get <APP>_<KIND>_DIR, upper-cased, - to _ | TestLayoutKindEnv | layout_test.go |
| 5.3 | anchored kinds get no <APP>_<KIND>_DIR | TestLayoutKindEnv | layout_test.go |
| 5.3 | no kind produces a reserved env name | TestPCReservedEnvNamesNeverProduced (new) | promise_c_test.go |
| 5.3 | author kind is listed by Paths and moved by its env var | TestPCAuthorKindEnvMovesIt (new) | promise_c_test.go |
| layout/kinds.md | Kinds(): standard kinds first, new ones in call order | TestLayoutWithKind/order:_standard_first,_new_kinds_appended_in_call_order | layout_test.go |
| layout/kinds.md | WithoutKind then WithKind adds a brand-new kind | TestLayoutWithKind/WithKind_after_WithoutKind_starts_fresh | layout_test.go |
| layout/kinds.md | dropped kind: Path, PathIn, Ensure give ErrUnknownKind with its message | TestPCDroppedKindIsUnknownEverywhere (new) | promise_c_test.go |
| layout/kinds.md | Anchored() kind stays in the home root in every profile | TestPCAnchoredAuthorKindStaysInBase (new) | promise_c_test.go |
| layout/kinds.md | a rename keeps the kind name, git rule, scope and env var | TestLayoutWithKind, TestRun | layout_test.go, examples/custom-layout/main_test.go |
| layout/kinds.md | Disposable() lets Ensure fall back to the temp folder | TestLayoutTempFallback | layout_test.go |
| 5.4 | Ensure and Init* create folders 0700 and files 0600 | TestLayoutEnsure, TestLayoutInitProject/creates_marker_and_home.jsonc, TestLayoutInitHome | layout_test.go |
| 5.4 | WithPerm sets the modes of created folders and written files | TestLayoutEnsure/WithPerm_custom_modes, TestLayoutWriteGitignore/new_file_gets_the_file_mode | layout_test.go |
| 5.4 | existing permissions are left alone | TestLayoutEnsure/existing_folder_perms_untouched, TestLayoutWriteGitignore/existing_file_keeps_its_mode | layout_test.go |
| 5.4 | Windows: protected DACL for the user SID and SYSTEM only | TestWinOwnerOnlyACL | platform_windows_test.go |
| 5.5 | disposable folder gets CACHEDIR.TAG with the standard signature | TestLayoutCacheTag | layout_test.go |
| 5.5 | non-disposable and file kinds are never tagged | TestLayoutCacheTag | layout_test.go |
| 5.5 | WithCacheTag(false) writes no tag | TestLayoutCacheTag/WithCacheTag(false) | layout_test.go |
| 5.5 | macOS: empty .metadata_never_index (tmutil exclusion best effort) | TestDarwinCacheIndexMarkers (new), TestDarwinPDCacheNeverIndex | promise_c_darwin_test.go, promise_d_darwin_test.go |
| 5.5 | macOS: tmutil is best effort, never blocks the cache | TestDarwinCacheWithoutTmutil (new) | promise_c_darwin_test.go |
| 5.5 | tmutil is the one command appdir runs | TestNoProcessStart | platform_test.go |
| 5.5 | Windows: NOT_CONTENT_INDEXED on disposable folders only | TestWinCacheIndexAttrOnlyOnTaggedCache (new), TestWinPDCacheNotContentIndexed | promise_c_windows_test.go, promise_d_windows_test.go |
| layout/files-and-permissions.md | tag written when created or missing; an edited tag is kept | TestLayoutCacheTag | layout_test.go |
| layout/files-and-permissions.md | WithCacheTag(false) never removes an earlier tag | TestPCCacheTagOffKeepsOldTag (new) | promise_c_test.go |
| layout/files-and-permissions.md | Ensure on a file kind creates its folder, never the file | TestLayoutEnsure/file_kind_creates_the_parent_only | layout_test.go |
| layout/files-and-permissions.md | ErrLayoutTooNew refuses writes in Ensure and Init* | TestLayoutLayoutVersion, TestLayoutInitProject/layout_too_new_is_refused | layout_test.go |
| 5.6 | a home folder may be a symlink | TestLayoutVerifyFile, TestResolveSymlinkedHome | layout_test.go, resolve_matrix_test.go |
| 5.6 | file kind may link only inside its home, else ErrSymlinkEscape | TestLayoutVerifyFile | layout_test.go |
| 5.6 | Ensure verifies file kinds that already exist | TestLayoutVerifyFile | layout_test.go |
| 5.6 | CheckDB runs the symlink rule | TestRuntimeCheckDB, TestPCVerifyFileMessages (new) | runtime_test.go, promise_c_unix_test.go |
| 5.6 | Windows: directory junctions count as links | TestWinJunctionEscape (new) | links_windows_test.go |
| 5.6 | appdir never creates links (outside the legacy copy) | TestPCNoLinkCreation (new), TestPDNeverMakesLinks | promise_c_test.go, promise_d_test.go |
| 5.6 | appdir never creates a link of its own (legacy copy keeps user links) | TestPCNoLinkCreation (new) | promise_c_test.go |
| layout/files-and-permissions.md | ErrSymlinkEscape texts: "links to ... outside the home", "broken symlink" | TestPCVerifyFileMessages (new) | promise_c_unix_test.go |
| layout/files-and-permissions.md | a broken link fails VerifyFile, Ensure and CheckDB | TestLayoutVerifyFile, TestPCVerifyFileMessages (new) | layout_test.go, promise_c_unix_test.go |
| layout/files-and-permissions.md | a folder kind that is a symlink is followed unchecked | TestPCFolderKindSymlinkFollowed (new) | promise_c_unix_test.go |
| layout/files-and-permissions.md | VerifyFile gives nil for a missing or plain file | TestLayoutVerifyFile | layout_test.go |
| layout/files-and-permissions.md | file outside every home checked against its own folder | TestLayoutVerifyFile/file_outside_every_home_checks_against_its_own_folder | layout_test.go |
| layout/files-and-permissions.md | dev db linked to the prod db in .<app>/ escapes | TestPCDevDBLinkedIntoMarkerEscapes (new), TestRuntimeCheckDB | promise_c_unix_test.go, runtime_test.go |
| 5.7 | block starts with * and holds the standard re-includes | TestLayoutGitignoreBlock | layout_test.go |
| 5.7 | committed file kind: parents re-included, then !p | TestLayoutGitignoreBlock | layout_test.go |
| 5.7 | committed folder kind: parents, then !p/ and !p/** | TestLayoutGitignoreBlock | layout_test.go |
| 5.7 | WithGitCommit: parents of the fixed prefix, then the pattern | TestLayoutGitignoreBlock, TestPCGitCommitQuestionMarkStopsParents (new) | layout_test.go, promise_c_test.go |
| 5.7 | WithGitIgnore equal to a commit entry drops it, not rewritten | TestLayoutGitignoreBlock, TestFixF12_GitIgnoreEqualToCommittedKind | layout_test.go, runtime_fix_test.go |
| 5.7 | other WithGitIgnore patterns follow the re-includes and win | TestLayoutGitignoreBlock | layout_test.go |
| 5.7 | new files and profiles/ stay ignored by * | TestLayoutGitignoreBlock/default_layout, TestPromiseB_GitignoreBlockIgnoresProfiles | layout_test.go, promise_b_test.go |
| 5.7 | WriteGitignore writes only in a git work tree (.git here or above) | TestLayoutWriteGitignore | layout_test.go |
| 5.7 | WriteGitignore writes into the project home's Base/.gitignore | TestPCWriteGitignoreIntoBase (new) | promise_c_test.go |
| 5.7 | replaces text between markers, keeps other lines, appends without | TestLayoutWriteGitignore | layout_test.go |
| 5.7 | missing, doubled or out-of-order marker: ErrGitignoreMarkers, no write | TestLayoutWriteGitignore | layout_test.go |
| layout/gitignore.md | the work-tree check walks up past $HOME | TestPCWriteGitignoreAboveHome (new) | promise_c_test.go |
| layout/gitignore.md | markers are whole lines; trailing \r allowed; indented counts missing | TestLayoutWriteGitignore/CRLF_markers_are_recognised, TestPCWriteGitignoreMarkerLines (new) | layout_test.go, promise_c_test.go |
| layout/gitignore.md | ErrGitignoreMarkers message text names the file | TestPCWriteGitignoreMarkerLines (new) | promise_c_test.go |
| layout/gitignore.md | dropping the gitignore kind turns the write off | TestLayoutWriteGitignore/gitignore_kind_removed_writes_nothing | layout_test.go |
| layout/gitignore.md | write is atomic; an existing .gitignore keeps its mode | TestLayoutWriteGitignore, TestLayoutWriteFailures | layout_test.go |
| layout/gitignore.md | appdir never runs git | TestNoProcessStart | platform_test.go |
| 5.8 | Init* write home.jsonc {app, layout, scope} | TestLayoutInitProject, TestLayoutInitHome | layout_test.go |
| 5.8 | atomic write: temp in the same folder, renamed, no temp left | TestAtomicWriteOverExisting, TestLayoutWriteFailures | platform_test.go, layout_test.go |
| 5.8 | atomic write fsyncs the temp file before the rename | — | not testable: durability across a crash needs a power cut; no seam observes fsync |
| 5.8 | Windows retries the rename on a sharing violation | TestWinAtomicWriteRetriesSharingViolation (new), TestWinPDRenameRetriesWhileTargetHeld | promise_c_windows_test.go, promise_d_windows_test.go |
| 5.8 | New reads home.jsonc and exposes it as Home.Info | TestLayoutInitProject, TestPCHomeInfoOfGlobalHome (new) | layout_test.go, promise_c_test.go |
| 5.8 | relative InitProject dir resolves against WithWorkDir | TestLayoutInitProject/relative_dir_resolves_against_the_working_folder | layout_test.go |
| 5.8 | unreadable home.jsonc: ErrBadHomeFile, named once, never replaced | TestFix2D6_InitBrokenHomeFile, TestFixTL4_InitHomeKeepsUnreadableHomeFile | fix2_test.go, coord_fix_test.go |
| 5.8 | Init* create the parents of a nested home kind | TestLayoutInitHome/home.jsonc_kind_renamed, TestPCInitProjectNestedHomeKind (new) | layout_test.go, promise_c_test.go |
| 5.8 | without the home kind Init* only create the folder | TestFixTL5_InitWithoutHomeKind | coord_fix_test.go |
| 5.8 | InitProject refuses the global home (release or current profile) | TestLayoutInitProject, TestFixF14_InitProjectRejectsGlobalHome | layout_test.go, resolve_fix_test.go |
| 5.8 | global home compared by identity (symlinked spelling refused) | TestLayoutInitProject/on_the_global_home_through_a_symlinked_path | layout_test.go |
| 5.8 | before ~/.<app> exists: parent identity + name, case-insensitive on macOS/Windows | TestFixF14_InitProjectRejectsGlobalHome, TestDarwinInitProjectHomeOtherCase (new), TestWinInitProjectHomeOtherCase (new) | resolve_fix_test.go, promise_c_darwin_test.go, promise_c_windows_test.go |
| 5.8 | refusal reads "init: <dir> is the global home, not a project" | TestRun | examples/project-init/main_test.go |
| 6.1 | read order: global pair, project pair, target pair, profile file | TestResolveConfigFilesMatrix | resolve_matrix_test.go |
| 6.1 | project config.jsonc Committed only inside a git work tree | TestResolveConfigFilesMatrix | resolve_matrix_test.go |
| 6.1 | non-release profile file last, in the data folder, Profile set | TestResolveConfigFilesMatrix | resolve_matrix_test.go |
| 6.1 | a release build never lists the profile file | TestResolveConfigFilesMatrix | resolve_matrix_test.go |
| 6.1 | missing files are listed | TestResolveConfigFilesMatrix | resolve_matrix_test.go |
| 6.1 | no $HOME: the global files are left out | TestResolveConfigFilesMatrix | resolve_matrix_test.go |
| 6.1 | --home/<APP>_HOME: one home's two files, Home global, not committed | TestResolveConfigFilesMatrix | resolve_matrix_test.go |
| 6.1 | broken pointer: marker files listed, target and profile files not | TestFixF11_ConfigFilesListsMarkerFiles | resolve_fix_test.go |
| 6.1 | a dropped config kind is left out of every home | TestResolveConfigFilesDroppedKind, TestRun | resolve_matrix_test.go, examples/config-files/main_test.go |
| 6.1 | later file wins per key (3000, 4000, dev 4100) | TestRun | examples/config-files/main_test.go |
| 6.1 | env and flags are merged after, in cfgkit | — | not testable: the merge is done by the config loader (cfgkit), not appdir |
| layout/config-files.md | pointer keys are read only from the config kinds that remain | TestResolveConfigFilesDroppedKind, TestRun | resolve_matrix_test.go, examples/config-files/main_test.go |
| layout/config-files.md | plain missing-file error fails the project "read <file>"; no home.jsonc = no info | TestPCConfigReaderPlainMissingError (new) | promise_c_test.go |
| layout/config-files.md | appdir reads only home.jsonc and files that may hold home:/db: | TestPCConfigReaderSeam (new), TestRun | promise_c_test.go, examples/config-files/main_test.go |
| 6.3 | WithConfigReader replaces the reader for home.jsonc and pointer keys | TestPCConfigReaderSeam (new) | promise_c_test.go |
| 6.3 | WithConfigReader(nil) is ErrInvalidOption | TestLayoutNewValidation/nil_config_reader | layout_test.go |
| 6.3 | default reader drops // and /* */ comments and trailing commas | TestStandardizeAccepted, TestPCDefaultReaderJSONC (new) | internal/jsonclite/jsonclite_test.go, promise_c_test.go |
| 6.3 | strings walked: // in a string is text, escapes honoured | TestStandardizeAccepted, TestPCDefaultReaderJSONC (new) | internal/jsonclite/jsonclite_test.go, promise_c_test.go |
| 6.3 | fails on an unterminated comment or string | TestStandardizeSyntaxErrors, TestPCDefaultReaderJSONC (new) | internal/jsonclite/jsonclite_test.go, promise_c_test.go |
| 6.3 | fails on a backslash before a raw newline in a string | TestStandardizeSyntaxErrors/escaped_newline_in_string, TestPCDefaultReaderJSONC (new) | internal/jsonclite/jsonclite_test.go, promise_c_test.go |
| 6.3 | fails on a top level that is not an object, null included | TestObject, TestPCDefaultReaderJSONC (new) | internal/jsonclite/jsonclite_test.go, promise_c_test.go |
| 6.3 | replaced by ubgo/jsonc when it exists | — | not testable: a plan for a module that does not exist yet |
| 7 | RunPath: lock, daemon.lock, <app>.pid, discovery.json, <app>.sock, socket in run/ | TestRuntimeRunPaths | runtime_test.go |
| 7 | run/ is in the data home of run's scope and profile | TestRuntimeRunPaths | runtime_test.go |
| 7 | WithRunFile renames; the name must be one path segment | TestRuntimeRunPaths, TestLayoutNewValidation | runtime_test.go, layout_test.go |
| 7 | Socket is run/<app>.sock while it fits WithSocketMaxLen | TestRuntimeSocket/short-under-run, TestRuntimeSocket/exact-max-len-stays | runtime_test.go |
| 7 | WithSocketMaxLen defaults to 100 bytes | TestPCSocketDefaultMaxLen (new) | promise_c_test.go |
| 7 | Linux fallback dir: $XDG_RUNTIME_DIR/<Name(app)>/ | TestRuntimeSocket/fallback-xdg, TestRuntimeSocket/fallback-prod-name | runtime_test.go |
| 7 | otherwise the literal /tmp/<Name(app)>-<uid>/ | TestPCSocketTmpFallbackWithoutXDG (new), TestDarwinSocketFallbackLiteralTmp | promise_c_unix_test.go, platform_darwin_test.go |
| 7 | hash: first 12 hex of SHA-256 of the real path of run/ | TestRuntimeSocket/fallback-xdg, TestRuntimeSocket/fallback-hash-stable-and-unique | runtime_test.go |
| 7 | fallback: Fallback true, PointerFile is run/socket | TestRuntimeSocket/fallback-xdg | runtime_test.go |
| 7 | Windows: always \\.\pipe\<Name(app)>-<user>-<hash>, Fallback true | TestWinNamedPipeShape | platform_windows_test.go |
| 7 | EnsureSocketDir creates run/ in every case | TestFixF8_EnsureSocketDirCreatesRun, TestFixF8_EnsureSocketDirFallbackCreatesRun, TestRuntimeEnsureSocketDir | runtime_fix_test.go, runtime_test.go |
| 7 | Unix fallback folder created 0700 | TestRuntimeEnsureSocketDir/fallback-creates-both | runtime_test.go |
| 7 | fallback folder refused (ErrUnsafeTemp) if a symlink or not a folder | TestRuntimeEnsureSocketDir/fallback-symlink-refused, TestRuntimeEnsureSocketDir/fallback-is-a-file-refused | runtime_test.go |
| 7 | fallback folder refused if owned by another uid | TestPCEnsureSocketDirRefusesForeignOwner (new), TestPDPrivateDirRefusesOtherOwner | promise_c_unix_test.go, promise_d_unix_test.go |
| 7 | Windows: EnsureSocketDir creates only run/, Dir empty | TestWinNamedPipeShape | platform_windows_test.go |
| 7 | DaemonProfile: flag, then <APP>_DAEMON_PROFILE, then own | TestRuntimeDaemonProfile | runtime_test.go |
| 7 | unknown daemon profile is ErrUnknownProfile from New | TestRuntimeDaemonProfile | runtime_test.go |
| 7 | DaemonDiscovery: the daemon profile's discovery file | TestRuntimeDaemonProfile | runtime_test.go |
| 7 | appdir only gives the path: nothing created or opened | TestPCRunFilesNotCreated (new) | promise_c_test.go |
| runtime/run-files-and-sockets.md | appdir creates none of the run files | TestPCRunFilesNotCreated (new), TestRuntimeRunPaths | promise_c_test.go, runtime_test.go |
| runtime/run-files-and-sockets.md | XDG_RUNTIME_DIR used only if an existing absolute folder | TestPCSocketTmpFallbackWithoutXDG (new) | promise_c_unix_test.go |
| runtime/run-files-and-sockets.md | dev and prod daemons never share a socket | TestRuntimeSocket/fallback-hash-stable-and-unique, TestRuntimeSocket/fallback-prod-name | runtime_test.go |
| runtime/run-files-and-sockets.md | project mode outside a project: ErrNoProject from RunPath and Socket | TestRuntimeRunPaths/no-project, TestRuntimeSocket/no-project | runtime_test.go |

## Sections 8–14: profiles, legacy move, tests, JSON, platforms

SPEC sections 8–14 and the profiles, legacy move and testing pages.

| Section | Promise | Test | File |
|---|---|---|---|
| 8.1 | --profile wins over env and build | TestRuntimeProfileMatrix/*/flag-beats-env, TestRuntimeProfileMatrix/*/flag-prod-beats-env-dev | runtime_test.go |
| 8.1 | --profile outside the list is ErrUnknownProfile | TestRuntimeProfileMatrix/*/flag-unknown | runtime_test.go |
| 8.1 | <APP>_PROFILE unknown is ErrUnknownProfile, never a prod fallback | TestRuntimeProfileMatrix/*/env-unknown, TestRuntimeProfileMatrix/*/env-unknown-allow-prod | runtime_test.go |
| 8.1 | dev build ignores env release profile, warns profile-env-ignored | TestRuntimeProfileMatrix/*/env-prod, TestRuntimeProfileWarningText | runtime_test.go |
| 8.1 | <APP>_ALLOW_PROD=1 (exactly "1") lets env prod through | TestRuntimeProfileMatrix/*/env-prod-allow-prod, TestRuntimeProfileMatrix/*/env-prod-allow-prod-not-1 | runtime_test.go |
| 8.1 | release build ignores any other env profile | TestRuntimeProfileMatrix/*/env-dev, TestRuntimeProfileMatrix/*/custom-list-env-staging | runtime_test.go |
| 8.1 | no flag/env: release→release profile, else dev profile | TestRuntimeProfileMatrix/*/none, TestRuntimeProfileMatrix/*/renamed-profiles-none | runtime_test.go |
| 8.1 | set-but-empty <APP>_PROFILE counts as unset | TestRuntimeProfileMatrix/*/env-empty-is-unset | runtime_test.go |
| profiles/profiles.md | unknown-profile error names its source and known list | TestPDUnknownProfileNamesSource (new) | promise_d_test.go |
| profiles/profiles.md | release/dev profile not in list is ErrInvalidOption | TestRuntimeProfileOptionsInvalid | runtime_test.go |
| profiles/profiles.md | WithProfiles("staging") alone fails New | TestPDProfilesListWithoutDefaults (new) | promise_d_test.go |
| profiles/profiles.md | unknown --daemon-profile / <APP>_DAEMON_PROFILE fails New | TestRuntimeDaemonProfile/flag-unknown, TestRuntimeDaemonProfile/env-unknown | runtime_test.go |
| 8.2 | WithRelease(true) is release "flag", wins over build info | TestRuntimeProfileMatrix/release-flag-beats-checkout/none | runtime_test.go |
| 8.2 | WithRelease(false) forces dev "dev: flag" | TestRuntimeProfileMatrix/dev-flag-beats-go-install/none | runtime_test.go |
| 8.2 | clean v-tag without vcs.revision is release "go-install" | TestRuntimeProfileMatrix/go-install-tag/none, TestRuntimeProfileMatrix/go-install-prerelease-tag/none | runtime_test.go |
| 8.2 | (devel) or empty version is dev | TestRuntimeProfileMatrix/devel/none, TestRuntimeProfileMatrix/empty-version/none | runtime_test.go |
| 8.2 | version without v prefix is dev | TestRuntimeProfileMatrix/no-v-prefix/none | runtime_test.go |
| 8.2 | every pseudo-version form is dev | TestRuntimeProfileMatrix/pseudo-no-tag/none, TestRuntimeProfileMatrix/pseudo-after-prerelease/none | runtime_test.go |
| 8.2 | +dirty is dev | TestRuntimeProfileMatrix/dirty-checkout/none, TestRuntimeProfileMatrix/dirty-without-vcs/none | runtime_test.go |
| 8.2 | any vcs.revision is dev, even on a tag | TestRuntimeProfileMatrix/tagged-checkout/none | runtime_test.go |
| 8.2 | no build info is dev (fails safe) | TestRuntimeProfileMatrix/no-build-info/none, TestRuntimeProfileMatrix/nil-build-info/none | runtime_test.go |
| 8.2 | ReleaseReason is the first rule that said dev | TestRuntimeProfileMatrix/devel/none | runtime_test.go |
| 8.2 | release detection uses the standard library only | TestPDStdlibOnlyModule (new) | promise_d_test.go |
| profiles/release-detection.md | ForProfile keeps the build's release decision and reason | TestPDForProfileKeepsReleaseDecision (new) | promise_d_test.go |
| profiles/release-detection.md | a test binary is a dev build | TestPDTestBinaryIsDev (new) | promise_d_test.go |
| 8.3 | Name/NameWith: base for release, base-<profile> otherwise | TestRuntimeProfileMatrix | runtime_test.go |
| 8.3 | Banner "" for release, "[app] profile: p → home" otherwise | TestRuntimeProfileMatrix, TestRuntimeBanner | runtime_test.go |
| profiles/names.md | Name adds the suffix and does not validate | TestPDNameDoesNotValidate (new) | promise_d_test.go |
| 8.4 | relative CheckDB path resolves against WithWorkDir | TestFixT2_CheckDBRelativeUsesWorkDir | coord_fix_test.go |
| 8.4 | release build always gets AccessReadWrite | TestRuntimeCheckDB/release-prod-global, TestRuntimeCheckDB/release-any-path | runtime_test.go |
| 8.4 | VerifyFile runs first, for every build | TestRuntimeCheckDB/symlink-escaping-home-release, TestRuntimeCheckDB/symlink-to-prod-db | runtime_test.go |
| 8.4 | prod view has no --home | TestRuntimeCheckDB/dev-home-flag-on-prod-home, TestPDCheckDBByBuildNotProfile/dev-build-home-elsewhere (new) | runtime_test.go, promise_d_test.go |
| 8.4 | prod view strips every <APP>_* variable | TestFixF4_ProdViewStripsAppVars, TestRuntimeCheckDB/dev-home-env-points-to-prod | runtime_fix_test.go, runtime_test.go |
| 8.4 | prod view keeps other vars for ${VAR} db: pointer | TestFixF4_ProdDBPointerWithEnvVar, TestRuntimeCheckDB/dev-prod-db-pointer-envvar | runtime_fix_test.go, runtime_test.go |
| 8.4 | both global and project (db:/home: pointer) dbs checked | TestRuntimeCheckDB/dev-prod-project-existing, TestRuntimeCheckDB/dev-prod-home-pointer, TestRuntimeCheckDB/dev-prod-db-pointer-relative-local | runtime_test.go |
| 8.4 | broken project view does not skip the global check | TestFixF4_GlobalCheckedWhenProjectViewErrors, TestRuntimeCheckDB/dev-prod-global-when-prod-project-broken | runtime_fix_test.go, runtime_test.go |
| 8.4 | match by os.SameFile, any spelling | TestRuntimeCheckDB/dev-prod-global-through-symlinked-folder, TestRuntimeCheckDB/dev-prod-global-unclean-spelling | runtime_test.go |
| 8.4 | file not yet: real parent + name, case-folded on macOS/Windows | TestRuntimeCheckDB/dev-prod-global-file-not-yet, TestRuntimeCheckDB/case-differs-file-not-yet | runtime_test.go |
| 8.4 | parent not yet: real paths compared, still refused | TestRuntimeCheckDB/dev-prod-global-home-not-yet, TestFixBX1_ProdDBBeforeProdHomeExists | runtime_test.go, runtime_fix_test.go |
| 8.4 | match without ALLOW_PROD=1 is ErrProdDB naming the way out | TestRuntimeCheckDB/dev-prod-global-existing, TestRuntimeCheckDB/allow-prod-not-1 | runtime_test.go |
| 8.4 | match with ALLOW_PROD=1 is AccessReadOnly | TestRuntimeCheckDB/allow-prod-global, TestRuntimeCheckDB/allow-prod-project | runtime_test.go |
| 8.4 | no match is AccessReadWrite (own db, copy elsewhere) | TestRuntimeCheckDB/dev-own-project-db, TestRuntimeCheckDB/dev-same-name-other-folder | runtime_test.go |
| profiles/prod-db-guard.md | guard follows the build: dev+--profile prod refused, release+--profile dev not | TestPDCheckDBByBuildNotProfile (new), TestRuntimeCheckDB/dev-profile-env-prod-ignored-still-guarded | promise_d_test.go, runtime_test.go |
| profiles/prod-db-guard.md | dev build ignores db: pointer, warns pointer-ignored-for-profile | TestRuntimeCheckDBPointerWarnsDev | runtime_test.go |
| profiles/prod-db-guard.md | AccessReadOnly is the caller's promise; appdir cannot enforce it | — | not testable: describes what the caller does after CheckDB returns |
| 8.5 | dev: <APP>_HOME / <APP>_<KIND>_DIR into prod home ignored, warns env-points-to-prod | TestResolveOverrideMatrix | resolve_matrix_test.go |
| 8.5 | leak filter only in non-release profiles | TestResolveOverrideMatrix | resolve_matrix_test.go |
| 8.5 | ChildEnv sets <APP>_PROFILE, replacing any value, keeps others | TestRuntimeChildEnv | runtime_test.go |
| 8.5 | CleanEnv drops every <PREFIX>_* var; pure function | TestRuntimeCleanEnv | runtime_test.go |
| profiles/child-processes.md | leak filter compares against the prod global home only | TestPDEnvLeakOnlyGlobalHome (new) | promise_d_test.go |
| 8.6 | ForProfile resolves same options and env as --profile p | TestRuntimeForProfile | runtime_test.go |
| 8.6 | ForProfile opens and creates nothing | TestPDForProfileCreatesNothing (new) | promise_d_test.go |
| 8.6 | ForProfile with an unlisted profile is ErrUnknownProfile | TestRuntimeForProfile/unknown | runtime_test.go |
| 9 | skip in non-release profile or with --home/<APP>_HOME | TestRuntimeLegacyMove/skipped-dev-build, TestRuntimeLegacyMove/skipped-home-flag, TestRuntimeLegacyMove/skipped-home-env | runtime_test.go |
| 9 | only the global home is moved, never a project folder | TestPDLegacyOnlyGlobalHomeMoves (new) | promise_d_test.go |
| 9 | old missing → MoveNone | TestRuntimeLegacyMove/old-missing | runtime_test.go |
| 9 | old holds only MOVED.txt → MoveAlready | TestRuntimeLegacyMove/same-disk-move-then-already | runtime_test.go |
| 9 | both exist → MoveBothExist, new used, nothing moves, Note set | TestRuntimeLegacyMove/both-exist | runtime_test.go |
| 9 | moving lock <parent>/<base>.moving.lock; held → ErrBusy | TestRuntimeLegacyMove/moving-lock-held | runtime_test.go |
| 9 | the moving lock file is never deleted | TestRuntimeLegacyMove/same-disk-move-then-already | runtime_test.go |
| 9 | re-check under the lock: one mover wins | TestRuntimeLegacyConcurrent | runtime_test.go |
| 9 | held daemon lock in old → ErrBusy, nothing moves | TestRuntimeLegacyMove/old-daemon-lock-held, TestRuntimeLegacyMove/custom-daemon-locks-held | runtime_test.go |
| 9 | daemon lock probe never creates a file | TestRuntimeLegacyMove/probe-never-creates-lock, TestHeldMissingDoesNotCreate | runtime_test.go, internal/flock/flock_test.go |
| 9 | same disk: one rename(old, new) | TestRuntimeLegacyMove/same-disk-move-then-already | runtime_test.go |
| 9 | other disk: copy, verify, rename, old kept .moved-<date> | TestLegacyXDisk_Move | legacy_xdisk_test.go |
| 9 | cross-disk copy follows a symlinked old home | TestLegacyXDisk_Move | legacy_xdisk_test.go |
| 9 | copy keeps links as links and modes, skips sockets | TestRuntimeLegacyCrashRecovery/copy-tree-redo-after-crash, TestLegacyXDisk_Move | runtime_test.go, legacy_xdisk_test.go |
| 9 | WithLegacyVerify failure stops the move, old intact | TestLegacyXDisk_CrashRecovery/after-copy-before-marker | legacy_xdisk_test.go |
| 9 | old recreated holding only MOVED.txt; data never deleted | TestRuntimeLegacyMove/same-disk-move-then-already | runtime_test.go |
| 9 | crash during copy: stale <new>.moving-tmp deleted, redone | TestLegacyXDisk_CrashRecovery/during-copy, TestRuntimeLegacyCrashRecovery/stale-moving-tmp-same-disk | legacy_xdisk_test.go, runtime_test.go |
| 9 | crash after tmp→new: .legacy-move finishes, not both-exist | TestLegacyXDisk_CrashRecovery/after-swap, TestRuntimeLegacyCrashRecovery/marker-in-new-old-still-there | legacy_xdisk_test.go, runtime_test.go |
| 9 | crash after old renamed: next run writes MOVED.txt, MoveDone | TestFixBX2_LegacyFinishesAfterCrashPastOldRename, TestLegacyXDisk_CrashRecovery/after-old-rename | runtime_fix_test.go, legacy_xdisk_test.go |
| 9 | lock: 0600, no .fence sidecar, no stray file in $HOME | TestPDLegacyLockNoSidecar (new), TestTryLockCreatesWithPermAndKeepsFile | promise_d_test.go, internal/flock/flock_test.go |
| runtime/legacy-move.md | a non-folder at the new home path is refused, kept | TestRuntimeLegacyMove/new-home-path-is-a-file, TestWinMoveLegacyKeepsFileAtNewHome | runtime_test.go, platform_windows_test.go |
| runtime/legacy-move.md | DaemonLocks default run/daemon.lock; custom list replaces it | TestRuntimeLegacyMove/old-daemon-lock-held, TestRuntimeLegacyMove/custom-daemon-locks-replace-default | runtime_test.go |
| runtime/legacy-move.md | several WithLegacyHome: first existing old folder wins | TestRuntimeLegacyMove/several-entries-first-existing-wins | runtime_test.go |
| runtime/legacy-move.md | no user home folder → MoveSkipped | TestRuntimeLegacyMove/skipped-no-user-home | runtime_test.go |
| runtime/legacy-move.md | unreadable device id is treated as another disk (copy) | TestPDLegacyUnknownDeviceCopies (new) | promise_d_test.go |
| 10 | ForTest points HOME, USERPROFILE, <APP>_HOME, XDG_*, agent dirs into root | TestRuntimeForTest/isolates | runtime_test.go |
| 10 | ForTest points TMPDIR, TMP, TEMP into root | TestForTestPointsTempIntoRoot | platform_test.go |
| 10 | ForTest unsets every other <APP>_* var | TestRuntimeForTestLeaksAppVars, TestPDForTestUnsetsEveryAppVar (new), TestRuntimeForTestPrefix | runtime_test.go, promise_d_test.go |
| 10 | ForTest unsets GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE, GIT_COMMON_DIR, ceilings, GIT_CONFIG* | TestRuntimeForTest/isolates | runtime_test.go |
| 10 | ForTest sets APPDIR_TEST_ROOT; env restored after the test | TestRuntimeForTest | runtime_test.go |
| 10 | macOS ForTest root under /tmp, removed at test end | TestDarwinForTestShortRoot, TestDarwinPDForTestRootRemoved (new) | platform_darwin_test.go, promise_d_darwin_test.go |
| 10 | New panics in a test binary on a path outside the root | TestRuntimeForTest/isolates, TestResolveTestGuard | runtime_test.go, resolve_matrix_test.go |
| 10 | Socket/EnsureSocketDir guard the fallback folder before creating it | TestFixX3_GuardCoversSocketFallback, TestRuntimeSocket | coord_fix_test.go, runtime_test.go |
| 10 | default root still refuses shared /tmp/<app>-<uid>/ socket folder | TestFix2L1_GuardDefaultRootRefusesSocketFallback | fix2_test.go |
| 10 | default root keeps the disposable temp fallback allowed | TestPDGuardDefaultRootAllowsTempFallback (new) | promise_d_unix_test.go |
| 10 | WithTestGuard(false) turns the guard off | TestResolveTestGuard, TestRuntimeForTest/isolates | resolve_matrix_test.go, runtime_test.go |
| testing/for-test.md | CheckDB's prod view runs with the guard off | TestPDCheckDBProdViewUnguarded (new) | promise_d_test.go |
| testing/for-test.md | guard also checks the socket fallback folder | TestFixX3_GuardCoversSocketFallback, TestFix2L1_GuardDefaultRootRefusesSocketFallback | coord_fix_test.go, fix2_test.go |
| testing/ci.md | workflow is workflow_dispatch only; proof step allows only two skips | TestPDWorkflowManualOnly (new) | promise_d_test.go |
| testing/ci.md | XDisk tests skip when unset, fail on same device | TestLegacyXDisk_Move, TestLegacyXDisk_CrashRecovery | legacy_xdisk_test.go |
| testing/ci.md | LockFileEx/flock held across two processes | TestTryLockAcrossProcesses | internal/flock/flock_process_test.go |
| 11 | Paths() in kind order, global before project for both kinds | TestPDPathsJSONOrder (new) | promise_d_test.go |
| 11 | under --home/<APP>_HOME a both kind appears once, home global | TestFixX1_OverrideBothKindsLabelledGlobal, TestPDPathsJSONOrder (new) | resolve_fix_test.go, promise_d_test.go |
| 11 | JSON keys kind, path, rule, fallback, home, type, git, note, error | TestPDPathsJSONOrder (new), TestPathsJSONParses | promise_d_test.go, examples/doctor/main_test.go |
| 12 | global home ~/.<app> (Unix), %USERPROFILE%\.<app> (Windows) | TestRuntimeProfileMatrix, TestWinUserProfileIsHome | runtime_test.go, platform_windows_test.go |
| 12 | owner-only: 0700/0600 mode bits; protected DACL user+SYSTEM | TestLayoutEnsure, TestWinOwnerOnlyACL | layout_test.go, platform_windows_test.go |
| 12 | stop at disk change: Stat_t.Dev; volume serial on Windows | TestPDWalkStopsAtDiskChange (new), TestWinDeviceIDMissingPath | promise_d_unix_test.go, platform_windows_test.go |
| 12 | locks: flock / LockFileEx, non-blocking, cross-process | TestTryLockSecondIsLocked, TestTryLockAcrossProcesses | internal/flock/flock_test.go, internal/flock/flock_process_test.go |
| 12 | atomic write: temp+rename over existing, no temp left | TestAtomicWriteOverExisting, TestWinAtomicWriteReplacesExisting | platform_test.go, platform_windows_test.go |
| 12 | Windows rename retries on sharing violation | TestWinPDRenameRetriesWhileTargetHeld (new) | promise_d_windows_test.go |
| 12 | socket: run/ or /tmp/<app>-<uid>/ or XDG; named pipe on Windows | TestRuntimeSocket, TestDarwinSocketFallbackLiteralTmp, TestWinNamedPipeShape | runtime_test.go, platform_darwin_test.go, platform_windows_test.go |
| 12 | appdir never makes links (copyTree only re-creates user links) | TestPDNeverMakesLinks (new) | promise_d_test.go |
| 12 | Windows junctions followed like symlinks | TestWinJunctionEscape (new) | links_windows_test.go |
| 12 | long paths: appdir only passes absolute (clean) paths | TestPDAbsolutePathsOnly (new) | promise_d_test.go |
| 12 | cache exclusions: CACHEDIR.TAG everywhere | TestLayoutCacheTag | layout_test.go |
| 12 | macOS cache gets .metadata_never_index | TestDarwinPDCacheNeverIndex (new) | promise_d_darwin_test.go |
| 12 | macOS Time Machine exclusion via tmutil | — | not testable: tmutil is best effort by design (failure ignored) and runners may lack Time Machine |
| 12 | Windows cache gets FILE_ATTRIBUTE_NOT_CONTENT_INDEXED | TestWinPDCacheNotContentIndexed (new) | promise_d_windows_test.go |
| 12 | root/sudo: euid 0 vs owner uid, ErrRootOwned unless allowed | TestPDRootCheckUnix (new) | promise_d_unix_test.go |
| 12 | root/sudo check is a no-op on Windows | TestWinPDRootChecksAreNoOps (new) | promise_d_windows_test.go |
| 12 | temp fallback owner check: uid + Lstat (Unix) | TestPDPrivateDirRefusesOtherOwner (new), TestLayoutTempFallback | promise_d_unix_test.go, layout_test.go |
| 12 | temp fallback on Windows: Lstat only, owner always ours | TestWinPDRootChecksAreNoOps (new) | promise_d_windows_test.go |
| 12 | network/synced folder check is not appdir's | — | not testable: absence of a feature owned by sqlitedb |
| 12 | appdir installs no service; tool uses Name/NameWith | TestNoProcessStart | platform_test.go |
| 13 | logs rotation is the tool's; appdir only gives logs/ | TestLayoutStandardKinds | layout_test.go |
| 13 | profile in discovery/handshake is the tool's protocol | — | not testable: a promise about the tool's wire protocol, outside appdir |
| 13 | prod/dev install entries never cross (agentinstall) | — | not testable: belongs to agentinstall; appdir's part is Name (8.3) |
| 14.1 | dev reads project config.jsonc/.local, profile config.local last | TestResolveConfigFilesMatrix | resolve_matrix_test.go |
| 14.2 | dev ignores db: pointer with pointer-ignored-for-profile | TestRuntimeCheckDBPointerWarnsDev | runtime_test.go |
| 14.3 | read-only home: disposable kind falls back to temp; off switch | TestLayoutTempFallback | layout_test.go |
| 14.4 | default mode is ModeProject | TestResolveCustomNames | resolve_matrix_test.go |
| 14.5 | module is github.com/ubgo/appdir | TestPDStdlibOnlyModule (new) | promise_d_test.go |

## Related

- [Testing setup](for-test.md): seams, `ForTest` and the test-binary guard used by these tests.
- [Real-OS test workflow](ci.md): where the Windows and macOS rows run.
- [Platforms](../reference/platforms.md): which platform rows are proven on real OSes.
