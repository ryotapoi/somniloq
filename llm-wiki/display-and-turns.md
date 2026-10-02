---
regen: compiled
sources:
  - docs/rules/scope.md
  - cmd/somniloq/search.go
  - cmd/somniloq/session_source.go
  - cmd/somniloq/session_resolution.go
  - cmd/somniloq/show.go
  - cmd/somniloq/outline.go
  - cmd/somniloq/turn.go
  - cmd/somniloq/sessions.go
  - cmd/somniloq/user_message_exclusion.go
  - cmd/somniloq/config.go
  - cmd/somniloq/format.go
  - cmd/somniloq/jsonout.go
  - internal/core/db_sessions_projects.go
  - internal/core/db_messages_summary.go
  - internal/core/db_search.go
---

# Display and turns

表示・ターン採番を変えるときは、`docs/rules/scope.md` で対象コマンドの契約を確認し、次の経路を辿る。

## search からセッションを再参照する

`cmd/somniloq/search.go` は `internal/core/db_search.go` の `SearchMessages` の結果に、`searchTurnsByUUID` でターンを付ける。結果の `source` と `session_id`（JSON では `sessionId`）を組で保持し、`show --source <source> --turn <N> <session_id>` または `outline --source <source> <session_id>` に渡す。

source の受理は `cmd/somniloq/session_source.go` の `parseSessionSource`、対象セッションの選択は `cmd/somniloq/session_resolution.go` の `resolveSessionByID` を読む。両コマンドの入口は `cmd/somniloq/show.go` と `cmd/somniloq/outline.go`。ID 解決を変える際は両方の呼び出しと `cmd/somniloq/session_source_test.go` / `cmd/somniloq/session_resolution_test.go` を一緒に確認する。

## メッセージとターンを変更する

解決したセッションの本文は `internal/core/db_messages_summary.go` の `GetMessages` から得る。`cmd/somniloq/turn.go` の `assignTurns` はその全メッセージ列を受け、show の `filterTurns` / `filterLastTurns`、outline の `userTurnMessages`、search の `searchTurnsByUUID` が番号を共有する。順序や採番を変えるときはこの経路と `cmd/somniloq/show_turn_test.go` / `cmd/somniloq/outline_test.go` / `cmd/somniloq/search_test.go` を確認する。

user message の表示除外は `cmd/somniloq/user_message_exclusion.go` の共有 matcher で行う。outline は全メッセージへ採番し、turn body size を計算した後に user message を除外するため、後続の番号は元のまま残る。show summary は時系列の `GetMessages` 列から user message を除外してから先頭 N 件を選ぶ。全文 show、turn/tail、search、保存処理は matcher を通らない。

`sessions` は `ListSessions` の行メタデータだけを出す。メッセージ本文の取得、user message による一覧行の除外、以前の非コマンド user-turn 案内はない。

## 出力を変更する

Markdown の本文整形は `cmd/somniloq/format.go` の `formatSession`、JSON の出力型と共通書き込みは `cmd/somniloq/jsonout.go`。show / outline / search のどの出力を変えるか決め、各コマンドでの値の構築と対応する出力テストを確認する。出力項目と形式の仕様は `docs/rules/scope.md` を参照する。
