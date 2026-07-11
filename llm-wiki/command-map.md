---
regen: full
sources:
  - cmd/somniloq/main.go
  - cmd/somniloq/import.go
  - cmd/somniloq/backfill.go
  - cmd/somniloq/sessions.go
  - cmd/somniloq/show.go
  - cmd/somniloq/format.go
  - cmd/somniloq/outline.go
  - cmd/somniloq/turn.go
  - cmd/somniloq/search.go
  - cmd/somniloq/filter.go
  - cmd/somniloq/projects.go
  - cmd/somniloq/jsonout.go
  - internal/core/db.go
  - internal/core/db_query.go
  - internal/core/import.go
  - internal/core/migrate_v04.go
  - internal/core/backfill.go
  - docs/rules/scope.md
  - docs/decisions/0012-json-output-schema.md
---

# Command map

CLI 入口を触る前に、まずこの表で「cmd 層」「core 層」「仕様/テスト」を揃える。cmd 層はフラグ・出力・exit code を持つが、DB 操作と JSONL パースは持たない。

| コマンド | cmd 入口 | core / ingest 側 | 代表テスト | 仕様ポインタ |
|---|---|---|---|---|
| global routing | `cmd/somniloq/main.go` | `internal/core/db.go` の `OpenDB` | `cmd/somniloq/version_test.go`, 各 cmd test | `docs/rules/scope.md` の CLI インターフェース |
| `import` | `cmd/somniloq/import.go` | `internal/core/import.go`, `internal/ingest/*` | `cmd/somniloq/import*_test.go`, `internal/core/import_test.go`, `internal/core/codex_import_test.go` | `docs/rules/scope.md` の 取り込み |
| `backfill` | `cmd/somniloq/backfill.go` | `internal/core/migrate_v04.go`, `internal/core/backfill.go` | `cmd/somniloq/backfill_test.go`, `internal/core/backfill_test.go` | `docs/rules/scope.md` の バックフィル |
| `sessions` | `cmd/somniloq/sessions.go` | `internal/core/db_query.go` の `ListSessions` | `cmd/somniloq/sessions_test.go`, `internal/core/db_query_test.go` | `docs/rules/scope.md` の セッション一覧 |
| `show` | `cmd/somniloq/show.go`, `cmd/somniloq/format.go` | `GetSession`, `LookupSessionsByID`, `GetMessages`, `GetSummaryMessages` | `cmd/somniloq/show*_test.go`, `cmd/somniloq/format_test.go` | `docs/rules/scope.md` の 内容表示 |
| `outline` | `cmd/somniloq/outline.go`, `cmd/somniloq/turn.go` | `GetMessages` | `cmd/somniloq/outline_test.go`, `cmd/somniloq/turn_test.go` | `docs/rules/scope.md` の アウトライン表示 |
| `search` | `cmd/somniloq/search.go`, `cmd/somniloq/filter.go` | `SearchMessages` | `cmd/somniloq/search_test.go`, `internal/core/db_search_test.go` | `docs/rules/scope.md` の 検索 |
| `projects` | `cmd/somniloq/projects.go` | `ListProjects` | `cmd/somniloq/jsonout_test.go`, `internal/core/db_query_test.go` | `docs/rules/scope.md` の プロジェクト一覧 |

## 変更時の読む順序

- 新フラグや出力列を足す: cmd 入口 -> `docs/rules/scope.md` -> README 両方 -> cmd test -> core query test。
- JSON 出力を変える: `cmd/somniloq/jsonout.go` -> 対象 cmd -> `docs/decisions/0012-json-output-schema.md` -> JSON tests。
- time / project filter を変える: `cmd/somniloq/filter.go` -> `internal/core/db_query.go` の `timeFilterConditions` / `projectsCondition` -> query tests。
- コマンド追加: `cmd/somniloq/main.go` の routing と usage、`docs/rules/scope.md`、README 両方、コマンド固有 test を同時に見る。
