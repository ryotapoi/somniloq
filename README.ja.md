# somniloq

somniloq は Claude Code / Codex / Cursor Agent のセッションログを SQLite に取り込み、セッションを横断して会話を検索・閲覧するローカル CLI。`~/.claude/projects/`、`~/.codex/sessions/`、`~/.cursor/projects/` の JSONL を読み取る。

[English README](README.md)

## インストール

Go 1.27.1 以降が必要。

```bash
go install github.com/ryotapoi/somniloq/cmd/somniloq@latest
```

## クイックスタート

```bash
somniloq import                         # 3 source の新しいログを取り込む
somniloq sessions --since 7d           # 最近のセッションを探す
somniloq search "auth bug"              # セッション横断で本文を検索する
somniloq search --since 7d --project somniloq "auth" # プロジェクトの直近メッセージを検索する
somniloq outline <session-id>           # 長いセッションのターンを一覧する
somniloq show --turn 12..18 <session-id> # 必要なターンを読む
```

期間などの search フラグは検索語より前に置きます。

`sessions` と `search` の結果には `source` が含まれる。同じセッション ID が複数の source にある場合は、`somniloq show --source codex <session-id>` のように指定する。省略すると `show` と `outline` は一方を選ばず候補を表示する。

## コマンド

| コマンド | 用途 |
|----------|------|
| `import` | 新しいログを取り込む。`--source claude-code`、`codex`、`cursor-agent` で対象を1つに絞れる。 |
| `sessions` | セッション一覧。`--since 24h` はセッション時刻、`--imported-since 24h` は最近保存・更新されたセッションで絞る。 |
| `projects` | プロジェクトとセッション件数を一覧する。 |
| `search` | メッセージ本文を検索する。`--project` や `--since` で絞れる。 |
| `outline` | 長い会話を読む前に、user ターンを一覧する。 |
| `show` | セッションを Markdown で読む。`--turn` や `--tail` で一部だけ読める。 |

旧形式 DB 向けの専用アップグレード・データ補正手段は提供しない。一般的な schema 管理は維持するが、v0.3 形式から現在の schema への移行成功は保証しない。

`import` はデフォルトで差分を取り込む。**`somniloq import --full` は再取り込み前に somniloq の DB 全体を削除する。** `--source` で1つの source を選んでも他の source の行を削除し、指定した source だけを再取り込みする。保持したいログの原本が揃っていることを確認してから使う。`--full` は確認を求め、`--yes` で確認を省略できる。

フラグ・出力形式・使用例は `somniloq <command> --help` を参照。`sessions`、`projects`、`search`、`outline` は `--format json` に対応し、`show` は Markdown または JSON で出力する。

`sessions` TSV は時刻範囲のタブ・改行を空白化し、8 列・1 セッション 1 行を保つ。JSON は保存済み timestamp の文字列をそのまま出す。

## 設定

任意の JSON 設定ファイルは `~/.somniloq/config.json`、DB のデフォルトは `~/.somniloq/somniloq.db`。別のパスはコマンド名の前にグローバルフラグ `--config` または `--db` で指定する。

```json
{
  "projectAliases": {"new-name": ["old-name"]},
  "excludeUserMessagePatterns": ["^<command-name>/clear</command-name>"],
  "dayBoundary": "04:00"
}
```

`projectAliases` は改名したプロジェクトをまとめ、`excludeUserMessagePatterns` は `outline` と `show --summary` の user message を表示から除外し、`dayBoundary` はローカル時刻で論理日の開始を指定する。pattern は trim 済みの本文全文に Go 正規表現で照合する。不要なキーは省略できる。

v0.13.0 では `commandPatterns` を自動移行しない。pattern を `excludeUserMessagePatterns` へ移すと、適用先がセッション一覧のヒントから表示メッセージへ変わる。slash で始まる本文も除外する場合は `^/` を明示する。`show --summary` は `/clear` や command caveat を既定では除外しないため、必要なら pattern を設定する。一度の summary だけ除外を無効にするには `--no-exclude-user-messages` を使う。sessions TSV は 2 列を削除して `source` が 8 列目へ移り、JSON は `nonCommandUserTurnCount` と `firstNonCommandUserLine` を削除する。

## 詳細

- [CLI の振る舞いと設定](docs/rules/scope.md)
- [目的と非目標](docs/rules/mission.md)
- [変更履歴](CHANGELOG.ja.md)

## ライセンス

[MIT License](LICENSE)
