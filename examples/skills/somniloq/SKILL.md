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
somniloq import --config default
```

全 DB コマンドで明示 `--config NAME_OR_PATH` が必要です。設定がなければ `somniloq config init` で default TOML を生成します（DB は作成しません）。既存の設定名・path を確認して選び、旧 JSON や旧 DB を自動移行するものと考えないでください。DB は import 時点のスナップショットです。自動更新ではありません。

既知の旧 DB の履歴保持には `somniloq migrate --config NAME_OR_PATH --from PATH` を使います。元は sidecar のない固定 standalone snapshot、設定の `db` は別の未存在または空 DB、inputs は残存 Codex ログの全 root にします。所属不明・ログ欠落・他 source の旧履歴は保持され、legacy REF で読めます。同じ snapshot と完了 receipt がある移行先に再実行できます。詳細な受理条件と部分失敗の扱いは `somniloq migrate --help` を確認してください。

## 代表的な探索導線

```bash
# セッションを眺める
somniloq sessions --config default --short

# キーワードで見つける
somniloq search --config default --format json "keyword"

# search または sessions が出した完全 REF を位置引数に渡す（裸 ID・短縮は不可）
# 長いセッションは、先に地図を見て必要な turn だけ読む
somniloq outline --config default <REF>
somniloq show --config default --turn 12..18 <REF>

# 検索結果を 50 件ずつページ単位で読む（最初、次）
somniloq search --config default --limit 50 --offset 0 "keyword"
somniloq search --config default --limit 50 --offset 50 "keyword"

# 機械処理するときは JSON を優先する
somniloq sessions --config default --format json
somniloq show --config default --format json <REF>
```

Cursor Agent の履歴には時刻がないことがあり、`--since` / `--until` を付けると対象外になります。時刻不明でも直近に取り込んだ履歴は、`somniloq sessions --config default --imported-since 24h` で探せます。日時で絞り込む必要がある場合は CLI help を確認してください。`search` の query、および `sessions`、`show`、`search` の `--project` では、`%`、`_`、`\` はワイルドカードではなく文字そのものとして扱う。`--since`、`--until`、`sessions --imported-since` は、相対時刻とローカルの日付・分単位日時に加え、`Z` または数値オフセット付きの RFC3339 instant を受け付ける。

追加 root は TOML の inputs に設定します。`import --config default --input PATH` を繰り返して入力を選べ、`--source` とは交差条件です。`--full` は選択入力だけを再構築し、他入力を保持します。Codex の子本人は継承文脈と分けて保存され、完全 REF で本人会話を選べます。Claude Code の子孫取り込み、まとまり検索・子孫選択・新しい原文 show は後続実装です。

## 詳細は CLI help を見る

この skill は CLI 構文の重複記述を避けるため、フラグ、出力列、実例の完全な説明を持ちません。必要なコマンドの help を直接確認してください。

```bash
somniloq --help
somniloq config init --help
somniloq import --help
somniloq migrate --help
somniloq sessions --help
somniloq search --help
somniloq outline --help
somniloq show --help
somniloq projects --help
```

`outline -> show --turn`、`search -> outline -> show --turn` などの横断的な使い方も各 command help にあります。
