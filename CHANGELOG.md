# Changelog

English · [日本語](CHANGELOG.ja.md)

## v0.14.1 — 2026-10-09

### Changed

- Reduced repeated Git calls in Codex import and migration by reusing repository resolution for the same working directory within each processing pass. Later runs retry resolution.
- Reduced migration memory use by retaining ownership evidence first and loading message bodies one conversation at a time. Strict validation and saving now share JSONL parsing results, and snapshot verification hashes files with a reusable buffer instead of allocating their full contents again. Peak memory still depends on the largest conversation.
- Reduced Codex import and migration allocations by comparing duplicate payload IDs against retained messages without copying full JSON signatures. Exact duplicate detection, conflict rejection, and message numbering are preserved.
- Recorded synthetic migration benchmarks and optimization evaluations. Migration continues to preserve ownership checks, snapshot-change detection, existing data on group failure, and rerun behavior; CLI, configuration, and database formats are unchanged.

## v0.14.0 — 2026-10-08

### Added

- Added TOML configuration with multiple log roots per source and `config init`. Database commands use the named `default` config when `--config` is omitted; initialization refuses an existing config or referenced database.
- Added input-scoped conversation identities and complete `slq1:...` REFs. Identical session IDs in different inputs remain separate, and Claude Code children and grandchildren retain independent original messages even when they share the root session ID.
- Added `migrate` to copy a fixed standalone snapshot of the supported legacy database into a separate destination. Only Codex history whose ownership is proven by remaining logs is replaced; other history remains accessible through legacy REFs. Normal commands reject legacy and unsupported schemas without modifying them. See the [migration guide](docs/migration.md).
- Added every-occurrence regular-expression search within a conversation and its confirmed descendants, with multiple `-e` patterns, `-F` literal matching, `--all` AND matching, original UTF-8 byte positions, and message numbers shared with `show`.
- Added third-party and Go license notices, a generation script, and a CI consistency check. Added CI badges and language navigation to the README pair.

### Changed

- Replaced `sessions` with grouped `search` listings. Listings return all results by default, with optional pagination and filters for input, source, project, activity dates, and import time. Results contain complete REFs and metadata without message excerpts; activity modes are `active`, `started`, `last`, and `overlap`.
- Redesigned `show` around original conversation messages. It accepts multiple complete REFs, optionally expands confirmed descendants, and supports role, message-number, and date filters, pagination, tail selection, and one-line text while retaining original blocks and numbering.
- Changed `search` and `show` JSON output to the `{items,total,count,limit,offset,hasMore,nextOffset}` envelope and TSV output to include page metadata and reversible field escaping. `projects` JSON remains an array.
- Replaced legacy JSON configuration with TOML; existing JSON configs are neither discovered nor automatically converted. Bare session IDs and shortened REFs are rejected. Removed `outline`, `show --summary`, turn selection, display-exclusion flags and configuration, Markdown output, and period-only retrieval without a REF. Update scripts and configuration before upgrading.
- Reduced repeated work by reusing Claude repository resolution for the same working directory, narrowing legacy migration reads to relevant UUIDs, and sharing relation resolution across multiple `show` REFs.

### Fixed

- Preserved Codex conversations' own messages separately from inherited context, including stable message ordering across multiple rollouts and parent metadata arriving later.
- Preserved Claude import times when original messages are unchanged, and rebuilt Codex messages after metadata-only rollout edits so incremental and full imports agree.
- Preserved valid Codex messages when parent metadata contains cycles; cyclic relationships remain unresolved.
- Prevented concurrent incremental imports from overwriting newer messages, cursors, and import times with stale snapshots.
- Preserved migrated conversation messages and cursors when a previously imported rollout is missing on a migration rerun.

## v0.13.0 — 2026-10-04

### Changed

