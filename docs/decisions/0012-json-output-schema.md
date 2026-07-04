# ADR 0012: JSON 出力スキーマ

## Status

Accepted

## Context

`--format json`（v0.6）は、Knowledge 側のスクリプトが show の Markdown を awk でパースしている脆さの解消先として追加する。一度出すと外部スクリプトが依存する出力契約になるため、スキーマの形を判断として記録する。

論点は 4 つ:

1. タイムスタンプをローカル表示形式（`2006-01-02 15:04`）にするか、DB 保存値（RFC3339 UTC）のままにするか
2. `--short` を JSON にも効かせるか、無視するか、エラーにするか
3. show の単一セッション指定をオブジェクトで出すか、配列で出すか
4. フィールド命名規則

## Considered Options

- **タイムスタンプ**: (A) TSV と同じローカル整形 / (B) 保存値の RFC3339 UTC をそのまま
- **--short**: (A) JSON でも `project` に反映 / (B) 無視して常に生 `repo_path` / (C) 併用エラー
- **show 単一セッション**: (A) オブジェクト 1 個 / (B) 要素 1 の配列
- **命名**: (A) snake_case（DB 列に揃える） / (B) camelCase

## Decision

We will treat JSON as machine-readable raw data with the following schema rules:

- タイムスタンプは保存値（RFC3339 UTC）をそのまま出す。ローカル整形はタイムゾーン情報を失い、分単位への切り詰めも起きるため、表示専用とする
- `--short` は JSON の `project` フィールドにも反映する。project alias に一致する場合は canonical 名のみを表示し、alias 非一致時は TSV/Markdown と同じ short/raw の表示規則に揃える
- 出力は常に JSON 配列（0 件は `[]`、show の単一セッション指定も要素 1 の配列）。消費側が単一・複数でパースを分岐しなくて済む
- フィールド名は camelCase。設定ファイル計画（`projectAliases`）と揃える
- 文字列は生値（TSV のタブ・改行置換はしない）。`title` は `custom_title` の生値で、Markdown 表示のような session_id フォールバックはしない

## Consequences

- スクリプトは jq でフィールドを直接取れる。タイムスタンプは `date` 等でパース可能な完全な値になる
- TSV/Markdown と JSON で同じ項目の表現が異なる（時刻・title フォールバック・サニタイズ）。「人間向けは表示整形、JSON は生データ」という線引きを scope.md に明記して吸収する
- alias canonical 表示または `--short` 付き JSON では生 `repo_path` が取れない。実害が出たら raw フィールド追加を別途検討する
- このスキーマはリリース後は互換性維持の対象（フィールドの削除・改名は破壊的変更）
