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

全 DB コマンドで `--config NAME_OR_PATH` 省略時は `default` を使います。設定欠落は import もエラーと作成案内を返し、自動生成しません。`somniloq config init [NAME]` は名前省略時 default。設定宛先と参照 DB の両方が未存在の場合だけ TOML を生成し、DB は作成しません。いずれかに既存ファイル・directory・symlink（dangling を含む）があれば拒否し、内容を保持します。名前の既定 DB と明示 `--db` に同じ存在判定を適用し、相対 DB は設定の実体親を基準に解決します。既存の設定名・path を確認して選び、旧 JSON や旧 DB を自動移行するものと考えないでください。DB は import 時点のスナップショットです。自動更新ではありません。

既知の旧 DB の履歴保持には `somniloq migrate --config NAME_OR_PATH --from PATH` を使います。元は sidecar のない固定 standalone snapshot、設定の `db` は別の未存在または空 DB、inputs は残存 Codex ログの全 root にします。所属不明・ログ欠落・他 source の旧履歴は保持され、legacy REF で読めます。同じ snapshot と完了 receipt がある移行先に再実行できます。詳細な受理条件と部分失敗の扱いは `somniloq migrate --help` を確認してください。

## 代表的な探索導線

```bash
# セッションを眺める
somniloq search --config default

# キーワードで見つける
somniloq search --config default --format json "keyword"

# search が出した完全 REF を位置引数に渡す（裸 ID・短縮は不可）
# user 発言の一行一覧から、必要な発言番号範囲を読む
somniloq show --config default <REF> --role user --one-line
somniloq show --config default <REF> --messages 12:18

# 検索結果を 50 件ずつページ単位で読む（最初、次）
somniloq search --config default --limit 50 --offset 0 "keyword"
somniloq search --config default --limit 50 --offset 50 "keyword"

# 機械処理するときは JSON を優先する
somniloq search --config default --format json
somniloq show --config default --format json <REF>
```

Cursor Agent の履歴には時刻がないことがあり、`--since` / `--until` を付けると対象外になります。時刻不明でも直近に取り込んだ履歴は、`somniloq search --config default --imported-since 2026-10-01T00:00:00+09:00` のように zone 付き RFC3339 で探せます。search の pattern は大小文字区別の Go regexp、固定文字列は -F。project は末尾名の大小文字区別 substring と完全一致 alias 展開です。search/show の日時は日付または zone 付き RFC3339 を指定し、相対時刻は受理しません。

追加 root は TOML の inputs に設定します。`import --config default --input PATH` を繰り返して入力を選べ、`--source` とは交差条件です。`--full` は選択入力だけを再構築し、他入力を保持します。Codex の子本人は継承文脈と分けて保存され、完全 REF で本人会話を選べます。Claude Code の子孫も別の本人 REF で保持します。`search --session REF PATTERN` は本人と確定子孫だけを検索し、祖先・兄弟・root 所属だけの子は含めません。pattern 必須の Go regexp 全一致検索です。一覧は本文抜粋なしのまとまり metadata と REF を返します。

複数の完全 REF を一回の呼び出しで渡し、指定日の実発言だけを Daily Note の材料として取得できます。REF の指定順・各会話の元の発言番号順を保ち、`--descendants` は確定子孫だけを展開して重複会話を除きます。role・発言番号・日時で絞った後に、発言単位で limit / offset / tail を適用します。`--one-line` は text だけを最初の一行へ短縮し、blocks は原文を保ちます。既定 TSV、JSON は `{items,total,count,limit,offset,hasMore,nextOffset}` envelope です。show の日時は日付または zone 付き RFC3339 で指定し、相対時刻は受理しません。旧 outline・summary・turn・表示除外・Markdown・REF なし期間入口は廃止しました。一覧は turn/snippet を返しません。search --session の `messageNumber` は show と共通の原文番号です。詳細は `somniloq show --help` を参照してください。

```sh
# REF1 / REF2 は search からコピーした完全 REF
somniloq show --config default REF1 REF2 --since 2026-10-01 --until 2026-10-01 --format json
somniloq show --config default REF --role user --one-line
somniloq show --config default REF --messages 12:18 --limit 50 --format json
```

一覧の既定20件から REF を選ぶ例です。全件が必要なら hasMore/nextOffset で続きのページを取得します。

```sh
sh <<'SH'
set -- $(somniloq search --config default --since 2026-10-01 --until 2026-10-01 --format json | jq -r '.items[].members[]' | sort -u)
if [ "$#" -gt 0 ]; then
  somniloq show --config default "$@" --since 2026-10-01 --until 2026-10-01 --format json
fi
SH
```

POSIX sh で実行する例です（対話 zsh でも `sh` が実行します）。members は root 所属だけの子を含むまとまり全体、matchedMembers は検索条件で選んだ候補本人です。完全 REF は空白や glob 文字を含まず、この sort 順が show の会話順になります。空の選択では show を呼びません。

## 詳細は CLI help を見る

この skill は CLI 構文の重複記述を避けるため、フラグ、出力列、実例の完全な説明を持ちません。必要なコマンドの help を直接確認してください。

```bash
somniloq --help
somniloq config init --help
somniloq import --help
somniloq migrate --help
somniloq search --help
somniloq show --help
somniloq projects --help
```

`search -> show` や `show --role user --one-line -> show --messages` の使い方も各 command help で確認します。

既知 REF の詳細は `somniloq search --config default --session REF -e "Inherited question" -e "Child answer" --all --format json` で検索します。本人と確定子孫の本文集合で AND を判定し、一致箇所の ref/messageNumber は show と共通です。-F は全 pattern を固定文字列にします。JSON/TSV は show と同じ page envelope（total は箇所数）、既定全件、明示 limit/offset だけ箇所単位です。フラグは位置 PATTERN より前に置き、patternIndexes は位置 PATTERN→-e 指定順です。session なし search も同じ照合器を使うまとまり一覧です。pattern 省略可、既定20件、同じ envelope の total はまとまり数。input/source は繰り返し OR、条件種間 AND。members は全まとまり、matchedMembers は候補本人。root project/title と全 member の本人原文日時を表示し、last 降順・未知最後・group key 順です。limit=0/末尾超過も total を返します。日付は dayBoundary を使い until 日付を翌日境界へ進め、日時上限は排他です。--time-mode active（既定）は期間内候補本文だけで照合し、pattern なしでも実発言が必要。started/last/overlap は全体の開始/最後・重なる期間で選び候補全文を照合します。明示 mode は期間必須、詳細は active のみ。--imported-since RFC3339 は候補本人の包含下限で、表示 members/日時は全体のままです。
