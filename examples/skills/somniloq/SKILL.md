---
name: somniloq
description: >
  Complete reference for the somniloq CLI — a tool that imports Claude Code and Codex session logs into SQLite
  and queries them. Use this skill whenever you need to look up past sessions, search conversation history,
  check what was worked on, list projects, or export session content. Trigger on: "session history",
  "past sessions", "what did I work on", "conversation log", "somniloq", or any request to search/browse
  Claude Code or Codex usage history — even if the user doesn't name the tool directly.
---

# somniloq CLI Reference

somniloq imports Claude Code session logs (JSONL under `~/.claude/projects/`) and Codex rollout logs (JSONL under `~/.codex/sessions/`) into a local SQLite database and lets you query them. It is already installed and available on the PATH.

## Quick start

```bash
somniloq import              # pull new sessions into the DB
somniloq sessions --since 7d # list recent sessions
somniloq outline <session-id>          # map a long session (turn / time / first line)
somniloq show --turn 40..60 <session-id>  # read only those turns
somniloq show <session-id>   # read a session in full
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

Scan `~/.claude/projects/` and `~/.codex/sessions/` and import JSONL files into SQLite. Default is differential — only new or grown files are processed.

```bash
somniloq import                # differential (default)
somniloq import --source claude-code
somniloq import --source codex
somniloq import --full         # drop and re-import everything (confirms y/N)
somniloq import --full --yes   # skip confirmation (for scripts/cron)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--source all|claude-code|codex` | `all` | Limit the import target. `all` imports both Claude Code and Codex into the same DB. |
| `--full` | false | Re-import everything. Prompts for confirmation unless `--yes` is given. In non-interactive environments (pipes, cron), `--yes` is required or the command errors. |
| `--yes` | false | Skip confirmation prompt. Only meaningful with `--full`. |

`--full` deletes the whole somniloq DB first. With `--source codex --full`, it still deletes Claude Code rows too, then re-imports only Codex.

CLI `--source` values use `claude-code`, but DB rows store the source as `claude_code`.

Output: `Imported <n> files (<scanned> scanned, <skipped> skipped, <failed> failed, <unparsed> unparsed lines)`

`unparsed lines` counts lines that could not be parsed (broken JSON, malformed payloads). Record types somniloq deliberately ignores are not counted.

Errors are non-fatal: unreadable directories and failed files are skipped, the rest is imported, skipped errors go to stderr, and the exit code is 1 when any occurred. Exit code 1 therefore does not mean the import did nothing.

---

### backfill

Repair existing DB rows produced by older versions. Run once after upgrading to v0.4 before importing; safe to re-run.

```bash
somniloq backfill              # repair (prompts y/N if rows will be deleted)
somniloq backfill --yes        # skip confirmation (for scripts/cron)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--yes` | false | Skip confirmation. Required in non-interactive environments when any rows would be deleted. |

What it does:

- Migrates v0.3 DBs to the v0.4 schema (`source` columns and `(source, session_id)` session keys).
- Resolves `repo_path` for sessions where it is `NULL` and `cwd` is non-empty.
- Deletes `sessions` rows that have no `messages` (leftover meta-only rows from v0.2.x).

Rows whose `repo_path` cannot be resolved (e.g. `cwd` is empty) stay `NULL` and will be retried on the next run.

---

### sessions

List sessions, newest first. Output is TSV.

```bash
somniloq sessions                                # all sessions
somniloq sessions --since 24h                    # last 24 hours
somniloq sessions --since 7d --project myapp     # filtered by project
somniloq sessions --since 2026-03-28 --until 2026-03-29
somniloq sessions --short                        # short project names
somniloq sessions --format json                  # JSON array instead of TSV
```

| Flag | Default | Description |
|------|---------|-------------|
| `--since` | — | Start time filter (see "Time filters") |
| `--until` | — | End time filter |
| `--project` | — | Substring match on `repo_path` |
| `--short` | false | Show repo basename (`filepath.Base(repo_path)`) |
| `--format` | tsv | `tsv` or `json` (see "JSON output") |

**Columns** (tab-separated):

```
SessionID    TimeRange    RepoPath    CustomTitle    MessageCount    BodySize
```

- `TimeRange` is displayed as `YYYY-MM-DD HH:MM ~ YYYY-MM-DD HH:MM` (started ~ ended) in local time. If `ended_at` is unavailable, the format is `YYYY-MM-DD HH:MM ~`.
- `RepoPath` column shows `repo_path` (e.g. `/Users/ryota/Sources/myapp`). Empty when unresolved.
- `--short` shows `filepath.Base(repo_path)` (e.g. `myapp`, hyphens preserved).
- `BodySize` is the total body size in bytes (sidechain excluded): approximately how much `show` would print (show adds Markdown headers on top). Use it to spot sessions worth `outline`-ing first — `MessageCount` alone can hide a single huge message.
- `--format json` fields: `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `messageCount`, `bodySize`.

---

### show

Display session content as Markdown. Accepts either a single session ID or a time range.

```bash
# single session (flags must come BEFORE the session-id)
somniloq show <session-id>
somniloq show --short <session-id>

