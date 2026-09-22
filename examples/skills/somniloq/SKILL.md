---
name: somniloq
description: >
  Claude Code / Codex / Cursor Agent のセッション履歴を取り込み、検索、一覧、本文参照するときに somniloq を使う。
  この skill は意図的に薄く保ち、構文、出力列、詳細な実例は CLI help を参照する。
---

# somniloq

somniloq は Claude Code / Codex / Cursor Agent のセッションログを SQLite に取り込み、過去セッションを一覧・検索・表示する CLI です。会話履歴、過去作業、セッション本文、プロジェクト別の作業履歴を調べるときに使います。

## 最初に確認すること

新しいセッションがあり得る場合は、検索や一覧の前に必ず取り込みます。

```bash
somniloq import
```

DB は import 時点のスナップショットです。自動更新ではありません。

## 代表的な探索導線

```bash
# セッションを眺める
somniloq sessions --short

# キーワードで見つける
somniloq search --format json "keyword"

# search または sessions が出した source は --source に、session ID は位置引数に渡す
# 長いセッションは、先に地図を見て必要な turn だけ読む
somniloq outline --source <source> <session-id>
somniloq show --source <source> --turn 12..18 <session-id>

# 検索結果を 50 件ずつページ単位で読む（最初、次）
somniloq search --limit 50 --offset 0 "keyword"
somniloq search --limit 50 --offset 50 "keyword"

# 機械処理するときは JSON を優先する
somniloq sessions --format json
somniloq show --source <source> --format json <session-id>
```

Cursor Agent の履歴には時刻がないことがあり、`--since` / `--until` を付けると対象外になります。時刻不明でも直近に取り込んだ履歴は、`somniloq sessions --imported-since 24h` で探せます。日時で絞り込む必要がある場合は CLI help を確認してください。`--since`、`--until`、`sessions --imported-since` は、相対時刻とローカルの日付・分単位日時に加え、`Z` または数値オフセット付きの RFC3339 instant を受け付ける。

## 詳細は CLI help を見る

この skill は CLI 構文の重複記述を避けるため、フラグ、出力列、実例の完全な説明を持ちません。必要なコマンドの help を直接確認してください。

```bash
somniloq --help
somniloq import --help
somniloq sessions --help
somniloq search --help
somniloq outline --help
somniloq show --help
somniloq projects --help
somniloq backfill --help
```

`outline -> show --turn`、`search -> outline -> show --turn` などの横断的な使い方も各 command help にあります。
