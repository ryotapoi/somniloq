---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/rules/architecture.md
  - docs/rules/constraints.md
  - docs/rules/verification.md
  - docs/specs/jsonl-schema.md
  - docs/specs/v0.14.0-contract.md
  - internal/core/db.go
  - internal/core/db_schema.go
  - internal/core/db_write.go
  - internal/core/import_codex.go
  - internal/core/db_import_state.go
  - internal/core/db_sessions_projects.go
  - internal/core/session_relations.go
  - internal/core/db_messages_summary.go
  - internal/core/db_search.go
  - internal/core/search_groups.go
  - internal/core/search_matches.go
  - cmd/somniloq/search_detail.go
  - cmd/somniloq/sessions.go
  - cmd/somniloq/show.go
  - cmd/somniloq/session_resolution.go
  - cmd/somniloq/jsonout.go
---

# Storage and query map

SQLite の変更目的から入口を選ぶ。schema・migration・書き込み・query の制約と検証は `docs/rules/constraints.md` と `docs/rules/verification.md`、保存形式と CLI の契約は `docs/rules/scope.md` を読む。cmd と core の責務境界は `docs/rules/architecture.md` に従う。

## schema と書き込み

- 列やテーブルを変える: `internal/core/db_schema.go` の `schema` と `internal/core/db.go` の `OpenDB` を読む。一般 schema 管理は `internal/core/db_schema.go` の ensure helpers と `tableColumnPresent`、検証は `internal/core/db_migration_test.go` を確認する。旧形式 DB のサポート境界は `docs/rules/scope.md` の repository 解決節を読む。
- import の保存を変える: `internal/core/db_write.go` の `importTx` と書き込み SQL、取り込み位置の読み取りは `internal/core/db_import_state.go` の `GetImportState` を確認する。Codex 同本人の複数 rollout と cursor の再構築境界は `internal/core/import_codex.go`、出自・blocks・本人発言番号・context の意味は `docs/specs/v0.14.0-contract.md` と `docs/specs/jsonl-schema.md` を先に読む。

## query と表示

- session 行の SELECT / scan 列を変える: `internal/core/db_sessions_projects.go` の `sessionRowSelect` と `scanSessionRow` を合わせて読み、列順の対応を確認する。直接の利用箇所は `ListSessions` / `GetSession` / `LookupSessionsByID` で、複数行の読み取りは `scanSessionRows` を共有する。表示への影響は `cmd/somniloq/sessions.go`、`cmd/somniloq/show.go`、`cmd/somniloq/session_resolution.go` と `cmd/somniloq/jsonout.go` で確認する。`ListProjects` は同じファイル内の別の集約 query。
- session / project の絞り込みや集約を変える: `ListSessions` と `ListProjects` の別経路を読み、共通の時刻条件は `timeFilterConditions`、session の project 条件は `projectsCondition` を確認する。search の候補条件は `internal/core/search_groups.go` の `SearchCandidates.accepts` と `candidateBodies` を辿る。
- 本文の取得や順序を変える: `internal/core/db_messages_summary.go` の `GetIdentityMessages`（show）を読み、本人原文の保存済み番号順と context 非混入を確認する。show の filter と表示変換は cmd 層で行う。表示・一覧・詳細への影響は [Display and turns](display-and-turns.md) を辿る。

SQLite driver 固有の補助知見が必要な場合は [SQLite driver notes](sqlite-driver-notes.md) を参照する。

- REF の本人・まとまり・確定子孫の解決は `internal/core/session_relations.go` の `ResolveSession`。入力と source 内の保存関係だけを辿り、Codex 欠落親 key と Claude root 所属を区別する。本人 show の選択と `SearchMessages` の `SessionREF` が共有する。検索 scope は SQL のページ化前に適用し、循環辺は未確定として診断する。回帰の入口は `internal/core/session_relations_test.go`。

- 全一致詳細は `internal/core/search_matches.go` の `PatternMatcher` / `SearchOccurrences` と `cmd/somniloq/search_detail.go`。候補は `search_groups.go` の `searchOwners` / `SearchCandidates.accepts`、候補本文は `candidateBodies` を一覧と共有し、本人原文の保存済み番号を保持する。関係・本文・件数は ReadSnapshot。
- まとまり一覧は `SearchGroups`。全 group metadata の日時読み取りは `readOwnerTimes`、一括関係解決は `resolveSessionGroups`。候補の本文照合を済ませた全結果に cmd が page を適用する。回帰は `search_groups_test.go` の候補/metadata と独立 writer 下の snapshot tests。
