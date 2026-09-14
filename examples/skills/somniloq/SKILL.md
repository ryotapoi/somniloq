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
somniloq search "keyword"

# 長いセッションは、先に地図を見て必要な turn だけ読む
somniloq outline <session-id>
somniloq show --turn 12..18 <session-id>

# 機械処理するときは JSON を優先する
somniloq sessions --format json
somniloq show --format json <session-id>
```

Cursor Agent の履歴には時刻がないことがあるため、`--since` / `--until` を付けると対象外になります。日時で絞り込む必要がある場合は CLI help を確認してください。

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
