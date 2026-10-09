# somniloq

[![CI](https://github.com/ryotapoi/somniloq/actions/workflows/ci.yml/badge.svg)](https://github.com/ryotapoi/somniloq/actions/workflows/ci.yml)

somniloq is a local CLI for importing Claude Code, Codex, and Cursor Agent JSONL session logs into SQLite, then searching and reading conversations across sessions. A TOML config selects the database and one or more log roots per source.

English · [日本語](README.ja.md)

## Install

Requires Go 1.27.1 or later.

```bash
go install github.com/ryotapoi/somniloq/cmd/somniloq@latest
```

## Quick start

```bash
somniloq config init                              # Create default TOML; no DB is created
somniloq import --config default                  # Import all configured inputs
somniloq search --config default --since 2026-10-01 # Find groups active since this date
somniloq search --config default "auth bug"        # Find groups by words in messages
somniloq show --config default REF --messages 12:18 # Read original message numbers
```

Every database command uses `default` when `--config NAME_OR_PATH` is omitted. An explicit config can appear before or after the command name. Put search flags before the query. `search` returns full `slq1:...` references that distinguish conversations with the same session ID in different inputs. Copy the complete REF into `show`; bare IDs and shortened references are rejected.

## Commands

| Command | Use |
|---------|-----|
| `config init` | Create a TOML config without opening a database. |
| `import` | Import new log content; repeat `--input PATH` to select roots, and use `--source claude-code`, `codex`, or `cursor-agent` to restrict sources. |
| `migrate` | Copy a fixed snapshot of a supported legacy database to another database, replacing history with the same ID as successfully parsed Codex owners. |
| `projects` | List projects and session counts. |
| `search` | List conversation groups without a pattern, or filter by words, time, input, source, and project. Use a known REF to find every matching occurrence. |
| `show` | Read original messages from multiple conversations as TSV or JSON, with message filters, pagination, and one-line output. |

`import` is incremental. Input selections are ORed, then intersected with `--source`. **`--full` rebuilds only the selected inputs' conversations and import state**, retaining other inputs. Check that the selected inputs' original logs are available. It asks for confirmation; `--yes` skips the prompt and is required in noninteractive environments.

When import fails after scanning starts, stdout retains the partial summary with committed files counted as imported. Failure reasons go to stderr and the command exits 1.

Codex import keeps each conversation's own messages separate from inherited context. The first valid session metadata identifies the conversation; an explicit parent ID is retained even if the parent arrives later. Text blocks, their source path and line, original timestamps (including unknown values), and one-based message numbers are stored. Multiple rollouts for the same conversation are ordered by relative path and physical line; incremental import rebuilds that order when earlier content changes. Use a child's full REF to read its own conversation. `show` returns original message numbers and text blocks.

New databases use schema revision 1. Normal commands reject legacy or unsupported databases, including the earlier root-only revision 1 shape, without modifying them. Use `somniloq migrate --config archive --from ./archive-snapshot.db` for the supported legacy shape. Set the config’s `db` to a missing or empty destination and configure all remaining Codex roots. Supply a fixed standalone snapshot without sidecars. Rerun only with the same snapshot and a completed copy receipt. Successfully parsed Codex owners replace all same-ID Codex legacy history, including when their own body is empty. Inherited context remains context. Old rows with other IDs remain even when physical lines match; history without a current owner and other sources remain accessible through legacy REFs. Successful empty replacements count as replaced and exit 0; processing failures cause exit 1. The JSON summary and stderr report these outcomes. Run a successful normal import with the same configuration afterward to capture appends and new child logs created during migration. Do not edit, delete, or move existing logs during migration. See the [migration contract](docs/migration.md) for details. Read commands reject missing databases without creating them.

Use `somniloq <command> --help` for flags and formats. `projects`, `search`, and `show` support `--format tsv|json`. JSON output uses an envelope for `show` and `search`, and an array for `projects`. Without `--session`, `search` lists conversation groups without message excerpts. Claude Code children and grandchildren have independent conversation REFs even when they share the root sessionId, and their original sidechain text is retained. Direct parents require matching Agent/Task calls and structured results in the same physical file; path-based root membership is stored separately. Incomplete scans or file reads preserve the affected input’s previously saved text, relations, and cursors; other inputs continue. `search --session REF PATTERN` uses the shared relation resolver to search the named conversation and confirmed descendants, excluding ancestors, siblings, and children known only by root membership. It requires a pattern and returns every occurrence using Go regular expressions, with support for multiple `-e` patterns, `-F` literal matching, and `--all` AND matching.

Pass multiple full REFs in one call to retrieve actual messages from a specific day for a daily note. `show` preserves the supplied REF order and each conversation's original message order. `--descendants` expands only confirmed descendants and removes duplicate conversations. Role, message-number, and time filters apply before message-level limit, offset, and tail selection. `--one-line` shortens only `text` to its first line; `blocks` retain the original text. TSV is the default; JSON uses the `{items,total,count,limit,offset,hasMore,nextOffset}` envelope. Dates and RFC3339 timestamps with a time zone are accepted; relative times are rejected. The former `outline`, summary, turn, display-exclusion, Markdown, and time-only retrieval without a REF have been removed. Search listings return REFs and metadata without message excerpts or turns. Detailed search uses the same original `messageNumber` values as `show`. See the [CLI contract](docs/cli-contract.md) for details.

```sh
# REF1 / REF2 are full REFs copied from search
somniloq show --config default REF1 REF2 --since 2026-10-01 --until 2026-10-01 --format json
somniloq show --config default REF --role user --one-line
somniloq show --config default REF --messages 12:18 --limit 50 --format json
```

Search listings use the `{items,total,count,limit,offset,hasMore,nextOffset}` JSON envelope and return all results by default (`limit=null`). Only an explicit `--limit N` caps the result count; `--offset` alone returns all remaining sorted results. Omit patterns to list groups. Multiple `-e` patterns use OR; `--all` requires all patterns across the candidate messages; `-F` matches literal strings. Repeated input or source filters use OR within each filter type, and different filter types use AND. Project filters match case-sensitive substrings of the final path component and expand exact aliases. `members` contains the whole group; `matchedMembers` contains the selected candidate conversations. Root metadata is never inferred from children. Groups are sorted by the latest original message timestamp across all members, newest first, with unknown times last and group keys breaking ties. `limit=0` and offsets beyond the end still return `total`.

Search accepts dates or RFC3339 timestamps with a time zone. Dates start at `dayBoundary`, which can be overridden on the CLI. An `--until` date includes the whole specified day; a timestamp upper bound is exclusive. `--time-mode` accepts `active` (default), `started`, `last`, or `overlap`; explicitly selecting a mode requires a time range. `active` matches candidate messages within the range and requires actual messages even without a pattern. Other modes select groups by the start, last activity, or overlapping activity interval across all members, then match all candidate messages. `--imported-since RFC3339` sets a lower bound on candidate conversations' import times; excluded parent messages cannot satisfy AND matching. Detailed search supports only `active`. Relative times, timestamps without a time zone, and empty values are rejected.

The following example selects REFs from all matching groups, which is the default. When you explicitly set a limit, use `hasMore` and `nextOffset` to retrieve subsequent pages.

```sh
sh <<'SH'
set -- $(somniloq search --config default --since 2026-10-01 --until 2026-10-01 --format json | jq -r '.items[].members[]' | sort -u)
if [ "$#" -gt 0 ]; then
  somniloq show --config default "$@" --since 2026-10-01 --until 2026-10-01 --format json
fi
SH
```

This example runs in POSIX `sh`, including when invoked from interactive zsh. `members` includes the entire group, including children known only by root membership; `matchedMembers` contains the candidates selected by search filters. Full REFs contain no whitespace or glob characters. The sort order becomes the conversation order in `show`. An empty selection does not invoke `show`.

Detailed search for a known REF returns all occurrences by default; explicit limit and offset operate on occurrences. Results include original UTF-8 byte positions, `matchText`, the full `lineText`, and message and occurrence numbers. Use the same REF and message number to retrieve the original with `show`. Put flags before the positional PATTERN. `patternIndexes` numbers the positional PATTERN first, followed by `-e` patterns in their supplied order. Listings support the same `-e`, `-F`, and `--all` options.

```sh
somniloq search --config default --session REF -e "Inherited question" -e "Child answer" --all --format json
somniloq search --config default --session REF -F --limit 20 --offset 20 "auth bug"
somniloq show --config default REF --messages 1:1 --format json
```

## Configuration

`somniloq config init [NAME] [--output PATH] [--db PATH]` defaults to name `default`, output `~/.somniloq/config/NAME.toml`, and database `~/.somniloq/NAME.db`. It creates only the config, and only when both its destination and referenced database are absent. Existing files, directories, and symlinks (including dangling symlinks) at either path are refused and preserved. This applies to both the default database and explicit `--db`; relative database paths resolve against the config's real parent directory. It creates config parent directories and prints the config's absolute path. `--db` is available only for init.

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

`projectAliases` optionally groups renamed projects (`[projectAliases]` followed by `new-name = ["old-name"]`). Overlapping groups are rejected. `dayBoundary` optionally sets the logical day boundary in local time. Unknown keys and invalid values are errors. Legacy JSON is not discovered or converted; `excludeUserMessagePatterns` is no longer a config key. `show` no longer provides display-exclusion flags.

`--config default` selects the named config; `--config ./archive.toml` selects a path. Omitting `--config` selects `default`. Missing configs exit 2 with setup instructions and create neither a database nor a config, including for import.

## More information

- [CLI reference and output formats](docs/cli-contract.md)
- [Project purpose and non-goals](docs/requirements.md)
- [Changelog](CHANGELOG.md)

## License

[MIT License](LICENSE)

See [THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt) for license and copyright notices for third-party libraries and Go itself.
