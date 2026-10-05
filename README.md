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
somniloq search --config default --since 2026-10-01 # 指定日以降のまとまりを探す
somniloq search --config default "auth bug"        # 本文の語からまとまりを探す
somniloq show --config default <REF> --messages 12:18 # 発言番号で読む
```

Every database command uses `default` when `--config NAME_OR_PATH` is omitted. An explicit config can appear before or after the command name. Put search flags before the query. `search` returns full `slq1:...` references that distinguish conversations with the same session ID in different inputs. Copy the complete REF into `show`; bare IDs and shortened references are rejected.

## Commands

| Command | Use |
|---------|-----|
| `config init` | Create a TOML config without opening a database. |
| `import` | Import new log content; repeat `--input PATH` to select roots, and use `--source claude-code`, `codex`, or `cursor-agent` to restrict sources. |
| `migrate` | Copy a fixed snapshot of a supported legacy database to another database, replacing only old rows proven by remaining Codex logs. |
| `projects` | List projects and session counts. |
| `search` | pattern なしで一覧し、語・期間・入力・source・project から作業のまとまりを選ぶ。既知 REF は全一致詳細で探す。 |
| `show` | 複数会話の原文を TSV / JSON で取得し、発言フィルタ・ページ・一行表示を選ぶ。 |

`import` is incremental. Input selections are ORed, then intersected with `--source`. **`--full` rebuilds only the selected inputs' conversations and import state**, retaining other inputs. Check that the selected inputs' original logs are available. It asks for confirmation; `--yes` skips the prompt and is required in noninteractive environments.

Codex import keeps each person's conversation separate from inherited context. The first valid session metadata identifies the conversation; an explicit parent ID is retained even if the parent arrives later. Text blocks, their source path and line, original timestamps (including unknown values), and one-based message numbers are stored. Multiple rollouts for the same conversation are ordered by relative path and physical line; incremental import rebuilds that order when earlier content changes. Use a child's full REF to read its own conversation. show は元の発言番号と原文 blocks を返します。

New databases use schema revision 1. Normal commands reject legacy or unsupported databases, including the earlier root-only revision 1 shape, without modifying them. Use `somniloq migrate --config archive --from ./archive-snapshot.db` for the supported legacy shape. Set the config’s `db` to a missing or empty destination and configure all remaining Codex roots. Supply a fixed standalone snapshot without sidecars. Rerun only with the same snapshot and a completed copy receipt. Unknown membership, missing logs, and other sources’ old history are retained and accessible through legacy REFs. See the [migration contract](docs/specs/v0.14.0-migration.md) for details. Read commands reject missing databases without creating them.

Use `somniloq <command> --help` for flags and formats. `projects`, `search`, and `show` support `--format tsv|json`. show と search の JSON は envelope、projects は配列。session なし search は本文抜粋を含まないまとまり一覧です。 Claude Code children and grandchildren have independent conversation REFs even when they share the root sessionId, and their original sidechain text is retained. Direct parents require matching Agent/Task calls and structured results in the same physical file; path-based root membership is stored separately. Incomplete scans or file reads preserve the affected input’s previously saved text, relations, and cursors; other inputs continue. `search --session REF PATTERN` uses the shared relation resolver to search the named conversation and confirmed descendants, excluding ancestors, siblings, and children known only by root membership. pattern 必須の Go regexp で全一致箇所を返し、複数 -e / -F / --all AND を利用できる。

複数の完全 REF を一回の呼び出しで渡し、指定日の実発言だけを Daily Note の材料として取得できます。REF の指定順・各会話の元の発言番号順を保ち、`--descendants` は確定子孫だけを展開して重複会話を除きます。role・発言番号・日時で絞った後に、発言単位で limit / offset / tail を適用します。`--one-line` は text だけを最初の一行へ短縮し、blocks は原文を保ちます。既定 TSV、JSON は `{items,total,count,limit,offset,hasMore,nextOffset}` envelope です。show の日時は日付または zone 付き RFC3339 で指定し、相対時刻は受理しません。旧 outline・summary・turn・表示除外・Markdown・REF なし期間入口は廃止しました。一覧は REF と metadata を返し、本文抜粋・turn を含みません。詳細 search の messageNumber は show と同じ原文番号です。詳細は [現行仕様](docs/rules/scope.md#内容表示show) を参照してください。

```sh
# REF1 / REF2 は search からコピーした完全 REF
somniloq show --config default REF1 REF2 --since 2026-10-01 --until 2026-10-01 --format json
somniloq show --config default REF --role user --one-line
somniloq show --config default REF --messages 12:18 --limit 50 --format json
```

一覧の JSON は `{items,total,count,limit,offset,hasMore,nextOffset}`、既定全件（limit=null）です。明示 `--limit N` だけが上限となり、`--offset` のみでは整列済み結果の残り全件を返します。pattern 省略で一覧、複数 -e は OR、--all は候補本文集合で AND、-F は固定文字列。input/source は繰り返しの OR、条件種間は AND。project は末尾名の大小文字区別 substring と完全一致 alias 展開です。members は全まとまり、matchedMembers は候補本人。root metadata を子で補完せず、全 members の本人原文日時で last 降順・未知最後・group key 順に整列します。limit=0 と末尾超過も total を返します。

search の日時は日付または zone 付き RFC3339。日付は dayBoundary（CLI で上書き可）が起点で、until 日付は指定日全体を含み、日時の上限は排他です。--time-mode は active（既定）/started/last/overlap、明示時は期間必須。active は期間内候補本文で照合し、pattern なしでも実発言が必要です。他 mode は全 members の開始/最後・重なる期間でまとまりを選び、候補全文を照合します。--imported-since RFC3339 は候補本人の取り込み下限で、除外された親本文を AND に使いません。詳細は active のみです。相対時刻・zone なし日時・空値は拒否します。

次は既定ページから REF を選ぶ例です。全件が必要なら hasMore/nextOffset を見てページを取得します。

```sh
sh <<'SH'
set -- $(somniloq search --config default --since 2026-10-01 --until 2026-10-01 --format json | jq -r '.items[].members[]' | sort -u)
if [ "$#" -gt 0 ]; then
  somniloq show --config default "$@" --since 2026-10-01 --until 2026-10-01 --format json
