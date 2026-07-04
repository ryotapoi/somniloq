# ADR 0015: 論理日境界はクエリ時に適用する

## Status

Accepted

## Context

v0.7 で、Knowledge 側に散っていた「04:00 から翌 04:00 を 1 日とみなす」運用を somniloq に移す。必要な効果は、`sessions` / `search` の date-only `--since` / `--until` を論理日の境界から解釈することと、`sessions` に論理日列を出すこと。

保存済み timestamp を書き換える、import 時に論理日を保存する、クエリ時だけ変換する、の選択肢がある。

## Decision

We will keep stored timestamps raw and apply `dayBoundary` only at query/display time.

- config に任意の `dayBoundary`（`HH:MM`、未指定時 `00:00`）を持つ
- `sessions` / `search` の `--day-boundary` で config を上書きできる
- date-only `--since` / `--until` だけに境界を適用する。相対時刻と絶対日時は従来どおり
- `sessions` の `logical_day` / `logicalDay` は `ended_at` 優先、無ければ `started_at` を使い、境界を基準にしたローカル日付文字列として出す
- セッションは途中で分割しない
- import と DB schema は変更しない

## Consequences

- 境界を変えても再 import が不要になる
- DB は事実としての timestamp だけを保持し、表示・検索の解釈は CLI 層に閉じる
- `show` / `projects` の date-only filter は今回の対象外として従来どおり 00:00 起点に留める
- `sessions` の TSV 列と JSON schema は外部契約として 1 フィールド増える
