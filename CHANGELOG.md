# Changelog

English | [日本語](CHANGELOG.ja.md)

## Unreleased

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
