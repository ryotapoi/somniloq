# somniloq

somniloq is a local CLI for importing Claude Code, Codex, and Cursor Agent session logs into SQLite, then searching and reading conversations across sessions. It reads JSONL under `~/.claude/projects/`, `~/.codex/sessions/`, and `~/.cursor/projects/`.

[日本語版 README](README.ja.md)

## Install

Requires Go 1.27.1 or later.

```bash
go install github.com/ryotapoi/somniloq/cmd/somniloq@latest
```

## Quick start

```bash
somniloq import                         # Import new log content from all three sources
somniloq sessions --since 7d           # Find recent sessions
somniloq search "auth bug"              # Search message bodies across sessions
somniloq outline <session-id>           # Skim a long session by turn
somniloq show --turn 12..18 <session-id> # Read selected turns
```

`sessions` and `search` results include a `source` value. If the same session ID exists in multiple sources, pass that value when reading it, for example `somniloq show --source codex <session-id>`. Without `--source`, `show` and `outline` report the matching sources instead of choosing one.

## Commands

| Command | Use |
|---------|-----|
| `import` | Import new log content; use `--source claude-code`, `codex`, or `cursor-agent` to select one source. |
| `sessions` | List sessions; `--since 24h` filters by session time, while `--imported-since 24h` finds sessions saved or updated recently. |
| `projects` | List projects and session counts. |
| `search` | Search message bodies; use `--project` or `--since` to narrow results. |
| `outline` | List user turns in a session before reading a long conversation. |
| `show` | Read a session in Markdown; use `--turn` or `--tail` to read part of it. |

No dedicated upgrade or data repair path is provided for legacy databases. General schema management remains, but migration from v0.3 databases to the current schema is not guaranteed.

`import` is incremental by default. **`somniloq import --full` deletes the entire somniloq database before re-importing.** This also applies when `--source` selects one source: rows from other sources are deleted, and only the selected source is re-imported. Check that the original logs for everything you want to keep are available before using it. `--full` asks for confirmation; `--yes` skips the prompt.

Use `somniloq <command> --help` for flags, output formats, and examples. `sessions`, `projects`, `search`, and `outline` support `--format json`; `show` supports Markdown or JSON.

## Configuration

The optional JSON config is `~/.somniloq/config.json`; the database defaults to `~/.somniloq/somniloq.db`. Set another path with the global `--config` or `--db` flag, before the command name.

```json
{
  "projectAliases": {"new-name": ["old-name"]},
  "commandPatterns": ["^Daily report"],
  "dayBoundary": "04:00"
}
```

`projectAliases` groups renamed projects, `commandPatterns` marks user turns treated as commands in session list hints, and `dayBoundary` sets the start of a logical day in local time. Omit any keys you do not need.

## More information

- [CLI behavior and configuration](docs/rules/scope.md)
- [Project purpose and non-goals](docs/rules/mission.md)
- [Changelog](CHANGELOG.md)

## License

[MIT License](LICENSE)
