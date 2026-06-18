---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/rules/architecture.md
  - docs/decisions/0003-backfill-as-separate-subcommand.md
  - docs/decisions/0004-codex-schema-and-migration.md
  - internal/core/db.go
  - internal/core/backfill.go
  - llm-wiki/sqlite-driver-notes.md
---

# Storage and query map

DB schema、migration、query helper を触るときの地図。SQL の意味や migration は High-risk なので、仕様・テスト・`project-risk-check` を同時に見る。

## 主な入口

- `internal/core/db.go`
  - `schema`: fresh DB の正。
  - `OpenDB`: schema 作成と lightweight migration の入口。
  - `tableColumnPresent`: `PRAGMA table_info` check-first helper。PRAGMA は placeholder を受けないため trusted internal constants だけを渡す。
  - `sessionRowSelect` / `scanSessionRow`: sessions 系表示の SELECT と scan shape。列を変えるなら両方を同時に変える。
  - `timeFilterConditions` / `projectsCondition`: sessions/projects/search が共有する filter 組み立て。
  - `GetMessages` / `GetSummaryMessages` / `SearchMessages`: sidechain 除外と rowid tie-break の中心。
- `internal/core/backfill.go`
  - `MigrateToV04IfNeeded`: v0.3 DB を v0.4 composite source schema へ rebuild。
  - `CountOrphanSessions`: destructive prompt の事前件数。
  - `Backfill`: orphan DELETE と `repo_path` 解決。

## 変更時の読む順序

- schema column を追加/削除する: `schema` -> `OpenDB` migration -> `docs/rules/scope.md` のテーブル設計 -> migration tests。
- sessions/projects の列や集計を変える: `sessionRowSelect` -> `scanSessionRow` -> `ListSessions` / `ListProjects` -> cmd formatter / JSON output。
- message order を変える: `GetMessages`, `GetSummaryMessages`, `SearchMessages` -> `cmd/somniloq/turn.go` -> outline/show/search tests。
- destructive backfill を変える: `cmd/somniloq/backfill.go` の prompt/TTY path と `internal/core/backfill.go` の DB path を一緒に見る。

## 罠へのポインタ

- modernc.org/sqlite / SQLite 固有の外部知見は [SQLite driver notes](sqlite-driver-notes.md)。
- v0.4 migration の設計判断は `docs/decisions/0004-codex-schema-and-migration.md`。
- backfill を import から独立させる判断は `docs/decisions/0003-backfill-as-separate-subcommand.md`。