- `outline` and `show --summary` now use the same optional `excludeUserMessagePatterns` configuration against each trimmed full user message. Repeated `--exclude-user-message-pattern` flags replace the configured list for one call; `--no-exclude-user-messages` disables exclusions for one call. Excluded outline entries retain the original turn numbering, and summary selects its first N messages after exclusions.
- `show --summary` no longer skips `/clear` and command caveats by default; `--include-clear` has been removed. The former `commandPatterns` setting is not migrated, and `sessions` no longer emits non-command user-turn counts or first-line hints. Its TSV output has eight columns, with `source` last, and its JSON output omits `nonCommandUserTurnCount` and `firstNonCommandUserLine`.
- The `backfill` command and dedicated legacy database migration and repair code have been removed. General schema management remains, but upgrading a v0.3 database to the current source-aware schema is not guaranteed.
- Session message retrieval now uses an index on source and session ID, reducing repeated full-message scans when reading multiple sessions. The index is also created for existing databases when opened.

### Fixed

- Concurrent incremental and full imports now detect a changed import cursor before writing, instead of reporting success while leaving previously imported message bodies missing.
- New database files and their newly created parent directories now have owner-only permissions. Existing files and directories retain their permissions.
- Repository detection for imported Claude Code and Codex sessions now ignores inherited `GIT_*` overrides, so a Git environment for another repository does not assign the wrong project. Previously stored paths are not changed automatically.
- `sessions` TSV now flattens tabs and line breaks in malformed stored timestamps, preserving one row and eight columns per session; JSON retains the original timestamp string.
- Time filters now reject RFC3339 numeric offsets with hours of 24 or greater or minutes of 60 or greater.
- Markdown CI now fails when link diagnostics report basename conflicts, missing targets, or broken anchors, even if the diagnostic command exits successfully.
- Search documentation now places flags before the query, matching the accepted CLI syntax.

## v0.12.5 — 2026-10-02

### Changed

- `search` now loads only message IDs and roles when assigning turn numbers, avoiding an additional fetch of every message body in a matching session. Search results, snippets, and turn numbers remain unchanged.

### Fixed

- Claude Code import now reports user and assistant records with a missing or empty `sessionId` or `uuid` as unparsed instead of saving them under empty IDs. Valid records around them continue to import normally; existing stored data is not repaired automatically.

## v0.12.4 — 2026-10-01

### Changed

- Simplified CLI, database query, and import internals while preserving their existing behavior; consolidated redundant tests and retained coverage for the public command and import contracts.

## v0.12.3 — 2026-10-01

### Fixed

- Differential import now retries an unfinished final JSONL line after the file grows, while still importing a complete final record without a newline.
- Codex import now reports `session_meta` records with a missing or empty ID as unparsed instead of merging subsequent messages into an empty-ID session.
- Malformed stored timestamps are treated as unknown for filtering and ordering, so one bad timestamp no longer prevents `sessions`, `projects`, `show`, or `search` from reading valid data. The original value remains in stored and JSON output.
- Claude Code and Codex sessions from existing linked Git worktrees now group under the main repository in project listings and filters. Previously stored non-NULL paths require a full re-import to update.
- `sessions` and `import` now reject unexpected positional arguments, including arguments that previously caused later flags to be ignored.
- Date-only time filters and `logical_day` now use the configured local clock boundary across daylight saving time changes.
- `sessions` and `projects` now flatten tabs and line breaks in TSV project names, preserving row and column boundaries while leaving JSON values unchanged.

## v0.12.2 — 2026-10-01

### Changed

- Consolidated redundant tests while retaining regression coverage for command output and import behavior; CLI and stored-data behavior are unchanged.

## v0.12.1 — 2026-09-24

### Changed

- The documentation now clarifies that `imported_at` is the start time of processing the JSONL file that last saved or updated a session; processing an unchanged file does not update it.
- Building from source now requires Go 1.27.1 or newer; CI uses Go 1.27.1.

### Fixed

- `import --full` now deletes all rows from the `messages`, `sessions`, and `import_state` tables in one transaction, so a deletion failure preserves their existing data instead of leaving them partly cleared.
- Stored RFC3339 timestamps are now compared by instant when selecting session start and end times and ordering messages, sessions, projects, and search results. Different UTC offsets or fractional-second precision no longer cause chronological misordering; message ordering keeps its existing `rowid` tie-break, and a valid timestamp can fill an empty session start time.
- `import` now returns an error if it cannot write an import diagnostic to stderr, instead of exiting successfully after a failed diagnostic write.

