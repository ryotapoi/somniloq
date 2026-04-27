---
name: somniloq
description: >
  Complete reference for the somniloq CLI — a tool that imports Claude Code session logs into SQLite
  and queries them. Use this skill whenever you need to look up past sessions, search conversation history,
  check what was worked on, list projects, or export session content. Trigger on: "session history",
  "past sessions", "what did I work on", "conversation log", "somniloq", or any request to search/browse
  Claude Code usage history — even if the user doesn't name the tool directly.
---

# somniloq CLI Reference

somniloq imports Claude Code session logs (JSONL under `~/.claude/projects/`) into a local SQLite database and lets you query them. It is already installed and available on the PATH.

## Quick start

```bash
somniloq import              # pull new sessions into the DB
somniloq sessions --since 7d # list recent sessions
somniloq show <session-id>   # read a session
```

Always run `somniloq import` first if the user might have new sessions since the last import.

---

## Global flag

| Flag | Default | Description |
|------|---------|-------------|
| `--db <path>` | `~/.somniloq/somniloq.db` | Override the database path. Must come **before** the subcommand. |
| `--version` | — | Print version and exit. |

```bash
somniloq --db /tmp/test.db sessions --since 7d
```

---

## Commands

### import

Scan `~/.claude/projects/` and import JSONL files into SQLite. Default is differential — only new or grown files are processed.

```bash
somniloq import                # differential (default)
somniloq import --full         # drop and re-import everything (confirms y/N)
somniloq import --full --yes   # skip confirmation (for scripts/cron)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--full` | false | Re-import everything. Prompts for confirmation unless `--yes` is given. In non-interactive environments (pipes, cron), `--yes` is required or the command errors. |
| `--yes` | false | Skip confirmation prompt. Only meaningful with `--full`. |

Output: `Imported <n> files (<scanned> scanned, <skipped> skipped, <failed> failed)`

---

### sessions

List sessions, newest first. Output is TSV.

```bash
somniloq sessions                                # all sessions
somniloq sessions --since 24h                    # last 24 hours
somniloq sessions --since 7d --project myapp     # filtered by project
somniloq sessions --since 2026-03-28 --until 2026-03-29
somniloq sessions --short                        # short project names
```

| Flag | Default | Description |
|------|---------|-------------|
| `--since` | — | Start time filter (see "Time filters") |
| `--until` | — | End time filter |
| `--project` | — | Substring match on `repo_path` (when resolved) or `project_dir` |
| `--short` | false | Show repo basename (or last hyphen element if `repo_path` unresolved) |

**Columns** (tab-separated):

```
SessionID    TimeRange    ProjectDir    CustomTitle    MessageCount
```

- `TimeRange` is displayed as `YYYY-MM-DD HH:MM ~ YYYY-MM-DD HH:MM` (started ~ ended) in local time. If `ended_at` is unavailable, the format is `YYYY-MM-DD HH:MM ~`.
- `ProjectDir` column shows `repo_path` when resolved (e.g. `/Users/ryota/Sources/myapp`); otherwise it falls back to the raw `project_dir` key with worktree suffix stripped.
- `--short` shows `filepath.Base(repo_path)` when resolved (e.g. `myapp`, hyphens preserved); otherwise the last hyphen-separated element of `project_dir`.

---

### show

Display session content as Markdown. Accepts either a single session ID or a time range.

```bash
# single session (flags must come BEFORE the session-id)
somniloq show <session-id>
somniloq show --short <session-id>

# by time range
somniloq show --since 24h
somniloq show --since 7d --project myapp
somniloq show --summary 1 --since 24h
somniloq show --short --since 24h
```

| Flag | Default | Description |
|------|---------|-------------|
| `--since` | — | Start time filter |
| `--until` | — | End time filter |
| `--project` | — | Substring match on `repo_path` (when resolved) or `project_dir` (time-range mode only) |
| `--short` | false | Show repo basename (or last hyphen element if `repo_path` unresolved) |
| `--summary <N>` | 0 | Show first N user messages per session after skipping `/clear` and `<local-command-caveat>`. `0` disables (full output). Requires an integer argument — bare `--summary` is an error (use `--summary 1` for the old default). |
| `--include-clear` | false | Requires `--summary >= 1`; disable `/clear` + caveat skipping (sidechain still excluded). Debug use. |
| `--format` | markdown | Output format (only `markdown` is supported) |

