---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/specs/v0.14.0-contract.md
  - internal/core/search_groups.go
  - cmd/somniloq/search_detail.go
  - cmd/somniloq/search.go
  - cmd/somniloq/show.go
  - cmd/somniloq/show_tsv.go
  - cmd/somniloq/config.go
  - cmd/somniloq/jsonout.go
  - internal/core/session_relations.go
  - internal/core/db_messages_summary.go
---

# Display and turns

表示・番号・ページを変えるときは `docs/rules/scope.md` の対象コマンド節を読み、show と search の選択・ページ経路を追う。

## show の選択から表示まで

- 入口は `cmd/somniloq/show.go` の `showCmd`。複数 REF の解決・確定子孫の展開は `internal/core/session_relations.go` の `ResolveSession`、本人原文の読み取りは `internal/core/db_messages_summary.go` の `GetIdentityMessages` を同じ `ReadSnapshot` 内で呼ぶ。REF の展開順と重複排除を変えるなら resolver と show の両方を見る。
- 発言 filter、ページ、一行化は show の cmd 層にある。番号範囲は `parseMessageRange`、日時の受理は `parseShowTime`、一行化は `showFirstLine` が入口。保存済み番号・raw timestamp・blocks の読み取りを変えずに text だけを変換する経路を確認する。回帰の入口は `cmd/somniloq/show_contract_test.go` と `cmd/somniloq/show_filters_test.go`。
- JSON の item/envelope 型と構築は `cmd/somniloq/jsonout.go`、TSV の page 行・header・escape は `cmd/somniloq/show_tsv.go`。両形式の件数と本文は show で組み立てる同じ結果を使う。出力契約の正本は `docs/specs/v0.14.0-contract.md` の「出力」。

## search の一覧と詳細

一覧は `cmd/somniloq/search.go` の `searchCmd` → `internal/core/search_groups.go` の `SearchGroups`。候補選択・照合・group metadata は core、ページ/envelope と TSV は cmd。`resolveSessionGroups` は共通の関係 graph を namespace ごとに一度構築する。回帰入口は `internal/core/search_groups_test.go` と `cmd/somniloq/search_test.go`。

全一致詳細は `cmd/somniloq/search_detail.go` → `SearchOccurrences`。保存済み番号を返し、show と一致する。候補本文取得は一覧と共通の `candidateBodies`、pattern は `PatternMatcher`。一覧は本文抜粋を返さない。
