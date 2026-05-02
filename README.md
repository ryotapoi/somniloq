# somniloq

A CLI tool that imports Claude Code session logs (JSONL) into SQLite for searching and browsing.
It parses JSONL files under `~/.claude/projects/`, enabling cross-session search of past conversations.

[日本語版 README](README.ja.md)

## Features

- **Differential import** — Auto-detects JSONL files under `~/.claude/projects/` and imports only what's new
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
# Import session logs
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
| `import` | Import JSONL files into SQLite |
| `backfill` | Repair existing DB rows (resolve missing `repo_path`, drop orphan sessions) |
| `sessions` | List sessions |
| `projects` | List projects with session counts |
| `show` | Show session content in Markdown |

### import

```bash
somniloq import              # differential import (default)
somniloq import --full       # full re-import (with confirmation)
somniloq import --full --yes # skip confirmation
```

### backfill

```bash
somniloq backfill            # repair existing rows (with confirmation if rows will be deleted)
somniloq backfill --yes      # skip confirmation
```

Repairs DB rows produced by older versions. Specifically:

- Resolves `repo_path` for sessions where it is `NULL` and `cwd` is non-empty.
- Deletes `sessions` rows that have no `messages` (leftover meta-only rows from v0.2.x).

When there are sessions to delete, `backfill` prompts before proceeding (default `No`). `--yes` skips the prompt; in non-interactive environments (pipes, CI), `--yes` is required if any rows would be deleted. Re-running is safe.

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
```

Output is TSV: `session_id`, `started_at ~ ended_at`, `repo_path`, `custom_title`, `message_count`

### projects

```bash
somniloq projects             # all projects
somniloq projects --since 7d  # projects active in the last 7 days
somniloq projects --short     # basename of repo_path
```

Output is TSV: `repo_path`, `session_count`

### show

```bash
somniloq show <session-id>                              # single session
somniloq show --since 24h                               # last 24 hours
somniloq show --since 2026-03-28 --until 2026-03-29     # date range
somniloq show --since 7d --project myapp                # filter by project
somniloq show --summary 1 --since 24h                   # first user message per session
somniloq show --summary 3 --since 24h                   # first 3 user messages per session
somniloq show --short --since 24h                       # basename of repo_path
```

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

## Upgrading from v0.2.x

v0.3 changes how sessions are aggregated and filtered. Existing databases need a one-time repair.

1. **Back up the DB.** `backfill` deletes orphan rows (see below), so copy `~/.somniloq/somniloq.db` aside first.
2. **Install the v0.3 binary**, then run:
   ```bash
   somniloq backfill
   ```
   This resolves `repo_path` for older rows and removes `sessions` rows that have no `messages` (leftovers from the v0.2.x meta-only INSERT path).
3. **Optional — refill JSONL you previously archived.** If you moved old JSONL out of `~/.claude/projects/`, copy only the missing files back, then re-import:
   ```bash
   cp -rn /path/to/old-projects/. ~/.claude/projects/
   somniloq import --full --yes
   ```

### CLI behavior changes

- `--project` now matches `repo_path` only. The previous fallback to a `project_dir` column is gone, so older sessions whose `repo_path` is still `NULL` will not match `--project` until you run `somniloq backfill`.
- `sessions` / `projects` TSV output shows `repo_path` directly (no `project_dir` fallback column).
- `--short` always shows `filepath.Base(repo_path)`.

## Documentation

- [Mission and non-goals](rules/mission.md)
- [Features, CLI, and schema](rules/scope.md)
- [Module structure and dependencies](rules/architecture.md)

## License

[MIT License](LICENSE)
