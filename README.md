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
somniloq show --config default <REF> --messages 12:18 # 発言番号で読む
```

Every database command requires `--config NAME_OR_PATH`, before or after the command name. Put search flags before the query. `sessions` and `search` return full `slq1:...` references that distinguish conversations with the same session ID in different inputs. Copy the complete REF into `show`; bare IDs and shortened references are rejected.

## Commands

| Command | Use |
|---------|-----|
| `config init` | Create a TOML config without opening a database. |
| `import` | Import new log content; repeat `--input PATH` to select roots, and use `--source claude-code`, `codex`, or `cursor-agent` to restrict sources. |
| `migrate` | Copy a fixed snapshot of a supported legacy database to another database, replacing only old rows proven by remaining Codex logs. |
| `sessions` | List sessions; `--since 24h` filters by session time, while `--imported-since 24h` finds sessions saved or updated recently. |
| `projects` | List projects and session counts. |
| `search` | Search message bodies; use `--project` or `--since` to narrow results. |
| `show` | 複数会話の原文を TSV / JSON で取得し、発言フィルタ・ページ・一行表示を選ぶ。 |

`import` is incremental. Input selections are ORed, then intersected with `--source`. **`--full` rebuilds only the selected inputs' conversations and import state**, retaining other inputs. Check that the selected inputs' original logs are available. It asks for confirmation; `--yes` skips the prompt and is required in noninteractive environments.

Codex import keeps each person's conversation separate from inherited context. The first valid session metadata identifies the conversation; an explicit parent ID is retained even if the parent arrives later. Text blocks, their source path and line, original timestamps (including unknown values), and one-based message numbers are stored. Multiple rollouts for the same conversation are ordered by relative path and physical line; incremental import rebuilds that order when earlier content changes. Use a child's full REF to read its own conversation. show は元の発言番号と原文 blocks を返します。

New databases use schema revision 1. Normal commands reject legacy or unsupported databases, including the earlier root-only revision 1 shape, without modifying them. Use `somniloq migrate --config archive --from ./archive-snapshot.db` for the supported legacy shape. Set the config’s `db` to a missing or empty destination and configure all remaining Codex roots. Supply a fixed standalone snapshot without sidecars. Rerun only with the same snapshot and a completed copy receipt. Unknown membership, missing logs, and other sources’ old history are retained and accessible through legacy REFs. See the [migration contract](docs/specs/v0.14.0-migration.md) for details. Read commands reject missing databases without creating them.

Use `somniloq <command> --help` for flags and formats. `sessions`, `projects`, `search`, and `show` support `--format tsv|json`. show 以外の JSON は配列、search は literal substring 検索です。 Claude Code children and grandchildren have independent conversation REFs even when they share the root sessionId, and their original sidechain text is retained. Direct parents require matching Agent/Task calls and structured results in the same physical file; path-based root membership is stored separately. Incomplete scans or file reads preserve the affected input’s previously saved text, relations, and cursors; other inputs continue. `search --session REF QUERY` uses the shared relation resolver to search the named conversation and confirmed descendants, excluding ancestors, siblings, and children known only by root membership. Query text remains required and uses LIKE. まとまり検索は後続実装です。

複数の完全 REF を一回の呼び出しで渡し、指定日の実発言だけを Daily Note の材料として取得できます。REF の指定順・各会話の元の発言番号順を保ち、`--descendants` は確定子孫だけを展開して重複会話を除きます。role・発言番号・日時で絞った後に、発言単位で limit / offset / tail を適用します。`--one-line` は text だけを最初の一行へ短縮し、blocks は原文を保ちます。既定 TSV、JSON は `{items,total,count,limit,offset,hasMore,nextOffset}` envelope です。show の日時は日付または zone 付き RFC3339 で指定し、相対時刻は受理しません。旧 outline・summary・turn・表示除外・Markdown・REF なし期間入口は廃止しました。現行 search の `turn` は show の `messageNumber` とは異なります。詳細は [現行仕様](docs/rules/scope.md#内容表示show) を参照してください。

```sh
# REF1 / REF2 は sessions / search からコピーした完全 REF
somniloq show --config default REF1 REF2 --since 2026-10-01 --until 2026-10-01 --format json
somniloq show --config default REF --role user --one-line
somniloq show --config default REF --messages 12:18 --limit 50 --format json
```

次は後続 search 実装後の最終契約例です。現在の search JSON は配列で、`.items[].members[]` や query なしの日付一覧はまだ利用できません。

```sh
set -- $(somniloq search --config default --since 2026-10-01 --until 2026-10-01 --time-mode active --format json | jq -r '.items[].members[]' | sort -u)
if [ "$#" -gt 0 ]; then
  somniloq show --config default "$@" --since 2026-10-01 --until 2026-10-01 --format json
fi
```

完全 REF は空白や glob 文字を含まず、この sort 順が show の会話順になります。

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

`projectAliases` optionally groups renamed projects (`[projectAliases]` followed by `new-name = ["old-name"]`). Overlapping groups are rejected. `dayBoundary` optionally sets the logical day boundary in local time. Unknown keys and invalid values are errors. Legacy JSON is not discovered or converted; `excludeUserMessagePatterns` is no longer a config key. show の表示除外フラグも提供しません。

`--config default` selects the named config; `--config ./archive.toml` selects a path. Missing or omitted configs exit 2 with setup instructions and create neither a database nor a config.

## More information

- [CLI behavior and configuration](docs/rules/scope.md)
- [Project purpose and non-goals](docs/rules/mission.md)
- [Changelog](CHANGELOG.md)

## License

[MIT License](LICENSE)
