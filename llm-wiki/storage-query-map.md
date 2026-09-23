---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/rules/architecture.md
  - docs/rules/constraints.md
  - docs/rules/verification.md
  - docs/specs/jsonl-schema.md
  - internal/core/db.go
  - internal/core/db_schema.go
  - internal/core/migrate_v04.go
  - internal/core/db_write.go
  - internal/core/db_import_state.go
  - internal/core/db_sessions_projects.go
  - internal/core/db_messages_summary.go
  - internal/core/db_search.go
  - internal/core/backfill.go
  - cmd/somniloq/backfill.go
  - cmd/somniloq/sessions.go
  - cmd/somniloq/show.go
  - cmd/somniloq/session_resolution.go
  - cmd/somniloq/jsonout.go
---

# Storage and query map

SQLite の変更目的から入口を選ぶ。schema・migration・書き込み・query の制約と検証は `docs/rules/constraints.md` と `docs/rules/verification.md`、保存形式と CLI の契約は `docs/rules/scope.md` を読む。cmd と core の責務境界は `docs/rules/architecture.md` に従う。

## schema と書き込み

- 列やテーブルを変える: `internal/core/db_schema.go` の `schema` と `internal/core/db.go` の `OpenDB` を読む。旧 DB の移行を変える場合は `internal/core/migrate_v04.go` の `MigrateToV04IfNeeded` と該当 migration test を確認する。
- import の保存を変える: `internal/core/db_write.go` の `importTx` と書き込み SQL、取り込み位置の読み取りは `internal/core/db_import_state.go` の `GetImportState` を確認する。JSONL 入力を変える場合は `docs/specs/jsonl-schema.md` も読む。
- backfill の削除・更新を変える: `cmd/somniloq/backfill.go` の確認導線から `internal/core/backfill.go` の `CountOrphanSessions` / `Backfill` を辿る。

## query と表示

- session 行の SELECT / scan 列を変える: `internal/core/db_sessions_projects.go` の `sessionRowColumns` を入口に、そこから導出する `sessionRowSelectExpressions` / `sessionRowSelect` と `scanSessionRow` を辿る。直接の利用箇所は `ListSessions` / `GetSession` / `LookupSessionsByID`。表示への影響は `cmd/somniloq/sessions.go`、`cmd/somniloq/show.go`、`cmd/somniloq/session_resolution.go` と `cmd/somniloq/jsonout.go` で確認する。`ListProjects` は同じファイル内の別の集約 query。
- session / project の絞り込みや集約を変える: `ListSessions` と `ListProjects` の別経路を読み、共通の時刻条件は `timeFilterConditions`、session の project 条件は `projectsCondition` を確認する。検索への影響は `internal/core/db_search.go` の `SearchMessages` まで辿る。
- 本文の取得や順序を変える: `internal/core/db_messages_summary.go` の `GetMessages` / `GetSummaryMessages` を読み、表示とターンへの影響は [Display and turns](display-and-turns.md) を辿る。

SQLite driver 固有の補助知見が必要な場合は [SQLite driver notes](sqlite-driver-notes.md) を参照する。