## v0.12.0 — 2026-09-23

### Added

- `sessions`, `projects`, `show`, and `search` now accept RFC3339 instants with seconds, fractional seconds, and `Z` or numeric UTC offsets for `--since` and `--until`; `sessions --imported-since` accepts the same format. An explicit offset specifies the same instant regardless of the runtime time zone.

### Changed

- Search queries and `--project` in `sessions`, `show`, and `search` now treat `%`, `_`, and `\` as literal text rather than wildcards. Existing substring matching, ASCII case-insensitivity, and project-alias expansion remain unchanged; no database migration, backfill, or re-import is required.

### Fixed

- RFC3339 time boundaries now preserve fractional seconds for exact comparisons, treat equivalent UTC offsets as the same instant, include `--since` boundaries, and exclude `--until` boundaries.

## v0.11.0 — 2026-09-21

### Added

- `show` and `outline` now accept `--source` to open a session from a specific import source when the same session ID exists in multiple sources; the existing ambiguity error remains when omitted.
- `search --format json` now emits machine-readable results with source, session ID, turn, timestamp, project, and matching snippet.
- `search --limit` and `--offset` now paginate results while preserving the existing search order; without pagination flags, all results are still returned.
- `sessions --imported-since` now lists sessions imported at or after a specified time, including sessions whose conversation timestamps are unknown.

## v0.10.1 — 2026-09-20

### Fixed

- `import --full` and `backfill` now treat confirmation-prompt read or write failures as command errors and stop before destructive work, rather than treating them as declined confirmations.

## v0.10.0 — 2026-09-16

### Added

- Cursor Agent transcripts under `~/.cursor/projects/` can now be imported by default or with `--source cursor-agent`.

### Changed

- `sessions` and `search` TSV output and `show` Markdown now identify each session's source. `show` and `outline` also report candidate sources when a session ID exists in multiple sources.
- Sessions without timestamp or repository metadata remain visible without filters, but do not match time or project filters; their time range is empty when both timestamps are unknown.

## v0.9.1 — 2026-09-06

### Fixed

- CLI commands now propagate stdout write failures and exit with an error instead of silently succeeding. This applies to `import`, `backfill`, `outline`, `projects`, `search`, `sessions`, and `show`.

## v0.9.0 — 2026-07-24

### Fixed

- Database query errors now include the operation and stage (query, scan, or iteration), while preserving wrapped causes and existing not-found behavior.

## v0.8.1 — 2026-07-23

### Changed

- Consolidated config-command flag declarations so help prechecks derive flag recognition and value consumption from the same `FlagSet` definitions.
- Removed the unused `ingest.Adapter.Source` method; import source identity remains defined by import source registration and JSONL processing.

## v0.8.0 — 2026-07-12

### Added

- `import` now reports up to the first five parse or normalization failures on stderr as `file:line: error`, making schema changes easier to diagnose. Existing summary counts, differential-import offsets, and exit-code semantics are unchanged.

### Changed

- Consolidated import transaction creation, source normalization, query filters, migration/backfill paths, and CLI formatting; expanded regression coverage for cross-source resolution, time boundaries, output schemas, and import/migration contracts.

### Fixed

- Persistence failures no longer mark a line as a successfully written body.

## v0.7.2 — 2026-07-04

### Changed

- Consolidated source-neutral message persistence and CLI/import helper paths, keeping both import adapters on the same write semantics.

## v0.7.1 — 2026-07-04

### Changed

- Split core database responsibilities across schema, write, query, and connection layers, and routed backfill database access through the DB execer abstraction.
- Expanded regression coverage for schema parity, migration race rechecks, unresolved backfill paths, PRAGMA restoration failures, and shrinking-file imports.
- Documented the limited `core` → `claudecode` dependency exception from ADR 0008.

## v0.7.0 — 2026-07-04

### Added

- `sessions` now reports non-command user-turn counts and the first non-command line as skip hints; `commandPatterns` configures which turns count as commands.
- Added logical-day support: configure `dayBoundary` or pass `--day-boundary` for date-only filters in `sessions`/`search`, and get a `logical_day` column from `sessions`.
- `outline` now reports the total body size for each turn, including replies.
- Search results now include turn numbers shared with `outline` and `show --turn`.
- Project aliases now canonicalize project display and merge alias-equivalent rows in `projects`; `--short` only shortens unaliased projects.
- Expanded subcommand help with output schemas, behavior notes, and examples.

### Fixed

- Codex root scan failures are now reported, while missing roots remain unused sources and descendant scan failures remain non-fatal.
- `--help` and commands that do not need config no longer fail because of a broken config file; usage-error formatting is consistent across subcommands.

## v0.6.0 — 2026-06-11

### Added

- Added `outline` for skimming long sessions by user-message turns.
- Added `show --turn` and `--tail` for partial session reads.
- Added `body_size` to `sessions` output.
- Added JSON output for `sessions`, `show`, `projects`, and `outline`.
- Added `search` for cross-session message search.
- Added `projectAliases` config to expand `--project` across repository renames.

## v0.5.0 — 2026-06-11

### Changed

- The `import` summary now includes an `unparsed lines` counter; scripts that parse the summary must account for the additional field.
- Directory scan errors are non-fatal: unreadable directories are skipped while other files continue importing, errors are listed on stderr, and the exit code is 1 when any error occurs. Missing source directories are treated as unused sources.
- Consolidated the shared JSONL import skeleton, subcommand exit-code handling, v0.3 → v0.4 migration preflight, and SQL-side sidechain filtering; reorganized database tests by concern.

## v0.4.0 — 2026-05-05

### Added

- Added Codex rollout JSONL as a first-class import source alongside Claude Code logs.
- Added `somniloq import --source all|claude-code|codex` and aggregate `sessions`/`projects` views across both sources.
- `backfill` now also migrates v0.3 databases to the v0.4 schema.

### Changed

- Sessions are keyed by `(source, session_id)` instead of `session_id`; v0.3 databases must run `somniloq backfill` once before importing with v0.4.
- `import` now defaults to both Claude Code and Codex. `--source` restricts the adapters, while `--full` clears the whole database even when a source is selected.

## v0.3.0 — 2026-05-02

### Added

- Added `somniloq backfill` as the single repair entry point: it resolves missing `repo_path` values, removes orphan sessions left by v0.2.x, prompts before deletion, and supports `--yes` (required for destructive non-interactive runs).

### Changed

- `projects` aggregation and `--project` filtering now use `repo_path` only; the `project_dir` column was removed, and old rows need `somniloq backfill` before they match `--project`.
- `ResolveRepoPath` now falls back to `cwd` when Git resolution fails, giving sessions outside a Git repository a stable key.
- `import` no longer creates sessions for meta-only records; conversation records (`user` / `assistant`) are the gate.

## v0.2.1 — 2026-04-22

### Changed

- `--summary N` now takes a count and shows the first N user messages per session (default 0 disables it); this replaces the boolean flag from v0.1.x.
- Summary output skips `/clear` echoes and `<local-command-caveat>` blocks by default; use `--include-clear` to retain them.
- `--summary` works in both session-ID and time-range modes.

### Fixed

- Corrected `show` usage text so flags appear before `<session-id>`, matching Go flag parsing order.

## v0.1.1 — 2026-04-02

### Added

- Added the `--version` flag backed by build information.

## v0.1.0 — 2026-04-01

### Added

- Initial somniloq CLI for importing and searching Claude Code session logs.
- Differentially import JSONL files from `~/.claude/projects/` into SQLite with `import`.
- List sessions with time and project filters and TSV output.
- List projects with session counts.
- Display session content as Markdown with `show`.
- Support local time zones for all inputs and outputs.
- Normalize sessions from Git worktrees.
- Add `--short` for compact project names.
- Add `--summary` for quick session overviews.
