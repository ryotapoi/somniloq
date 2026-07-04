---
name: somniloq
description: >
  Use somniloq when you need to import, search, list, or inspect Claude Code and Codex session history.
  This skill intentionally stays thin; the CLI help is the reference for flags, output columns, and examples.
---

# somniloq

somniloq は Claude Code / Codex のセッションログを SQLite に取り込み、過去セッションを一覧・検索・表示する CLI です。会話履歴、過去作業、セッション本文、プロジェクト別の作業履歴を調べるときに使います。

## 最初に確認すること

新しいセッションがあり得る場合は、検索や一覧の前に必ず取り込みます。

```bash
somniloq import
```

DB は import 時点のスナップショットです。自動更新ではありません。

## 代表的な探索導線

```bash
# 最近のセッションを眺める
somniloq sessions --since 7d --short

# キーワードで見つける
somniloq search --since 7d "keyword"

# 長いセッションは、先に地図を見て必要な turn だけ読む
somniloq outline <session-id>
somniloq show --turn 12..18 <session-id>

# 機械処理するときは JSON を優先する
somniloq sessions --since 7d --format json
somniloq show --format json <session-id>
```

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