# partial reads (turn numbers come from `somniloq outline`)
somniloq show --turn 40 <session-id>
somniloq show --turn 40..60 <session-id>
somniloq show --tail 3 <session-id>
somniloq show --format json <session-id>

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
| `--project` | — | Substring match on `repo_path` (time-range mode only) |
| `--short` | false | Show repo basename (`filepath.Base(repo_path)`) |
| `--summary <N>` | 0 | Show first N user messages per session after skipping `/clear` and `<local-command-caveat>`. `0` disables (full output). Requires an integer argument — bare `--summary` is an error (use `--summary 1` for the old default). |
| `--include-clear` | false | Requires `--summary >= 1`; disable `/clear` + caveat skipping (sidechain still excluded). Debug use. |
| `--turn <N\|N..M>` | — | Show only turn N or turns N..M (inclusive). Turn numbers match `outline`. A turn = a user message plus the replies that follow it. Out-of-range turns print the session header with no body (exit 0). |
| `--tail <N>` | 0 | Show only the last N turns. `0` disables. |
| `--format` | markdown | `markdown` or `json` (see "JSON output") |

**Constraints:**
- `<session-id>` and `--since`/`--until` are mutually exclusive.
- `--project` only applies in time-range mode. `--summary`, `--turn`, and `--tail` work in both modes (applied per session in time-range mode).
- `--turn` and `--tail` are mutually exclusive, and neither can be combined with `--summary`.
- `--include-clear` without `--summary >= 1` is an error.
- `--summary` takes an integer (v0.2+). Earlier versions accepted a bare `--summary` flag; that form no longer works.
- Single-session `show <session-id>` searches across Claude Code and Codex. If the same `session_id` exists in multiple sources, somniloq exits with an ambiguity error and prints the matching source/session candidates.

**Output structure:**

```markdown
## Session Title

- **Session**: `<session-id>`
- **Project**: `<repo-path>`
- **Started**: `<started_at ~ ended_at>`

### User

<message>

### Assistant

<message>
```

Multiple sessions are separated by `---`. Sidechain messages (subagent internals) are excluded.

---

### outline

Map a long session before reading it: one TSV row per user message with its turn number, local time, and first line. Use the turn numbers with `show --turn`.

```bash
somniloq outline <session-id>
somniloq outline --format json <session-id>
```

**Columns** (tab-separated):

```
Turn    Time    FirstLine
```

- Turn numbers are 1-based and increment on each user message (sidechain excluded, same as `show`).
- Synthetic user messages (`/clear` echo, `<local-command-caveat>`) count as turns and are listed.
- Recommended flow for long sessions: `outline` → pick the relevant turns → `show --turn N..M`.
- `--format json` fields: `turn`, `timestamp`, `firstLine`.

---

### projects

List projects with session counts, sorted by most-recently-active first. Output is TSV.

```bash
somniloq projects                          # all projects
somniloq projects --since 7d               # active in last 7 days
somniloq projects --short                  # short project names
somniloq projects --since 30d --short
somniloq projects --format json
```

| Flag | Default | Description |
|------|---------|-------------|
| `--since` | — | Start time filter |
| `--until` | — | End time filter |
| `--short` | false | Show repo basename (`filepath.Base(repo_path)`) |
| `--format` | tsv | `tsv` or `json` (see "JSON output") |

**Columns** (tab-separated):

```
RepoPath    SessionCount
```

`RepoPath` shows `repo_path` (empty when unresolved). Worktree and subdirectory sessions are merged into their root project via `repo_path` aggregation in SQL — session counts are combined. `--format json` fields: `project`, `sessionCount`.

---

## JSON output

`sessions`, `projects`, `outline` (`--format tsv|json`) and `show` (`--format markdown|json`) emit JSON for scripting — prefer it over parsing TSV/Markdown. Shared rules:

- Always a JSON array; empty results print `[]`. Single-session `show` is an array with one element.
- Timestamps are the stored RFC3339 UTC values (e.g. `2026-06-11T05:08:19.794Z`), not the local-time display format.
- Strings are raw (no tab/newline sanitizing). `title` is the raw custom title — no session-id fallback.
- `project` honors `--short`; without it you get the raw `repo_path`.
- `show --format json` elements: `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `messages` (array of `role`, `content`, `timestamp`). `--summary` / `--turn` / `--tail` filtering applies to `messages`.

```bash
# sessions of the last day as id + size pairs
somniloq sessions --since 24h --format json | jq -r '.[] | "\(.sessionId)\t\(.bodySize)"'

# all user messages of a session
somniloq show --format json <session-id> | jq -r '.[0].messages[] | select(.role == "user") | .content'
```

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
- **`--project`** is a substring match against `repo_path` — `--project app` matches `myapp` and `app-server`. Slash-spanning queries (e.g. `--project Sources/ryot`) also work.
- **All timestamps** in output are local time.
- Every subcommand supports `--help` for a quick flag reference.
