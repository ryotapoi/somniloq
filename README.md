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
| `sessions` | List sessions |
| `projects` | List projects with session counts |
| `show` | Show session content in Markdown |

### import

```bash
somniloq import              # differential import (default)
somniloq import --full       # full re-import (with confirmation)
somniloq import --full --yes # skip confirmation
```

### sessions

```bash
somniloq sessions                        # all sessions
somniloq sessions --since 24h            # last 24 hours
somniloq sessions --since 7d             # last 7 days
somniloq sessions --since 2026-03-28     # after a date (local time)
somniloq sessions --until 2026-03-28     # before a date (local time)
somniloq sessions --since 7d --until 2h  # 7 days ago to 2 hours ago
somniloq sessions --project myapp        # filter by project name
somniloq sessions --short                # shorten project names
```

Output is TSV: `session_id`, `started_at ~ ended_at`, `project_dir`, `custom_title`, `message_count`

### projects

```bash
somniloq projects             # all projects
somniloq projects --since 7d  # projects active in the last 7 days
somniloq projects --short     # shorten project names
```

Output is TSV: `project_dir`, `session_count`

### show

```bash
somniloq show <session-id>                              # single session
somniloq show --since 24h                               # last 24 hours
somniloq show --since 2026-03-28 --until 2026-03-29     # date range
somniloq show --since 7d --project myapp                # filter by project
somniloq show --summary 1 --since 24h                   # first user message per session
somniloq show --short --since 24h                       # shorten project names
```

## Common Options

| Option | Description | Default |
|--------|-------------|---------|
| `--db <path>` | Path to SQLite database | `~/.somniloq/somniloq.db` |
| `--version` | Print version and exit | — |

### Time Filters

`--since` and `--until` accept the following formats:

| Format | Example | Meaning |
|--------|---------|---------|
| Relative | `30m`, `24h`, `7d` | That amount of time ago |
| Date | `2026-03-28` | 00:00 local time on that day |
| Datetime | `2026-03-28T15:00` | Exact local time |

## Documentation

- [Mission and non-goals](rules/mission.md)
- [Features, CLI, and schema](rules/scope.md)
- [Module structure and dependencies](rules/architecture.md)

## License

[MIT License](LICENSE)