**Constraints:**
- `<session-id>` and `--since`/`--until` are mutually exclusive.
- `--project` only applies in time-range mode. `--summary` works in both modes.
- `--include-clear` without `--summary >= 1` is an error.
- `--summary` takes an integer (v0.2+). Earlier versions accepted a bare `--summary` flag; that form no longer works.

**Output structure:**

```markdown
## Session Title

- **Session**: `<session-id>`
- **Project**: `<project-dir>`
- **Started**: `<started_at ~ ended_at>`

### User

<message>

### Assistant

<message>
```

Multiple sessions are separated by `---`. Sidechain messages (subagent internals) are excluded.

---

### projects

List projects with session counts, sorted by most-recently-active first. Output is TSV.

```bash
somniloq projects                          # all projects
somniloq projects --since 7d               # active in last 7 days
somniloq projects --short                  # short project names
somniloq projects --since 30d --short
```

| Flag | Default | Description |
|------|---------|-------------|
| `--since` | — | Start time filter |
| `--until` | — | End time filter |
| `--short` | false | Show repo basename (or last hyphen element if `repo_path` unresolved) |

**Columns** (tab-separated):

```
ProjectDir    SessionCount
```

`ProjectDir` shows `repo_path` when resolved; otherwise it falls back to the raw `project_dir` key. Worktree and subdirectory sessions are merged into their root project via `repo_path` aggregation in SQL — session counts are combined.

---

## Time filters

`--since` and `--until` accept relative or absolute values. Absolute dates are interpreted as **local time**.

| Format | Example | Meaning |
|--------|---------|---------|
| Relative | `30m`, `24h`, `7d` | That amount of time ago from now |
| Absolute date | `2026-03-28` | That day at 00:00 local time |
| Absolute datetime | `2026-03-28T15:00` | Exact local time |

Supported relative units: `m` (minutes), `h` (hours), `d` (days).

`--until` with a date-only value includes the whole day (resolves to next day 00:00).

Both flags can combine:

```bash
somniloq sessions --since 7d --until 2h   # last 7 days, excluding most recent 2 hours
```

---

## Recipes

```bash
# daily activity summary
somniloq import && somniloq show --summary 1 --since 24h

# quick scan of recent work
somniloq import && somniloq sessions --since 7d --short

# find sessions about a topic
somniloq sessions --since 7d | grep -i "auth"
somniloq show --since 7d | grep -i "auth" -B2 -A5

# project overview
somniloq projects --since 30d --short
somniloq sessions --since 30d --project somniloq --short

# export
somniloq show --since 24h > daily-log.md

# count sessions per project this week
somniloq sessions --since 7d --short | cut -f3 | sort | uniq -c | sort -rn

# show the most recent session
somniloq show "$(somniloq sessions --since 24h | head -1 | cut -f1)"
```

---

## Notes

- **Import is not automatic.** The database is a snapshot at import time.
- **Flags must come before positional arguments.** `somniloq show --short <session-id>` works, but `somniloq show <session-id> --short` does not. This applies to all subcommands.
- **TSV output** from `sessions` and `projects` is pipe-friendly. Use `cut`, `awk`, or `column -t -s $'\t'` to reshape.
- **Sidechain messages** (subagent conversations) are excluded from `show`.
- **Empty messages** (tool_use-only turns with no text) are excluded at import time.
- **`--project`** is a substring match against both `repo_path` (when resolved) and `project_dir` — `--project app` matches `myapp` and `app-server`. With `repo_path`, slash-spanning queries (e.g. `--project Sources/ryot`) also work.
- **All timestamps** in output are local time.
- Every subcommand supports `--help` for a quick flag reference.