fi
SH
```

POSIX sh で実行する例です（対話 zsh でも `sh` が実行します）。members は root 所属だけの子を含むまとまり全体、matchedMembers は検索条件で選んだ候補本人です。完全 REF は空白や glob 文字を含まず、この sort 順が show の会話順になります。空の選択では show を呼びません。

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

`projectAliases` optionally groups renamed projects (`[projectAliases]` followed by `new-name = ["old-name"]`). Overlapping groups are rejected. `dayBoundary` optionally sets the logical day boundary in local time. Unknown keys and invalid values are errors. Legacy JSON is not discovered or converted; `excludeUserMessagePatterns` is no longer a config key. show の表示除外フラグも提供しません。

`--config default` selects the named config; `--config ./archive.toml` selects a path. Omitting `--config` selects `default`. Missing configs exit 2 with setup instructions and create neither a database nor a config, including for import.

## More information

- [CLI behavior and configuration](docs/rules/scope.md)
- [Project purpose and non-goals](docs/rules/mission.md)
- [Changelog](CHANGELOG.md)

## License

[MIT License](LICENSE)

既知 REF の詳細検索は既定全件で、明示 limit/offset は一致箇所単位です。原文 UTF-8 byte 位置、matchText、行全体の lineText と番号を返し、同じ REF/番号で show に戻れます。フラグは位置 PATTERN より前に置き、patternIndexes は位置 PATTERN が先頭、次に -e の指定順です。一覧でも同じ -e/-F/--all を使えます。

```sh
somniloq search --config default --session REF -e "Inherited question" -e "Child answer" --all --format json
somniloq search --config default --session REF -F --limit 20 --offset 20 "auth bug"
somniloq show --config default REF --messages 1:1 --format json
```
