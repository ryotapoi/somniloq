---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/specs/v0.14.0-contract.md
  - cmd/somniloq/search.go
  - cmd/somniloq/show.go
  - cmd/somniloq/show_tsv.go
  - cmd/somniloq/turn.go
  - cmd/somniloq/sessions.go
  - cmd/somniloq/config.go
  - cmd/somniloq/jsonout.go
  - internal/core/session_relations.go
  - internal/core/db_messages_summary.go
  - internal/core/db_search.go
---

# Display and turns

表示・番号・ページを変えるときは `docs/rules/scope.md` の対象コマンド節を読み、show と旧 search の採番経路を分けて追う。

## show の選択から表示まで

- 入口は `cmd/somniloq/show.go` の `showCmd`。複数 REF の解決・確定子孫の展開は `internal/core/session_relations.go` の `ResolveSession`、本人原文の読み取りは `internal/core/db_messages_summary.go` の `GetIdentityMessages` を同じ `ReadSnapshot` 内で呼ぶ。REF の展開順と重複排除を変えるなら resolver と show の両方を見る。
- 発言 filter、ページ、一行化は show の cmd 層にある。番号範囲は `parseMessageRange`、日時の受理は `parseShowTime`、一行化は `showFirstLine` が入口。保存済み番号・raw timestamp・blocks の読み取りを変えずに text だけを変換する経路を確認する。回帰の入口は `cmd/somniloq/show_contract_test.go` と `cmd/somniloq/show_filters_test.go`。
- JSON の item/envelope 型と構築は `cmd/somniloq/jsonout.go`、TSV の page 行・header・escape は `cmd/somniloq/show_tsv.go`。両形式の件数と本文は show で組み立てる同じ結果を使う。出力契約の正本は `docs/specs/v0.14.0-contract.md` の「出力」。

## 現行 search の turn

`cmd/somniloq/search.go` は `internal/core/db_search.go` の `SearchMessages` の結果に、`searchTurnsByUUID` で旧 turn を付ける。`cmd/somniloq/turn.go` の `assignTurns` は本人メッセージ列の user 発言から採番する。show の `messageNumber` は保存済み発言番号なので、この turn を show の番号範囲へ流用しない。再参照は検索結果の完全 REF を show に渡して本人原文の番号を確認する。search の新照合器・まとまり一覧は後続実装。

旧 turn helper の変更時は `cmd/somniloq/search_test.go` と `cmd/somniloq/turn_test.go` を確認する。show はこの helper を使わない。旧 outline / summary / turn / 表示除外の入口はない。

`sessions` は `ListSessions` の行メタデータだけを出す。本人本文の取得や発言 filter は show 側で行う。
