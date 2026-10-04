# somniloq

somniloq is a local CLI for importing Claude Code, Codex, and Cursor Agent JSONL session logs into SQLite, then searching and reading conversations across sessions. A TOML config selects the database and one or more log roots per source.

[日本語版 README](README.ja.md)

## Install

Requires Go 1.27.1 or later.

```bash
go install github.com/ryotapoi/somniloq/cmd/somniloq@latest
```

## Quick start

```bash
somniloq config init                              # Create default TOML; no DB is created
somniloq import --config default                  # Import all configured inputs
somniloq sessions --config default --since 7d      # Find recent sessions
somniloq search --config default "auth bug"        # Search message bodies
somniloq outline --config default <REF>            # Use the full REF from sessions/search
somniloq show --config default --turn 12..18 <REF> # Read selected turns
```

Every database command requires `--config NAME_OR_PATH`, before or after the command name. Put search flags before the query. `sessions` and `search` return full `slq1:...` references that distinguish conversations with the same session ID in different inputs. Copy the complete REF into `show` or `outline`; bare IDs and shortened references are rejected.

## Commands

| Command | Use |
|---------|-----|
| `config init` | Create a TOML config without opening a database. |
| `import` | Import new log content; repeat `--input PATH` to select roots, and use `--source claude-code`, `codex`, or `cursor-agent` to restrict sources. |
| `sessions` | List sessions; `--since 24h` filters by session time, while `--imported-since 24h` finds sessions saved or updated recently. |
| `projects` | List projects and session counts. |
| `search` | Search message bodies; use `--project` or `--since` to narrow results. |
| `outline` | List user turns in a conversation selected by full REF. |
| `show` | Read a conversation in Markdown; use `--turn` or `--tail` to read part of it. |

`import` is incremental. Input selections are ORed, then intersected with `--source`. **`--full` rebuilds only the selected inputs' conversations and import state**, retaining other inputs. Check that the selected inputs' original logs are available. It asks for confirmation; `--yes` skips the prompt and is required in noninteractive environments.

New databases use schema revision 1. Normal commands reject legacy or unsupported databases without modifying them. A dedicated `migrate` command is planned. Read commands reject missing databases without creating them.

Use `somniloq <command> --help` for flags and formats. `sessions`, `projects`, `search`, and `outline` support `--format json`; `show` supports Markdown or JSON. JSON still returns arrays, and search still uses literal substring matching. Parent/child ingestion, grouped search, and the redesigned original-text `show` interface are planned separately.

## Configuration

`somniloq config init [NAME] [--output PATH] [--db PATH]` defaults to name `default`, output `~/.somniloq/config/NAME.toml`, and database `~/.somniloq/NAME.db`. It creates parent directories, refuses existing destinations including symlinks, and prints the config's absolute path. `--db` is available only for init.

The generated config includes these three inputs:

```toml
db = "~/.somniloq/default.db"
dayBoundary = "00:00"

[[inputs]]
name = "Claude"
source = "claude-code"
root = "~/.claude/projects"

[[inputs]]
name = "Codex"
source = "codex"
root = "~/.codex/sessions"

[[inputs]]
name = "Cursor"
source = "cursor-agent"
root = "~/.cursor/projects"
```

Add more `[[inputs]]` entries for additional roots. The same source and resolved root are scanned once; input names are optional display labels and do not affect identity. Missing roots import zero files. Relative database and root paths resolve against the config's real parent directory, including when the config is a symlink. Only a leading `~` or `~/` expands to home; environment variables do not expand.

`projectAliases` optionally groups renamed projects (`[projectAliases]` followed by `new-name = ["old-name"]`). Overlapping groups are rejected. `dayBoundary` optionally sets the logical day boundary in local time. Unknown keys and invalid values are errors. Legacy JSON is not discovered or converted; `excludeUserMessagePatterns` is no longer a config key. Current outline/summary exclusions can be specified with `--exclude-user-message-pattern`.

`--config default` selects the named config; `--config ./archive.toml` selects a path. Missing or omitted configs exit 2 with setup instructions and create neither a database nor a config.

## More information

- [CLI behavior and configuration](docs/rules/scope.md)
- [Project purpose and non-goals](docs/rules/mission.md)
- [Changelog](CHANGELOG.md)

## License

[MIT License](LICENSE)
