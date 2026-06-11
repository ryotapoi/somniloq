# somniloq

A CLI tool that imports Claude Code and Codex session logs (JSONL) into SQLite for searching and browsing.
It parses JSONL files under `~/.claude/projects/` and `~/.codex/sessions/`, enabling cross-session search of past conversations.

[日本語版 README](README.ja.md)

## Features

- **Differential import** — Auto-detects Claude Code and Codex JSONL files and imports only what's new
- **Cross-session search** — Filter by project name and time range to quickly find past conversations
- **Markdown output** — Export session content as Markdown for daily notes and retrospectives
- **Built for Coding Agents** — Invoke from skills to use past sessions as context
- **Fully local** — No external services required. Pure Go + SQLite

## Installation

```bash
go install github.com/ryotapoi/somniloq/cmd/somniloq@latest
```

## Quick Start

```bash
# Import Claude Code and Codex session logs
somniloq import

# List sessions
somniloq sessions

# Sessions from the last 24 hours
somniloq sessions --since 24h

# Show session content
somniloq show <session-id>

# Export the last week as Markdown
somniloq show --since 7d
```

## Commands

| Command | Description |
|---------|-------------|
| `import` | Import Claude Code and Codex JSONL files into SQLite |
| `backfill` | Migrate/repair existing DB rows |
| `sessions` | List sessions |
| `projects` | List projects with session counts |
| `show` | Show session content in Markdown |
| `outline` | List a session's user messages as turn number, time, and first line |

### import

```bash
somniloq import              # differential import (default)
somniloq import --source claude-code
somniloq import --source codex
somniloq import --full       # full re-import (with confirmation)
somniloq import --full --yes # skip confirmation
```

Imports Claude Code JSONL from `~/.claude/projects/` and Codex rollout JSONL from `~/.codex/sessions/`. Use `--source all|claude-code|codex` to limit the import target. The default is `all`.

`--full` always clears the whole somniloq DB before re-importing. If you run `somniloq import --source codex --full`, Claude Code rows are deleted too, then only Codex logs are imported.

Errors are non-fatal: lines that cannot be parsed (broken JSON, malformed payloads) are skipped and counted in the summary (`... N unparsed lines`), and unreadable directories or files are skipped while the rest is still imported. Skipped errors are listed on stderr and the exit code is 1 when any occurred. A missing source directory is treated as an unused source, not an error.

### backfill

```bash
somniloq backfill            # repair existing rows (with confirmation if rows will be deleted)
somniloq backfill --yes      # skip confirmation
```

Migrates and repairs DB rows produced by older versions. Specifically:

- Migrates v0.3 databases to the v0.4 schema (`source` columns and `(source, session_id)` session keys).
- Resolves `repo_path` for sessions where it is `NULL` and `cwd` is non-empty.
- Deletes `sessions` rows that have no `messages` (leftover meta-only rows from v0.2.x).

Run `backfill` once after upgrading to v0.4 before importing. When there are sessions to delete, `backfill` prompts before proceeding (default `No`). `--yes` skips the prompt; in non-interactive environments (pipes, CI), `--yes` is required if any rows would be deleted. Re-running is safe.

### sessions

```bash
somniloq sessions                        # all sessions
somniloq sessions --since 24h            # last 24 hours
somniloq sessions --since 7d             # last 7 days
somniloq sessions --since 2026-03-28     # after a date (local time)
somniloq sessions --until 2026-03-28     # before a date (local time)
somniloq sessions --since 7d --until 2h  # 7 days ago to 2 hours ago
somniloq sessions --project myapp        # substring match against repo_path
somniloq sessions --short                # basename of repo_path
somniloq sessions --format json          # JSON array instead of TSV
```

Output is TSV: `session_id`, `started_at ~ ended_at`, `repo_path`, `custom_title`, `message_count`, `body_size`

`body_size` is the total body size in bytes (sidechain excluded), so you can tell whether a session is large before `show`ing it.

`--format json` emits a JSON array with `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `messageCount`, `bodySize`. JSON timestamps are the stored RFC3339 UTC values (see "JSON output" below).

### projects

```bash
somniloq projects             # all projects
somniloq projects --since 7d  # projects active in the last 7 days
somniloq projects --short     # basename of repo_path
somniloq projects --format json
```

Output is TSV: `repo_path`, `session_count`. With `--format json`: `project`, `sessionCount`.

### show

```bash
somniloq show <session-id>                              # single session
somniloq show --since 24h                               # last 24 hours
somniloq show --since 2026-03-28 --until 2026-03-29     # date range
somniloq show --since 7d --project myapp                # filter by project
somniloq show --summary 1 --since 24h                   # first user message per session
somniloq show --summary 3 --since 24h                   # first 3 user messages per session
somniloq show --short --since 24h                       # basename of repo_path
somniloq show --turn 40..60 <session-id>                # only turns 40-60
somniloq show --tail 3 <session-id>                     # only the last 3 turns
somniloq show --format json <session-id>                # JSON instead of Markdown
```

`--turn` / `--tail` use the same turn numbering as `outline` (1-based, incremented on each user message), so you can skim the outline first and read only the range you need. A turn includes the user message and the replies that follow it. `--turn` and `--tail` are mutually exclusive, cannot be combined with `--summary`, and in bulk mode (`--since`/`--until`) apply to each listed session independently.

`--format json` emits a JSON array of sessions — always an array, even for a single session ID — where each element has `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, and `messages` (`role`, `content`, `timestamp`). `--summary` / `--turn` / `--tail` filtering applies to `messages` as-is.

### outline

```bash
somniloq outline <session-id>                 # user messages as turn number, time, and first line
somniloq outline --format json <session-id>  # JSON instead of TSV
```

Grasp the structure of a long session before `show`ing it in full. Output is TSV: `turn`, `time`, `first_line`. Turn numbers start at 1 and increment on each user message (sidechain rows are excluded). With `--format json`: `turn`, `timestamp`, `firstLine`.

### JSON output

`sessions`, `projects`, `outline` (`--format tsv|json`) and `show` (`--format markdown|json`) support JSON output for scripts. Rules common to all commands:

- Always a JSON array; empty results print `[]`.
- Timestamps are the stored RFC3339 UTC values, not the local-time display format.
- Strings are raw values (no tab/newline sanitizing; JSON escaping covers it). `title` is the raw custom title with no session-id fallback.
- `project` honors `--short`; without it you get the raw `repo_path`.

## Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `--db <path>` | Path to SQLite database | `~/.somniloq/somniloq.db` |
| `--version` | Print version and exit | — |

> Requires SQLite 3.35 or later (for `ALTER TABLE ... DROP COLUMN`). The bundled `modernc.org/sqlite` driver ships with a recent SQLite, so no separate install is needed.

### Time Filters

`--since` and `--until` accept the following formats:

| Format | Example | Meaning |
|--------|---------|---------|
| Relative | `30m`, `24h`, `7d` | That amount of time ago |
| Date | `2026-03-28` | 00:00 local time on that day |
| Datetime | `2026-03-28T15:00` | Exact local time |

## Upgrading to v0.4

v0.4 adds Codex support and changes the session key to include `source`. Existing databases need a one-time migration/repair through `backfill`.

1. **Back up the DB.** `backfill` deletes orphan rows (see below), so copy `~/.somniloq/somniloq.db` aside first.
2. **Install the v0.4 binary**, then run:
   ```bash
   somniloq backfill
   ```
   This migrates v0.3 rows to the v0.4 schema, resolves `repo_path` for older rows, and removes `sessions` rows that have no `messages` (leftovers from the v0.2.x meta-only INSERT path).
3. **Import current logs.**
   ```bash
   somniloq import
   ```
4. **Optional — refill JSONL you previously archived.** If you moved old Claude Code JSONL out of `~/.claude/projects/`, copy only the missing files back, then re-import:
   ```bash
   cp -rn /path/to/old-projects/. ~/.claude/projects/
   somniloq import --full --yes
   ```

### CLI behavior changes

- `--project` now matches `repo_path` only. The previous fallback to a `project_dir` column is gone, so older sessions whose `repo_path` is still `NULL` will not match `--project` until you run `somniloq backfill`.
- `sessions` / `projects` TSV output shows `repo_path` directly (no `project_dir` fallback column).
- `--short` always shows `filepath.Base(repo_path)`.
- `import` now imports both Claude Code and Codex logs by default. Use `--source claude-code` or `--source codex` to import only one source.

## Documentation

- [Mission and non-goals](rules/mission.md)
- [Features, CLI, and schema](rules/scope.md)
- [Module structure and dependencies](rules/architecture.md)

## License

[MIT License](LICENSE)
